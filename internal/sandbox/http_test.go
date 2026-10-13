package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSandboxHTTPStrictCommands(t *testing.T) {
	h := NewHandler(New("unused"), nil, nil)
	for _, body := range []string{`{"product":"a","product":"b"}`, `{"product":"a"}` + strings.Repeat(" ", 65536)} {
		r := httptest.NewRequest("POST", "/v1/payments", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 400 && w.Code != 413 {
			t.Fatalf("unsafe command accepted: %d %s", w.Code, w.Body.String())
		}
	}
}
func TestSandboxHTTPEnrollmentToAuthorization(t *testing.T) {
	dsn := os.Getenv("UNDERGRID_SANDBOX_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	c, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(context.Background())
	if err = Migrate(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	runtimeDSN := os.Getenv("UNDERGRID_SANDBOX_TEST_RUNTIME_DATABASE_URL")
	if runtimeDSN == "" {
		runtimeDSN = dsn
	}
	s := New(runtimeDSN)
	o := Owner{Issuer: "https://owner.fixture", Subject: randomID()}
	h := NewHandler(s, func(r *http.Request) (Owner, error) {
		if r.Header.Get("Fixture-Owner") != "verified" {
			return Owner{}, ErrUnauthorized
		}
		return o, nil
	}, map[string]Quote{"platform-test": {Product: "platform-test", Asset: "USD", Amount: "500", Fee: "0", Payee: "stripe-test-account", Rail: "stripe_test", TermsID: "fixture-test", TermsHash: strings.Repeat("a", 64)}})
	call := func(method, path, body, token string, owner bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "fixture-"+method+"-"+path)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if owner {
			r.Header.Set("Fixture-Owner", "verified")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call("POST", "/v1/actors", "{}", "", false); w.Code != 401 {
		t.Fatalf("unauth enrollment %d", w.Code)
	}
	if w := call("POST", "/v1/actors", "{}", "", true); w.Code != 201 {
		t.Fatalf("enrollment %d %s", w.Code, w.Body.String())
	}
	policy := Policy{Asset: "USD", Cap: "500", Products: []string{"platform-test"}, Payees: []string{"stripe-test-account"}, ExpiresAt: time.Now().Add(time.Hour)}
	body, _ := json.Marshal(map[string]any{"version": 1, "policy": policy})
	if w := call("PUT", "/v1/owner/policy", string(body), "", true); w.Code != 200 {
		t.Fatalf("policy %d %s", w.Code, w.Body.String())
	}
	w := call("POST", "/v1/owner/credentials", `{"name":"client","scopes":["payments:quote","payments:authorize","payments:read"]}`, "", true)
	if w.Code != 201 {
		t.Fatalf("credential %d %s", w.Code, w.Body.String())
	}
	var token struct {
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &token)
	w = call("POST", "/v1/payments", `{"product":"platform-test"}`, token.Token, false)
	if w.Code != 201 {
		t.Fatalf("quote %d %s", w.Code, w.Body.String())
	}
	w2 := call("POST", "/v1/payments", `{"product":"platform-test"}`, token.Token, false)
	if w2.Code != 201 || !bytes.Equal(w.Body.Bytes(), w2.Body.Bytes()) {
		t.Fatalf("HTTP quote retry changed quote: %d %s", w2.Code, w2.Body.String())
	}
	var p Payment
	json.Unmarshal(w.Body.Bytes(), &p)
	body, _ = json.Marshal(map[string]string{"quote_hash": p.Hash})
	w = call("POST", "/v1/payments/"+p.ID+"/authorize", string(body), token.Token, false)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"state":"authorized"`)) {
		t.Fatalf("authorize %d %s", w.Code, w.Body.String())
	}
	if w = call("GET", "/v1/payments/"+p.ID, "", "ug_network_credential", false); w.Code != 401 {
		t.Fatalf("network token accepted %d", w.Code)
	}
	// Quotes cannot choose payee, price or legal text through the agent API.
	if w = call("POST", "/v1/payments", `{"product":"platform-test","amount_atomic":"1"}`, token.Token, false); w.Code != 400 {
		t.Fatalf("price injected %d", w.Code)
	}
	other, err := s.Enroll(context.Background(), Owner{Issuer: o.Issuer, Subject: randomID()})
	if err != nil || other == "" {
		t.Fatal(err)
	}
}
