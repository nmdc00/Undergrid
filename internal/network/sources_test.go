package network

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSourceWorkerRejectsUnreviewedConfiguration(t *testing.T) {
	s, _ := fixture(t)
	path := filepath.Join(t.TempDir(), "sources.json")
	if err := os.WriteFile(path, []byte(`[{"id":"invalid","kind":"capability_manifest","endpoint":"http://169.254.169.254/latest/meta-data"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERGRID_SOURCES_FILE", path)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := RunWorker(ctx, s.dsn, "development"); err == nil {
		t.Fatal("worker ignored unsafe source configuration instead of failing closed")
	}
}

// The controlled publisher advertises an otherwise public HTTPS name. Only the
// test transport maps that name to its local TLS listener; production resolves
// and validates the connected IP independently on every dial.
func TestSourceRefreshPermittedLifecycle(t *testing.T) {
	_, db := fixture(t)
	var requests atomic.Int64
	var fetchWithOpenTransaction atomic.Bool
	var phase atomic.Int64
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var transactions int
		err := db.QueryRow(r.Context(), `SELECT count(DISTINCT a.pid) FROM pg_stat_activity a JOIN pg_locks l ON l.pid=a.pid JOIN pg_class c ON c.oid=l.relation JOIN pg_namespace n ON n.oid=c.relnamespace WHERE a.datname=current_database() AND a.state='idle in transaction' AND l.granted AND n.nspname='undergrid' AND c.relname='external_sources'`).Scan(&transactions)
		if err != nil {
			t.Error("inspect source transaction during publisher request", err)
		}
		if transactions > 0 {
			fetchWithOpenTransaction.Store(true)
		}
		if r.URL.Path != "/capabilities" {
			t.Errorf("fetched unapproved path %s", r.URL)
			http.Error(w, "no", 404)
			return
		}
		if phase.Load() == 1 {
			if r.Header.Get("If-None-Match") != "\"v1\"" {
				t.Error("missing conditional validator")
			}
			w.WriteHeader(304)
			return
		}
		if phase.Load() == 2 {
			w.WriteHeader(http.StatusGone)
			return
		}
		w.Header().Set("ETag", "\"v1\"")
		io.WriteString(w, `{"items":[{"id":"cap-1","version":"1","url":"https://publisher.example/tool","title":"Useful dataset","summary":"not permitted","tags":["json"],"status":"active","updated_at":"2026-10-14T12:00:00Z","command":"curl http://169.254.169.254/","body":"private body","comments_url":"https://evil.example/comments"}],"next_page":0}`)
	}))
	defer publisher.Close()
	cfg := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example/capabilities", Publisher: "Fixture publisher", AccessBasis: "fixture explicit permission", RepublicationBasis: "fixture metadata permission", TombstoneBasis: "fixture minimal provenance retention", Fields: []string{"canonical_url", "title", "tags"}}
	r, err := NewSourceRefresher(db.Config().ConnString(), []SourceConfig{cfg})
	if err != nil {
		t.Fatal(err)
	}
	transport := publisher.Client().Transport.(*http.Transport).Clone()
	address := strings.TrimPrefix(publisher.URL, "https://")
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	transport.TLSClientConfig.InsecureSkipVerify = true // Controlled TLS endpoint; never exposed as operator configuration.
	r.client = &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if err = r.RefreshDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fetchWithOpenTransaction.Load() {
		t.Fatal("source fetch held a database transaction during external HTTP")
	}
	var successDelay float64
	if err = db.QueryRow(context.Background(), `SELECT extract(epoch FROM next_refresh-last_success)::double precision FROM undergrid.external_sources WHERE id=$1`, cfg.ID).Scan(&successDelay); err != nil {
		t.Fatal(err)
	}
	if successDelay != 21600 {
		t.Fatalf("normal source success schedule changed: got %v seconds, want six hours", successDelay)
	}
	var metadata string
	var status string
	if err = db.QueryRow(context.Background(), "SELECT metadata::text,status FROM undergrid.external_observations WHERE source_id=$1", cfg.ID).Scan(&metadata, &status); err != nil {
		t.Fatal(err)
	}
	if status != "active" || strings.Contains(metadata, "not permitted") || strings.Contains(metadata, "private body") || strings.Contains(metadata, "169.254") || !strings.Contains(metadata, "Useful dataset") {
		t.Fatalf("metadata permission failure %s %s", status, metadata)
	}
	if requests.Load() != 1 {
		t.Fatal("embedded URL fetched")
	}
	phase.Store(1)
	forceSourceDue(t, db, cfg.ID)
	if err = r.RefreshDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.external_observations WHERE source_id=$1", cfg.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("conditional duplicated observations %d %v", count, err)
	}
	phase.Store(2)
	forceSourceDue(t, db, cfg.ID)
	if err = r.RefreshDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(context.Background(), "SELECT metadata::text,status FROM undergrid.external_observations WHERE source_id=$1", cfg.ID).Scan(&metadata, &status); err != nil {
		t.Fatal(err)
	}
	if status != "deleted" || metadata != "{}" {
		t.Fatalf("deleted content retained %s %s", status, metadata)
	}
}
func forceSourceDue(t *testing.T, db *pgx.Conn, id string) {
	t.Helper()
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.external_sources SET next_refresh=now()-interval '1 second' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
}

func TestSourceGitHubRequestFilteringAndFieldAllowlist(t *testing.T) {
	cfg := SourceConfig{Endpoint: "https://api.github.com/repos/example/project/issues", Kind: "github_requests", Labels: []string{"request"}, Fields: []string{"canonical_url", "title", "updated_at", "tags"}}
	page := []byte(`[{"id":101,"html_url":"https://github.com/example/project/issues/1","title":"Wanted JSON export","updated_at":"2026-10-15T10:00:00Z","state":"open","labels":[{"name":"request"}],"body":"do not copy","user":{"email":"private@example.com"}},{"id":102,"html_url":"https://github.com/example/project/pull/2","title":"A pull request","updated_at":"2026-10-15T10:00:00Z","state":"open","labels":[{"name":"request"}],"pull_request":{}},{"id":103,"html_url":"https://github.com/example/project/issues/3","title":"Wrong label","updated_at":"2026-10-15T10:00:00Z","state":"open","labels":[{"name":"bug"}]}]`)
	items, more, err := parseSourcePage(cfg, page, 1)
	if err != nil || more || len(items) != 1 || items[0].key != "101" {
		t.Fatalf("filtered GitHub result %v %v %v", items, more, err)
	}
	tags, ok := items[0].metadata["tags"].([]string)
	if !ok || len(tags) != 1 || tags[0] != "request" {
		t.Fatal("approved structural issue labels missing")
	}
	raw, _ := json.Marshal(items[0].metadata)
	if strings.Contains(string(raw), "copy") || strings.Contains(string(raw), "email") {
		t.Fatal("unapproved GitHub fields copied")
	}
}
func TestSourceConfigurationAndIPGuards(t *testing.T) {
	base := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example/feed", Publisher: "publisher", AccessBasis: "access evidence", RepublicationBasis: "republication evidence", TombstoneBasis: "retention evidence", Fields: []string{"title"}}
	for _, endpoint := range []string{"http://publisher.example/feed", "https://127.0.0.1/feed", "https://169.254.169.254/feed", "https://[::1]/feed", "https://10.1.1.1/feed", "https://publisher.example:8443/feed", "https://user:secret@publisher.example/feed", "https://publisher.example/feed?q=x"} {
		c := base
		c.Endpoint = endpoint
		if _, err := NewSourceRefresher("", []SourceConfig{c}); err == nil {
			t.Fatalf("unsafe source accepted %s", endpoint)
		}
	}
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "::ffff:127.0.0.1", "100.64.0.1"} {
		if publicSourceIP(net.ParseIP(address)) {
			t.Fatalf("unsafe DNS address %s", address)
		}
	}
	c := base
	c.Fields = []string{"body"}
	if _, err := NewSourceRefresher("", []SourceConfig{c}); err == nil {
		t.Fatal("body copying allowed")
	}
	c = base
	c.Kind = "github_requests"
	c.Endpoint = "https://api.github.com/repos/example/project/issues"
	if _, err := NewSourceRefresher("", []SourceConfig{c}); err == nil {
		t.Fatal("GitHub source without opted-in labels")
	}
}
func TestSourceMalformedPageDoesNotPartiallyReplaceSnapshot(t *testing.T) {
	_, db := fixture(t)
	var phase atomic.Int64
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if phase.Load() == 1 {
			io.WriteString(w, `{"items":[{"id":"new","version":"1","url":"https://publisher.example/new","title":"new","status":"active"}],"next_page":2}`)
			return
		}
		io.WriteString(w, `{"items":[{"id":"old","version":"1","url":"https://publisher.example/old","title":"old","status":"active"}],"next_page":0}`)
	}))
	defer publisher.Close()
	cfg := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example/feed", Publisher: "fixture", AccessBasis: "fixture access", RepublicationBasis: "fixture reuse", TombstoneBasis: "fixture tombstone", Fields: []string{"title"}}
	refresher, err := NewSourceRefresher(db.Config().ConnString(), []SourceConfig{cfg})
	if err != nil {
		t.Fatal(err)
	}
	transport := publisher.Client().Transport.(*http.Transport).Clone()
	address := strings.TrimPrefix(publisher.URL, "https://")
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	transport.TLSClientConfig.InsecureSkipVerify = true
	refresher.client = &http.Client{Transport: transport}
	if err = refresher.RefreshDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	phase.Store(1)
	forceSourceDue(t, db, cfg.ID)
	if err = refresher.RefreshDue(context.Background()); err == nil {
		t.Fatal("malformed second page accepted")
	}
	var title string
	var count int
	if err = db.QueryRow(context.Background(), `SELECT count(*),max(metadata->>'title') FROM undergrid.external_observations WHERE source_id=$1 AND status='active'`, cfg.ID).Scan(&count, &title); err != nil || count != 1 || title != "old" {
		t.Fatalf("partial failed snapshot replaced good content %d %s %v", count, title, err)
	}
	if _, err = db.Exec(context.Background(), `UPDATE undergrid.external_observations SET checked_at=now()-interval '25 hours' WHERE source_id=$1`, cfg.ID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.eligible_external_observations WHERE source_id=$1`, cfg.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale source eligible %d %v", count, err)
	}
	if _, err = db.Exec(context.Background(), "SET ROLE undergrid_app"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(context.Background(), "SELECT * FROM undergrid.external_sources"); err == nil {
		t.Fatal("application read privileged source configuration")
	}
}

func TestSourceInvalidSnapshotEnvelopeRejected(t *testing.T) {
	cfg := SourceConfig{Kind: "capability_manifest", Fields: []string{"title"}}
	for _, body := range []string{`{}`, `null`, `{"items":null}`, `{"items":[],"next_page":7}`} {
		if _, _, err := parseSourcePage(cfg, []byte(body), 1); err == nil {
			t.Fatalf("invalid snapshot accepted: %s", body)
		}
	}
}

func TestSourceGitHubRefreshTakedownAndApprovalRemoval(t *testing.T) {
	_, db := fixture(t)
	var requests atomic.Int64
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Query().Get("labels") != "request" || r.URL.Query().Get("state") != "all" || r.URL.Query().Get("page") != "1" {
			t.Errorf("unbounded or undesignated GitHub request %s", r.URL)
		}
		io.WriteString(w, `[{"id":101,"html_url":"https://github.com/example/project/issues/1","title":"Wanted JSON export","updated_at":"2026-10-15T10:00:00Z","state":"open","labels":[{"name":"request"}],"body":"unapproved body"},{"id":102,"html_url":"https://github.com/example/project/pull/2","title":"pull request","updated_at":"2026-10-15T10:00:00Z","state":"open","labels":[{"name":"request"}],"pull_request":{}}]`)
	}))
	defer publisher.Close()
	cfg := SourceConfig{ID: NewID(), Kind: "github_requests", Endpoint: "https://api.github.com/repos/example/project/issues", Publisher: "fixture", AccessBasis: "fixture access", RepublicationBasis: "fixture metadata", TombstoneBasis: "fixture provenance", Fields: []string{"title", "canonical_url"}, Labels: []string{"request"}}
	refresher, err := NewSourceRefresher(db.Config().ConnString(), []SourceConfig{cfg})
	if err != nil {
		t.Fatal(err)
	}
	transport := publisher.Client().Transport.(*http.Transport).Clone()
	address := strings.TrimPrefix(publisher.URL, "https://")
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	transport.TLSClientConfig.InsecureSkipVerify = true
	refresher.client = &http.Client{Transport: transport}
	if err = refresher.RefreshDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.eligible_external_observations WHERE source_id=$1 AND kind='external_request'`, cfg.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("GitHub worker result %d %v", count, err)
	}
	cfg.Takedowns = []string{"101"}
	refresher.configs = []SourceConfig{cfg}
	if err = refresher.RefreshDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	var state, metadata string
	if err = db.QueryRow(context.Background(), `SELECT status,metadata::text FROM undergrid.external_observations WHERE source_id=$1`, cfg.ID).Scan(&state, &metadata); err != nil || state != "taken_down" || metadata != "{}" {
		t.Fatalf("takedown retained data %s %s %v", state, metadata, err)
	}
	refresher.configs = nil
	if err = refresher.RefreshDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.eligible_external_observations WHERE source_id=$1`, cfg.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("removed approval still eligible %d %v", count, err)
	}
	if requests.Load() != 2 {
		t.Fatalf("unexpected recursive or removed-source fetches %d", requests.Load())
	}
}

