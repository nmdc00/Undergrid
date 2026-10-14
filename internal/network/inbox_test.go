package network

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type testClient struct {
	t            *testing.T
	s            *Service
	owner, token string
	observe      func(*http.Request, *httptest.ResponseRecorder, any)
}

func (c testClient) call(method, path, key string, body any) (int, map[string]any) {
	c.t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewReader(canonical(body)))
	r.Header.Set("X-Test-Owner", c.owner)
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	c.s.ServeHTTP(w, r)
	if c.observe != nil {
		c.observe(r, w, body)
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		c.t.Fatal(err)
	}
	return w.Code, out
}
func fixture(t *testing.T) (*Service, *pgx.Conn) {
	t.Helper()
	dsn := os.Getenv("UNDERGRID_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("real PostgreSQL required")
	}
	db, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(context.Background()) })
	if err = Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	s := New(dsn, "development", func(r *http.Request) (Owner, error) {
		return Owner{"https://fixture.test", r.Header.Get("X-Test-Owner"), true}, nil
	})
	s.cursorKey = bytes.Repeat([]byte{42}, 32)
	s.termsVersion = "test-v1"
	if err = s.ConfigureCredentialIssuance("network-test-key", bytes.Repeat([]byte{41}, 32)); err != nil {
		t.Fatal(err)
	}
	return s, db
}
func client(t *testing.T, s *Service) testClient {
	t.Helper()
	c := testClient{t: t, s: s, owner: NewID()}
	code, out := c.call("POST", "/v1/principals", "", map[string]string{"name": "test owner", "terms_version": "test-v1"})
	if code != 201 {
		t.Fatalf("register %d %v", code, out)
	}
	code, out = c.call("POST", "/v1/agents", "fixture-agent-creation", map[string]any{"name": "listener", "scopes": []string{"intents:read", "intents:write", "inbox:read"}})
	if code != 201 {
		t.Fatalf("agent %d %v", code, out)
	}
	c.token = out["token"].(string)
	return c
}
func intentFixture() IntentInput {
	return IntentInput{Objective: "European source catalog", Output: Output{"json_dataset", "source_catalog_v1"}, HardConstraints: Constraints{[]string{"source_id", "source_url"}, []string{"CC0-1.0"}}, Visibility: "private", Watch: Watch{true, []string{"inbox"}}, ExpiresAt: time.Now().Add(24 * time.Hour)}
}

func TestInboxReceiptAndRecovery(t *testing.T) {
	s, db := fixture(t)
	a, b := client(t, s), client(t, s)
	code, created := a.call("POST", "/v1/intents", "inbox-intent", intentFixture())
	if code != 201 {
		t.Fatal(code, created)
	}
	code, page := a.call("GET", "/v1/inbox", "", nil)
	if code != 200 {
		t.Fatal(code, page)
	}
	events := page["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("events %v", events)
	}
	e := events[0].(map[string]any)
	if e["content_trust"] != "untrusted" {
		t.Fatal("missing trust label")
	}
	cursor := page["next_cursor"].(string)
	code, _ = b.call("GET", "/v1/inbox?cursor="+cursor, "", nil)
	if code != 403 {
		t.Fatalf("cross-agent cursor %d", code)
	}
	for range 2 {
		code, out := a.call("POST", "/v1/inbox/"+e["event_id"].(string)+"/ack", "same-ack", map[string]any{})
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	var state string
	if err := db.QueryRow(context.Background(), "SELECT state FROM undergrid.intents WHERE id=$1", created["id"]).Scan(&state); err != nil || state != "active" {
		t.Fatalf("receipt changed business state %s %v", state, err)
	}
	code, empty := a.call("GET", "/v1/inbox?cursor="+cursor, "", nil)
	if code != 200 || len(empty["events"].([]any)) != 0 {
		t.Fatal(code, empty)
	}
	code, snapshot := a.call("GET", "/v1/agent-context", "", nil)
	if code != 200 || len(snapshot["intent_watches"].([]any)) != 1 {
		t.Fatal(code, snapshot)
	}
	// Expired cursors must request canonical resynchronization, never silently skip.
	_, me := a.call("GET", "/v1/me", "", nil)
	old := s.encodeCursor(me["agent_id"].(string), 1, time.Now().Add(-31*24*time.Hour))
	code, _ = a.call("GET", "/v1/inbox?cursor="+old, "", nil)
	if code != 410 {
		t.Fatalf("expired cursor %d", code)
	}
}
