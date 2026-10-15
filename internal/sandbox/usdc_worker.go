package sandbox

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"math/big"
	"strings"
	"time"
)

func (s *USDCService) RunWorker(ctx context.Context) error {
	if !s.verified.Load() {
		if err := s.VerifyConfiguration(ctx); err != nil {
			return err
		}
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := s.tickUSDC(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func (s *USDCService) tickUSDC(ctx context.Context) error {
	type item struct{ ID, State string }
	items := []item{}
	err := s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text,state FROM sandbox_payments WHERE quote->>'rail'='x402_test' AND quote->>'product'=$1 AND quote->>'payee'=$2 AND state IN('authorized','submitting','pending','unknown','settled') ORDER BY updated_at,id LIMIT 100`, s.config.Product, s.config.Payee)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var i item
			if err = rows.Scan(&i.ID, &i.State); err != nil {
				return err
			}
			items = append(items, i)
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	for _, i := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if i.State == "authorized" {
			_ = s.DispatchPayment(ctx, i.ID)
		} else {
			_ = s.ReconcilePayment(ctx, i.ID)
		}
	}
	return nil
}
func (s *USDCService) paymentAllowed(p Payment) bool {
	return p.Quote.Asset == "USDC" && p.Quote.Rail == "x402_test" && p.Quote.Payee == s.config.Payee && p.Quote.Product == s.config.Product
}
func (s *USDCService) DispatchPayment(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if !s.verified.Load() {
		return ErrUnauthorized
	}
	p, _, err := s.store.usdcPayment(ctx, id)
	if err != nil {
		return err
	}
	if !s.paymentAllowed(p) || p.State != "authorized" {
		return ErrConflict
	}
	if err = s.checkChain(ctx); err != nil {
		return err
	}
	var raw string
	if err = s.rpc(ctx, "eth_blockNumber", []any{}, &raw); err != nil {
		return err
	}
	start, err := chainQuantity(raw)
	if err != nil {
		return err
	}
	used, err := s.nonceUsed(ctx, p)
	if err != nil {
		return err
	}
	// Any disclosure of a transferable signature is an external spending attempt.
	// Claim before /verify, not merely before /settle, and never repeat an attempt.
	p, err = s.store.beginDispatch(ctx, id, start)
	if err != nil {
		return err
	}
	unknown := func(cause error) error { return errors.Join(cause, s.store.MarkUnknown(ctx, id)) }
	if used {
		if err = s.store.MarkUnknown(ctx, id); err != nil {
			return err
		}
		return s.ReconcilePayment(ctx, id)
	}
	authorization := p.walletAuthorization.Authorization
	requirements := map[string]any{"scheme": "exact", "network": "eip155:84532", "amount": authorization.Value, "asset": USDCTestToken, "payTo": p.Quote.Payee, "maxTimeoutSeconds": int64(time.Until(p.Quote.ExpiresAt).Seconds()), "extra": map[string]any{"assetTransferMethod": "eip3009", "name": p.walletAuthorization.Domain.Name, "version": p.walletAuthorization.Domain.Version}}
	payload := map[string]any{"x402Version": 2, "accepted": requirements, "resource": map[string]any{"url": s.config.ResourceURL, "mimeType": "application/json"}, "payload": map[string]any{"signature": p.walletAuthorization.Signature, "authorization": map[string]any{"from": authorization.From, "to": authorization.To, "value": authorization.Value, "validAfter": authorization.ValidAfter, "validBefore": authorization.ValidBefore, "nonce": authorization.Nonce}}}
	request := map[string]any{"x402Version": 2, "paymentPayload": payload, "paymentRequirements": requirements}
	var verified struct {
		Valid bool   `json:"isValid"`
		Payer string `json:"payer"`
	}
	if err = s.request(ctx, "POST", strings.TrimRight(s.config.FacilitatorURL, "/")+"/verify", request, &verified); err != nil {
		return unknown(err)
	}
	if !verified.Valid || (verified.Payer != "" && strings.ToLower(verified.Payer) != authorization.From) {
		return unknown(ErrConflict)
	}
	var settled struct {
		Success     bool   `json:"success"`
		Transaction string `json:"transaction"`
		Network     string `json:"network"`
		Payer       string `json:"payer"`
	}
	if err = s.request(ctx, "POST", strings.TrimRight(s.config.FacilitatorURL, "/")+"/settle", request, &settled); err != nil {
		return unknown(err)
	}
	if !chainHash(settled.Transaction) || settled.Network != "eip155:84532" || (settled.Payer != "" && strings.ToLower(settled.Payer) != authorization.From) {
		return unknown(ErrConflict)
	}
	candidate := usdcObservation{Transaction: settled.Transaction, Block: "0", Finality: "candidate", Evidence: encode(map[string]any{"transaction_hash": settled.Transaction, "provider_result": "unverified_candidate", "designation": "TEST"})}
	if err = s.store.recordUSDC(ctx, id, candidate); err != nil {
		return unknown(err)
	}
	// A facilitator success flag is never an economic fact. Only independent RPC
	// receipt/finality verification can settle, including a late reported failure.
	return s.ReconcilePayment(ctx, id)
}
func (s *USDCService) ReconcilePayment(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if !s.verified.Load() {
		return ErrUnauthorized
	}
	p, claim, err := s.store.usdcPayment(ctx, id)
	if err != nil {
		return err
	}
	if !s.paymentAllowed(p) || (p.State != "submitting" && p.State != "pending" && p.State != "unknown" && p.State != "settled") {
		return ErrConflict
	}
	if err = s.checkChain(ctx); err != nil {
		return err
	}
	previous, err := s.store.lastUSDCObservation(ctx, id)
	if err != nil {
		return err
	}
	if previous != nil && previous.BlockHash != "" && previous.Finality != "orphaned" {
		number, _ := new(big.Int).SetString(previous.Block, 10)
		header, err := s.header(ctx, "0x"+number.Text(16))
		if err != nil {
			return err
		}
		if header.Hash != previous.BlockHash {
			correction := *previous
			correction.Finality = "orphaned"
			correction.Evidence = encode(map[string]any{"transaction_hash": previous.Transaction, "block_hash": previous.BlockHash, "block_number": previous.Block, "canonical_replacement_hash": header.Hash, "designation": "TEST"})
			if err = s.store.recordUSDC(ctx, id, correction); err != nil {
				return err
			}
			claim.Scan = number
		}
	}
	candidate := claim.Transaction
	if candidate != "" {
		proof, err := s.verifyReceipt(ctx, p, candidate, claim.Start)
		if err != nil {
			if p.State != "settled" {
				_ = s.store.MarkUnknown(ctx, id)
			}
			return err
		}
		if proof != nil {
			return s.store.recordUSDC(ctx, id, *proof)
		}
	}
	candidates, err := s.findUSDCTransactions(ctx, p, claim)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		proof, err := s.verifyReceipt(ctx, p, candidate, claim.Start)
		if err != nil {
			return err
		}
		if proof != nil {
			return s.store.recordUSDC(ctx, id, *proof)
		}
	}
	return nil
}
func (s *USDCService) findUSDCTransactions(ctx context.Context, p Payment, claim usdcClaim) ([]string, error) {
	var raw string
	if err := s.rpc(ctx, "eth_blockNumber", []any{}, &raw); err != nil {
		return nil, err
	}
	latest, err := chainQuantity(raw)
	if err != nil {
		return nil, err
	}
	from := new(big.Int)
	if claim.Scan != nil {
		from.Set(claim.Scan)
	}
	if claim.Start != nil && from.Cmp(claim.Start) <= 0 {
		from.Add(claim.Start, big.NewInt(1))
	}
	if from.Cmp(latest) > 0 {
		return nil, nil
	}
	to := new(big.Int).Add(from, big.NewInt(2047))
	if to.Cmp(latest) > 0 {
		to.Set(latest)
	}
	a := p.walletAuthorization.Authorization
	var logs []usdcLog
	filter := map[string]any{"address": USDCTestToken, "topics": []string{usdcTopic("AuthorizationUsed(address,bytes32)"), usdcAddressWord(a.From), a.Nonce}, "fromBlock": "0x" + from.Text(16), "toBlock": "0x" + to.Text(16)}
	if err = s.rpc(ctx, "eth_getLogs", []any{filter}, &logs); err != nil {
		return nil, err
	}
	if len(logs) > 100 {
		return nil, ErrInvalid
	}
	candidates := []string{}
	seen := map[string]bool{}
	for _, log := range logs {
		block, err := chainQuantity(log.Block)
		if err != nil || block.Cmp(from) < 0 || block.Cmp(to) > 0 || log.Removed || strings.ToLower(log.Address) != USDCTestToken || len(log.Topics) != 3 || log.Topics[0] != usdcTopic("AuthorizationUsed(address,bytes32)") || log.Topics[1] != usdcAddressWord(a.From) || log.Topics[2] != a.Nonce || !chainHash(log.Transaction) {
			return nil, ErrConflict
		}
		if !seen[log.Transaction] {
			seen[log.Transaction] = true
			candidates = append(candidates, log.Transaction)
		}
	}
	if len(candidates) == 0 {
		finalized, err := s.header(ctx, "finalized")
		if err != nil {
			return nil, err
		}
		number, _ := chainQuantity(finalized.Number)
		if number.Cmp(to) < 0 {
			to.Set(number)
		}
		if to.Cmp(from) >= 0 {
			next := new(big.Int).Add(to, big.NewInt(1))
			err = s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, "UPDATE sandbox_usdc_claims SET scan_block=GREATEST(scan_block,$2::numeric) WHERE payment_id=$1", p.ID, next.String())
				return err
			})
			if err != nil {
				return nil, err
			}
		}
	}
	return candidates, nil
}