func TestSourceVersionsDoNotInflateLeadsAndTakedownErasesHistory(t *testing.T) {
	_, db := fixture(t)
	cfg := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example/feed", Publisher: "fixture", AccessBasis: "fixture access", RepublicationBasis: "fixture metadata", TombstoneBasis: "fixture retention", Fields: []string{"title"}}
	var version atomic.Int64
	version.Store(1)
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"items":[{"id":"stable","version":"%d","url":"https://publisher.example/tool","title":"Permitted","status":"active"}],"next_page":0}`, version.Load())
	}))
	defer publisher.Close()
	refresher, err := NewSourceRefresher(db.Config().ConnString(), []SourceConfig{cfg})
	if err != nil {
		t.Fatal(err)
	}
	transport := publisher.Client().Transport.(*http.Transport).Clone()
	address := strings.TrimPrefix(publisher.URL, "https://")
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	transport.TLSClientConfig.InsecureSkipVerify = true
	refresher.client = &http.Client{Transport: transport}
	if err = refresher.RefreshDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	version.Store(2)
	forceSourceDue(t, db, cfg.ID)
	if err = refresher.RefreshDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	var total, active int
	if err = db.QueryRow(context.Background(), `SELECT count(*),count(*) FILTER(WHERE status='active') FROM undergrid.external_observations WHERE source_id=$1`, cfg.ID).Scan(&total, &active); err != nil || total != 2 || active != 1 {
		t.Fatalf("version provenance or active count %d/%d %v", active, total, err)
	}
	cfg.Takedowns = []string{"stable"}
	refresher.configs = []SourceConfig{cfg}
	if err = refresher.RefreshDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.external_observations WHERE source_id=$1 AND (status='active' OR metadata<>'{}')`, cfg.ID).Scan(&active); err != nil || active != 0 {
		t.Fatalf("takedown retained historical content %d %v", active, err)
	}
}

