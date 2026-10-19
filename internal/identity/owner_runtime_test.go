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
	code, retried := call(a, "POST", "/v1/agents", "https://undergrid.test", "fixture-csrf", body)
	if code != 201 || retried["id"] != out["id"] || retried["credential_id"] != out["credential_id"] || retried["token"] != out["token"] {
		t.Fatalf("same-session owner creation retry: %d %#v", code, retried)
	}
	credential := out["credential_id"].(string)
	code, ownerMe := call(a, "GET", "/v1/owner/me", "", "", nil)
	if code != 200 {
		t.Fatal("owner me", code, ownerMe)
	}
	code, recipientAgent := call(b, "POST", "/v1/agents", "https://undergrid.test", "fixture-csrf", map[string]any{"name": "Recipient reader", "scopes": []string{"intents:read"}})
	if code != 201 {
		t.Fatal("recipient agent", code, recipientAgent)
	}
	code, recipientMe := call(b, "GET", "/v1/owner/me", "", "", nil)
	if code != 200 {
		t.Fatal("recipient owner me", code, recipientMe)
	}
	roomPath := "/v1/owner/agents/" + out["id"].(string) + "/swarms"
	roomBody := map[string]any{"objective": "Private browser room", "visibility": "private", "history_policy": "full", "recruitment_roles": []any{}}
	for _, attack := range [][2]string{{"https://attacker.test", "fixture-csrf"}, {"https://undergrid.test", ""}, {"https://undergrid.test", "wrong"}} {
		if status, result := call(a, "POST", roomPath, attack[0], attack[1], roomBody); status != 401 {
			t.Fatal("room CSRF bypass", status, result)
		}
	}
	if status, result := call(b, "POST", roomPath, "https://undergrid.test", "fixture-csrf", roomBody); status != 404 {
		t.Fatal("foreign browser room authority", status, result)
	}
	roomCode, room := call(a, "POST", roomPath, "https://undergrid.test", "fixture-csrf", roomBody)
	if roomCode != 201 || room["visibility"] != "private" || room["history_policy"] != "full" {
		t.Fatal("browser room creation", roomCode, room)
	}
	if status, result := call(a, "POST", roomPath, "https://undergrid.test", "fixture-csrf", roomBody); status != 201 || result["id"] != room["id"] {
		t.Fatal("browser room replay", status, result)
	}
	var roomCount int
	if err = db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.swarms WHERE creator_principal_id=$1", ownerMe["id"]).Scan(&roomCount); err != nil || roomCount != 1 {
		t.Fatal("browser room CSRF/replay effects", roomCount, err)
	}
	intentExpiry := time.Now().Add(2 * time.Hour).UTC()
	code, restricted := call(a, "POST", "/v1/owner/intents", "https://undergrid.test", "fixture-csrf", map[string]any{
		"agent_id": out["id"],
		"intent": map[string]any{
			"objective":        "Browser-controlled restricted request",
			"output":           map[string]string{"kind": "json_dataset", "schema_family": "source_catalog_v1"},
			"hard_constraints": map[string]any{"required_fields": []string{"source_id"}, "license_ids": []string{"CC0-1.0"}},
			"visibility":       "restricted",
			"budget":           nil,
			"watch":            map[string]any{"enabled": false, "channels": []string{"inbox"}},
			"expires_at":       intentExpiry,
		},
	})
	if code != 201 {
		t.Fatal("restricted intent", code, restricted)
	}
	grantPath := "/v1/owner/intents/" + restricted["id"].(string) + "/grants"
	grantBody := map[string]any{"principal_id": recipientMe["id"], "fields": []string{"objective"}, "expires_at": intentExpiry.Add(-time.Minute)}
	for _, attack := range [][2]string{{"https://attacker.test", "fixture-csrf"}, {"https://undergrid.test", ""}, {"https://undergrid.test", "wrong"}} {
		code, _ := call(a, "POST", grantPath, attack[0], attack[1], grantBody)
		if code != 401 {
			t.Fatalf("intention grant CSRF bypass: %d", code)
		}
	}
	var deniedGrants int
	if err = db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.intent_grants WHERE intent_id=$1", restricted["id"]).Scan(&deniedGrants); err != nil || deniedGrants != 0 {
		t.Fatalf("CSRF-denied grant persisted: %d %v", deniedGrants, err)
	}
	code, grant := call(a, "POST", grantPath, "https://undergrid.test", "fixture-csrf", grantBody)
	if code != 201 {
		t.Fatal("CSRF-protected grant", code, grant)
	}
	request := httptest.NewRequest("GET", "https://undergrid.test/v1/intents/"+restricted["id"].(string), nil)
	request.Header.Set("Authorization", "Bearer "+recipientAgent["token"].(string))
	response := httptest.NewRecorder()
	service.ServeHTTP(response, request)
	var shared map[string]any
	_ = json.Unmarshal(response.Body.Bytes(), &shared)
	if response.Code != 200 || shared["objective"] != "Browser-controlled restricted request" || shared["budget"] != nil {
		t.Fatalf("browser grant recipient read %d %#v", response.Code, shared)
	}
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
	if code != 200 || len(list["items"].([]any)) != 1 || list["items"].([]any)[0].(map[string]any)["id"] != recipientAgent["id"] {
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
