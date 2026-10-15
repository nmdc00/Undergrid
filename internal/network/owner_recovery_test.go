package network

import (
	"context"
	"testing"
	"time"
)

func TestOwnerPolicyCommandRecoversOriginalResult(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	code, original := a.call("GET", "/v1/owner/policy", "", nil)
	if code != 200 {
		t.Fatal(code, original)
	}
	policy := map[string]any{"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), "max_active_tasks": 1}
	code, first := a.versioned("POST", "/v1/owner/policy", "lost-policy-result", original["etag"].(string), policy)
	if code != 200 {
		t.Fatal(code, first)
	}
	code, replay := a.versioned("POST", "/v1/owner/policy", "lost-policy-result", original["etag"].(string), policy)
	if code != 200 || digest(canonical(first)) != digest(canonical(replay)) {
		t.Fatalf("lost response did not recover original policy: %d %v", code, replay)
	}
	if code, _ = a.versioned("POST", "/v1/owner/policy", "lost-policy-result", first["etag"].(string), policy); code != 409 {
		t.Fatal("changed precondition reused command", code)
	}
	changed := map[string]any{"expires_at": policy["expires_at"], "max_active_tasks": 2}
	if code, _ = a.versioned("POST", "/v1/owner/policy", "lost-policy-result", original["etag"].(string), changed); code != 409 {
		t.Fatal("changed body reused command", code)
	}
	if code, _ = a.versioned("POST", "/v1/owner/policy", "later-policy", first["etag"].(string), changed); code != 200 {
		t.Fatal("later policy failed", code)
	}
	code, replay = a.versioned("POST", "/v1/owner/policy", "lost-policy-result", original["etag"].(string), policy)
	if code != 200 || digest(canonical(first)) != digest(canonical(replay)) {
		t.Fatal("replay returned current instead of original policy", code, replay)
	}
	var receipts, events int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.idempotency WHERE owner_principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1) AND operation='POST /v1/owner/policy'", a.owner).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1) AND type='principal.policy_updated'", a.owner).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if receipts != 2 || events != 2 {
		t.Fatalf("replay repeated effects or lacks receipts: %d/%d", receipts, events)
	}
	if code, _ = a.versioned("POST", "/v1/owner/policy", "", first["etag"].(string), changed); code != 400 {
		t.Fatal("policy accepted no command key", code)
	}
}

func TestOwnerRevocationPersistsCommandReceipt(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	_, list := a.call("GET", "/v1/agents", "", nil)
	agent := list["items"].([]any)[0].(map[string]any)
	credential := agent["credentials"].([]any)[0].(map[string]any)
	path := "/v1/owner/credentials/" + credential["id"].(string) + "/revoke"
	for i := 0; i < 2; i++ {
		if code, out := a.call("POST", path, "lost-revocation", map[string]any{}); code != 200 {
			t.Fatal(code, out)
		}
	}
	var receipts, events int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.idempotency WHERE owner_principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1) AND operation=$2", a.owner, "POST "+path).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='credential.revoked'", credential["id"]).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 || events != 1 {
		t.Fatalf("revocation missing durable receipt or repeated effects: %d/%d", receipts, events)
	}
	if code, _ := a.call("POST", path, "", map[string]any{}); code != 400 {
		t.Fatal("revocation accepted no command key", code)
	}
}

func TestOwnerPolicyConcurrentReplayAndFailedCommandRollback(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	_, current := a.call("GET", "/v1/owner/policy", "", nil)
	original := current["etag"].(string)
	bad := map[string]any{"expires_at": time.Now().Add(time.Hour), "max_active_tasks": 51}
	if code, _ := a.versioned("POST", "/v1/owner/policy", "concurrent-policy", original, bad); code != 422 {
		t.Fatal("invalid policy committed", code)
	}
	good := map[string]any{"expires_at": bad["expires_at"], "max_active_tasks": 1}
	codes := make(chan int, 6)
	for i := 0; i < 6; i++ {
		go func() {
			code, _ := a.versioned("POST", "/v1/owner/policy", "concurrent-policy", original, good)
			codes <- code
		}()
	}
	for i := 0; i < 6; i++ {
		if code := <-codes; code != 200 {
			t.Fatal("concurrent recovery failed", code)
		}
	}
	var receipts, events int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.idempotency WHERE owner_principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1) AND operation='POST /v1/owner/policy'", a.owner).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1) AND type='principal.policy_updated'", a.owner).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 || events != 1 {
		t.Fatalf("concurrent/failed command repeated effects: %d/%d", receipts, events)
	}
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.principals SET status='suspended' WHERE subject=$1", a.owner); err != nil {
		t.Fatal(err)
	}
	if code, _ := a.versioned("POST", "/v1/owner/policy", "concurrent-policy", original, good); code != 403 {
		t.Fatal("inactive owner recovered receipt", code)
	}
}