func TestSourceRateLimitAndRedirectFailClosed(t *testing.T) {
	_, db := fixture(t)
	var phase atomic.Int64
	var redirected atomic.Int64
	var requests atomic.Int64
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer target.Close()
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if phase.Load() == 0 {
			w.Header().Set("Retry-After", "172800")
			w.WriteHeader(429)
			return
		}
		http.Redirect(w, r, target.URL, 302)
	}))
	defer publisher.Close()
	cfg := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example/feed", Publisher: "fixture", AccessBasis: "fixture access", RepublicationBasis: "fixture metadata", TombstoneBasis: "fixture provenance", Fields: []string{"title"}}
	refresher, err := NewSourceRefresher(db.Config().ConnString(), []SourceConfig{cfg})
	if err != nil {
		t.Fatal(err)
	}
	transport := publisher.Client().Transport.(*http.Transport).Clone()
	address := strings.TrimPrefix(publisher.URL, "https://")
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	transport.TLSClientConfig.InsecureSkipVerify = true
	refresher.client.Transport = transport
	if err = refresher.RefreshDue(context.Background()); err == nil {
		t.Fatal("429 treated as successful check")
	}
	var deferred bool
	var success bool
	if err = db.QueryRow(context.Background(), `SELECT next_refresh>now()+interval '47 hours 59 minutes',last_success IS NOT NULL FROM undergrid.external_sources WHERE id=$1`, cfg.ID).Scan(&deferred, &success); err != nil || !deferred || success {
		t.Fatalf("rate-limit schedule %v success %v %v", deferred, success, err)
	}
	phase.Store(1)
	forceSourceDue(t, db, cfg.ID)
	if err = refresher.RefreshDue(context.Background()); err == nil {
		t.Fatal("redirect treated as successful check")
	}
	if requests.Load() != 2 {
		t.Fatalf("followed source redirect through transport: %d requests", requests.Load())
	}
	if redirected.Load() != 0 {
		t.Fatal("followed source redirect")
	}
}

