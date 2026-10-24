package network

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"testing"
	"time"
)

func TestAgentSigningControlChallengeIsBoundedAndSingleUse(t *testing.T) {
	s, db := fixture(t)
	a, b := client(t, s), client(t, s)
	_, list := a.call("GET", "/v1/agents", "", nil)
	agent := list["items"].([]any)[0].(map[string]any)
	id := agent["id"].(string)
	public, private, _ := ed25519.GenerateKey(nil)
	input := map[string]any{"public_key": base64.RawURLEncoding.EncodeToString(public)}
	code, _ := ownerCallMatch(b, "POST", "/v1/owner/agents/"+id+"/control-challenge", "foreign", "\"1\"", input)
	if code != 404 {
		t.Fatalf("foreign challenge %d", code)
	}
	code, ch := ownerCallMatch(a, "POST", "/v1/owner/agents/"+id+"/control-challenge", "challenge", "\"1\"", input)
	if code != 201 {
		t.Fatal(code, ch)
	}
	signed := base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, []byte(ch["challenge"].(string))))
	proof := map[string]any{"challenge": ch["challenge"], "signature": signed}
	code, out := ownerCallMatch(a, "POST", "/v1/owner/agents/"+id+"/control-proof", "proof", ch["etag"].(string), proof)
	if code != 200 || out["key_control_verified"] != true || out["endpoint_control_verified"] != false {
		t.Fatal(code, out)
	}
	code, _ = ownerCallMatch(a, "POST", "/v1/owner/agents/"+id+"/control-proof", "replay", ch["etag"].(string), proof)
	if code != 409 {
		t.Fatalf("proof replay %d", code)
	}
	var consumed bool
	if err := db.QueryRow(context.Background(), "SELECT control_nonce_hash IS NULL FROM undergrid.agents WHERE id=$1", id).Scan(&consumed); err != nil || !consumed {
		t.Fatal(err, consumed)
	}
}

func TestCredentialRotationReplayRestartAndRecovery(t *testing.T) {
	s, db := fixture(t)
	if err := s.ConfigureCredentialIssuance("fixture-key", bytes.Repeat([]byte{9}, 32)); err != nil {
		t.Fatal(err)
	}
	a, b := client(t, s), client(t, s)
	_, list := a.call("GET", "/v1/agents", "", nil)
	agent := list["items"].([]any)[0].(map[string]any)
	id := agent["id"].(string)
	credential := agent["credentials"].([]any)[0].(map[string]any)["id"].(string)
	input := map[string]any{"scopes": []string{"intents:read"}}
	path := "/v1/owner/credentials/" + credential + "/rotate"
	code, _ := ownerCallMatch(b, "POST", path, "foreign", "\"1\"", input)
	if code != 404 {
		t.Fatal(code)
	}
	code, issued := ownerCallMatch(a, "POST", path, "rotate", "\"1\"", input)
	if code != 201 {
		t.Fatal(code, issued)
	}
	old := a.token
	newToken := issued["token"].(string)
	if newToken == old {
		t.Fatal("token unchanged")
	}
	code, _ = a.call("GET", "/v1/me", "", nil)
	if code != 401 {
		t.Fatalf("old credential active %d", code)
	}
	restarted := New(s.dsn, "development", s.ownerAuth)
	if err := restarted.ConfigureCredentialIssuance("fixture-key", bytes.Repeat([]byte{9}, 32)); err != nil {
		t.Fatal(err)
	}
	restarted.termsVersion = "test-v1"
	a.s = restarted
	code, replayed := ownerCallMatch(a, "POST", path, "rotate", "\"1\"", input)
	if code != 201 || replayed["token"] != newToken {
		t.Fatal("lost rotation response", code, replayed)
	}
	var raw string
	if err := db.QueryRow(context.Background(), "SELECT response::text FROM undergrid.idempotency WHERE actor_id=$1 AND operation=$2", id, "POST "+path).Scan(&raw); err != nil || bytes.Contains([]byte(raw), []byte(newToken)) {
		t.Fatal("plaintext token persisted", err, raw)
	}
	a.token = newToken
	code, _ = a.call("GET", "/v1/me", "", nil)
	if code != 200 {
		t.Fatal(code)
	}
	_, list = a.call("GET", "/v1/agents", "", nil)
	current := list["items"].([]any)[0].(map[string]any)
	code, recovered := ownerCallMatch(a, "POST", "/v1/owner/agents/"+id+"/recover-credential", "recover", current["etag"].(string), input)
	if code != 201 {
		t.Fatal(code, recovered)
	}
	code, _ = a.call("GET", "/v1/me", "", nil)
	if code != 401 {
		t.Fatalf("recovery left prior token live %d", code)
	}
	code, _ = ownerCallMatch(a, "POST", path, "rotate", "\"1\"", input)
	if code != 409 {
		t.Fatalf("superseded replay leaked token %d", code)
	}
	a.token = recovered["token"].(string)
	code, _ = a.call("GET", "/v1/me", "", nil)
	if code != 200 {
		t.Fatal(code)
	}
}

