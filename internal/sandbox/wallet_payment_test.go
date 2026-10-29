package sandbox

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

type payerTestCommand struct {
	Hash          string            `json:"quote_hash"`
	Chain         int64             `json:"chain_id"`
	Token         string            `json:"token_address"`
	Signature     string            `json:"signature"`
	Authorization payerTestTransfer `json:"authorization"`
}
type payerTestTransfer struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Value       string `json:"value"`
	ValidAfter  string `json:"valid_after"`
	ValidBefore string `json:"valid_before"`
	Nonce       string `json:"nonce"`
}

// Independent EIP-712 oracle uses the published EIP-3009 transfer typehash and
// writes standard ABI words directly; it never calls the production digest.
func payerOracleDigest(t *testing.T, c payerTestCommand, name, version string) []byte {
	t.Helper()
	word := func(value string) []byte {
		n, ok := new(big.Int).SetString(value, 10)
		if !ok || n.Sign() < 0 || n.BitLen() > 256 {
			t.Fatal("invalid oracle integer")
		}
		return n.FillBytes(make([]byte, 32))
	}
	addr := func(value string) []byte {
		raw, e := hex.DecodeString(value[2:])
		if e != nil || len(raw) != 20 {
			t.Fatal("invalid oracle address")
		}
		return append(make([]byte, 12), raw...)
	}
	domain := bytes.Join([][]byte{keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)")), keccak256([]byte(name)), keccak256([]byte(version)), word(new(big.Int).SetInt64(c.Chain).String()), addr(c.Token)}, nil)
	typeHash, _ := hex.DecodeString("7c7c6cdb67a18743f49ec6fa9b35f50d52ed05cbed4cc592e13b44501c1a2267")
	nonce, e := hex.DecodeString(c.Authorization.Nonce[2:])
	if e != nil || len(nonce) != 32 {
		t.Fatal("invalid oracle nonce")
	}
	transfer := bytes.Join([][]byte{typeHash, addr(c.Authorization.From), addr(c.Authorization.To), word(c.Authorization.Value), word(c.Authorization.ValidAfter), word(c.Authorization.ValidBefore), nonce}, nil)
	return keccak256(bytes.Join([][]byte{{0x19, 0x01}, keccak256(domain), keccak256(transfer)}, nil))
}
func payerOracleSign(t *testing.T, c payerTestCommand, key *secp256k1.PrivateKey, name, version string) string {
	compact := ecdsa.SignCompact(key, payerOracleDigest(t, c, name, version), false)
	return "0x" + hex.EncodeToString(append(append([]byte(nil), compact[1:]...), compact[0]))
}
func payerCommandFor(t *testing.T, p Payment, key *secp256k1.PrivateKey) payerTestCommand {
	t.Helper()
	var envelope struct {
		Payer string `json:"payer"`
		Nonce string `json:"nonce"`
	}
	if e := json.Unmarshal(p.SignedQuote.Payload, &envelope); e != nil {
		t.Fatal(e)
	}
	amount, _ := new(big.Int).SetString(p.Quote.Amount, 10)
	fee, _ := new(big.Int).SetString(p.Quote.Fee, 10)
	c := payerTestCommand{Hash: p.Hash, Chain: 84532, Token: "0x036cbd53842c5426634e7929541ec2318f3dcf7e", Authorization: payerTestTransfer{From: envelope.Payer, To: p.Quote.Payee, Value: amount.Add(amount, fee).String(), ValidAfter: "0", ValidBefore: new(big.Int).SetInt64(p.Quote.ExpiresAt.Unix()).String(), Nonce: envelope.Nonce}}
	c.Signature = payerOracleSign(t, c, key, "USDC", "2")
	return c
}
func newPayerFixture(t *testing.T) (*walletHTTPFixture, *secp256k1.PrivateKey, Payment) {
	t.Helper()
	f := newWalletHTTPFixture(t)
	signing := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{12}, 32))
	configured, e := f.store.WithUSDCSigning(USDCSigningConfig{KeyID: "payer-fixture", PrivateKey: signing, PublicKeys: map[string]ed25519.PublicKey{"payer-fixture": signing.Public().(ed25519.PublicKey)}})
	if e != nil {
		t.Fatal(e)
	}
	configured, e = configured.WithUSDCTokenDomain(USDCTokenDomain{Name: "USDC", Version: "2"})
	if e != nil {
		t.Fatal(e)
	}
	f.store = configured
	key, address := walletTestKey(t, 7)
	c, e := f.store.CreateWalletChallenge(context.Background(), f.owner, "bind", address, walletTestOrigin)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.ProveWalletControl(context.Background(), f.owner, c.ID, "proof", signWalletMessage(t, key, c.Message), walletTestOrigin); e != nil {
		t.Fatal(e)
	}
	q := Quote{Product: "usdc", Asset: "USDC", Rail: "x402_test", Amount: "1000000", Fee: "100", Payee: "0x0000000000000000000000000000000000000001", TermsID: "test", TermsHash: hash([]byte("test"))}
	if e = f.store.SetPolicy(context.Background(), f.owner, 2, Policy{Asset: "USDC", Cap: "9999999", Products: []string{"usdc"}, Payees: []string{q.Payee}, ExpiresAt: time.Now().Add(time.Hour)}); e != nil {
		t.Fatal(e)
	}
	f.handler = mustWalletHandler(t, f.store, nil, map[string]Quote{"usdc": q})
	r := f.call(t, "", http.MethodPost, "/v1/payments", "quote", map[string]string{"product": "usdc"}, f.token)
	if r.Code != 201 {
		t.Fatalf("quote %d %s", r.Code, r.Body.String())
	}
	var p Payment
	if e = json.Unmarshal(r.Body.Bytes(), &p); e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.Authorize(context.Background(), f.token, p.ID, p.Hash, "reserve"); e != nil {
		t.Fatal(e)
	}
	return f, key, p
}

