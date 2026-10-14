package network

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestOwnerAgentCreationResponseLossReplay(t *testing.T) {
	s, db := fixture(t)
	if err := s.ConfigureCredentialIssuance("owner-issue-fixture", bytes.Repeat([]byte{71}, 32)); err != nil {
		t.Fatal(err)
	}
	a, b := client(t, s), client(t, s)
	var jobsBefore int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE kind='builder_recruitment_v1' AND args->>'swarm_id'='' ").Scan(&jobsBefore); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"name": "retryable listener", "scopes": []string{"intents:read", "intents:write", "inbox:read"}, "profile": map[string]any{"public": true, "recruitment": true}}
	call := func(c testClient, key string, body any) (int, map[string]any) {
		t.Helper()
		r := httptest.NewRequest("POST", "/v1/agents", bytes.NewReader(canonical(body)))
		r.Header.Set("X-Test-Owner", c.owner)
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		c.s.ServeHTTP(w, r)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	if code, out := call(a, "", body); code != 400 || out["error"].(map[string]any)["code"] != "idempotency_key_required" {
		t.Fatalf("missing owner command key: %d %#v", code, out)
	}

	code, first := call(a, "first-agent-form-01", body)
	if code != 201 || first["token"] == nil {
		t.Fatalf("initial owner agent: %d %#v", code, first)
	}
	code, replay := call(a, "first-agent-form-01", body)
	if code != 201 || replay["id"] != first["id"] || replay["credential_id"] != first["credential_id"] || replay["token"] != first["token"] {
		t.Fatalf("same-key response-loss replay differs: %d %#v", code, replay)
	}
	changed := map[string]any{"name": "changed listener", "scopes": body["scopes"]}
	if code, _ = call(a, "first-agent-form-01", changed); code != 409 {
		t.Fatalf("changed-body replay: %d", code)
	}
	if code, other := call(b, "first-agent-form-01", body); code != 201 || other["id"] == first["id"] {
		t.Fatalf("key was not owner-scoped: %d %#v", code, other)
	}

	var agents, credentials, issuedEvents, recruitJobs int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.agents WHERE principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1)", a.owner).Scan(&agents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.credentials c JOIN undergrid.agents a ON a.id=c.agent_id WHERE a.principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1)", a.owner).Scan(&credentials); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events e JOIN undergrid.credentials c ON c.id=e.aggregate_id JOIN undergrid.agents a ON a.id=c.agent_id WHERE a.principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1) AND e.type='credential.issued'", a.owner).Scan(&issuedEvents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE kind='builder_recruitment_v1' AND args->>'swarm_id'='' ").Scan(&recruitJobs); err != nil {
		t.Fatal(err)
	}
	if agents != 2 || credentials != 2 || issuedEvents != 2 || recruitJobs != jobsBefore+2 {
		t.Fatalf("same-owner effects agents=%d credentials=%d issued-events=%d unexpected-recruit-jobs=%d", agents, credentials, issuedEvents, recruitJobs)
	}
	var raw []byte
	if err := db.QueryRow(context.Background(), "SELECT response FROM undergrid.idempotency WHERE owner_principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1) AND operation=$2 AND key=$3", a.owner, "POST /v1/agents", "first-agent-form-01").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(first["token"].(string))) {
		t.Fatal("persisted owner creation result contains bearer token")
	}
	agentRead, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer agentRead.Rollback(context.Background())
	if _, err = agentRead.Exec(context.Background(), "SET LOCAL ROLE undergrid_app; SET LOCAL search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	if _, err = agentRead.Exec(context.Background(), "SELECT set_config('undergrid.principal_id',$1,true),set_config('undergrid.agent_id',$2,true)", first["principal_id"], first["id"]); err != nil {
		t.Fatal(err)
	}
	var visible int
	if err = agentRead.QueryRow(context.Background(), "SELECT count(*) FROM idempotency WHERE owner_principal_id=$1 AND operation='POST /v1/agents' AND key='first-agent-form-01'", first["principal_id"]).Scan(&visible); err != nil || visible != 0 {
		t.Fatalf("agent context could read owner command result: visible=%d err=%v", visible, err)
	}
	// A fresh service process with the same operator key can recreate the token.
	restarted := New(s.dsn, s.environment, s.ownerAuth)
	restarted.termsVersion = "test-v1"
	if err := restarted.ConfigureCredentialIssuance("owner-issue-fixture", bytes.Repeat([]byte{71}, 32)); err != nil {
		t.Fatal(err)
	}
	a.s = restarted
	code, afterRestart := call(a, "first-agent-form-01", body)
	if code != 201 || afterRestart["token"] != first["token"] {
		t.Fatalf("replay after restart: %d %#v", code, afterRestart)
	}
	if code, _ = a.call("POST", "/v1/owner/credentials/"+first["credential_id"].(string)+"/revoke", "revoke-first-token", map[string]any{}); code != 200 {
		t.Fatalf("revoke created credential: %d", code)
	}
	if code, _ = call(a, "first-agent-form-01", body); code != 409 {
		t.Fatalf("revoked credential replay was regenerated: %d", code)
	}

	// A bearer credential does not establish the verified owner command context.
	r := httptest.NewRequest("POST", "/v1/agents", bytes.NewReader(canonical(body)))
	r.Header.Set("Authorization", "Bearer "+first["token"].(string))
	r.Header.Set("Idempotency-Key", "bearer-attempt-001")
	w := httptest.NewRecorder()
	restarted.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("agent bearer claimed owner command: %d %s", w.Code, w.Body.String())
	}
}

