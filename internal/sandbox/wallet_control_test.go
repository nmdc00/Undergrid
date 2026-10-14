package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/sha3"
)

const walletTestOrigin = "https://sandbox-wallet.fixture.invalid"

type walletChallengeResponse struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	ChainID   int64     `json:"chain_id"`
	Message   string    `json:"message"`
	ExpiresAt time.Time `json:"expires_at"`
	State     string    `json:"state"`
}

type walletBindingResponse struct {
	ID          string    `json:"id"`
	Address     string    `json:"address"`
	ChainID     int64     `json:"chain_id"`
	Epoch       int64     `json:"epoch"`
	ProofHash   string    `json:"proof_hash"`
	BoundAt     time.Time `json:"bound_at"`
	Designation string    `json:"designation"`
}

type walletHTTPFixture struct {
	db      *pgx.Conn
	store   *Store
	handler http.Handler
	owner   Owner
	actorID string
	token   string
}

func mustWalletHandler(t *testing.T, store *Store, owner OwnerAuthenticator, products map[string]Quote) *Handler {
	t.Helper()
	h, err := NewHandler(store, owner, products, walletTestOrigin)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestWalletControlHandlerRequiresFixedHTTPSOrigin(t *testing.T) {
	for _, origin := range []string{"", "http://sandbox.example", "https://sandbox.example/path", "https://user@sandbox.example", "https://sandbox.example?x=1"} {
		if _, err := NewHandler(New("unused"), nil, nil, origin); err == nil {
			t.Fatalf("unsafe wallet signing origin accepted: %q", origin)
		}
	}
	if _, err := NewHandler(New("unused"), nil, nil, walletTestOrigin); err != nil {
		t.Fatalf("valid sandbox origin rejected: %v", err)
	}
}

func newWalletHTTPFixture(t *testing.T) *walletHTTPFixture {
	t.Helper()
	dsn := os.Getenv("UNDERGRID_SANDBOX_TEST_DATABASE_URL")
	runtime := os.Getenv("UNDERGRID_SANDBOX_TEST_RUNTIME_DATABASE_URL")
	if dsn == "" || runtime == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("CI requires isolated sandbox migration and runtime databases")
		}
		t.Skip("requires isolated financial PostgreSQL with nonprivileged sandbox runtime")
	}
	ctx := context.Background()
	db, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	owner := Owner{Issuer: "https://owner.wallet-fixture.invalid", Subject: randomID()}
	store := New(runtime)
	actorID, err := store.Enroll(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SetPolicy(ctx, owner, 1, Policy{Asset: "USD", Cap: "1000", Products: []string{"platform-test"}, Payees: []string{"stripe-test-account"}, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	token, err := store.IssueCredential(ctx, owner, "wallet-agent", []string{"payments:quote", "payments:authorize", "payments:read"})
	if err != nil {
		t.Fatal(err)
	}
	h := mustWalletHandler(t, store, func(r *http.Request) (Owner, error) {
		subject := r.Header.Get("X-Test-Owner")
		if subject == "" {
			return Owner{}, ErrUnauthorized
		}
		return Owner{Issuer: owner.Issuer, Subject: subject}, nil
	}, map[string]Quote{"platform-test": {Product: "platform-test"}})
	return &walletHTTPFixture{db: db, store: store, handler: h, owner: owner, actorID: actorID, token: token}
}

func (f *walletHTTPFixture) call(t *testing.T, owner, method, path, key string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if owner != "" {
		req.Header.Set("X-Test-Owner", owner)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp := httptest.NewRecorder()
	f.handler.ServeHTTP(resp, req)
	return resp
}

func walletTestKey(t *testing.T, suffix byte) (*secp256k1.PrivateKey, string) {
	t.Helper()
	keyBytes := make([]byte, 32)
	keyBytes[31] = suffix
	key := secp256k1.PrivKeyFromBytes(keyBytes)
	pub := key.PubKey().SerializeUncompressed()
	digest := keccak256(pub[1:])
	address := checksumFixtureWalletAddress("0x" + hex.EncodeToString(digest[12:]))
	return key, address
}

func signWalletMessage(t *testing.T, key *secp256k1.PrivateKey, message string) string {
	t.Helper()
	preimage := []byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message))
	digest := keccak256(preimage)
	compact := ecdsa.SignCompact(key, digest, false)
	recoveryID := compact[0] - 27
	if recoveryID > 1 {
		t.Fatal("fixture signer produced unsupported recovery id")
	}
	raw := append(append([]byte(nil), compact[1:]...), 27+recoveryID)
	return "0x" + hex.EncodeToString(raw)
}

func keccak256(message []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write(message)
	return h.Sum(nil)
}

func checksumFixtureWalletAddress(lower string) string {
	body := strings.TrimPrefix(strings.ToLower(lower), "0x")
	hash := keccak256([]byte(body))
	var out strings.Builder
	out.WriteString("0x")
	for i, c := range body {
		if c >= 'a' && c <= 'f' {
			nibble := hash[i/2] & 0x0f
			if i%2 == 0 {
				nibble = hash[i/2] >> 4
			}
			if nibble >= 8 {
				c -= 'a' - 'A'
			}
		}
		out.WriteByte(byte(c))
	}
	return out.String()
}

func decodeWalletChallenge(t *testing.T, resp *httptest.ResponseRecorder) walletChallengeResponse {
	t.Helper()
	var out walletChallengeResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode challenge: %v (%s)", err, resp.Body.String())
	}
	return out
}