func TestPayerSignatureIsNotExposedToPaymentReaders(t *testing.T) {
	f, key, p := newPayerFixture(t)
	command := payerCommandFor(t, p, key)
	issued := f.call(t, "", http.MethodPost, "/v1/payments/"+p.ID+"/wallet-authorization", "private-payer-proof", command, f.token)
	if issued.Code != 200 {
		t.Fatalf("payer proof: %d %s", issued.Code, issued.Body.String())
	}
	reader, err := f.store.IssueCredential(context.Background(), f.owner, "payment-reader", []string{"payments:read"})
	if err != nil {
		t.Fatal(err)
	}
	read := f.call(t, "", http.MethodGet, "/v1/payments/"+p.ID, "", nil, reader)
	if read.Code != 200 {
		t.Fatalf("payment read: %d %s", read.Code, read.Body.String())
	}
	for _, response := range []string{issued.Body.String(), read.Body.String()} {
		if strings.Contains(response, command.Signature) {
			t.Fatal("public payment response exposed a broadcastable payer signature")
		}
		var receipt map[string]any
		if err := json.Unmarshal([]byte(response), &receipt); err != nil {
			t.Fatal(err)
		}
		if digest, ok := receipt["wallet_authorization_hash"].(string); !ok || len(digest) != 64 {
			t.Fatal("public payment lost its durable payer authorization receipt")
		}
	}
}