func TestOwnerAgentCreationWithoutIssuanceKeyRollsBack(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	unconfigured := New(s.dsn, s.environment, s.ownerAuth)
	body := map[string]any{"name": "unconfigured create", "scopes": []string{"intents:read"}}
	r := httptest.NewRequest("POST", "/v1/agents", bytes.NewReader(canonical(body)))
	r.Header.Set("X-Test-Owner", a.owner)
	r.Header.Set("Idempotency-Key", "unconfigured-owner-agent")
	w := httptest.NewRecorder()
	unconfigured.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("missing dedicated issuance key status=%d body=%s", w.Code, w.Body.String())
	}
	var agents, credentials, results int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.agents WHERE principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1)", a.owner).Scan(&agents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.credentials c JOIN undergrid.agents a ON a.id=c.agent_id WHERE a.principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1)", a.owner).Scan(&credentials); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.idempotency WHERE owner_principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1) AND key='unconfigured-owner-agent'", a.owner).Scan(&results); err != nil {
		t.Fatal(err)
	}
	if agents != 1 || credentials != 1 || results != 0 {
		t.Fatalf("missing-key partial write agents=%d credentials=%d command-results=%d", agents, credentials, results)
	}
}

func TestOwnerAgentCreationReplayWindowAndKeyIDFailClosed(t *testing.T) {
	s, db := fixture(t)
	const issuanceKeyID = "owner-replay-test-key"
	key := bytes.Repeat([]byte{73}, 32)
	if err := s.ConfigureCredentialIssuance(issuanceKeyID, key); err != nil {
		t.Fatal(err)
	}
	a := client(t, s)
	body := map[string]any{"name": "windowed listener", "scopes": []string{"intents:read"}}
	const commandKey = "owner-replay-window-key-001"
	call := func() (int, map[string]any) {
		t.Helper()
		r := httptest.NewRequest("POST", "/v1/agents", bytes.NewReader(canonical(body)))
		r.Header.Set("X-Test-Owner", a.owner)
		r.Header.Set("Idempotency-Key", commandKey)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	code, first := call()
	if code != 201 || first["token"] == nil {
		t.Fatalf("initial owner agent: %d %#v", code, first)
	}
	var agentsBefore, credentialsBefore, issuedBefore, idempotencyBefore int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.agents WHERE principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1)", a.owner).Scan(&agentsBefore); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.credentials WHERE agent_id=$1", first["id"]).Scan(&credentialsBefore); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='credential.issued'", first["credential_id"]).Scan(&issuedBefore); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.idempotency WHERE owner_principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1) AND operation='POST /v1/agents' AND key=$2", a.owner, commandKey).Scan(&idempotencyBefore); err != nil {
		t.Fatal(err)
	}

	s.credentialClock = func() time.Time { return time.Now().Add(16 * time.Minute) }
	if code, out := call(); code != 409 || out["error"].(map[string]any)["code"] != "credential_replay_unavailable" {
		t.Fatalf("expired owner replay window was not denied: %d %#v", code, out)
	}
	s.credentialClock = nil
	if err := s.ConfigureCredentialIssuance("changed-owner-replay-key", key); err != nil {
		t.Fatal(err)
	}
	if code, out := call(); code != 409 || out["error"].(map[string]any)["code"] != "credential_replay_unavailable" {
		t.Fatalf("owner replay under changed issuance key ID was not denied: %d %#v", code, out)
	}

	var agentsAfter, credentialsAfter, issuedAfter, idempotencyAfter int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.agents WHERE principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1)", a.owner).Scan(&agentsAfter); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.credentials WHERE agent_id=$1", first["id"]).Scan(&credentialsAfter); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='credential.issued'", first["credential_id"]).Scan(&issuedAfter); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.idempotency WHERE owner_principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1) AND operation='POST /v1/agents' AND key=$2", a.owner, commandKey).Scan(&idempotencyAfter); err != nil {
		t.Fatal(err)
	}
	if agentsAfter != agentsBefore || credentialsAfter != credentialsBefore || issuedAfter != issuedBefore || idempotencyAfter != idempotencyBefore {
		t.Fatalf("denied replay changed effects: agents %d->%d credentials %d->%d events %d->%d results %d->%d", agentsBefore, agentsAfter, credentialsBefore, credentialsAfter, issuedBefore, issuedAfter, idempotencyBefore, idempotencyAfter)
	}
}

