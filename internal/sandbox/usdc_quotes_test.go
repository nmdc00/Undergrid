package sandbox

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestUSDCQuoteHTTPRequiresWalletAndTrustedSigner(t *testing.T) {
	f := newWalletHTTPFixture(t)
	f.handler = mustWalletHandler(t, f.store, nil, map[string]Quote{"usdc": {Product: "usdc", Asset: "USDC", Rail: "x402_test", Amount: "1000000", Fee: "0", Payee: "0x0000000000000000000000000000000000000001", TermsID: "test", TermsHash: hash([]byte("test")), ExpiresAt: time.Now().Add(time.Hour)}})
	r := f.call(t, "", http.MethodPost, "/v1/payments", "quote", map[string]string{"product": "usdc"}, f.token)
	if r.Code != 401 {
		t.Fatalf("unsigned/unbound USDC must fail closed with authorization error: %d %s", r.Code, r.Body.String())
	}
	var p Payment
	if json.Unmarshal(r.Body.Bytes(), &p) == nil && p.ID != "" {
		t.Fatal("created unsigned payment")
	}
}

func TestUSDCQuoteSignedSnapshotAndRebinding(t *testing.T) {
	f := newWalletHTTPFixture(t)
	seed := make([]byte, 32)
	seed[0] = 7
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	configured, err := f.store.WithUSDCSigning(USDCSigningConfig{KeyID: "fixture-1", PrivateKey: priv, PublicKeys: map[string]ed25519.PublicKey{"fixture-1": pub}})
	if err != nil {
		t.Fatal(err)
	}
	f.store = configured
	q := Quote{Product: "usdc", Asset: "USDC", Rail: "x402_test", Amount: "1000000", Fee: "100", Payee: "0x0000000000000000000000000000000000000001", TermsID: "test", TermsHash: hash([]byte("test"))}
	f.handler = mustWalletHandler(t, configured, func(r *http.Request) (Owner, error) {
		if r.Header.Get("X-Test-Owner") != f.owner.Subject {
			return Owner{}, ErrUnauthorized
		}
		return f.owner, nil
	}, map[string]Quote{"usdc": q})
	bind := func(suffix byte) WalletBinding {
		key, address := walletTestKey(t, suffix)
		c, err := f.store.CreateWalletChallenge(context.Background(), f.owner, randomID(), address, walletTestOrigin)
		if err != nil {
			t.Fatal(err)
		}
		b, err := f.store.ProveWalletControl(context.Background(), f.owner, c.ID, randomID(), signWalletMessage(t, key, c.Message), walletTestOrigin)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	missing := f.call(t, "", http.MethodPost, "/v1/payments", "before-binding", map[string]string{"product": "usdc"}, f.token)
	if missing.Code != 401 {
		t.Fatalf("configured signer created unbound quote: %d", missing.Code)
	}
	for _, field := range []string{"payer", "chain_id", "token_address", "nonce", "amount_atomic", "payee", "expires_at"} {
		body := map[string]any{"product": "usdc", field: "tampered"}
		response := f.call(t, "", http.MethodPost, "/v1/payments", randomID(), body, f.token)
		if response.Code != 400 {
			t.Fatalf("client-controlled %s accepted: %d", field, response.Code)
		}
	}
	original := bind(7)
	if err = f.store.SetPolicy(context.Background(), f.owner, 2, Policy{Asset: "USDC", Cap: "10000000", Products: []string{"usdc"}, Payees: []string{q.Payee}, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	quote := func(key string) Payment {
		r := f.call(t, "", http.MethodPost, "/v1/payments", key, map[string]string{"product": "usdc"}, f.token)
		if r.Code != 201 {
			t.Fatalf("quote %d %s", r.Code, r.Body.String())
		}
		var p Payment
		if err = json.Unmarshal(r.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := quote("first")
	var wg sync.WaitGroup
	results := make(chan Payment, 6)
	failures := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			offer := q
			offer.ExpiresAt = time.Now().UTC().Add(15 * time.Minute)
			result, e := f.store.createCatalogQuote(context.Background(), f.token, "concurrent", offer)
			if e != nil {
				failures <- e
			} else {
				results <- result
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	var concurrentID string
	for result := range results {
		if concurrentID == "" {
			concurrentID = result.ID
		}
		if result.ID != concurrentID {
			t.Fatal("concurrent replay created second quote")
		}
	}
	if _, e := f.store.Authorize(context.Background(), f.token, p.ID, hash([]byte("tamper")), "tamper"); !errors.Is(e, ErrConflict) {
		t.Fatal("wrong quote hash authorized")
	}
	foreignOwner := Owner{Issuer: f.owner.Issuer, Subject: randomID()}
	if _, e := f.store.Enroll(context.Background(), foreignOwner); e != nil {
		t.Fatal(e)
	}
	foreignToken, e := f.store.IssueCredential(context.Background(), foreignOwner, "foreign", []string{"payments:read", "payments:authorize"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.GetPayment(context.Background(), foreignToken, p.ID); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("foreign actor read signed quote")
	}
	if _, e = f.store.Authorize(context.Background(), foreignToken, p.ID, p.Hash, "foreign"); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("foreign actor authorized quote")
	}

	if p.SignedQuote == nil || !ed25519.Verify(pub, p.SignedQuote.Payload, p.SignedQuote.Signature) {
		t.Fatal("independent signature verification failed")
	}
	var envelope map[string]any
	if err = json.Unmarshal(p.SignedQuote.Payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["actor_id"] != f.actorID || envelope["wallet_binding_id"] != original.ID || envelope["payer"] != original.Address || envelope["chain_id"] != float64(84532) || envelope["token_address"] != "0x036cbd53842c5426634e7929541ec2318f3dcf7e" || envelope["designation"] != "TEST" {
		t.Fatalf("wrong signed envelope: %v", envelope)
	}
	if string(quote("first").SignedQuote.Payload) != string(p.SignedQuote.Payload) {
		t.Fatal("retry changed signed bytes")
	}
	for _, field := range []string{"actor_id", "credential_id", "payer", "chain_id", "token_address", "nonce", "wallet_binding_epoch", "quote"} {
		old := envelope[field]
		envelope[field] = "tampered"
		tampered, _ := json.Marshal(envelope)
		if ed25519.Verify(pub, tampered, p.SignedQuote.Signature) {
			t.Fatalf("tamper accepted: %s", field)
		}
		envelope[field] = old
	}
	if _, err = f.store.Authorize(context.Background(), f.token, p.ID, p.Hash, "auth-first"); err != nil {
		t.Fatal(err)
	}
	dispatched, err := f.store.BeginDispatch(context.Background(), p.ID)
	if err != nil || dispatched.State != "submitting" {
		t.Fatalf("dispatch %s %v", dispatched.State, err)
	}
	if err = f.store.MarkUnknown(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	stale := quote("stale")
	staleAuthorized := quote("stale-authorized")
	if _, err = f.store.Authorize(context.Background(), f.token, staleAuthorized.ID, staleAuthorized.Hash, "authorize-stale"); err != nil {
		t.Fatal(err)
	}
	bind(8)
	if _, err = f.store.Authorize(context.Background(), f.token, stale.ID, stale.Hash, "stale"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale authorization accepted: %v", err)
	}
	if canceled, err := f.store.BeginDispatch(context.Background(), staleAuthorized.ID); !errors.Is(err, ErrUnauthorized) || canceled.State != "canceled" {
		t.Fatalf("stale dispatch %s %v", canceled.State, err)
	}
	current, err := f.store.GetPayment(context.Background(), f.token, p.ID)
	if err != nil || current.State != "unknown" || !bytes.Equal(current.SignedQuote.Payload, p.SignedQuote.Payload) {
		t.Fatalf("unknown payment changed: %v %s", err, current.State)
	}
	if _, err = f.store.BeginDispatch(context.Background(), p.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("unknown payment redispatched")
	}
	if quote("first").ID != p.ID {
		t.Fatal("unknown retry created fresh charge")
	}
	restarted, err := New(f.store.dsn).WithUSDCSigning(USDCSigningConfig{KeyID: "fixture-1", PrivateKey: priv, PublicKeys: map[string]ed25519.PublicKey{"fixture-1": pub}})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := restarted.GetPayment(context.Background(), f.token, p.ID)
	if err != nil || !bytes.Equal(recovered.SignedQuote.Signature, p.SignedQuote.Signature) {
		t.Fatal("restart lost quote signature")
	}
	if _, err = restarted.GetPayment(context.Background(), "ug_network_not_sandbox", p.ID); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("network token accepted")
	}
	historical := quote("historical-key")
	nextSeed := make([]byte, 32)
	nextSeed[0] = 22
	nextPrivate := ed25519.NewKeyFromSeed(nextSeed)
	rotated, e := New(f.store.dsn).WithUSDCSigning(USDCSigningConfig{KeyID: "fixture-2", PrivateKey: nextPrivate, PublicKeys: map[string]ed25519.PublicKey{"fixture-1": pub, "fixture-2": nextPrivate.Public().(ed25519.PublicKey)}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = rotated.Authorize(context.Background(), f.token, historical.ID, historical.Hash, "historical-auth"); e != nil {
		t.Fatalf("historical configured key rejected: %v", e)
	}

	// Exercise every nullable USDC shape component under the real app role.
	columns := []string{"wallet_binding_id", "wallet_binding_epoch", "signed_quote_payload", "signed_quote_signature", "signed_quote_key_id"}
	for nullIndex, column := range columns {
		expressions := append([]string(nil), columns...)
		expressions[nullIndex] = "NULL"
		err = f.store.transaction(context.Background(), "undergrid_sandbox_app", func(tx pgx.Tx) error {
			a, e := agentContext(context.Background(), tx, f.token, "payments:quote")
			if e != nil {
				return e
			}
			_, e = tx.Exec(context.Background(), `INSERT INTO sandbox_payments(id,actor_id,credential_id,quote,quote_hash,asset,amount,fee,provider_key,wallet_binding_id,wallet_binding_epoch,signed_quote_payload,signed_quote_signature,signed_quote_key_id) SELECT $1,actor_id,credential_id,quote,quote_hash,asset,amount,fee,$2,`+strings.Join(expressions, ",")+` FROM sandbox_payments WHERE id=$3 AND actor_id=$4`, randomID(), randomID(), p.ID, a.actor)
			return e
		})
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != "23514" {
			t.Fatalf("NULL %s accepted or wrong error: %v", column, err)
		}
	}

}

func TestUSDCSigningConfigurationRequiresExplicitMatchingKeys(t *testing.T) {
	seed := make([]byte, 32)
	seed[0] = 99
	private := ed25519.NewKeyFromSeed(seed)
	public := private.Public().(ed25519.PublicKey)
	keysJSON, _ := json.Marshal(map[string]string{"key1": base64.StdEncoding.EncodeToString(public)})
	c, err := ParseUSDCSigningConfig("key1", base64.StdEncoding.EncodeToString(private), string(keysJSON))
	if err != nil {
		t.Fatal(err)
	}
	configured, err := New("unused").WithUSDCSigning(c)
	if err != nil {
		t.Fatal(err)
	}
	c.PrivateKey[0] ^= 1
	c.PublicKeys["key1"][0] ^= 1
	if bytes.Equal(configured.usdcSigning.PrivateKey, c.PrivateKey) || bytes.Equal(configured.usdcSigning.PublicKeys["key1"], c.PublicKeys["key1"]) {
		t.Fatal("configuration retained mutable caller key")
	}
	for _, args := range [][3]string{{"", "", "{}"}, {"key1", "garbage", string(keysJSON)}, {"wrong", base64.StdEncoding.EncodeToString(configured.usdcSigning.PrivateKey), string(keysJSON)}} {
		if _, err = ParseUSDCSigningConfig(args[0], args[1], args[2]); err == nil {
			t.Fatal("unsafe signing configuration accepted")
		}
	}
}
