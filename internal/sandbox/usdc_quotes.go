package sandbox

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

const USDCTestChain int64 = 84532
const USDCTestToken = "0x036cbd53842c5426634e7929541ec2318f3dcf7e"

// SignedQuote exposes the original bytes. Clients verify them directly rather
// than reserializing a JSON object whose formatting may differ.
type SignedQuote struct {
	KeyID     string `json:"key_id"`
	Payload   []byte `json:"payload"`
	Signature []byte `json:"signature"`
}
type usdcEnvelope struct {
	Version     string `json:"version"`
	Designation string `json:"designation"`
	Payment     string `json:"payment_id"`
	Actor       string `json:"actor_id"`
	Credential  string `json:"credential_id"`
	Binding     string `json:"wallet_binding_id"`
	Epoch       int64  `json:"wallet_binding_epoch"`
	Payer       string `json:"payer"`
	Chain       int64  `json:"chain_id"`
	Token       string `json:"token_address"`
	Nonce       string `json:"nonce"`
	Quote       Quote  `json:"quote"`
}
type USDCSigningConfig struct {
	KeyID      string
	PrivateKey ed25519.PrivateKey
	PublicKeys map[string]ed25519.PublicKey
}

// WithUSDCSigning copies trusted startup keys. It never creates production keys
// and retains explicitly configured public historical keys for persisted quotes.
func (s *Store) WithUSDCSigning(c USDCSigningConfig) (*Store, error) {
	if c.KeyID == "" || len(c.KeyID) > 128 || len(c.PrivateKey) != ed25519.PrivateKeySize || len(c.PublicKeys) == 0 {
		return nil, ErrInvalid
	}
	keys := make(map[string]ed25519.PublicKey, len(c.PublicKeys))
	for id, k := range c.PublicKeys {
		if id == "" || len(id) > 128 || len(k) != ed25519.PublicKeySize {
			return nil, ErrInvalid
		}
		keys[id] = append(ed25519.PublicKey(nil), k...)
	}
	if !bytes.Equal(c.PrivateKey.Public().(ed25519.PublicKey), keys[c.KeyID]) {
		return nil, ErrInvalid
	}
	clone := *s
	clone.usdcSigning = &USDCSigningConfig{KeyID: c.KeyID, PrivateKey: append(ed25519.PrivateKey(nil), c.PrivateKey...), PublicKeys: keys}
	return &clone, nil
}
func validUSDCOffer(q Quote) bool {
	address, err := normalizeWalletAddress(q.Payee)
	return err == nil && address == q.Payee && q.Asset == "USDC" && q.Rail == "x402_test"
}
func (s *Store) signUSDCQuote(ctx context.Context, tx pgx.Tx, a identity, id string, q Quote) (*SignedQuote, string, int64, error) {
	if s.usdcSigning == nil {
		return nil, "", 0, ErrUnauthorized
	}
	var binding, payer string
	var epoch int64
	err := tx.QueryRow(ctx, `SELECT w.id::text,w.epoch,w.address FROM sandbox_actors a JOIN sandbox_wallet_challenges w ON w.id=a.wallet_binding_id AND w.actor_id=a.id AND w.epoch=a.wallet_binding_epoch WHERE a.id=$1 AND w.state='bound' AND w.chain_id=$2`, a.actor, USDCTestChain).Scan(&binding, &epoch, &payer)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", 0, ErrUnauthorized
	}
	if err != nil {
		return nil, "", 0, err
	}
	payload := usdcQuotePayload(id, a.actor, a.credential, binding, epoch, payer, q)
	return &SignedQuote{KeyID: s.usdcSigning.KeyID, Payload: payload, Signature: ed25519.Sign(s.usdcSigning.PrivateKey, payload)}, binding, epoch, nil
}

// Verify persisted authority before either authorization or dispatch. The actor
// lock is owned by the caller and serializes rebinding with these decisions.
func (s *Store) checkUSDCQuote(ctx context.Context, tx pgx.Tx, p Payment, current bool) error {
	if p.Quote.Asset != "USDC" {
		return nil
	}
	if s.usdcSigning == nil || p.SignedQuote == nil {
		return ErrUnauthorized
	}
	signed := p.SignedQuote
	publicKey := s.usdcSigning.PublicKeys[signed.KeyID]
	if len(publicKey) != ed25519.PublicKeySize {
		return ErrUnauthorized
	}
	if !ed25519.Verify(publicKey, signed.Payload, signed.Signature) || hash(signed.Payload) != p.Hash {
		return ErrConflict
	}
	var actor, credential, binding, payer string
	var epoch int64
	err := tx.QueryRow(ctx, `SELECT p.actor_id::text,p.credential_id::text,p.wallet_binding_id::text,p.wallet_binding_epoch,w.address FROM sandbox_payments p JOIN sandbox_wallet_challenges w ON w.id=p.wallet_binding_id AND w.actor_id=p.actor_id AND w.epoch=p.wallet_binding_epoch WHERE p.id=$1`, p.ID).Scan(&actor, &credential, &binding, &epoch, &payer)
	if err != nil {
		return err
	}
	expected := usdcQuotePayload(p.ID, actor, credential, binding, epoch, payer, p.Quote)
	if !bytes.Equal(expected, signed.Payload) || !validUSDCOffer(p.Quote) {
		return ErrConflict
	}
	if current {
		var valid bool
		if err = tx.QueryRow(ctx, `SELECT wallet_binding_id=$2 AND wallet_binding_epoch=$3 FROM sandbox_actors WHERE id=$1`, actor, binding, epoch).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return ErrConflict
		}
	}
	return nil
}

func validQuoteRail(q Quote) bool {
	return q.Rail == "stripe_test" && q.Asset == "USD" || validUSDCOffer(q)
}

// ParseUSDCSigningConfig accepts only explicit trusted deployment configuration.
func ParseUSDCSigningConfig(keyID, privateKey, publicKeys string) (USDCSigningConfig, error) {
	c := USDCSigningConfig{KeyID: keyID, PublicKeys: map[string]ed25519.PublicKey{}}
	raw, err := base64.StdEncoding.DecodeString(privateKey)
	if err != nil {
		return c, ErrInvalid
	}
	c.PrivateKey = ed25519.PrivateKey(raw)
	var keys map[string]string
	if err = json.Unmarshal([]byte(publicKeys), &keys); err != nil {
		return c, ErrInvalid
	}
	for id, encoded := range keys {
		b, e := base64.StdEncoding.DecodeString(encoded)
		if e != nil {
			return c, ErrInvalid
		}
		c.PublicKeys[id] = ed25519.PublicKey(b)
	}
	if _, err = New("").WithUSDCSigning(c); err != nil {
		return c, err
	}
	return c, nil
}

func usdcQuotePayload(id, actor, credential, binding string, epoch int64, payer string, q Quote) []byte {
	return encode(usdcEnvelope{Version: "undergrid-usdc-quote-v1", Designation: "TEST", Payment: id, Actor: actor, Credential: credential, Binding: binding, Epoch: epoch, Payer: payer, Chain: USDCTestChain, Token: USDCTestToken, Nonce: id, Quote: q})
}