func TestWalletPaymentAuthorizationHTTP(t *testing.T) {
	f, key, p := newPayerFixture(t)
	command := payerCommandFor(t, p, key)
	if command.Authorization.Value != "1000100" {
		t.Fatal("gross includes wrong fee")
	}
	path := "/v1/payments/" + p.ID + "/wallet-authorization"
	for _, mutation := range []func(*payerTestCommand){
		func(c *payerTestCommand) { c.Hash = strings.Repeat("0", 64) },
		func(c *payerTestCommand) { c.Chain = 1 },
		func(c *payerTestCommand) { c.Token = "0x0000000000000000000000000000000000000001" },
		func(c *payerTestCommand) { c.Authorization.From = "0x0000000000000000000000000000000000000002" },
		func(c *payerTestCommand) { c.Authorization.To = "0x0000000000000000000000000000000000000002" },
		func(c *payerTestCommand) { c.Authorization.Value = "1000200" },
		func(c *payerTestCommand) { c.Authorization.Nonce = "0x" + strings.Repeat("0", 64) },
		func(c *payerTestCommand) { c.Authorization.ValidAfter = "1" },
		func(c *payerTestCommand) { c.Authorization.ValidBefore = "1" },
	} {
		altered := command
		mutation(&altered)
		altered.Signature = payerOracleSign(t, altered, key, "USDC", "2")
		r := f.call(t, "", http.MethodPost, path, randomID(), altered, f.token)
		if r.Code != 409 && r.Code != 400 {
			t.Fatalf("tampered transfer accepted %d %s", r.Code, r.Body.String())
		}
	}
	for _, signature := range []string{payerOracleSign(t, command, key, "wrong-token-name", "2"), payerOracleSign(t, command, key, "USDC", "wrong-version"), "0x" + strings.Repeat("0", 130)} {
		altered := command
		altered.Signature = signature
		r := f.call(t, "", http.MethodPost, path, randomID(), altered, f.token)
		if r.Code != 401 && r.Code != 400 {
			t.Fatalf("bad signature accepted %d %s", r.Code, r.Body.String())
		}
	}
	if r := f.call(t, "", http.MethodPost, path, "network", command, "ug_network"); r.Code != 401 {
		t.Fatal("network credential accepted")
	}
	sibling, e := f.store.IssueCredential(context.Background(), f.owner, "sibling", []string{"payments:authorize", "payments:read"})
	if e != nil {
		t.Fatal(e)
	}
	if r := f.call(t, "", http.MethodPost, path, "sibling", command, sibling); r.Code != 401 {
		t.Fatal("different quote credential accepted")
	}
	foreign := Owner{Issuer: f.owner.Issuer, Subject: randomID()}
	if _, e = f.store.Enroll(context.Background(), foreign); e != nil {
		t.Fatal(e)
	}
	foreignToken, e := f.store.IssueCredential(context.Background(), foreign, "foreign", []string{"payments:authorize", "payments:read"})
	if e != nil {
		t.Fatal(e)
	}
	if r := f.call(t, "", http.MethodPost, path, "foreign", command, foreignToken); r.Code != 401 {
		t.Fatal("foreign actor accepted")
	}
	var beforeAuth int
	if e = f.db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_sandbox.sandbox_payments WHERE id=$1 AND payer_authorization IS NOT NULL", p.ID).Scan(&beforeAuth); e != nil || beforeAuth != 0 {
		t.Fatal("denied proof changed persistence")
	}
	// A missing external proof leaves the separate policy reservation intact.
	if _, e = f.store.BeginDispatch(context.Background(), p.ID); !errors.Is(e, ErrUnauthorized) {
		t.Fatalf("missing proof dispatched: %v", e)
	}
	retained, e := f.store.GetPayment(context.Background(), f.token, p.ID)
	if e != nil || retained.State != "authorized" {
		t.Fatal("missing proof released reservation")
	}
	var wg sync.WaitGroup
	codes := make(chan int, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := f.call(t, "", http.MethodPost, path, "external-payer", command, f.token)
			codes <- r.Code
		}()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 200 {
			t.Fatalf("concurrent proof failed: %d", code)
		}
	}
	r := f.call(t, "", http.MethodPost, path, "external-payer", command, f.token)
	if r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	var result Payment
	if e = json.Unmarshal(r.Body.Bytes(), &result); e != nil || len(result.WalletAuthorizationHash) != 64 {
		t.Fatal("missing authorization receipt")
	}
	altered := command
	altered.Authorization.Value = "1"
	if r = f.call(t, "", http.MethodPost, path, "external-payer", altered, f.token); r.Code != 409 {
		t.Fatal("changed retry accepted")
	}
	if r = f.call(t, "", http.MethodPost, path, "different-key", command, f.token); r.Code != 409 {
		t.Fatal("duplicate independent proof inserted")
	}
	var records int
	var reserved string
	if e = f.db.QueryRow(context.Background(), "SELECT count(*),sum(amount+fee)::text FROM undergrid_sandbox.sandbox_payments WHERE id=$1 AND state='authorized' AND payer_authorization IS NOT NULL", p.ID).Scan(&records, &reserved); e != nil || records != 1 || reserved != "1000100" {
		t.Fatalf("duplicate financial effect %d %s %v", records, reserved, e)
	}
	restarted, e := New(f.store.dsn).WithUSDCVerification(f.store.usdcVerification)
	if e != nil {
		t.Fatal(e)
	}
	restarted, e = restarted.WithUSDCTokenDomain(USDCTokenDomain{Name: "USDC", Version: "2"})
	if e != nil {
		t.Fatal(e)
	}
	var submitted WalletPaymentCommand
	if e = json.Unmarshal(encode(command), &submitted); e != nil {
		t.Fatal(e)
	}
	replayed, e := restarted.AuthorizeWalletPayment(context.Background(), f.token, p.ID, "external-payer", submitted)
	if e != nil || replayed.walletAuthorization == nil {
		t.Fatal("restart recovery failed")
	}
	persistedBefore, e := f.store.GetPayment(context.Background(), f.token, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	if dispatched, e := restarted.BeginDispatch(context.Background(), p.ID); e != nil || dispatched.State != "submitting" {
		t.Fatalf("valid payer dispatch failed: %v", e)
	}
	if e = restarted.MarkUnknown(context.Background(), p.ID); e != nil {
		t.Fatal(e)
	}
	newKey, address := walletTestKey(t, 9)
	challenge, e := f.store.CreateWalletChallenge(context.Background(), f.owner, "rebind", address, walletTestOrigin)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.ProveWalletControl(context.Background(), f.owner, challenge.ID, "rebind-proof", signWalletMessage(t, newKey, challenge.Message), walletTestOrigin); e != nil {
		t.Fatal(e)
	}
	replayed, e = restarted.AuthorizeWalletPayment(context.Background(), f.token, p.ID, "external-payer", submitted)
	if e != nil || replayed.State != "unknown" || !bytes.Equal(encode(replayed.walletAuthorization), encode(persistedBefore.walletAuthorization)) || replayed.ProviderKey != persistedBefore.ProviderKey {
		t.Fatal("unknown authorization/snapshot/key changed after rebind")
	}
	if _, e = restarted.BeginDispatch(context.Background(), p.ID); !errors.Is(e, ErrConflict) {
		t.Fatal("unknown redispatched")
	}
	// Raw application-role attempts cannot null any normalized proof component.
	for _, column := range []string{"payer_authorization", "payer_authorization_hash", "payer_authorization_key", "payer_authorization_request_hash"} {
		e = f.store.transaction(context.Background(), "undergrid_sandbox_app", func(tx pgx.Tx) error {
			a, err := agentContext(context.Background(), tx, f.token, "payments:quote")
			if err != nil {
				return err
			}
			_, err = tx.Exec(context.Background(), `INSERT INTO sandbox_payments(id,actor_id,credential_id,quote,quote_hash,asset,amount,fee,provider_key,state,wallet_binding_id,wallet_binding_epoch,signed_quote_payload,signed_quote_signature,signed_quote_key_id,payer_authorization,payer_authorization_hash,payer_authorization_key,payer_authorization_request_hash) SELECT $1,actor_id,credential_id,quote,quote_hash,asset,amount,fee,$2,state,wallet_binding_id,wallet_binding_epoch,signed_quote_payload,signed_quote_signature,signed_quote_key_id,`+strings.Join(payerNullExpressions(column), ",")+` FROM sandbox_payments WHERE id=$3 AND actor_id=$4`, randomID(), randomID(), p.ID, a.actor)
			return err
		})
		var pgerr *pgconn.PgError
		if !errors.As(e, &pgerr) || pgerr.Code != "23514" {
			t.Fatalf("NULL %s accepted %v", column, e)
		}
	}
	// A complete outer record must not hide a NULL token-domain field.
	e = f.store.transaction(context.Background(), "undergrid_sandbox_app", func(tx pgx.Tx) error {
		a, err := agentContext(context.Background(), tx, f.token, "payments:quote")
		if err != nil {
			return err
		}
		_, err = tx.Exec(context.Background(), `INSERT INTO sandbox_payments(id,actor_id,credential_id,quote,quote_hash,asset,amount,fee,provider_key,state,wallet_binding_id,wallet_binding_epoch,signed_quote_payload,signed_quote_signature,signed_quote_key_id,payer_authorization,payer_authorization_hash,payer_authorization_key,payer_authorization_request_hash) SELECT $1,actor_id,credential_id,quote,quote_hash,asset,amount,fee,$2,state,wallet_binding_id,wallet_binding_epoch,signed_quote_payload,signed_quote_signature,signed_quote_key_id,jsonb_set(payer_authorization,'{domain,version}','null'::jsonb),payer_authorization_hash,payer_authorization_key,payer_authorization_request_hash FROM sandbox_payments WHERE id=$3 AND actor_id=$4`, randomID(), randomID(), p.ID, a.actor)
		return err
	})
	var pgerr *pgconn.PgError
	if !errors.As(e, &pgerr) || pgerr.Code != "23514" {
		t.Fatalf("NULL domain version accepted: %v", e)
	}
	if e = f.store.RevokeCredentials(context.Background(), f.owner); e != nil {
		t.Fatal(e)
	}
	if r = f.call(t, "", http.MethodPost, path, "external-payer", command, f.token); r.Code != 401 {
		t.Fatal("revoked credential replay accepted")
	}
}
func payerNullExpressions(column string) []string {
	expressions := []string{"payer_authorization", "payer_authorization_hash", "payer_authorization_key", "payer_authorization_request_hash"}
	for i, v := range expressions {
		if v == column {
			expressions[i] = "NULL"
		}
	}
	return expressions
}
func TestUSDCQuoteRejectsUint256GrossOverflow(t *testing.T) {
	f, _, _ := newPayerFixture(t)
	q := Quote{Product: "usdc", Asset: "USDC", Rail: "x402_test", Amount: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)).String(), Fee: "1", Payee: "0x0000000000000000000000000000000000000001", TermsID: "test", TermsHash: hash([]byte("test")), ExpiresAt: time.Now().Add(time.Hour)}
	if _, err := f.store.CreateQuote(context.Background(), f.token, "overflow", q); err == nil {
		t.Fatal("uint256 overflow quote minted")
	}
}
func TestUSDCDispatchRequiresIndependentPayerAuthorization(t *testing.T) {
	f, _, p := newPayerFixture(t)
	if _, err := f.store.BeginDispatch(context.Background(), p.ID); err == nil {
		t.Fatal("server reservation dispatched without external payer authority")
	}
}

