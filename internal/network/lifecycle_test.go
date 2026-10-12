package network

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

// These tests exercise the HTTP boundary, transaction logic and real RLS. The
// verified-owner fixture replaces only the external OIDC provider, not storage.
func TestDurableParticipation(t *testing.T) {
	dsn := os.Getenv("UNDERGRID_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("UNDERGRID_TEST_DATABASE_URL is required; no mocked persistence fallback")
	}
	ctx := context.Background()
	db, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	issuer := "https://identity.test/" + t.Name()
	service := New(dsn, "development", func(r *http.Request) (Owner, error) {
		return Owner{Issuer: issuer, Subject: r.Header.Get("X-Test-Owner"), Verified: true}, nil
	})
	call := func(owner, token, method, path, key, etag string, body any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("X-Test-Owner", owner)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Idempotency-Key", key)
		r.Header.Set("If-Match", etag)
		w := httptest.NewRecorder()
		service.ServeHTTP(w, r)
		var result map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &result)
		return w.Code, result
	}
	suffix := NewID()
	register := func(owner string) string {
		t.Helper()
		code, p := call(owner, "", "POST", "/v1/principals", "", "", map[string]any{"name": owner, "terms_version": "test-v1"})
		if code != 201 {
			t.Fatalf("register %d %#v", code, p)
		}
		code, a := call(owner, "", "POST", "/v1/agents", "", "", map[string]any{"name": "listener", "scopes": []string{"intents:read", "intents:write", "inbox:read"}})
		if code != 201 {
			t.Fatalf("agent %d %#v", code, a)
		}
		return a["token"].(string)
	}
	a, b := register("A"+suffix), register("B"+suffix)
	body := map[string]any{"objective": "European source catalog", "output": map[string]any{"kind": "json_dataset", "schema_family": "source_catalog_v1"}, "hard_constraints": map[string]any{"required_fields": []string{"source_id", "source_url"}, "license_ids": []string{"CC0-1.0"}}, "visibility": "private", "watch": map[string]any{"enabled": true, "channels": []string{"inbox"}}, "expires_at": "2099-01-01T00:00:00Z"}
	code, intent := call("", a, "POST", "/v1/intents", "create-1", "", body)
	if code != 201 {
		t.Fatalf("create %d %#v", code, intent)
	}
	id := intent["id"].(string)
	code, replay := call("", a, "POST", "/v1/intents", "create-1", "", body)
	if code != 201 || replay["id"] != id {
		t.Fatalf("retry duplicated result: %d %#v", code, replay)
	}
	body["objective"] = "Changed request"
	code, _ = call("", a, "POST", "/v1/intents", "create-1", "", body)
	if code != 409 {
		t.Fatalf("conflicting replay: %d", code)
	}
	code, _ = call("", b, "GET", "/v1/intents/"+id, "", "", nil)
	if code != 404 {
		t.Fatalf("private leak: %d", code)
	}
	code, changed := call("", a, "PATCH", "/v1/intents/"+id, "withdraw-1", intent["etag"].(string), map[string]any{"state": "withdrawn"})
	if code != 200 || changed["state"] != "withdrawn" {
		t.Fatalf("withdraw %d %#v", code, changed)
	}
	code, _ = call("", a, "PATCH", "/v1/intents/"+id, "stale-1", intent["etag"].(string), map[string]any{"state": "paused"})
	if code != 409 {
		t.Fatalf("stale write %d", code)
	}
	// A fresh handler/connection observes acknowledged state after restart.
	service = New(dsn, "development", nil)
	code, recovered := call("", a, "GET", "/v1/intents/"+id, "", "", nil)
	if code != 200 || recovered["state"] != "withdrawn" {
		t.Fatalf("restart lost state: %d %#v", code, recovered)
	}
	var revisions, events int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid.intent_revisions WHERE intent_id=$1", id).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1", id).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if revisions != 1 || events != 2 {
		t.Fatalf("persistence revisions=%d events=%d", revisions, events)
	}
	code, _ = call("", a, "DELETE", "/v1/credentials/current", "revoke-1", "", nil)
	if code != 200 {
		t.Fatalf("revoke %d", code)
	}
	code, _ = call("", a, "GET", "/v1/intents/"+id, "", "", nil)
	if code != 401 {
		t.Fatalf("revoked credential worked: %d", code)
	}
}
