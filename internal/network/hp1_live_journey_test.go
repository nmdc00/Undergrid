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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	hp1LiveOwnerJourney(t, false, false)
}

func TestHP1LiveOwnerSourceNotificationReceiptDismissalAndRestart(t *testing.T) {
	hp1LiveOwnerJourney(t, true, false)
}

func TestHP1LiveOwnerPartialRemainderNotificationReceiptDismissalAndRestart(t *testing.T) {
	hp1LiveOwnerJourney(t, true, true)
}

func hp1LiveOwnerJourney(t *testing.T, withWatch, withPartial bool) {
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
	verifyBuild := exec.CommandContext(ctx, node, filepath.Join(root, "web", "scripts", "record-live-build.mjs"), "--check")
	verifyBuild.Dir = filepath.Join(root, "web")
	buildProof, err := verifyBuild.CombinedOutput()
	if err != nil {
		t.Fatalf("frozen frontend provenance: %v: %s", err, buildProof)
	}
	t.Logf("Verified frontend source/build manifest: %s", buildProof)
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
	var settleWorker func()
	if withWatch {
		controller, settle := hp1SourceWorkerController(t, ctx, admin, withPartial)
		settleWorker = settle
		defer controller.Close()
		command.Env = append(command.Env, "UNDERGRID_HP1_WATCH_CONTROL_URL="+controller.URL)
		if withPartial {
			command.Env = append(command.Env, "UNDERGRID_HP1_PARTIAL_JOURNEY=1")
		}
	}
	output, err := command.CombinedOutput()
	t.Logf("Live Chromium output:\n%s", output)
	if err != nil {
		t.Fatalf("live owner journey: %v", err)
	}
	if settleWorker != nil {
		settleWorker()
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
	if withWatch {
		var opportunities, events, inbox, dismissed, receipts, authorityEffects int
		if err = admin.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM undergrid.opportunities),
 (SELECT count(*) FROM undergrid.events WHERE type='opportunity.available'),
 (SELECT count(*) FROM undergrid.inbox b JOIN undergrid.events e ON e.id=b.event_id WHERE e.type='opportunity.available'),
 (SELECT count(*) FROM undergrid.opportunities WHERE state='dismissed'),
 (SELECT count(*) FROM undergrid.inbox b JOIN undergrid.events e ON e.id=b.event_id WHERE e.type='opportunity.available' AND b.acked_at IS NOT NULL),
 ((SELECT count(*) FROM undergrid.tasks)+(SELECT count(*) FROM undergrid.approvals)+(SELECT count(*) FROM undergrid.swarms))`).Scan(&opportunities, &events, &inbox, &dismissed, &receipts, &authorityEffects); err != nil {
			t.Fatal(err)
		}
		expected := 2
		if withPartial {
			expected = 4
		}
		if opportunities != expected || events != expected || inbox != expected || dismissed != 1 || receipts != 1 || authorityEffects != 0 {
			t.Fatalf("source/receipt/dismissal effects: opportunities=%d events=%d inbox=%d dismissed=%d receipts=%d task/right/room-effects=%d", opportunities, events, inbox, dismissed, receipts, authorityEffects)
		}
	}
	if withPartial {
		// Each legitimate input change has exactly one normalized recommendation,
		// event and inbox row; changing requirements is not a duplicate source scan.
		rows, err := admin.Query(ctx, `SELECT o.external_intent_revision,o.evidence->>'source_version',count(DISTINCT o.id),count(DISTINCT e.id),count(DISTINCT b.event_id)
 FROM undergrid.opportunities o JOIN undergrid.events e ON e.aggregate_id=o.id AND e.type='opportunity.available'
 JOIN undergrid.inbox b ON b.event_id=e.id GROUP BY 1,2 ORDER BY 1,2`)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for rows.Next() {
			var revision, opportunities, events, inbox int
			var sourceVersion string
			if err = rows.Scan(&revision, &sourceVersion, &opportunities, &events, &inbox); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			key := fmt.Sprintf("%d/%s", revision, sourceVersion)
			if opportunities != 1 || events != 1 || inbox != 1 {
				rows.Close()
				t.Fatalf("duplicate effects for %s: %d/%d/%d", key, opportunities, events, inbox)
			}
			seen[key] = true
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		rows.Close()
		if len(seen) != 4 || !seen["1/1"] || !seen["2/1"] || !seen["2/2"] || !seen["2/3"] {
			t.Fatalf("missing revision/source outcomes: %v", seen)
		}
		var revision, partialEvents int
		var document []byte
		if err = admin.QueryRow(ctx, `SELECT i.current_revision,r.document,(SELECT count(*) FROM undergrid.events WHERE type='intent.partially_satisfied') FROM undergrid.intents i JOIN undergrid.intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision`).Scan(&revision, &document, &partialEvents); err != nil {
			t.Fatal(err)
		}
		var current map[string]any
		if err = json.Unmarshal(document, &current); err != nil {
			t.Fatal(err)
		}
		fields := current["hard_constraints"].(map[string]any)["required_fields"].([]any)
		audit := current["partial_satisfaction"].(map[string]any)
		if revision != 2 || partialEvents != 1 || len(fields) != 1 || fields[0] != "source_url" || audit["evidence_class"] != "owner_declared_partial_satisfaction" {
			t.Fatalf("wrong immutable partial remainder: rev=%d events=%d doc=%s", revision, partialEvents, document)
		}
	}
}

// The fixture issues one-use codes bound to S256 verifier, redirect and nonce.
// The browser follows real redirects and the app verifies signed tokens normally.
func hp1OIDCIssuer(t *testing.T) *httptest.Server {
	return hpLiveOIDCIssuer(t, func(*http.Request) string { return "hp1-unfamiliar-owner" })
}

// Identity selection is owned by the controlled issuer, never the application.
// Bind it to the authorization grant so token redemption cannot switch actors.
func hpLiveOIDCIssuer(t *testing.T, subjectForAuthorization func(*http.Request) string) *httptest.Server {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "hp1-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	type grant struct{ nonce, challenge, redirect, subject string }
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
			subject := subjectForAuthorization(r)
			if subject == "" {
				http.Error(w, "controlled identity not selected", 400)
				return
			}
			code := NewID()
			lock.Lock()
			codes[code] = grant{q.Get("nonce"), q.Get("code_challenge"), redirect.String(), subject}
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
			token, err := jwt.Signed(signer).Claims(map[string]any{"iss": issuer, "sub": g.subject, "aud": "hp1-browser", "exp": time.Now().Add(5 * time.Minute).Unix(), "iat": time.Now().Unix(), "auth_time": time.Now().Unix(), "nonce": g.nonce, "email_verified": true}).Serialize()
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

// The controller is a separate test-only listener, never a product route. It
// changes publisher input or worker lifecycle; it cannot write product rows.
func hp1SourceWorkerController(t *testing.T, ctx context.Context, admin *pgx.Conn, withPartial bool) (*httptest.Server, func()) {
	t.Helper()
	role := "hp1_worker_" + strings.ReplaceAll(NewID(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" LOGIN NOSUPERUSER NOBYPASSRLS; GRANT undergrid_worker TO "+pgx.Identifier{role}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	workerDSN := networkFixtureRoleDSN(t, admin, role)
	var version atomic.Int64
	var publisherCalls atomic.Int64
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		publisherCalls.Add(1)
		body, bodyErr := io.ReadAll(io.LimitReader(r.Body, 1))
		if r.Method != "GET" || r.URL.Path != "/capabilities" || r.URL.RawQuery != "" || bodyErr != nil || len(body) != 0 || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("publisher received unexpected/private request")
			http.Error(w, "unapproved request", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if version.Load() == 0 {
			_, _ = w.Write([]byte(`{"items":[],"next_page":0}`))
			return
		}
		providedFields := []string{"source_id"}
		if withPartial && version.Load() >= 2 {
			providedFields = []string{"source_url"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{map[string]any{"id": "environmental-catalogue", "version": fmt.Sprint(version.Load()), "url": "https://publisher.example/environmental-catalogue", "title": "Environmental source catalogue", "status": "active", "capability_claim": map[string]any{"version": "capability-claim-v1", "output": map[string]any{"kind": "json_dataset", "schema_family": "source_catalog_v1"}, "provided_fields": providedFields, "fields_complete": true, "license_ids": []string{"CC0-1.0"}}}}, "next_page": 0})
	}))
	t.Cleanup(publisher.Close)
	cfg := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example/capabilities", Publisher: "HP1 controlled publisher", AccessBasis: "LOCAL FIXTURE permission only", RepublicationBasis: "LOCAL FIXTURE metadata permission only", TombstoneBasis: "LOCAL FIXTURE provenance retention only", PublicPermissionNotice: "Controlled fixture metadata; not a qualified real source", Fields: []string{"canonical_url", "title", "capability_claim"}}
	refresher := externalNotificationRefresher(t, workerDSN, cfg, publisher)
	// Accelerate only this fixture's successful refresh schedule. Due checks and
	// all persisted timestamps still use the actual PostgreSQL clock.
	refresher.successfulRefreshInterval = 100 * time.Millisecond
	waitDue := func() error {
		deadline := time.Now().Add(8 * time.Second)
		for {
			var due bool
			err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM undergrid.external_sources WHERE id=$1 AND next_refresh<=now())`, cfg.ID).Scan(&due)
			if err != nil || due {
				return err
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("controlled source did not become due on actual clock")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	var mutex sync.Mutex
	var stop context.CancelFunc
	var done chan error
	stopWorker := func() error {
		if stop == nil {
			return nil
		}
		stop()
		select {
		case err := <-done:
			stop = nil
			done = nil
			return err
		case <-time.After(12 * time.Second):
			return fmt.Errorf("HP1 worker did not terminate")
		}
	}
	startWorker := func() {
		workerCtx, cancel := context.WithCancel(ctx)
		stop = cancel
		done = make(chan error, 1)
		go func() { done <- runWorkerWithSources(workerCtx, workerDSN, "development", refresher) }()
	}
	t.Cleanup(func() {
		mutex.Lock()
		defer mutex.Unlock()
		if err := stopWorker(); err != nil {
			t.Error(err)
		}
		t.Logf("Controlled publisher actual GET count: %d", publisherCalls.Load())
	})
	var firstOpportunity string
	var receiptBefore time.Time
	assertReceipt := func() error {
		var after time.Time
		if err := admin.QueryRow(ctx, `SELECT b.acked_at FROM undergrid.inbox b JOIN undergrid.events e ON e.id=b.event_id WHERE e.aggregate_id=$1 AND e.type='opportunity.available'`, firstOpportunity).Scan(&after); err != nil {
			return err
		}
		if !after.Equal(receiptBefore) {
			return fmt.Errorf("specific discovery receipt timestamp changed across restart")
		}
		return nil
	}
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		if r.Method != "POST" {
			http.Error(w, "test control requires POST", 405)
			return
		}
		var err error
		switch r.URL.Path {
		case "/start":
			if stop == nil {
				startWorker()
			}
		case "/publish/1", "/publish/2", "/publish/3":
			if strings.HasSuffix(r.URL.Path, "/1") {
				version.Store(1)
			} else if strings.HasSuffix(r.URL.Path, "/2") {
				version.Store(2)
			} else {
				version.Store(3)
			}
			beforeGETs := publisherCalls.Load()
			// Initial configuration comes from the worker's normal startup approval
			// synchronization. A version change waits for real scheduled eligibility;
			// RefreshSource is not a forced fetch and retains every approval guard.
			err = waitDue()
			if err == nil {
				err = refresher.RefreshSource(r.Context(), cfg.ID)
			}
			if err == nil && publisherCalls.Load() <= beforeGETs {
				err = fmt.Errorf("publisher update did not perform an actual HTTPS GET")
			}
		case "/restart-unchanged":
			err = stopWorker()
			if err == nil {
				err = admin.QueryRow(ctx, `SELECT o.id::text,b.acked_at FROM undergrid.opportunities o JOIN undergrid.events e ON e.aggregate_id=o.id AND e.type='opportunity.available' JOIN undergrid.inbox b ON b.event_id=e.id WHERE o.state='dismissed'`).Scan(&firstOpportunity, &receiptBefore)
			}
			if err == nil {
				err = waitDue()
			}
			if err == nil {
				restartedAt := time.Now()
				beforeGETs := publisherCalls.Load()
				startWorker()
				deadline := time.Now().Add(8 * time.Second)
				var snapshot string
				err = admin.QueryRow(ctx, externalSnapshotSQL, cfg.ID).Scan(&snapshot)
				for err == nil {
					var fetched, matched bool
					var pending, failed int
					err = admin.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM undergrid_queue.river_job WHERE kind='permissioned_source_refresh_v1' AND args->>'source_id'=$1 AND state='completed' AND attempted_at>=$2),
 EXISTS(SELECT 1 FROM undergrid_queue.river_job WHERE kind='external_notification_scan_v1' AND args->>'source_id'=$1 AND args->>'snapshot_hash'=$3 AND COALESCE(args->>'intention_id','')<>'' AND (NOT $4::boolean OR args->>'revision'='2') AND state='completed' AND attempted_at>=$2),
 (SELECT count(*) FROM undergrid_queue.river_job WHERE state NOT IN('completed','cancelled','discarded') AND (
 (kind='external_notification_scan_v1' AND args->>'source_id'=$1 AND args->>'snapshot_hash'=$3) OR
 (kind='permissioned_source_refresh_v1' AND args->>'source_id'=$1) OR
 (kind='intention_scan_v1' AND args->>'cause_id' IN(SELECT id::text FROM undergrid.intents)))),
 (SELECT count(*) FROM undergrid_queue.river_job WHERE state IN('cancelled','discarded') AND ((kind='external_notification_scan_v1' AND args->>'source_id'=$1 AND args->>'snapshot_hash'=$3) OR (kind='permissioned_source_refresh_v1' AND args->>'source_id'=$1)))`, cfg.ID, restartedAt, snapshot, withPartial).Scan(&fetched, &matched, &pending, &failed)
					if err != nil {
						break
					}
					if failed != 0 {
						err = fmt.Errorf("unchanged replay has %d failed/cancelled descendants", failed)
						break
					}
					if fetched && matched && pending == 0 && publisherCalls.Load() > beforeGETs {
						break
					}
					if time.Now().After(deadline) {
						err = fmt.Errorf("unchanged restart pipeline not settled: fetched=%v matched=%v pending=%d", fetched, matched, pending)
						break
					}
					time.Sleep(20 * time.Millisecond)
				}
			}
			if err == nil {
				var opportunities, events, inbox int
				err = admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM undergrid.opportunities),(SELECT count(*) FROM undergrid.events WHERE type='opportunity.available'),(SELECT count(*) FROM undergrid.inbox b JOIN undergrid.events e ON e.id=b.event_id WHERE e.type='opportunity.available')`).Scan(&opportunities, &events, &inbox)
				expected := 1
				if withPartial {
					expected = 3
				}
				if err == nil && (opportunities != expected || events != expected || inbox != expected) {
					err = fmt.Errorf("unchanged restart duplicated effects: %d/%d/%d", opportunities, events, inbox)
				}
			}
			if err == nil {
				err = assertReceipt()
			}
			if err == nil {
				t.Logf("Unchanged source refresh completed after actual worker restart; normalized recommendation/event/inbox counts and receipt timestamp preserved for %s", firstOpportunity)
			}
		default:
			http.Error(w, "unknown test control", 404)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	return controller, func() {
		mutex.Lock()
		defer mutex.Unlock()
		if err := stopWorker(); err != nil {
			t.Fatal(err)
		}
		if firstOpportunity == "" {
			t.Fatal("restart proof never selected the acknowledged discovery")
		}
		if err := assertReceipt(); err != nil {
			t.Fatal(err)
		}
	}
}
