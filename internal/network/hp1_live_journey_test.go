//go:build hp1_live

package network

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"undergrid/internal/identity"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/jackc/pgx/v5"
)

// This is a live browser/domain seam, not selected identity-provider or source
// qualification. Setup creates schema/LOGINs only; all product rows come from
// Chromium commands through the real encrypted session and domain transactions.
func TestHP1LiveOwnerSetupAndHonestGap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	adminDSN := hp1RunDatabase(t, ctx)
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	if err = Migrate(ctx, admin); err != nil {
		t.Fatal(err)
	}
	var principals int
	if err = admin.QueryRow(ctx, "SELECT count(*) FROM undergrid.principals").Scan(&principals); err != nil || principals != 0 {
		t.Fatalf("HP1 requires an empty account population; got %d: %v", principals, err)
	}
	role := "hp1_api_" + strings.ReplaceAll(NewID(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" LOGIN NOSUPERUSER NOBYPASSRLS; GRANT undergrid_app TO "+pgx.Identifier{role}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	apiDSN := networkFixtureRoleDSN(t, admin, role)
	mux := http.NewServeMux()
	app := httptest.NewTLSServer(mux)
	defer app.Close()
	sessions, err := identity.NewSessionManager(app.URL, bytes.Repeat([]byte{11}, 32))
	if err != nil {
		t.Fatal(err)
	}
	issuer := hp1OIDCIssuer(t)
	defer issuer.Close()
	oidcCtx := oidc.ClientContext(ctx, issuer.Client())
	login, err := identity.NewLogin(oidcCtx, sessions, issuer.URL, "hp1-browser", "hp1-fixture-secret")
	if err != nil {
		t.Fatal(err)
	}
	service := New(apiDSN, "development", func(r *http.Request) (Owner, error) {
		o, err := sessions.Authenticate(r)
		return Owner{Issuer: o.Issuer, Subject: o.Subject, Verified: err == nil}, err
	})
	if err = service.Configure(bytes.Repeat([]byte{12}, 32), "hp1-local-fixture-terms"); err != nil {
		t.Fatal(err)
	}
	if err = service.ConfigureCredentialIssuance("hp1-fixture-key", bytes.Repeat([]byte{13}, 32)); err != nil {
		t.Fatal(err)
	}
	mux.HandleFunc("/auth/login", login.Start)
	// Only the controlled issuer's TLS root is trusted by this fixture context.
	// Cookies, nonce, PKCE, token signatures, session and CSRF are production code.
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		login.Callback(w, r.WithContext(oidc.ClientContext(r.Context(), issuer.Client())))
	})
	mux.HandleFunc("/auth/session", sessions.Session)
	mux.HandleFunc("/auth/logout", sessions.Logout)
	mux.Handle("/v1/", service)
	root, err := filepath.Abs("../../")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "web", "dist", "index.html")); err != nil {
		t.Fatal("build actual console before live acceptance", err)
	}
	mux.Handle("/", http.FileServer(http.Dir(filepath.Join(root, "web", "dist"))))
	node := os.Getenv("UNDERGRID_TEST_NODE")
	if node == "" {
		node = "/home/nmcdc/.nvm/versions/node/v24.15.0/bin/node"
	}
	chromium := os.Getenv("UNDERGRID_TEST_CHROMIUM")
	if chromium == "" {
		chromium = "/home/nmcdc/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome"
	}
	artifacts, err := os.MkdirTemp("/tmp", "undergrid-hp1-browser-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Retained browser artifacts: %s", artifacts)
	command := exec.CommandContext(ctx, node, filepath.Join(root, "web", "node_modules", "@playwright", "test", "cli.js"), "test", "--config", "playwright.live.config.ts")
	command.Dir = filepath.Join(root, "web")
	command.Env = append(os.Environ(), "UNDERGRID_HP1_BASE_URL="+app.URL, "UNDERGRID_TEST_CHROMIUM="+chromium, "UNDERGRID_HP1_BROWSER_ARTIFACTS="+artifacts)
	output, err := command.CombinedOutput()
	t.Logf("Live Chromium output:\n%s", output)
	if err != nil {
		t.Fatalf("live owner journey: %v", err)
	}
	var intents, credentials int
	var visibility, state string
	var watching bool
	if err = admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM undergrid.principals),(SELECT count(*) FROM undergrid.credentials),(SELECT count(*) FROM undergrid.intents)`).Scan(&principals, &credentials, &intents); err != nil {
		t.Fatal(err)
	}
	if principals != 1 || credentials != 1 || intents != 1 {
		t.Fatalf("unexpected setup effects: principals=%d credentials=%d intents=%d", principals, credentials, intents)
	}
	if err = admin.QueryRow(ctx, `SELECT r.document->>'visibility',i.state,i.watch_enabled FROM undergrid.intents i JOIN undergrid.intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision`).Scan(&visibility, &state, &watching); err != nil {
		t.Fatal(err)
	}
	if visibility != "private" || state != "active" || !watching {
		t.Fatalf("private active watch not persisted: %s %s %v", visibility, state, watching)
	}
}

// The fixture issues one-use codes bound to S256 verifier, redirect and nonce.
// The browser follows real redirects and the app verifies signed tokens normally.
func hp1OIDCIssuer(t *testing.T) *httptest.Server {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "hp1-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	type grant struct{ nonce, challenge, redirect string }
	var lock sync.Mutex
	codes := map[string]grant{}
	var issuer string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "hp1-fixture", Algorithm: "RS256", Use: "sig"}}})
		case "/authorize":
			q := r.URL.Query()
			if q.Get("client_id") != "hp1-browser" || q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" || q.Get("state") == "" {
				http.Error(w, "invalid authorization", 400)
				return
			}
			redirect, err := url.Parse(q.Get("redirect_uri"))
			if err != nil || redirect.Scheme != "https" || redirect.Path != "/auth/callback" {
				http.Error(w, "invalid redirect", 400)
				return
			}
			code := NewID()
			lock.Lock()
			codes[code] = grant{q.Get("nonce"), q.Get("code_challenge"), redirect.String()}
			lock.Unlock()
			values := redirect.Query()
			values.Set("code", code)
			values.Set("state", q.Get("state"))
			redirect.RawQuery = values.Encode()
			http.Redirect(w, r, redirect.String(), http.StatusFound)
		case "/token":
			if r.Method != "POST" || r.ParseForm() != nil {
				http.Error(w, "invalid token request", 400)
				return
			}
			lock.Lock()
			g, ok := codes[r.Form.Get("code")]
			delete(codes, r.Form.Get("code"))
			lock.Unlock()
			verifier := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			client, secret, basic := r.BasicAuth()
			if !ok || !basic || client != "hp1-browser" || secret != "hp1-fixture-secret" || base64.RawURLEncoding.EncodeToString(verifier[:]) != g.challenge || r.Form.Get("redirect_uri") != g.redirect {
				http.Error(w, "invalid bound code", 400)
				return
			}
			token, err := jwt.Signed(signer).Claims(map[string]any{"iss": issuer, "sub": "hp1-unfamiliar-owner", "aud": "hp1-browser", "exp": time.Now().Add(5 * time.Minute).Unix(), "iat": time.Now().Unix(), "auth_time": time.Now().Unix(), "nonce": g.nonce, "email_verified": true}).Serialize()
			if err != nil {
				http.Error(w, "signing failed", 500)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fixture-oidc-access-not-agent-credential", "token_type": "Bearer", "id_token": token})
		default:
			http.Error(w, fmt.Sprintf("unknown fixture path %s", r.URL.Path), 404)
		}
	}))
	issuer = server.URL
	return server
}

// Fixture infrastructure only: a genuine PostgreSQL template copy preserves the
// normal migration ledger. Runs are retained as evidence, never deleted/reset.
func hp1RunDatabase(t *testing.T, ctx context.Context) string {
	t.Helper()
	adminDSN := os.Getenv("UNDERGRID_HP1_ADMIN_DATABASE_URL")
	if adminDSN == "" {
		explicit := os.Getenv("UNDERGRID_HP1_DATABASE_URL")
		if explicit == "" {
			t.Fatal("set UNDERGRID_HP1_ADMIN_DATABASE_URL for isolated template copies, or a dedicated empty UNDERGRID_HP1_DATABASE_URL")
		}
		return explicit
	}
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	const template = "undergrid_hp1_template"
	var exists bool
	if err = conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", template).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		if _, err = conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{template}.Sanitize()+" TEMPLATE template0"); err != nil {
			t.Fatal(err)
		}
	}
	base, err := pgx.ParseConfig(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	// ConnConfig.ConnString preserves its original input; construct database URLs
	// explicitly instead of accidentally reconnecting to the administrative DB.
	parsed, err := url.Parse(adminDSN)
	if err != nil || parsed.Scheme != "postgres" {
		t.Fatal("HP1 admin DSN must be an explicit postgres URL", err)
	}
	templateURL := *parsed
	templateURL.Path = "/" + template
	fixture, err := pgx.Connect(ctx, templateURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, fixture); err != nil {
		fixture.Close(context.Background())
		t.Fatal(err)
	}
	var count int
	if err = fixture.QueryRow(ctx, "SELECT count(*) FROM undergrid.principals").Scan(&count); err != nil || count != 0 {
		fixture.Close(context.Background())
		t.Fatal("template contains product owners", count, err)
	}
	if err = fixture.Close(ctx); err != nil {
		t.Fatal(err)
	}
	name := "hp1_run_" + strings.ReplaceAll(NewID(), "-", "")
	if _, err = conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE "+pgx.Identifier{template}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	runURL := *parsed
	runURL.Path = "/" + name
	t.Logf("Retained isolated network database: %s on port %d", name, base.Port)
	return runURL.String()
}