func TestSourceBoundedPaginationUsesConfiguredEndpoint(t *testing.T) {
	_, db := fixture(t)
	var requests atomic.Int64
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/feed" {
			t.Errorf("unapproved pagination path %s", r.URL.Path)
		}
		if r.Header.Get("If-None-Match") != "" {
			t.Error("page-one validator incorrectly used for multipage snapshot")
		}
		w.Header().Set("ETag", "\"page-validator\"")
		if r.URL.Query().Get("page") == "2" {
			io.WriteString(w, `{"items":[{"id":"second","version":"1","url":"https://publisher.example/second","title":"second","status":"active"}],"next_page":0}`)
			return
		}
		io.WriteString(w, `{"items":[{"id":"first","version":"1","url":"https://publisher.example/first","title":"first","status":"active"}],"next_page":2,"next_url":"http://169.254.169.254/latest/meta-data"}`)
	}))
	defer publisher.Close()
	cfg := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example/feed", Publisher: "fixture", AccessBasis: "fixture access", RepublicationBasis: "fixture metadata", TombstoneBasis: "fixture retention", Fields: []string{"title"}}
	refresher, err := NewSourceRefresher(db.Config().ConnString(), []SourceConfig{cfg})
	if err != nil {
		t.Fatal(err)
	}
	transport := publisher.Client().Transport.(*http.Transport).Clone()
	address := strings.TrimPrefix(publisher.URL, "https://")
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	transport.TLSClientConfig.InsecureSkipVerify = true
	refresher.client.Transport = transport
	for pass := 0; pass < 2; pass++ {
		if pass > 0 {
			forceSourceDue(t, db, cfg.ID)
		}
		if err = refresher.RefreshDue(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	var total int
	if err = db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.eligible_external_observations WHERE source_id=$1`, cfg.ID).Scan(&total); err != nil || total != 2 || requests.Load() != 4 {
		t.Fatalf("pagination result %d requests %d %v", total, requests.Load(), err)
	}
}

func TestSourceRefreshUsesRestrictedWorkerLogin(t *testing.T) {
	_, db := fixture(t)
	ctx := context.Background()
	role := "source_runner_" + strings.ReplaceAll(NewID(), "-", "")
	identifier := pgx.Identifier{role}.Sanitize()
	if _, err := db.Exec(ctx, "CREATE ROLE "+identifier+" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, "DROP ROLE "+identifier)
	if _, err := db.Exec(ctx, "GRANT undergrid_worker TO "+identifier); err != nil {
		t.Fatal(err)
	}
	runtimeConfig := db.Config().Copy()
	runtimeConfig.User = role
	runtimeDB, err := pgx.ConnectConfig(ctx, runtimeConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer runtimeDB.Close(ctx)
	var super, bypass bool
	if err = runtimeDB.QueryRow(ctx, "SELECT rolsuper,rolbypassrls FROM pg_roles WHERE rolname=current_user").Scan(&super, &bypass); err != nil || super || bypass {
		t.Fatalf("not a restricted runtime login %v %v %v", super, bypass, err)
	}
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"items":[{"id":"restricted","version":"1","url":"https://publisher.example/tool","title":"Worker metadata","status":"active"}],"next_page":0}`)
	}))
	defer publisher.Close()
	cfg := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example/feed", Publisher: "fixture", AccessBasis: "fixture access", RepublicationBasis: "fixture metadata", TombstoneBasis: "fixture retention", Fields: []string{"title"}}
	refresher, err := NewSourceRefresher(networkFixtureRoleDSN(t, db, role), []SourceConfig{cfg})
	if err != nil {
		t.Fatal(err)
	}
	actualConnection, err := pgx.Connect(ctx, refresher.dsn)
	if err != nil {
		t.Fatal(err)
	}
	var actualLogin string
	err = actualConnection.QueryRow(ctx, "SELECT session_user").Scan(&actualLogin)
	actualConnection.Close(ctx)
	if err != nil || actualLogin != role {
		t.Fatalf("refresher does not use restricted login: got=%s expected=%s err=%v", actualLogin, role, err)
	}
	transport := publisher.Client().Transport.(*http.Transport).Clone()
	address := strings.TrimPrefix(publisher.URL, "https://")
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	transport.TLSClientConfig.InsecureSkipVerify = true
	refresher.client.Transport = transport
	if err = refresher.RefreshDue(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = runtimeDB.QueryRow(ctx, `SELECT count(*) FROM undergrid.eligible_external_observations WHERE source_id=$1`, cfg.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("worker login source ingestion %d %v", count, err)
	}
}