func TestOwnerAgentCreationConcurrentSameKey(t *testing.T) {
	s, db := fixture(t)
	if err := s.ConfigureCredentialIssuance("owner-issue-fixture", bytes.Repeat([]byte{72}, 32)); err != nil {
		t.Fatal(err)
	}
	a := client(t, s)
	body := map[string]any{"name": "one listener", "scopes": []string{"intents:read"}}
	const key = "parallel-owner-create-001"
	var wg sync.WaitGroup
	type result struct {
		code int
		body map[string]any
	}
	results := make(chan result, 8)
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("POST", "/v1/agents", bytes.NewReader(canonical(body)))
			r.Header.Set("X-Test-Owner", a.owner)
			r.Header.Set("Idempotency-Key", key)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			var out map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &out)
			results <- result{w.Code, out}
		}()
	}
	wg.Wait()
	close(results)
	var agentID, token string
	for result := range results {
		if result.code != 201 {
			t.Fatalf("parallel replay %d %#v", result.code, result.body)
		}
		if agentID == "" {
			agentID = result.body["id"].(string)
			token = result.body["token"].(string)
		}
		if result.body["id"] != agentID || result.body["token"] != token {
			t.Fatalf("parallel command returned multiple agents/tokens: %#v", result.body)
		}
	}
	var agents, credentials int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.agents WHERE principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1)", a.owner).Scan(&agents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.credentials c JOIN undergrid.agents a ON a.id=c.agent_id WHERE a.principal_id=(SELECT id FROM undergrid.principals WHERE subject=$1)", a.owner).Scan(&credentials); err != nil {
		t.Fatal(err)
	}
	if agents != 2 || credentials != 2 {
		t.Fatal(fmt.Sprintf("parallel duplicate effects agents=%d credentials=%d", agents, credentials))
	}
}
