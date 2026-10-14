package sandbox

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type USDCTokenDomain struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
type TransferAuthorization struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Value       string `json:"value"`
	ValidAfter  string `json:"valid_after"`
	ValidBefore string `json:"valid_before"`
	Nonce       string `json:"nonce"`
}
type WalletPaymentCommand struct {
	Hash          string                `json:"quote_hash"`
	Chain         int64                 `json:"chain_id"`
	Token         string                `json:"token_address"`
	Signature     string                `json:"signature"`
	Authorization TransferAuthorization `json:"authorization"`
}
type WalletPaymentAuthorization struct {
	WalletPaymentCommand
	Domain USDCTokenDomain `json:"domain"`
}

// These are trusted deployment assumptions, not a token-contract RPC check.
func (s *Store) WithUSDCTokenDomain(domain USDCTokenDomain) (*Store, error) {
	for _, v := range []string{domain.Name, domain.Version} {
		if v == "" || len(v) > 128 || strings.TrimSpace(v) != v || !utf8.ValidString(v) {
			return nil, ErrInvalid
		}
	}
	clone := *s
	clone.usdcTokenDomain = &domain
	return &clone, nil
}
func usdcGross(q Quote) (string, error) {
	if !positive(q.Amount, false) || !positive(q.Fee, true) {
		return "", ErrInvalid
	}
	amount, _ := new(big.Int).SetString(q.Amount, 10)
	fee, _ := new(big.Int).SetString(q.Fee, 10)
	total := amount.Add(amount, fee)
	if total.BitLen() > 256 {
		return "", ErrInvalid
	}
	return total.String(), nil
}
func abiUint256(value string) ([]byte, error) {
	if !positive(value, true) {
		return nil, ErrInvalid
	}
	n, _ := new(big.Int).SetString(value, 10)
	if n.BitLen() > 256 {
		return nil, ErrInvalid
	}
	return n.FillBytes(make([]byte, 32)), nil
}
func abiAddress(address string) ([]byte, error) {
	normalized, err := normalizeWalletAddress(address)
	if err != nil || normalized != address {
		return nil, ErrInvalid
	}
	raw, err := hex.DecodeString(address[2:])
	if err != nil {
		return nil, ErrInvalid
	}
	return append(make([]byte, 12), raw...), nil
}
func transferAuthorizationDigest(c WalletPaymentCommand, domain USDCTokenDomain) ([]byte, error) {
	if c.Chain != USDCTestChain || c.Token != USDCTestToken {
		return nil, ErrInvalid
	}
	from, err := abiAddress(c.Authorization.From)
	if err != nil {
		return nil, err
	}
	to, err := abiAddress(c.Authorization.To)
	if err != nil {
		return nil, err
	}
	value, err := abiUint256(c.Authorization.Value)
	if err != nil {
		return nil, err
	}
	after, err := abiUint256(c.Authorization.ValidAfter)
	if err != nil {
		return nil, err
	}
	before, err := abiUint256(c.Authorization.ValidBefore)
	if err != nil {
		return nil, err
	}
	if len(c.Authorization.Nonce) != 66 || !strings.HasPrefix(c.Authorization.Nonce, "0x") {
		return nil, ErrInvalid
	}
	nonce, err := hex.DecodeString(c.Authorization.Nonce[2:])
	if err != nil || len(nonce) != 32 {
		return nil, ErrInvalid
	}
	chain, _ := abiUint256(big.NewInt(USDCTestChain).String())
	token, _ := abiAddress(USDCTestToken)
	separator := walletKeccak256(bytes.Join([][]byte{walletKeccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)")), walletKeccak256([]byte(domain.Name)), walletKeccak256([]byte(domain.Version)), chain, token}, nil))
	transfer := walletKeccak256(bytes.Join([][]byte{walletKeccak256([]byte("TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)")), from, to, value, after, before, nonce}, nil))
	return walletKeccak256(bytes.Join([][]byte{{0x19, 0x01}, separator, transfer}, nil)), nil
}
func (s *Store) verifyPayerAuthorization(p Payment, record WalletPaymentAuthorization, current bool) error {
	if s.usdcTokenDomain == nil || record.Domain != *s.usdcTokenDomain {
		return ErrUnauthorized
	}
	if p.Quote.Asset != "USDC" || p.SignedQuote == nil || record.Hash != p.Hash {
		return ErrConflict
	}
	var envelope usdcEnvelope
	if err := json.Unmarshal(p.SignedQuote.Payload, &envelope); err != nil {
		return err
	}
	gross, err := usdcGross(p.Quote)
	if err != nil {
		return err
	}
	if record.Chain != USDCTestChain || record.Token != USDCTestToken || record.Authorization.From != envelope.Payer || record.Authorization.To != p.Quote.Payee || record.Authorization.Value != gross || record.Authorization.Nonce != envelope.Nonce || record.Authorization.ValidAfter != "0" || record.Authorization.ValidBefore != big.NewInt(p.Quote.ExpiresAt.Unix()).String() {
		return ErrConflict
	}
	if current && (time.Now().Unix() <= 0 || time.Now().Unix() >= p.Quote.ExpiresAt.Unix()) {
		return ErrConflict
	}
	signature, err := parseWalletSignature(record.Signature)
	if err != nil {
		return err
	}
	digest, err := transferAuthorizationDigest(record.WalletPaymentCommand, record.Domain)
	if err != nil {
		return err
	}
	payer, err := recoverEOADigest(digest, signature)
	if err != nil {
		return err
	}
	if payer != envelope.Payer {
		return ErrUnauthorized
	}
	return nil
}
func (s *Store) checkPayerAuthorization(ctx context.Context, tx pgx.Tx, p Payment) error {
	if p.Quote.Asset != "USDC" {
		return nil
	}
	if p.walletAuthorization == nil {
		return ErrUnauthorized
	}
	var storedHash string
	if err := tx.QueryRow(ctx, "SELECT payer_authorization_hash FROM sandbox_payments WHERE id=$1", p.ID).Scan(&storedHash); err != nil {
		return err
	}
	if hash(encode(p.walletAuthorization)) != storedHash {
		return ErrConflict
	}
	return s.verifyPayerAuthorization(p, *p.walletAuthorization, true)
}

func (s *Store) AuthorizeWalletPayment(ctx context.Context, token, id, key string, c WalletPaymentCommand) (Payment, error) {
	var p Payment
	err := s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		a, err := agentContext(ctx, tx, token, "payments:authorize")
		if err != nil {
			return err
		}
		digest := hash(encode(struct {
			ID      string
			Command WalletPaymentCommand
		}{id, c}))
		old, err := replay(ctx, tx, a, "wallet_authorization", key, digest)
		if err != nil {
			return err
		}
		if old != "" {
			p, err = loadPayment(ctx, tx, old)
			return err
		}
		p, err = loadPayment(ctx, tx, id)
		if err != nil {
			return err
		}
		if s.usdcTokenDomain == nil {
			return ErrUnauthorized
		}
		if p.State != "authorized" || p.Quote.Asset != "USDC" || p.walletAuthorization != nil {
			return ErrConflict
		}
		var valid bool
		err = tx.QueryRow(ctx, `SELECT credential_id=$2 AND policy_version=$3 AND reservation_period=(now() AT TIME ZONE 'UTC')::date FROM sandbox_payments WHERE id=$1`, id, a.credential, a.version).Scan(&valid)
		if err != nil {
			return err
		}
		if !valid || !permits(a.policy, p.Quote) {
			return ErrUnauthorized
		}
		if err = s.checkUSDCQuote(ctx, tx, p, true); err != nil {
			return err
		}
		record := WalletPaymentAuthorization{WalletPaymentCommand: c, Domain: *s.usdcTokenDomain}
		if err = s.verifyPayerAuthorization(p, record, true); err != nil {
			return err
		}
		signature, err := parseWalletSignature(c.Signature)
		if err != nil {
			return err
		}
		record.Signature = "0x" + hex.EncodeToString(signature)
		_, err = tx.Exec(ctx, `UPDATE sandbox_payments SET payer_authorization=$2,payer_authorization_hash=$3,payer_authorization_key=$4,payer_authorization_request_hash=$5,updated_at=now() WHERE id=$1`, id, encode(record), hash(encode(record)), key, digest)
		if err != nil {
			return err
		}
		if err = remember(ctx, tx, a, "wallet_authorization", key, digest, id); err != nil {
			return err
		}
		p.walletAuthorization = &record
		p.WalletAuthorizationHash = hash(encode(record))
		return nil
	})
	return p, err
}