func TestUSDCQuoteUint256GrossBoundary(t *testing.T) {
	f, _, _ := newPayerFixture(t)
	maximum := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	q := Quote{Product: "usdc", Asset: "USDC", Rail: "x402_test", Amount: new(big.Int).Sub(new(big.Int).Set(maximum), big.NewInt(100)).String(), Fee: "100", Payee: "0x0000000000000000000000000000000000000001", TermsID: "test", TermsHash: hash([]byte("test")), ExpiresAt: time.Now().Add(time.Hour)}
	p, err := f.store.CreateQuote(context.Background(), f.token, "boundary", q)
	if err != nil {
		t.Fatalf("uint256 maximum rejected: %v", err)
	}
	var signed struct {
		Quote Quote `json:"quote"`
	}
	if err = json.Unmarshal(p.SignedQuote.Payload, &signed); err != nil {
		t.Fatal(err)
	}
	amount, _ := new(big.Int).SetString(signed.Quote.Amount, 10)
	fee, _ := new(big.Int).SetString(signed.Quote.Fee, 10)
	if amount.Add(amount, fee).Cmp(maximum) != 0 {
		t.Fatal("maximum gross truncated")
	}
	q.Amount = new(big.Int).Add(new(big.Int).Set(maximum), big.NewInt(1)).String()
	q.Fee = "0"
	if _, err = f.store.CreateQuote(context.Background(), f.token, "amount-overflow", q); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overlarge amount minted: %v", err)
	}
}