func TestSourceWorkerLoadsConfiguredApprovalFile(t *testing.T) {
	service, db := fixture(t)
	var oldPause *time.Time
	err := db.QueryRow(context.Background(), "SELECT paused_at FROM undergrid_queue.river_queue WHERE name='default'").Scan(&oldPause)
	if err != nil && err != pgx.ErrNoRows {
		t.Fatal(err)
	}
	if _, err = db.Exec(context.Background(), "INSERT INTO undergrid_queue.river_queue(name,paused_at) VALUES('default',now()) ON CONFLICT(name) DO UPDATE SET paused_at=excluded.paused_at"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec(context.Background(), "UPDATE undergrid_queue.river_queue SET paused_at=$1 WHERE name='default'", oldPause); err != nil {
			t.Error(err)
		}
	}()
	cfg := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://undergrid-source.invalid/feed", Publisher: "fixture", AccessBasis: "fixture access", RepublicationBasis: "fixture metadata", TombstoneBasis: "fixture retention", Fields: []string{"title"}}
	raw, _ := json.Marshal([]SourceConfig{cfg})
	path := filepath.Join(t.TempDir(), "sources.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERGRID_SOURCES_FILE", path)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunWorker(ctx, service.dsn, "development") }()
	deadline := time.Now().Add(10 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		var exists bool
		if err := db.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM undergrid.external_sources WHERE id=$1 AND config->>'Endpoint'=$2 AND last_error='refresh_failed')`, cfg.ID, cfg.Endpoint).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			found = true
			break
		}
		select {
		case err := <-done:
			t.Fatalf("worker stopped before source configuration loaded: %v", err)
		case <-time.After(25 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("source worker shutdown: %v", err)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("source worker failed bounded shutdown")
	}
	if !found {
		t.Fatal("configured source refresh starved behind paused default queue")
	}
	// .invalid is deliberately unresolvable: this proves startup/job wiring,
	// not a successful publisher/provider qualification.
}

func TestSourceFetchDoesNotBlockRevocationOrDuplicateConcurrentRequests(t *testing.T) {
	_, db := fixture(t)
	cfg := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example/feed", Publisher: "fixture", AccessBasis: "fixture access", RepublicationBasis: "fixture metadata", TombstoneBasis: "fixture provenance", Fields: []string{"title"}}
	entered, release := make(chan struct{}), make(chan struct{})
	var requests atomic.Int64
	var released atomic.Bool
	unblock := func() {
		if released.CompareAndSwap(false, true) {
			close(release)
		}
	}
	defer unblock()
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		io.WriteString(w, `{"items":[{"id":"pending","version":"1","url":"https://publisher.example/tool","title":"Obsolete response","status":"active"}],"next_page":0}`)
	}))
	defer publisher.Close()
	refresher, err := NewSourceRefresher(db.Config().ConnString(), []SourceConfig{cfg})
	if err != nil {
		t.Fatal(err)
	}
	transport := publisher.Client().Transport.(*http.Transport).Clone()
	address := strings.TrimPrefix(publisher.URL, "https://")
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	transport.TLSClientConfig.InsecureSkipVerify = true
	refresher.client.Transport = transport
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- refresher.RefreshDue(ctx) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("publisher fetch never started")
	}
	// A concurrent worker may register unchanged approval, but must not fetch twice.
	contender, err := NewSourceRefresher(db.Config().ConnString(), []SourceConfig{cfg})
	if err != nil {
		t.Fatal(err)
	}
	contender.client = refresher.client
	short, stop := context.WithTimeout(ctx, 2*time.Second)
	err = contender.RefreshDue(short)
	stop()
	if err != nil {
		t.Fatal("concurrent same-source fetch did not skip promptly", err)
	}
	if requests.Load() != 1 {
		t.Fatal("concurrent workers duplicated publisher requests")
	}
	// Revocation must commit while the original HTTP response is still pending.
	revoker, err := NewSourceRefresher(db.Config().ConnString(), nil)
	if err != nil {
		t.Fatal(err)
	}
	short, stop = context.WithTimeout(ctx, 2*time.Second)
	err = revoker.RefreshDue(short)
	stop()
	if err != nil {
		t.Fatal("external HTTP blocked approval revocation", err)
	}
	unblock()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("obsolete fetch did not finish")
	}
	var rows, eligible int
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM undergrid.external_observations WHERE source_id=$1),(SELECT count(*) FROM undergrid.eligible_external_observations WHERE source_id=$1)`, cfg.ID).Scan(&rows, &eligible); err != nil {
		t.Fatal(err)
	}
	if rows != 0 || eligible != 0 {
		t.Fatalf("in-flight fetch resurrected revoked approval: rows=%d eligible=%d", rows, eligible)
	}
}

func TestSourceConfigurationRejectsTrailingOrOversizedInput(t *testing.T) {
	for _, input := range []string{"[] {}", "[]" + strings.Repeat(" ", 128*1024)} {
		path := filepath.Join(t.TempDir(), "sources.json")
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("UNDERGRID_SOURCES_FILE", path)
		if _, err := sourceConfiguration("unused"); err == nil {
			t.Fatal("accepted malformed or oversized source configuration")
		}
	}
}