func TestControlRejectsBadSignatureExpiryAndReplacement(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	_, list := a.call("GET", "/v1/agents", "", nil)
	agent := list["items"].([]any)[0].(map[string]any)
	id := agent["id"].(string)
	public, private, _ := ed25519.GenerateKey(nil)
	other, wrong, _ := ed25519.GenerateKey(nil)
	_ = other
	path := "/v1/owner/agents/" + id
	code, ch := ownerCallMatch(a, "POST", path+"/control-challenge", "expiry-challenge", agent["etag"].(string), map[string]any{"public_key": base64.RawURLEncoding.EncodeToString(public)})
	if code != 201 {
		t.Fatal(code, ch)
	}
	code, replay := ownerCallMatch(a, "POST", path+"/control-challenge", "expiry-challenge", agent["etag"].(string), map[string]any{"public_key": base64.RawURLEncoding.EncodeToString(public)})
	if code != 201 || replay["challenge"] != ch["challenge"] {
		t.Fatal("challenge response loss", code, replay)
	}
	proof := map[string]any{"challenge": ch["challenge"], "signature": base64.RawURLEncoding.EncodeToString(ed25519.Sign(wrong, []byte(ch["challenge"].(string))))}
	code, _ = ownerCallMatch(a, "POST", path+"/control-proof", "bad-signature", ch["etag"].(string), proof)
	if code != 422 {
		t.Fatal(code)
	}
	proof["signature"] = base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, []byte(ch["challenge"].(string))))
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.agents SET control_expires_at=now()-interval '1 second' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	code, _ = ownerCallMatch(a, "POST", path+"/control-proof", "expired-signature", ch["etag"].(string), proof)
	if code != 409 {
		t.Fatalf("expired proof accepted %d", code)
	}
	code, next := ownerCallMatch(a, "POST", path+"/control-challenge", "replacement", ch["etag"].(string), map[string]any{"public_key": base64.RawURLEncoding.EncodeToString(public)})
	if code != 201 {
		t.Fatal(code, next)
	}
	code, _ = ownerCallMatch(a, "POST", path+"/control-proof", "old-challenge", next["etag"].(string), proof)
	if code != 409 {
		t.Fatalf("replaced proof accepted %d", code)
	}
}

func TestRotationConcurrencyScopeEscalationAndUnconfiguredRollback(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	// The shared fixture configures issuance so initial owner agent creation can
	// use the production path; this scenario deliberately removes it to prove
	// rotation leaves the prior credential live when operator configuration is
	// absent.
	s.issuanceKey = nil
	s.issuanceKeyID = ""
	_, list := a.call("GET", "/v1/agents", "", nil)
	agent := list["items"].([]any)[0].(map[string]any)
	id := agent["id"].(string)
	credential := agent["credentials"].([]any)[0].(map[string]any)["id"].(string)
	path := "/v1/owner/credentials/" + credential + "/rotate"
	code, _ := ownerCallMatch(a, "POST", path, "unconfigured", "\"1\"", map[string]any{"scopes": []string{"intents:read"}})
	if code != 503 {
		t.Fatal(code)
	}
	code, _ = a.call("GET", "/v1/me", "", nil)
	if code != 200 {
		t.Fatalf("failed rotation revoked live key %d", code)
	}
	if err := s.ConfigureCredentialIssuance("fixture-key", bytes.Repeat([]byte{9}, 32)); err != nil {
		t.Fatal(err)
	}
	code, _ = ownerCallMatch(a, "POST", path, "escalation", "\"1\"", map[string]any{"scopes": []string{"swarms:create"}})
	if code != 403 {
		t.Fatal(code)
	}
	result := make(chan int, 2)
	for _, key := range []string{"concurrent-a", "concurrent-b"} {
		go func(key string) {
			code, _ := ownerCallMatch(a, "POST", path, key, "\"1\"", map[string]any{"scopes": []string{"intents:read"}})
			result <- code
		}(key)
	}
	first, second := <-result, <-result
	if !((first == 201 && second == 409) || (first == 409 && second == 201)) {
		t.Fatalf("rotation race %d %d", first, second)
	}
	var live int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.credentials WHERE agent_id=$1 AND revoked_at IS NULL", id).Scan(&live); err != nil || live != 1 {
		t.Fatal(live, err)
	}
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.credentials SET scopes=ARRAY['swarms:create'] WHERE agent_id=$1", id); err == nil {
		t.Fatal("immutable issuance metadata changed")
	}
}