func decodeWalletBinding(t *testing.T, resp *httptest.ResponseRecorder) walletBindingResponse {
	t.Helper()
	var out walletBindingResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode binding: %v (%s)", err, resp.Body.String())
	}
	return out
}

func TestWalletControlHTTPReplayRebindAndOwnerIsolation(t *testing.T) {
	f := newWalletHTTPFixture(t)
	ctx := context.Background()
	if unbound := f.call(t, f.owner.Subject, http.MethodGet, "/v1/owner/wallet-binding", "", nil, ""); unbound.Code != http.StatusNotFound {
		t.Fatalf("unbound wallet projection: %d %s", unbound.Code, unbound.Body.String())
	}
	key1, address1 := walletTestKey(t, 1)
	if malformed := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges", "malformed-wallet", map[string]string{"address": "0x00000000000000000000000000000000000000xz"}, ""); malformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed address accepted: %d %s", malformed.Code, malformed.Body.String())
	}
	badChecksum := []byte(address1)
	for i := 2; i < len(badChecksum); i++ {
		if badChecksum[i] >= 'a' && badChecksum[i] <= 'f' {
			badChecksum[i] -= 'a' - 'A'
			break
		}
		if badChecksum[i] >= 'A' && badChecksum[i] <= 'F' {
			badChecksum[i] += 'a' - 'A'
			break
		}
	}
	if invalidChecksum := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges", "checksum-wallet", map[string]string{"address": string(badChecksum)}, ""); invalidChecksum.Code != http.StatusBadRequest {
		t.Fatalf("invalid mixed-case checksum accepted: %d %s", invalidChecksum.Code, invalidChecksum.Body.String())
	}
	challengeBody := map[string]string{"address": address1}
	challengeResp := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges", "issue-wallet-1", challengeBody, "")
	if challengeResp.Code != http.StatusCreated {
		t.Fatalf("owner challenge: %d %s", challengeResp.Code, challengeResp.Body.String())
	}
	challenge1 := decodeWalletChallenge(t, challengeResp)
	if challenge1.ChainID != 84532 || challenge1.Address != strings.ToLower(address1) || !strings.Contains(challenge1.Message, "Chain ID: 84532") || !strings.Contains(challenge1.Message, "does not authorize payment") || !strings.Contains(challenge1.Message, walletTestOrigin) {
		t.Fatalf("challenge omitted exact origin, chain, normalized address, or control-only purpose: %+v", challenge1)
	}
	issuedReplay := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges", "issue-wallet-1", challengeBody, "")
	if issuedReplay.Code != challengeResp.Code || issuedReplay.Body.String() != challengeResp.Body.String() {
		t.Fatalf("challenge idempotency did not recover exact response: %d %s", issuedReplay.Code, issuedReplay.Body.String())
	}
	changedChallenge := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges", "issue-wallet-1", map[string]string{"address": strings.ToLower("0x0000000000000000000000000000000000000001")}, "")
	if changedChallenge.Code != http.StatusConflict {
		t.Fatalf("changed challenge request reused key: %d %s", changedChallenge.Code, changedChallenge.Body.String())
	}
	if denied := f.call(t, "", http.MethodPost, "/v1/owner/wallet-challenges", "agent-wallet", challengeBody, f.token); denied.Code != http.StatusUnauthorized {
		t.Fatalf("agent token could create owner wallet challenge: %d %s", denied.Code, denied.Body.String())
	}
	otherOwner := randomID()
	foreign := f.call(t, otherOwner, http.MethodGet, "/v1/owner/wallet-binding", "", nil, "")
	if foreign.Code != http.StatusNotFound && foreign.Code != http.StatusUnauthorized {
		t.Fatalf("other owner learned current wallet binding: %d %s", foreign.Code, foreign.Body.String())
	}
	proof := signWalletMessage(t, key1, challenge1.Message)
	proofBody := map[string]string{"signature": proof}
	if _, err := f.store.ProveWalletControl(ctx, f.owner, challenge1.ID, "wrong-origin", proof, "https://other-financial-origin.invalid"); err != ErrConflict {
		t.Fatalf("challenge signed for old origin was not rejected: %v", err)
	}
	foreignProof := f.call(t, otherOwner, http.MethodPost, "/v1/owner/wallet-challenges/"+challenge1.ID+"/prove", "foreign-wallet-proof", proofBody, "")
	if foreignProof.Code != http.StatusNotFound && foreignProof.Code != http.StatusUnauthorized {
		t.Fatalf("other owner consumed a foreign challenge: %d %s", foreignProof.Code, foreignProof.Body.String())
	}
	wrongAddressKey, _ := walletTestKey(t, 2)
	if malformed := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges/"+challenge1.ID+"/prove", "malformed-signature", map[string]string{"signature": "0x1234"}, ""); malformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed signature accepted: %d %s", malformed.Code, malformed.Body.String())
	}
	wrongAddress := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges/"+challenge1.ID+"/prove", "wrong-address", map[string]string{"signature": signWalletMessage(t, wrongAddressKey, challenge1.Message)}, "")
	if wrongAddress.Code != http.StatusBadRequest {
		t.Fatalf("signature by another address accepted: %d %s", wrongAddress.Code, wrongAddress.Body.String())
	}
	mutatedMessageProof := signWalletMessage(t, key1, strings.Replace(challenge1.Message, "does not authorize payment", "authorizes payment", 1))
	if changed := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges/"+challenge1.ID+"/prove", "altered-message", map[string]string{"signature": mutatedMessageProof}, ""); changed.Code != http.StatusBadRequest {
		t.Fatalf("signature of altered content accepted: %d %s", changed.Code, changed.Body.String())
	}
	proofResp := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges/"+challenge1.ID+"/prove", "bind-wallet-1", proofBody, "")
	if proofResp.Code != http.StatusOK {
		t.Fatalf("valid owner proof: %d %s", proofResp.Code, proofResp.Body.String())
	}
	binding1 := decodeWalletBinding(t, proofResp)
	if binding1.Epoch != 1 || binding1.ChainID != 84532 || binding1.Address != strings.ToLower(address1) || binding1.ProofHash == "" || !strings.HasPrefix(binding1.Designation, "TEST") {
		t.Fatalf("unexpected control binding: %+v", binding1)
	}
	rawProof, err := hex.DecodeString(strings.TrimPrefix(proof, "0x"))
	if err != nil {
		t.Fatal(err)
	}
	proofDigest := sha256.Sum256(rawProof)
	if binding1.ProofHash != hex.EncodeToString(proofDigest[:]) {
		t.Fatalf("proof receipt is not the signature digest: %s", binding1.ProofHash)
	}
	var rawSignatureColumns int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='undergrid_sandbox'
		AND table_name='sandbox_wallet_challenges' AND column_name IN('signature','raw_signature')`).Scan(&rawSignatureColumns); err != nil || rawSignatureColumns != 0 {
		t.Fatalf("wallet table retains raw signature: columns=%d err=%v", rawSignatureColumns, err)
	}
	proofReplay := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges/"+challenge1.ID+"/prove", "bind-wallet-1", proofBody, "")
	if proofReplay.Code != proofResp.Code || proofReplay.Body.String() != proofResp.Body.String() {
		t.Fatalf("proof retry did not recover exact original result: %d %s", proofReplay.Code, proofReplay.Body.String())
	}
	if replay := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges/"+challenge1.ID+"/prove", "different-key-replay", proofBody, ""); replay.Code != http.StatusConflict {
		t.Fatalf("consumed challenge replay accepted: %d %s", replay.Code, replay.Body.String())
	}
	changedProofBody := map[string]string{"signature": "0x" + strings.Repeat("00", 65)}
	if changed := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges/"+challenge1.ID+"/prove", "bind-wallet-1", changedProofBody, ""); changed.Code != http.StatusConflict {
		t.Fatalf("changed proof reused command key: %d %s", changed.Code, changed.Body.String())
	}
	current := f.call(t, f.owner.Subject, http.MethodGet, "/v1/owner/wallet-binding", "", nil, "")
	if current.Code != http.StatusOK || decodeWalletBinding(t, current).ID != binding1.ID {
		t.Fatalf("current binding read: %d %s", current.Code, current.Body.String())
	}
	if agentRead := f.call(t, "", http.MethodGet, "/v1/owner/wallet-binding", "", nil, f.token); agentRead.Code != http.StatusUnauthorized {
		t.Fatalf("agent token read owner wallet binding: %d %s", agentRead.Code, agentRead.Body.String())
	}

	key2, address2 := walletTestKey(t, 2)
	challenge2Resp := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges", "issue-wallet-2", map[string]string{"address": address2}, "")
	if challenge2Resp.Code != http.StatusCreated {
		t.Fatalf("rebind challenge: %d %s", challenge2Resp.Code, challenge2Resp.Body.String())
	}
	challenge2 := decodeWalletChallenge(t, challenge2Resp)
	proof2Body := map[string]string{"signature": signWalletMessage(t, key2, challenge2.Message)}
	proof2Resp := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges/"+challenge2.ID+"/prove", "bind-wallet-2", proof2Body, "")
	if proof2Resp.Code != http.StatusOK {
		t.Fatalf("valid wallet rebind: %d %s", proof2Resp.Code, proof2Resp.Body.String())
	}
	binding2 := decodeWalletBinding(t, proof2Resp)
	if binding2.Epoch != 2 || binding2.ID == binding1.ID || binding2.Address != strings.ToLower(address2) {
		t.Fatalf("wallet rebind did not advance immutable epoch: old=%+v new=%+v", binding1, binding2)
	}
	oldProofRetry := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges/"+challenge1.ID+"/prove", "bind-wallet-1", proofBody, "")
	if oldProofRetry.Code != proofResp.Code || oldProofRetry.Body.String() != proofResp.Body.String() {
		t.Fatalf("old proof retry changed after wallet rebind: %d %s", oldProofRetry.Code, oldProofRetry.Body.String())
	}
	var oldAddress, oldProof string
	var oldEpoch int64
	if err := f.db.QueryRow(ctx, `SELECT address,proof_hash,epoch FROM undergrid_sandbox.sandbox_wallet_challenges
		WHERE id=$1 AND state='bound'`, binding1.ID).Scan(&oldAddress, &oldProof, &oldEpoch); err != nil {
		t.Fatal(err)
	}
	if oldAddress != strings.ToLower(address1) || oldProof != binding1.ProofHash || oldEpoch != 1 {
		t.Fatalf("historical wallet binding changed after rebind: %s %s %d", oldAddress, oldProof, oldEpoch)
	}
	var bindingCount int
	if err = f.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_wallet_challenges WHERE actor_id=$1 AND state='bound'", f.actorID).Scan(&bindingCount); err != nil || bindingCount != 2 {
		t.Fatalf("wallet history did not retain exactly both bindings: count=%d err=%v", bindingCount, err)
	}
}

func TestWalletControlExpiredAndSupersededChallengesReject(t *testing.T) {
	f := newWalletHTTPFixture(t)
	key, address := walletTestKey(t, 3)
	f.store.walletChallengeTTL = time.Millisecond
	first := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges", "expired-wallet", map[string]string{"address": address}, "")
	if first.Code != http.StatusCreated {
		t.Fatalf("wallet challenge: %d %s", first.Code, first.Body.String())
	}
	expired := decodeWalletChallenge(t, first)
	time.Sleep(3 * time.Millisecond)
	if resp := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges/"+expired.ID+"/prove", "expired-proof", map[string]string{"signature": signWalletMessage(t, key, expired.Message)}, ""); resp.Code != http.StatusBadRequest {
		t.Fatalf("expired challenge accepted: %d %s", resp.Code, resp.Body.String())
	}
	second := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges", "superseded-wallet", map[string]string{"address": address}, "")
	if second.Code != http.StatusCreated {
		t.Fatalf("replacement challenge: %d %s", second.Code, second.Body.String())
	}
	stale := decodeWalletChallenge(t, second)
	third := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges", "newer-wallet", map[string]string{"address": address}, "")
	if third.Code != http.StatusCreated {
		t.Fatalf("new challenge: %d %s", third.Code, third.Body.String())
	}
	if resp := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges/"+stale.ID+"/prove", "stale-proof", map[string]string{"signature": signWalletMessage(t, key, stale.Message)}, ""); resp.Code != http.StatusConflict {
		t.Fatalf("superseded challenge accepted: %d %s", resp.Code, resp.Body.String())
	}
}

func TestWalletControlConcurrentProofCreatesOneBindingAndRLSIsActorScoped(t *testing.T) {
	f := newWalletHTTPFixture(t)
	key, address := walletTestKey(t, 4)
	challengeResp := f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges", "concurrent-wallet", map[string]string{"address": address}, "")
	if challengeResp.Code != http.StatusCreated {
		t.Fatalf("wallet challenge: %d %s", challengeResp.Code, challengeResp.Body.String())
	}
	challenge := decodeWalletChallenge(t, challengeResp)
	body := map[string]string{"signature": signWalletMessage(t, key, challenge.Message)}
	const workers = 8
	responses := make(chan *httptest.ResponseRecorder, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses <- f.call(t, f.owner.Subject, http.MethodPost, "/v1/owner/wallet-challenges/"+challenge.ID+"/prove", "concurrent-proof", body, "")
		}()
	}
	wg.Wait()
	close(responses)
	var expected string
	for resp := range responses {
		if resp.Code != http.StatusOK {
			t.Fatalf("concurrent proof did not return idempotent result: %d %s", resp.Code, resp.Body.String())
		}
		if expected == "" {
			expected = resp.Body.String()
		} else if resp.Body.String() != expected {
			t.Fatal("concurrent proof returned multiple wallet bindings")
		}
	}
	var binding walletBindingResponse
	if err := json.Unmarshal([]byte(expected), &binding); err != nil {
		t.Fatal(err)
	}
	if binding.Epoch != 1 || binding.ChainID != 84532 {
		t.Fatalf("concurrent proof result: %+v", binding)
	}

	otherOwner := Owner{Issuer: f.owner.Issuer, Subject: randomID()}
	otherActor, err := f.store.Enroll(context.Background(), otherOwner)
	if err != nil {
		t.Fatal(err)
	}
	otherChallenge := f.call(t, otherOwner.Subject, http.MethodPost, "/v1/owner/wallet-challenges", "other-wallet", map[string]string{"address": address}, "")
	if otherChallenge.Code != http.StatusCreated {
		t.Fatalf("other actor challenge: %d %s", otherChallenge.Code, otherChallenge.Body.String())
	}
	foreignChallenge := decodeWalletChallenge(t, otherChallenge)
	if foreignChallenge.ID == challenge.ID {
		t.Fatal("challenge ids not isolated")
	}
	app, err := pgx.Connect(context.Background(), os.Getenv("UNDERGRID_SANDBOX_TEST_RUNTIME_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close(context.Background())
	tx, err := app.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), "SET LOCAL ROLE undergrid_sandbox_app"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), "SELECT set_config('undergrid_sandbox.actor',$1,true)", f.actorID); err != nil {
		t.Fatal(err)
	}
	var visible int
	if err = tx.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_sandbox.sandbox_wallet_challenges WHERE actor_id=$1", otherActor).Scan(&visible); err != nil {
		t.Fatal(err)
	}
	if visible != 0 {
		t.Fatalf("wallet challenge RLS exposed foreign actor: %d", visible)
	}
}
