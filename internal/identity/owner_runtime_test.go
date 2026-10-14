package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
	"undergrid/internal/network"
)

// Encrypted browser cookies and CSRF reach the real owner transaction and RLS.
// Identity itself is a signed session fixture, not external provider verification.
func TestBrowserOwnerDomainCSRFAndIsolation(t *testing.T) {
	dsn := os.Getenv("UNDERGRID_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("real PostgreSQL required")
	}
	db, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(context.Background())
	if err = network.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	sessions, err := NewSessionManager("https://undergrid.test", bytes.Repeat([]byte{5}, 32))
	if err != nil {
		t.Fatal(err)
	}
	role := "owner_browser_" + strings.ReplaceAll(network.NewID(), "-", "")
	if _, err = db.Exec(context.Background(), "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" LOGIN NOSUPERUSER NOBYPASSRLS; GRANT undergrid_app TO "+pgx.Identifier{role}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, err := pgx.Connect(context.Background(), dsn)
		if err == nil {
			defer cleanup.Close(context.Background())
			_, err = cleanup.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{role}.Sanitize())
		}
		if err != nil {
			t.Error(err)
		}
	})
	configuration, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	runtimeURL := url.URL{Scheme: "postgres", User: url.User(role), Path: "/" + configuration.Database}
	query := url.Values{"host": {configuration.Host}, "port": {strconv.Itoa(int(configuration.Port))}, "sslmode": {"disable"}}
	runtimeURL.RawQuery = query.Encode()
	service := network.New(runtimeURL.String(), "development", func(r *http.Request) (network.Owner, error) {
		o, err := sessions.Authenticate(r)
		return network.Owner{Issuer: o.Issuer, Subject: o.Subject, Verified: err == nil}, err
	})
	if err = service.Configure(bytes.Repeat([]byte{6}, 32), "fixture-approved-terms"); err != nil {
		t.Fatal(err)
	}
	if err = service.ConfigureCredentialIssuance("browser-fixture-key", bytes.Repeat([]byte{10}, 32)); err != nil {
		t.Fatal(err)
	}
	cookie := func(subject string) *http.Cookie {
		w := httptest.NewRecorder()
		if err = sessions.writeCookie(w, ownerCookie, session{Issuer: "https://fixture.identity", Subject: subject, CSRF: "fixture-csrf", Expires: time.Now().Add(time.Minute).Unix()}, 60); err != nil {
			t.Fatal(err)
		}
		return w.Result().Cookies()[0]
	}
	a, b := cookie(network.NewID()), cookie(network.NewID())
	call := func(c *http.Cookie, method, path, origin, csrf string, body any) (int, map[string]any) {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "https://undergrid.test"+path, bytes.NewReader(raw))
		r.AddCookie(c)
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("Idempotency-Key", method+path)
		r.Header.Set("If-Match", `"1"`)
		w := httptest.NewRecorder()
		service.ServeHTTP(w, r)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	for _, c := range []*http.Cookie{a, b} {
		code, out := call(c, "POST", "/v1/principals", "https://undergrid.test", "fixture-csrf", map[string]string{"name": "Fixture owner", "terms_version": "fixture-approved-terms"})
		if code != 201 {
			t.Fatal(code, out)
		}
	}
	body := map[string]any{"name": "Private browser listener", "scopes": []string{"intents:read"}}
	for _, attack := range [][2]string{{"https://attacker.test", "fixture-csrf"}, {"https://undergrid.test", "wrong"}, {"https://undergrid.test", ""}} {
		code, _ := call(a, "POST", "/v1/agents", attack[0], attack[1], body)
		if code != 401 {
			t.Fatalf("csrf bypass: %d", code)
		}
	}
	code, out := call(a, "POST", "/v1/agents", "https://undergrid.test", "fixture-csrf", body)
	if code != 201 {
		t.Fatal(code, out)
	}
	credential := out["credential_id"].(string)
	rotationPath := "/v1/owner/credentials/" + credential + "/rotate"
	rotation := map[string]any{"scopes": []string{"intents:read"}}
	for _, attack := range [][2]string{{"https://attacker.test", "fixture-csrf"}, {"https://undergrid.test", ""}} {
		code, _ := call(a, "POST", rotationPath, attack[0], attack[1], rotation)
		if code != 401 {
			t.Fatalf("rotation CSRF bypass %d", code)
		}
	}
	code, _ = call(b, "POST", rotationPath, "https://undergrid.test", "fixture-csrf", rotation)
	if code != 404 {
		t.Fatalf("foreign rotation %d", code)
	}
	code, rotated := call(a, "POST", rotationPath, "https://undergrid.test", "fixture-csrf", rotation)
	if code != 201 {
		t.Fatal(code, rotated)
	}
	code, replayed := call(a, "POST", rotationPath, "https://undergrid.test", "fixture-csrf", rotation)
	if code != 201 || replayed["token"] != rotated["token"] {
		t.Fatal("browser rotation replay", code, replayed)
	}
	credential = rotated["credential_id"].(string)

	code, list := call(b, "GET", "/v1/agents", "", "", nil)
	if code != 200 || len(list["items"].([]any)) != 0 {
		t.Fatal("owner isolation", code, list)
	}
	code, _ = call(b, "POST", "/v1/owner/credentials/"+credential+"/revoke", "https://undergrid.test", "fixture-csrf", map[string]any{})
	if code != 404 {
		t.Fatalf("foreign owner revoke %d", code)
	}
	code, _ = call(a, "POST", "/v1/owner/credentials/"+credential+"/revoke", "https://undergrid.test", "fixture-csrf", map[string]any{})
	if code != 200 {
		t.Fatal(code)
	}
	var revoked bool
	if err = db.QueryRow(context.Background(), "SELECT revoked_at IS NOT NULL FROM undergrid.credentials WHERE id=$1", credential).Scan(&revoked); err != nil || !revoked {
		t.Fatal("revoke not durable", err)
	}
}