func TestSigningChallengeCannotSurviveAuthorityVersionChange(t *testing.T) {
	s, _ := fixture(t)
	if err := s.ConfigureCredentialIssuance("fixture-key", bytes.Repeat([]byte{9}, 32)); err != nil {
		t.Fatal(err)
	}
	a := client(t, s)
	_, list := a.call("GET", "/v1/agents", "", nil)
	agent := list["items"].([]any)[0].(map[string]any)
	id := agent["id"].(string)
	public, private, _ := ed25519.GenerateKey(nil)
	code, ch := ownerCallMatch(a, "POST", "/v1/owner/agents/"+id+"/control-challenge", "version-challenge", agent["etag"].(string), map[string]any{"public_key": base64.RawURLEncoding.EncodeToString(public)})
	if code != 201 {
		t.Fatal(code, ch)
	}
	code, _ = ownerCallMatch(a, "POST", "/v1/owner/agents/"+id+"/recover-credential", "change-authority", ch["etag"].(string), map[string]any{"scopes": []string{"intents:read"}})
	if code != 201 {
		t.Fatal(code)
	}
	_, list = a.call("GET", "/v1/agents", "", nil)
	current := list["items"].([]any)[0].(map[string]any)
	proof := map[string]any{"challenge": ch["challenge"], "signature": base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, []byte(ch["challenge"].(string))))}
	code, _ = ownerCallMatch(a, "POST", "/v1/owner/agents/"+id+"/control-proof", "stale-authority-proof", current["etag"].(string), proof)
	if code != 409 {
		t.Fatalf("challenge crossed authority version %d", code)
	}
}

func TestRotationReplayWindowAndIssuanceKeyMismatch(t *testing.T) {
	s, _ := fixture(t)
	if err := s.ConfigureCredentialIssuance("fixture-key", bytes.Repeat([]byte{9}, 32)); err != nil {
		t.Fatal(err)
	}
	a := client(t, s)
	_, list := a.call("GET", "/v1/agents", "", nil)
	agent := list["items"].([]any)[0].(map[string]any)
	credential := agent["credentials"].([]any)[0].(map[string]any)["id"].(string)
	path := "/v1/owner/credentials/" + credential + "/rotate"
	input := map[string]any{"scopes": []string{"intents:read"}}
	code, issued := ownerCallMatch(a, "POST", path, "window", "\"1\"", input)
	if code != 201 {
		t.Fatal(code, issued)
	}
	s.credentialClock = func() time.Time { return time.Now().Add(16 * time.Minute) }
	code, _ = ownerCallMatch(a, "POST", path, "window", "\"1\"", input)
	if code != 409 {
		t.Fatalf("expired replay returned bearer %d", code)
	}
	s.credentialClock = nil
	if err := s.ConfigureCredentialIssuance("different-key-id", bytes.Repeat([]byte{9}, 32)); err != nil {
		t.Fatal(err)
	}
	code, _ = ownerCallMatch(a, "POST", path, "window", "\"1\"", input)
	if code != 409 {
		t.Fatalf("wrong key ID returned bearer %d", code)
	}
	if err := s.ConfigureCredentialIssuance("fixture-key", bytes.Repeat([]byte{10}, 32)); err != nil {
		t.Fatal(err)
	}
	code, _ = ownerCallMatch(a, "POST", path, "window", "\"1\"", input)
	if code != 409 {
		t.Fatalf("wrong key secret returned bearer %d", code)
	}
	a.token = issued["token"].(string)
	code, _ = a.call("GET", "/v1/me", "", nil)
	if code != 200 {
		t.Fatalf("key rollover changed existing digest auth %d", code)
	}
}

func TestCredentialIssuanceConfigurationHasNoImplicitKey(t *testing.T) {
	s := New("", "production", nil)
	s.cursorKey = bytes.Repeat([]byte{7}, 32)
	for _, input := range []struct {
		id  string
		key []byte
	}{{"", bytes.Repeat([]byte{9}, 32)}, {"key", nil}, {"key", bytes.Repeat([]byte{9}, 31)}, {"key", bytes.Repeat([]byte{9}, 33)}, {"key", s.cursorKey}} {
		if err := s.ConfigureCredentialIssuance(input.id, input.key); err == nil {
			t.Fatal("invalid issuance key config accepted")
		}
	}
	key := bytes.Repeat([]byte{9}, 32)
	if err := s.ConfigureCredentialIssuance("fixture-only", key); err != nil {
		t.Fatal(err)
	}
	key[0] = 0
	if s.issuanceKey[0] != 9 {
		t.Fatal("caller mutated owned issuance key")
	}
}
