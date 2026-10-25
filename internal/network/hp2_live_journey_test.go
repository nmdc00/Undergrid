//go:build hp1_live && hp2_live

package network

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"undergrid/internal/identity"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
)

// This is an opt-in setup foundation, not the complete HP2 journey. The second
// tag avoids adding unfinished HP2 expectations to HP1's established live gate.
func TestHP2LiveThreeOwnerScopedSetupFoundation(t *testing.T) {
	hp2LiveOwnerSetup(t, false)
}

func TestHP2LiveOwnerPublicationAndWithdrawal(t *testing.T) {
	hp2LiveOwnerSetup(t, true)
}

func hp2LiveOwnerSetup(t *testing.T, publication bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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
	var initialOwners int
	if err = admin.QueryRow(ctx, "SELECT count(*) FROM undergrid.principals").Scan(&initialOwners); err != nil || initialOwners != 0 {
		t.Fatalf("HP2 setup requires empty product population: %d %v", initialOwners, err)
	}
	role := "hp2_api_" + strings.ReplaceAll(NewID(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" LOGIN NOSUPERUSER NOBYPASSRLS; GRANT undergrid_app TO "+pgx.Identifier{role}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	apiDSN := networkFixtureRoleDSN(t, admin, role)
	mux := http.NewServeMux()
	app := httptest.NewTLSServer(mux)
	defer app.Close()
	sessions, err := identity.NewSessionManager(app.URL, bytes.Repeat([]byte{21}, 32))
	if err != nil {
		t.Fatal(err)
	}
	issuer := hpLiveOIDCIssuer(t, func(r *http.Request) string {
		cookie, err := r.Cookie("hp2-fixture-subject")
		if err != nil {
			return ""
		}
		switch cookie.Value {
		case "buyer-a", "buyer-b", "builder-c":
			return "hp2-controlled-" + cookie.Value
		}
		return ""
	})
	defer issuer.Close()
	login, err := identity.NewLogin(oidc.ClientContext(ctx, issuer.Client()), sessions, issuer.URL, "hp1-browser", "hp1-fixture-secret")
	if err != nil {
		t.Fatal(err)
	}
	service := New(apiDSN, "development", func(r *http.Request) (Owner, error) {
		o, err := sessions.Authenticate(r)
		return Owner{Issuer: o.Issuer, Subject: o.Subject, Verified: err == nil}, err
	})
	if err = service.Configure(bytes.Repeat([]byte{22}, 32), "hp2-local-fixture-terms"); err != nil {
		t.Fatal(err)
	}
	if err = service.ConfigureCredentialIssuance("hp2-fixture-key", bytes.Repeat([]byte{23}, 32)); err != nil {
		t.Fatal(err)
	}
	mux.HandleFunc("/auth/login", login.Start)
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
		t.Fatal("build the actual console before HP2 acceptance", err)
	}
	mux.Handle("/", http.FileServer(http.Dir(filepath.Join(root, "web", "dist"))))
	node := os.Getenv("UNDERGRID_TEST_NODE")
	if node == "" {
		node = "/home/nmcdc/.nvm/versions/node/v24.15.0/bin/node"
	}
	verify := exec.CommandContext(ctx, node, filepath.Join(root, "web", "scripts", "record-live-build.mjs"), "--check")
	verify.Dir = filepath.Join(root, "web")
	proof, err := verify.CombinedOutput()
	if err != nil {
		t.Fatalf("frozen frontend provenance: %v: %s", err, proof)
	}
	t.Logf("Verified HP2 frontend source/build manifest: %s", proof)
	chromium := os.Getenv("UNDERGRID_TEST_CHROMIUM")
	if chromium == "" {
		chromium = "/home/nmcdc/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome"
	}
	artifacts, err := os.MkdirTemp("/tmp", "undergrid-hp2-browser-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Retained HP2 browser artifacts: %s", artifacts)
	browser := exec.CommandContext(ctx, node, filepath.Join(root, "web", "node_modules", "@playwright", "test", "cli.js"), "test", "--config", "playwright.hp2.config.ts")
	browser.Dir = filepath.Join(root, "web")
	browser.Env = append(os.Environ(), "UNDERGRID_HP1_BASE_URL="+app.URL, "UNDERGRID_HP2_ISSUER_URL="+issuer.URL, "UNDERGRID_TEST_CHROMIUM="+chromium, "UNDERGRID_HP1_BROWSER_ARTIFACTS="+artifacts)
	if publication {
		browser.Env = append(browser.Env, "UNDERGRID_HP2_OWNER_PUBLICATION=1")
	}
	output, err := browser.CombinedOutput()
	t.Logf("Actual HP2 Chromium output:\n%s", output)
	if err != nil {
		t.Fatalf("HP2 setup foundation: %v", err)
	}
	// Read-only normalized attribution and effect checks. Names encode fixture
	// identities only; separate accounts do not establish independent ownership.
	rows, err := admin.Query(ctx, `SELECT p.subject,count(DISTINCT a.id),count(DISTINCT c.id),count(DISTINCT i.id),bool_and(COALESCE(CASE WHEN $1 AND p.subject='hp2-controlled-buyer-a' THEN i.visibility='redacted_public' AND i.state='withdrawn' AND NOT i.watch_enabled ELSE i.visibility='private' AND i.state='active' AND i.watch_enabled END,true))
 FROM undergrid.principals p LEFT JOIN undergrid.agents a ON a.principal_id=p.id LEFT JOIN undergrid.credentials c ON c.agent_id=a.id LEFT JOIN undergrid.intents i ON i.principal_id=p.id GROUP BY p.subject ORDER BY p.subject`, publication)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for rows.Next() {
		var subject string
		var agents, credentials, intents int
		var expectedState bool
		if err = rows.Scan(&subject, &agents, &credentials, &intents, &expectedState); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		expectedIntents := 1
		if subject == "hp2-controlled-builder-c" {
			expectedIntents = 0
		}
		if agents != 1 || credentials != 1 || intents != expectedIntents || !expectedState {
			rows.Close()
			t.Fatalf("unexpected %s setup effects: agents=%d credentials=%d intents=%d expected_state=%v", subject, agents, credentials, intents, expectedState)
		}
		seen[subject] = true
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	if len(seen) != 3 || !seen["hp2-controlled-buyer-a"] || !seen["hp2-controlled-buyer-b"] || !seen["hp2-controlled-builder-c"] {
		t.Fatalf("wrong controlled identities: %v", seen)
	}
	var collaborationEffects int
	if err = admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM undergrid.swarms)+(SELECT count(*) FROM undergrid.memberships)+(SELECT count(*) FROM undergrid.plan_versions)+(SELECT count(*) FROM undergrid.approvals)+(SELECT count(*) FROM undergrid.tasks)`).Scan(&collaborationEffects); err != nil {
		t.Fatal(err)
	}
	if collaborationEffects != 0 {
		t.Fatalf("setup fabricated %d collaboration effects", collaborationEffects)
	}
	if publication {
		var revisions, revisionEvents, withdrawalEvents, publicCurrent, version, currentRevision int
		var unchangedPrivateFields bool
		var firstProjection, latestProjection []byte
		if err = admin.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM undergrid.intent_revisions),
 (SELECT count(*) FROM undergrid.events WHERE type='intent.revised'),
 (SELECT count(*) FROM undergrid.events WHERE type='intent.withdrawn'),
 (SELECT count(*) FROM undergrid.public_intents()),
 (SELECT i.version FROM undergrid.intents i JOIN undergrid.principals p ON p.id=i.principal_id WHERE p.subject='hp2-controlled-buyer-a'),
 (SELECT i.current_revision FROM undergrid.intents i JOIN undergrid.principals p ON p.id=i.principal_id WHERE p.subject='hp2-controlled-buyer-a'),
 (SELECT first.document-'public_summary'=latest.document-'public_summary' FROM undergrid.intents i JOIN undergrid.principals p ON p.id=i.principal_id JOIN undergrid.intent_revisions first ON first.intent_id=i.id AND first.revision=1 JOIN undergrid.intent_revisions latest ON latest.intent_id=i.id AND latest.revision=i.current_revision WHERE p.subject='hp2-controlled-buyer-a'),
 (SELECT r.public_projection FROM undergrid.intent_revisions r JOIN undergrid.intents i ON i.id=r.intent_id JOIN undergrid.principals p ON p.id=i.principal_id WHERE p.subject='hp2-controlled-buyer-a' AND r.revision=1),
 (SELECT r.public_projection FROM undergrid.intent_revisions r JOIN undergrid.intents i ON i.id=r.intent_id JOIN undergrid.principals p ON p.id=i.principal_id WHERE p.subject='hp2-controlled-buyer-a' AND r.revision=i.current_revision)`).Scan(&revisions, &revisionEvents, &withdrawalEvents, &publicCurrent, &version, &currentRevision, &unchangedPrivateFields, &firstProjection, &latestProjection); err != nil {
			t.Fatal(err)
		}
		if revisions != 3 || revisionEvents != 1 || withdrawalEvents != 1 || publicCurrent != 0 || version != 3 || currentRevision != 2 || !unchangedPrivateFields || !bytes.Contains(firstProjection, []byte("Public environmental catalogue")) || !bytes.Contains(latestProjection, []byte("Updated public environmental catalogue")) {
			t.Fatalf("publication/revocation effects: revisions=%d revised=%d withdrawn=%d public=%d version=%d head=%d conserved=%v first=%s latest=%s", revisions, revisionEvents, withdrawalEvents, publicCurrent, version, currentRevision, unchangedPrivateFields, firstProjection, latestProjection)
		}
	}
}