func TestWalletPayerAuthorizationRebindingBeforeCommandAndDispatch(t *testing.T) {
	for _, beforeCommand := range []bool{true, false} {
		f, key, p := newPayerFixture(t)
		command := payerCommandFor(t, p, key)
		if !beforeCommand {
			r := f.call(t, "", http.MethodPost, "/v1/payments/"+p.ID+"/wallet-authorization", "payer", command, f.token)
			if r.Code != 200 {
				t.Fatal(r.Body.String())
			}
		}
		replacement, address := walletTestKey(t, 18)
		challenge, err := f.store.CreateWalletChallenge(context.Background(), f.owner, "rebind", address, walletTestOrigin)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.store.ProveWalletControl(context.Background(), f.owner, challenge.ID, "proof-rebind", signWalletMessage(t, replacement, challenge.Message), walletTestOrigin); err != nil {
			t.Fatal(err)
		}
		if beforeCommand {
			r := f.call(t, "", http.MethodPost, "/v1/payments/"+p.ID+"/wallet-authorization", "payer", command, f.token)
			if r.Code != 409 {
				t.Fatalf("stale payer authority accepted: %d %s", r.Code, r.Body.String())
			}
		} else {
			var key, period, proof string
			if err := f.db.QueryRow(context.Background(), "SELECT provider_key,reservation_period::text,payer_authorization_hash FROM undergrid_sandbox.sandbox_payments WHERE id=$1", p.ID).Scan(&key, &period, &proof); err != nil {
				t.Fatal(err)
			}
			// Rebinding denies new dispatch but cannot revoke the accepted proof
			// held by its original payer. Retain historical-only reconciliation.
			if retained, err := f.store.BeginDispatch(context.Background(), p.ID); !errors.Is(err, ErrUnauthorized) || retained.State != "unknown" {
				t.Fatalf("stale payer dispatch: %s %v", retained.State, err)
			}
			var retained bool
			var effects int
			if err := f.db.QueryRow(context.Background(), `SELECT p.state='unknown' AND p.provider_key=$2 AND p.reservation_period::text=$3 AND p.payer_authorization_hash=$4 AND c.start_block IS NULL AND c.authorization_hash=$4 AND c.provider_key=$2 FROM undergrid_sandbox.sandbox_payments p JOIN undergrid_sandbox.sandbox_usdc_claims c ON c.payment_id=p.id WHERE p.id=$1`, p.ID, key, period, proof).Scan(&retained); err != nil {
				t.Fatal(err)
			}
			if err := f.db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1", p.ID).Scan(&effects); err != nil {
				t.Fatal(err)
			}
			if !retained || effects != 0 {
				t.Fatalf("historical proof retained=%v effects=%d", retained, effects)
			}
			if _, err := f.store.BeginDispatch(context.Background(), p.ID); !errors.Is(err, ErrConflict) {
				t.Fatalf("historical claim redispatched: %v", err)
			}
		}
	}
}

func TestWalletPayerAuthorizationExpiredQuote(t *testing.T) {
	f, key, original := newPayerFixture(t)
	q := original.Quote
	q.ExpiresAt = time.Now().UTC().Add(4 * time.Second)
	p, err := f.store.CreateQuote(context.Background(), f.token, "short-expiry", q)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Authorize(context.Background(), f.token, p.ID, p.Hash, "reserve-short"); err != nil {
		t.Fatal(err)
	}
	command := payerCommandFor(t, p, key)
	time.Sleep(time.Until(time.Unix(q.ExpiresAt.Unix(), 0)) + 10*time.Millisecond)
	r := f.call(t, "", http.MethodPost, "/v1/payments/"+p.ID+"/wallet-authorization", "expired", command, f.token)
	if r.Code != 409 {
		t.Fatalf("expired quote accepted %d %s", r.Code, r.Body.String())
	}
}
