package sandbox

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type USDCSettlementStatus struct {
	Transaction string    `json:"transaction_hash"`
	BlockHash   string    `json:"block_hash,omitempty"`
	Block       string    `json:"block_number"`
	Finality    string    `json:"finality"`
	CheckedAt   time.Time `json:"checked_at"`
	Designation string    `json:"designation"`
}
type usdcClaim struct {
	Start, Scan *big.Int
	Transaction string
	Sequence    int64
	Previous    *usdcObservation
}

func (s *Store) verifyHistoricalUSDC(ctx context.Context, tx pgx.Tx, p Payment) error {
	if p.Quote.Asset != "USDC" || p.Quote.Rail != "x402_test" || p.walletAuthorization == nil {
		return ErrConflict
	}
	if err := s.checkUSDCQuote(ctx, tx, p, false); err != nil {
		return err
	}
	if hash(encode(p.walletAuthorization)) != p.WalletAuthorizationHash {
		return ErrConflict
	}
	historical := *s
	domain := p.walletAuthorization.Domain
	historical.usdcTokenDomain = &domain
	return historical.verifyPayerAuthorization(p, *p.walletAuthorization, false)
}
func (s *Store) claimUSDCDispatch(ctx context.Context, tx pgx.Tx, p Payment, start *big.Int) error {
	if err := s.verifyHistoricalUSDC(ctx, tx, p); err != nil {
		return err
	}
	var block any
	scan := "0"
	if start != nil {
		block = start.String()
		scan = new(big.Int).Add(start, big.NewInt(1)).String()
	}
	_, err := tx.Exec(ctx, `INSERT INTO sandbox_usdc_claims(payment_id,chain_id,token,payer,nonce,quote_hash,authorization_hash,provider_key,start_block,scan_block) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, p.ID, USDCTestChain, USDCTestToken, p.walletAuthorization.Authorization.From, p.walletAuthorization.Authorization.Nonce, p.Hash, p.WalletAuthorizationHash, p.ProviderKey, block, scan)
	var duplicate *pgconn.PgError
	if errors.As(err, &duplicate) && duplicate.Code == "23505" {
		return ErrConflict
	}
	return err
}
func (s *Store) usdcPayment(ctx context.Context, id string) (Payment, usdcClaim, error) {
	var p Payment
	var claim usdcClaim
	err := s.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		var err error
		p, err = loadPayment(ctx, tx, id)
		if err != nil {
			return err
		}
		if err = s.verifyHistoricalUSDC(ctx, tx, p); err != nil {
			return err
		}
		var start *string
		var scan, quote, proof, key, payer, nonce string
		err = tx.QueryRow(ctx, `SELECT start_block::text,scan_block::text,quote_hash,authorization_hash,provider_key,payer,nonce FROM sandbox_usdc_claims WHERE payment_id=$1`, id).Scan(&start, &scan, &quote, &proof, &key, &payer, &nonce)
		if errors.Is(err, pgx.ErrNoRows) && p.State == "authorized" {
			return nil
		}
		if err != nil {
			return err
		}
		if quote != p.Hash || proof != p.WalletAuthorizationHash || key != p.ProviderKey || payer != p.walletAuthorization.Authorization.From || nonce != p.walletAuthorization.Authorization.Nonce {
			return ErrConflict
		}
		if start != nil {
			claim.Start, _ = new(big.Int).SetString(*start, 10)
		}
		claim.Scan, _ = new(big.Int).SetString(scan, 10)
		previous := &usdcObservation{}
		err = tx.QueryRow(ctx, `SELECT sequence,transaction_hash,block_hash,block_number::text,finality,evidence FROM sandbox_usdc_observations WHERE payment_id=$1 ORDER BY sequence DESC LIMIT 1`, id).Scan(&claim.Sequence, &previous.Transaction, &previous.BlockHash, &previous.Block, &previous.Finality, &previous.Evidence)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		claim.Transaction = previous.Transaction
		claim.Previous = previous
		return err
	})
	return p, claim, err
}
func (s *Store) recordUSDC(ctx context.Context, id string, proof usdcObservation) error {
	return s.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		var actor string
		if err := tx.QueryRow(ctx, "SELECT actor_id::text FROM sandbox_payments WHERE id=$1", id).Scan(&actor); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT id FROM sandbox_actors WHERE id=$1 FOR UPDATE", actor); err != nil {
			return err
		}
		p, err := loadPayment(ctx, tx, id)
		if err != nil {
			return err
		}
		if err = s.verifyHistoricalUSDC(ctx, tx, p); err != nil {
			return err
		}
		var claim bool
		err = tx.QueryRow(ctx, `SELECT quote_hash=$2 AND authorization_hash=$3 AND provider_key=$4 FROM sandbox_usdc_claims WHERE payment_id=$1`, id, p.Hash, p.WalletAuthorizationHash, p.ProviderKey).Scan(&claim)
		if err != nil {
			return err
		}
		if !claim {
			return ErrConflict
		}
		if p.State != "submitting" && p.State != "unknown" && p.State != "pending" && p.State != "settled" {
			return ErrConflict
		}
		var oldTransaction, oldBlock, oldFinality string
		var observedSequence int64
		err = tx.QueryRow(ctx, `SELECT sequence,transaction_hash,block_hash,finality FROM sandbox_usdc_observations WHERE payment_id=$1 ORDER BY sequence DESC LIMIT 1`, id).Scan(&observedSequence, &oldTransaction, &oldBlock, &oldFinality)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		// The proof was observed outside the transaction. Actor serialization alone
		// cannot stop an older RPC snapshot arriving after a newer committed reorg.
		if observedSequence != proof.expectedSequence {
			return ErrConflict
		}
		if oldFinality != "" && oldFinality != "candidate" && oldFinality != "orphaned" && proof.Finality != "orphaned" {
			if oldTransaction != proof.Transaction || oldBlock != proof.BlockHash {
				return ErrConflict
			}
			if oldFinality == "finalized" && proof.Finality != "finalized" {
				return ErrConflict
			}
			if oldFinality == "safe" && proof.Finality == "provisional" {
				return ErrConflict
			}
		}
		if oldTransaction == proof.Transaction && oldBlock == proof.BlockHash && oldFinality == proof.Finality {
			return nil
		}
		observation := randomID()
		if _, err = tx.Exec(ctx, `INSERT INTO sandbox_usdc_observations(id,payment_id,transaction_hash,block_number,block_hash,finality,evidence) VALUES($1,$2,$3,$4,$5,$6,$7)`, observation, id, proof.Transaction, proof.Block, proof.BlockHash, proof.Finality, []byte(proof.Evidence)); err != nil {
			return err
		}
		state := "pending"
		if proof.Finality == "candidate" || proof.Finality == "orphaned" {
			state = "unknown"
		}
		if proof.Finality == "finalized" {
			state = "settled"
			base := "usdc:84532:" + USDCTestToken + ":" + proof.Transaction + ":" + proof.BlockHash + ":" + proof.TransferIndex
			for _, effect := range []struct{ kind, amount string }{{"charge", p.Quote.Amount}, {"fee", p.Quote.Fee}} {
				if effect.amount == "0" {
					continue
				}
				fact := base + ":" + effect.kind
				var previous string
				err = tx.QueryRow(ctx, `SELECT payment_id::text FROM sandbox_effects WHERE provider='x402_test' AND account=$1 AND environment='test' AND fact_id=$2`, p.Quote.Payee, fact).Scan(&previous)
				if err == nil && previous != id {
					return ErrConflict
				}
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
				if _, err = tx.Exec(ctx, `INSERT INTO sandbox_effects(provider,account,environment,fact_id,payment_id,kind,asset,amount,evidence) VALUES('x402_test',$1,'test',$2,$3,$4,'USDC',$5,$6) ON CONFLICT DO NOTHING`, p.Quote.Payee, fact, id, effect.kind, effect.amount, []byte(proof.Evidence)); err != nil {
					return err
				}
			}
		}
		if proof.Finality == "orphaned" {
			// Corrections reference immutable economic facts; canonical observations
			// determine current validity and allow the same block to re-canonicalize.
			if _, err = tx.Exec(ctx, `INSERT INTO sandbox_effects(provider,account,environment,fact_id,payment_id,kind,asset,amount,evidence,reverses_fact_id)
    SELECT provider,account,environment,$2||':'||fact_id,payment_id,'reorg',asset,amount,$3,fact_id FROM sandbox_effects WHERE payment_id=$1 AND provider='x402_test' AND kind IN('charge','fee') AND evidence->>'transaction_hash'=$4 AND evidence->>'block_hash'=$5`, id, observation, []byte(proof.Evidence), proof.Transaction, proof.BlockHash); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE sandbox_usdc_claims SET scan_block=LEAST(scan_block,$2::numeric) WHERE payment_id=$1`, id, proof.Block); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE sandbox_payments SET state=$2,provider_reference=$3,updated_at=now() WHERE id=$1`, id, state, proof.Transaction)
		return err
	})
}
func loadUSDCStatus(ctx context.Context, tx pgx.Tx, p *Payment) error {
	if p.Quote.Asset != "USDC" {
		return nil
	}
	var status USDCSettlementStatus
	err := tx.QueryRow(ctx, `SELECT transaction_hash,block_hash,block_number::text,finality,checked_at FROM sandbox_usdc_observations WHERE payment_id=$1 ORDER BY sequence DESC LIMIT 1`, p.ID).Scan(&status.Transaction, &status.BlockHash, &status.Block, &status.Finality, &status.CheckedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err == nil {
		status.Designation = "TEST — NOT A LIVE COMMITMENT"
		p.Settlement = &status
	}
	return err
}
