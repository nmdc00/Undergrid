package network

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestOwnerConsoleRegistrationAndIntentionIsolation(t *testing.T) {
	s, db := fixture(t)
	a, b := client(t, s), client(t, s)
	code, config := a.call("GET", "/v1/owner/config", "", nil)
	if code != 200 || config["terms_version"] != "test-v1" {
		t.Fatal(code, config)
	}
	code, me := a.call("GET", "/v1/owner/me", "", nil)
	if code != 200 || me["id"] == nil {
		t.Fatal(code, me)
	}
	code, agents := a.call("GET", "/v1/agents", "", nil)
	if code != 200 {
		t.Fatal(code, agents)
	}
	agent := agents["items"].([]any)[0].(map[string]any)["id"].(string)
	body := map[string]any{}
	_ = json.Unmarshal(canonical(intentFixture()), &body)
	body = map[string]any{"agent_id": agent, "intent": body}
	code, created := a.call("POST", "/v1/owner/intents", "owner-create", body)
	if code != 201 {
		t.Fatal(code, created)
	}
	id := created["id"].(string)
	code, replay := a.call("POST", "/v1/owner/intents", "owner-create", body)
	if code != 201 || replay["id"] != id {
		t.Fatal("owner replay duplicated", code, replay)
	}
	var events int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1", id).Scan(&events); err != nil || events != 1 {
		t.Fatal("owner replay changed effects", events, err)
	}
	code, _ = a.call("POST", "/v1/owner/intents/"+id, "wrong-method", intentFixture())
	if code != 405 {
		t.Fatalf("unsupported method mutated intent: %d", code)
	}
	code, results := a.call("GET", "/v1/owner/intents/"+id+"/opportunities", "", nil)
	if code != 200 || len(results["items"].([]any)) != 0 {
		t.Fatal(code, results)
	}
	code, _ = b.call("GET", "/v1/owner/intents/"+id+"/opportunities", "", nil)
	if code != 404 {
		t.Fatalf("foreign result leak %d", code)
	}

	code, list := a.call("GET", "/v1/owner/intents", "", nil)
	if code != 200 || len(list["items"].([]any)) != 1 {
		t.Fatal(code, list)
	}
	code, _ = b.call("GET", "/v1/owner/intents/"+id, "", nil)
	if code != 404 {
		t.Fatalf("foreign intent leaked: %d", code)
	}
	code, _ = b.call("POST", "/v1/owner/intents", "foreign-agent", body)
	if code != 404 {
		t.Fatalf("foreign agent admitted: %d", code)
	}
	code, out := ownerCallMatch(a, "PATCH", "/v1/owner/intents/"+id, "pause-owner", created["etag"].(string), map[string]any{"state": "paused"})
	if code != 200 || out["state"] != "paused" {
		t.Fatal(code, out)
	}
	code, _ = ownerCallMatch(a, "PATCH", "/v1/owner/intents/"+id, "stale-owner", created["etag"].(string), map[string]any{"state": "active"})
	if code != 409 {
		t.Fatalf("stale owner accepted: %d", code)
	}
}

func ownerCallMatch(c testClient, method, path, key, match string, body any) (int, map[string]any) {
	r := httptest.NewRequest(method, path, bytes.NewReader(canonical(body)))
	r.Header.Set("X-Test-Owner", c.owner)
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("If-Match", match)
	w := httptest.NewRecorder()
	c.s.ServeHTTP(w, r)
	if c.observe != nil {
		c.observe(r, w, body)
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}
