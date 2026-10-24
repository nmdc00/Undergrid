package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/riverqueue/river"
)

// Publisher data is a controlled HTTPS fixture, not provider or relevance
// qualification. Both the source refresh and watch scan use real persistence.
func TestExternalOpportunityRefreshWatchLifecycle(t *testing.T) {
	s, db, worker, owner := externalNotificationFixture(t)
	marker := "catalogue-" + NewID()
	input := intentFixture()
	input.Objective = marker
	code, intent := owner.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, intent)
	}
	_, initialInbox := owner.call("GET", "/v1/inbox", "", nil)
	inboxPath := "/v1/inbox?cursor=" + initialInbox["next_cursor"].(string)
	var phase atomic.Int64
	var requests atomic.Int64
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/capabilities" {
			t.Errorf("followed embedded URL %s", r.URL)
			http.NotFound(w, r)
			return
		}
		if phase.Load() == 2 {
			w.WriteHeader(http.StatusGone)
			return
		}
		fmt.Fprintf(w, `{"items":[{"id":"catalogue","version":"%d","url":"https://publisher.example/catalogue","title":%q,"summary":"PRIVATE omitted field","tags":["json"],"status":"active"},{"id":"irrelevant","version":"1","url":"https://publisher.example/other","title":"Unrelated weather tool","status":"active"}],"next_page":0}`, phase.Load()+1, marker)
	}))
	defer publisher.Close()
	cfg := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example/capabilities", Publisher: "Controlled publisher", AccessBasis: "fixture access", RepublicationBasis: "fixture redistribution", TombstoneBasis: "fixture tombstone", PublicPermissionNotice: "Permitted title and URL", Fields: []string{"canonical_url", "title"}}
	refresher := externalNotificationRefresher(t, worker.dsn, cfg, publisher)
	var err error
	ctx := context.Background()
	if err = refresher.RefreshDue(ctx); err != nil {
		t.Fatal(err)
	}
	if err = worker.ScanDueIntentions(ctx); err != nil {
		t.Fatal(err)
	}
	path := "/v1/intents/" + intent["id"].(string) + "/opportunities"
	code, page := owner.call("GET", path, "", nil)
	if code != 200 || len(page["items"].([]any)) != 1 {
		t.Fatalf("external opportunity missing after real source refresh/watch: %d %v", code, page)
	}
	assertExternalOpportunityRevisionConstraint(t, worker.dsn, intent["id"].(string), page["items"].([]any)[0].(map[string]any)["observation_id"].(string))
	limited := owner
	limited.token = "ug_" + NewID()
	if _, err = db.Exec(ctx, `INSERT INTO undergrid.credentials(id,agent_id,digest,scopes,audience,expires_at) SELECT $1,agent_id,$2,ARRAY['inbox:read'],'undergrid-network',now()+interval '1 hour' FROM undergrid.intents WHERE id=$3`, NewID(), sha256Bytes(limited.token), intent["id"]); err != nil {
		t.Fatal(err)
	}
	code, limitedPage := limited.call("GET", path, "", nil)
	if code != 403 {
		t.Fatal("limited credential unexpectedly read opportunities", code, limitedPage)
	}
	code, limitedContext := limited.call("GET", "/v1/agent-context", "", nil)
	if code != 200 || len(limitedContext["external_opportunities"].([]any)) != 0 {
		t.Fatal("inbox-only credential leaked external opportunity details", code, limitedContext)
	}
	code, ownerPage := ownerCallMatch(owner, "GET", "/v1/owner/intents/"+intent["id"].(string)+"/opportunities", "", "", nil)
	if code != 200 || len(ownerPage["items"].([]any)) != 1 {
		t.Fatal("owner external opportunity hidden", code, ownerPage)
	}
	foreignOwner := client(t, s)
	code, foreignPage := ownerCallMatch(foreignOwner, "GET", "/v1/owner/intents/"+intent["id"].(string)+"/opportunities", "", "", nil)
	if code != 404 {
		t.Fatal("foreign owner read external opportunities", code, foreignPage)
	}
	validateExternalOpportunityContract(t, page)
	item := page["items"].([]any)[0].(map[string]any)
	code, selected := owner.call("GET", "/v1/opportunities/"+item["id"].(string), "", nil)
	if code != 200 || selected["id"] != item["id"] {
		t.Fatal("current selected external opportunity unreadable", code, selected)
	}
	if strings.Contains(string(canonical(selected)), "PRIVATE omitted field") || strings.Contains(string(canonical(selected)), "/capabilities") {
		t.Fatal("private source material leaked", selected)
	}
	code, currentContext := owner.call("GET", "/v1/agent-context", "", nil)
	if code != 200 || len(currentContext["external_opportunities"].([]any)) != 1 {
		t.Fatal("missing current context opportunity", code, currentContext)
	}

	if item["origin"] != "external_capability" || item["kind"] != "external_capability" {
		t.Fatal("external evidence relabeled", item)
	}
	if item["evidence"].(map[string]any)["hard_constraints"] != "unknown" {
		t.Fatal("claimed verified fit", item)
	}
	code, inbox := owner.call("GET", inboxPath, "", nil)
	if code != 200 || len(inbox["events"].([]any)) != 1 {
		t.Fatalf("missing atomic inbox %d %v", code, inbox)
	}
	var opportunityCount, eventCount, inboxCount int
	count := func() {
		t.Helper()
		if err := db.QueryRow(ctx, `SELECT count(*),count(e.id),count(n.event_id) FROM undergrid.opportunities o LEFT JOIN undergrid.events e ON e.aggregate_id=o.id AND e.type='opportunity.available' LEFT JOIN undergrid.inbox n ON n.event_id=e.id WHERE o.source_intent_id=$1`, intent["id"]).Scan(&opportunityCount, &eventCount, &inboxCount); err != nil {
			t.Fatal(err)
		}
	}
	count()
	if opportunityCount != 1 || eventCount != 1 || inboxCount != 1 {
		t.Fatal("partial publication", opportunityCount, eventCount, inboxCount)
	}
	forceSourceDue(t, db, cfg.ID)
	if err = refresher.RefreshDue(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE undergrid.intents SET next_scan=now() WHERE id=$1`, intent["id"]); err != nil {
		t.Fatal(err)
	}
	if err = worker.ScanDueIntentions(ctx); err != nil {
		t.Fatal(err)
	}
	count()
	if opportunityCount != 1 || eventCount != 1 || inboxCount != 1 {
		t.Fatal("unchanged snapshot renotified", opportunityCount, eventCount, inboxCount)
	}
	// The normal worker must consume committed source notification wakeups.
	configPath := filepath.Join(t.TempDir(), "sources.json")
	configJSON, _ := json.Marshal([]SourceConfig{cfg})
	if err = os.WriteFile(configPath, configJSON, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERGRID_SOURCES_FILE", configPath)
	var queuedSnapshots int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM undergrid_queue.river_job WHERE kind='external_notification_scan_v1' AND args->>'source_id'=$1 AND NOT(args ? 'intention_id') AND NOT(args ? 'after_id')`, cfg.ID).Scan(&queuedSnapshots); err != nil {
		t.Fatal(err)
	}
	if queuedSnapshots != 1 {
		t.Fatal("unchanged source snapshot duplicated durable fanout", queuedSnapshots)
	}
	phase.Store(1)
	forceSourceDue(t, db, cfg.ID)
	if err = refresher.RefreshDue(ctx); err != nil {
		t.Fatal(err)
	}
	workerCtx, stop := context.WithCancel(ctx)
	stopped := make(chan error, 1)
	go func() { stopped <- RunWorker(workerCtx, worker.dsn, "development") }()
	t.Cleanup(func() {
		stop()
		select {
		case err := <-stopped:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(8 * time.Second):
			t.Error("external worker failed to stop")
		}
	})
	deadline := time.Now().Add(8 * time.Second)
	for {
		code, page = owner.call("GET", path, "", nil)
		var completed int
		if err = db.QueryRow(ctx, `SELECT count(*) FROM undergrid_queue.river_job WHERE kind='external_notification_scan_v1' AND args->>'intention_id'=$1 AND state='completed'`, intent["id"]).Scan(&completed); err != nil {
			t.Fatal(err)
		}
		if code == 200 && len(page["items"].([]any)) == 1 && page["items"].([]any)[0].(map[string]any)["id"] != item["id"] && completed > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("durable source wakeup did not publish new version", code, page)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if code != 200 || len(page["items"].([]any)) != 1 || page["items"].([]any)[0].(map[string]any)["id"] == item["id"] {
		t.Fatal("new source version did not replace current suggestion", code, page)
	}
	code, old := owner.call("GET", "/v1/opportunities/"+item["id"].(string), "", nil)
	if code != 404 {
		t.Fatal("old source version readable", code, old)
	}
	// Every read surface rechecks current permission, intention and observation.
	assertHidden := func() {
		t.Helper()
		for _, endpoint := range []string{path, "/v1/opportunities"} {
			code, value := owner.call("GET", endpoint, "", nil)
			if code != 200 || len(value["items"].([]any)) != 0 {
				t.Fatal("revoked opportunity list", endpoint, code, value)
			}
		}
		code, value := owner.call("GET", inboxPath, "", nil)
		if code != 200 {
			t.Fatal("revoked inbox", code, value)
		}
		for _, event := range value["events"].([]any) {
			if event.(map[string]any)["type"] == "opportunity.available" {
				t.Fatal("revoked external inbox", value)
			}
		}
		code, value = owner.call("GET", "/v1/agent-context", "", nil)
		if code != 200 || len(value["external_opportunities"].([]any)) != 0 {
			t.Fatal("revoked context", code, value)
		}
	}
	for _, pair := range [][2]string{
		{"UPDATE undergrid.intents SET watch_enabled=false WHERE id=$1", "UPDATE undergrid.intents SET watch_enabled=true WHERE id=$1"},
		{"UPDATE undergrid.intents SET state='paused' WHERE id=$1", "UPDATE undergrid.intents SET state='active' WHERE id=$1"},
		{"UPDATE undergrid.intents SET expires_at=now()-interval '1 second' WHERE id=$1", "UPDATE undergrid.intents SET expires_at=now()+interval '1 day' WHERE id=$1"},
	} {
		if _, err = db.Exec(ctx, pair[0], intent["id"]); err != nil {
			t.Fatal(err)
		}
		assertHidden()
		if _, err = db.Exec(ctx, pair[1], intent["id"]); err != nil {
			t.Fatal(err)
		}
	}
	var sourceConfig []byte
	var lastSuccess time.Time
	if err = db.QueryRow(ctx, `SELECT config,last_success FROM undergrid.external_sources WHERE id=$1`, cfg.ID).Scan(&sourceConfig, &lastSuccess); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE undergrid.external_sources SET config=config-'PublicPermissionNotice' WHERE id=$1`, cfg.ID); err != nil {
		t.Fatal(err)
	}
	assertHidden()
	if _, err = db.Exec(ctx, `UPDATE undergrid.external_sources SET config=$2::jsonb WHERE id=$1`, cfg.ID, sourceConfig); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE undergrid.external_sources SET last_success=now()-interval '25 hours' WHERE id=$1`, cfg.ID); err != nil {
		t.Fatal(err)
	}
	assertHidden()
	if _, err = db.Exec(ctx, `UPDATE undergrid.external_sources SET last_success=$2 WHERE id=$1`, cfg.ID, lastSuccess); err != nil {
		t.Fatal(err)
	}
	input.Objective = "unrelated-revision-" + NewID()
	code, revised := revisionCall(owner, "PATCH", "/v1/intents/"+intent["id"].(string), NewID(), intent["etag"].(string), map[string]any{"revision": input})
	if code != 200 {
		t.Fatal("revise watch", code, revised)
	}
	assertHidden()
	phase.Store(2)
	forceSourceDue(t, db, cfg.ID)
	if err = refresher.RefreshDue(ctx); err != nil {
		t.Fatal(err)
	}
	code, page = owner.call("GET", path, "", nil)
	if code != 200 || len(page["items"].([]any)) != 0 {
		t.Fatal("withdrawn external suggestion readable", code, page)
	}
	code, inbox = owner.call("GET", inboxPath, "", nil)
	if code != 200 {
		t.Fatal("withdrawn external inbox", code, inbox)
	}
	for _, event := range inbox["events"].([]any) {
		if event.(map[string]any)["type"] == "opportunity.available" {
			t.Fatal("withdrawn external notification readable", inbox)
		}
	}
	assertHidden()
	if requests.Load() != 4 {
		t.Fatal("unexpected external requests", requests.Load())
	}
}

func validateExternalOpportunityContract(t *testing.T, response map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(response)
	command := exec.Command("node", "--input-type=module", "-e", `
 import fs from 'node:fs'; import Ajv from 'ajv/dist/2020.js'; import addFormats from 'ajv-formats';
 const d=JSON.parse(fs.readFileSync('../internal/network/api/openapi.json'));const a=new Ajv({strict:false});addFormats(a);a.addSchema({$id:'urn:undergrid:external-opportunity',components:d.components});
 const validate=a.compile({$ref:'urn:undergrid:external-opportunity#/components/schemas/Opportunities'});const value=JSON.parse(fs.readFileSync(0,'utf8'));if(!validate(value))throw Error(JSON.stringify(validate.errors));
 for(const mutate of [x=>x.items[0].lead.config={secret:'private'},x=>x.items[0].origin='registered_intent',x=>x.items[0].origin='external_request',x=>x.items[0].evidence.hard_constraints='verified']){const bad=structuredClone(value);mutate(bad);if(validate(bad))throw Error('external evidence/permission oracle accepted');}
 `)
	command.Dir = "../../web"
	command.Stdin = strings.NewReader(string(raw))
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("external opportunity schema proof: %v %s", err, out)
	}
}

func externalNotificationFixture(t *testing.T) (*Service, *pgx.Conn, *Service, testClient) {
	t.Helper()
	s, db := fixture(t)
	apiRole, workerRole := "external_api_"+strings.ReplaceAll(NewID(), "-", ""), "external_worker_"+strings.ReplaceAll(NewID(), "-", "")
	for role, group := range map[string]string{apiRole: "undergrid_app", workerRole: "undergrid_worker"} {
		identifier := pgx.Identifier{role}.Sanitize()
		if _, err := db.Exec(context.Background(), "CREATE ROLE "+identifier+" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS; GRANT "+group+" TO "+identifier); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Exec(context.Background(), "DROP ROLE "+identifier) })
	}
	s.dsn = networkFixtureRoleDSN(t, db, apiRole)
	worker := New(networkFixtureRoleDSN(t, db, workerRole), "development", nil)
	owner := client(t, s)
	return s, db, worker, owner
}

func externalNotificationRefresher(t *testing.T, dsn string, cfg SourceConfig, publisher *httptest.Server) *SourceRefresher {
	t.Helper()
	refresher, err := NewSourceRefresher(dsn, []SourceConfig{cfg})
	if err != nil {
		t.Fatal(err)
	}
	transport := publisher.Client().Transport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(publisher.URL, "https://"))
	}
	transport.TLSClientConfig.InsecureSkipVerify = true // Test-only local controlled HTTPS routing.
	refresher.client = &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return refresher
}

func TestExternalOpportunityQueuedContinuationReachesTail(t *testing.T) {
	_, db, worker, owner := externalNotificationFixture(t)
	ctx := context.Background()
	marker := "tail-" + NewID()
	input := intentFixture()
	input.Objective = marker
	code, intent := owner.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, intent)
	}
	_, initial := owner.call("GET", "/v1/inbox", "", nil)
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/capabilities" {
			t.Errorf("unexpected publisher path %s", r.URL)
			http.NotFound(w, r)
			return
		}
		items := []map[string]any{}
		for i := 0; i < 55; i++ {
			items = append(items, map[string]any{"id": fmt.Sprintf("lead-%03d", i), "version": "1", "url": fmt.Sprintf("https://publisher.example/leads/%d", i), "title": marker, "status": "active"})
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items, "next_page": 0})
	}))
	defer publisher.Close()
	cfg := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example/capabilities", Publisher: "Controlled tail publisher", AccessBasis: "fixture access", RepublicationBasis: "fixture redistribution", TombstoneBasis: "fixture tombstone", PublicPermissionNotice: "Permitted title and URL", Fields: []string{"canonical_url", "title"}}
	refresher := externalNotificationRefresher(t, worker.dsn, cfg, publisher)
	if err := refresher.RefreshDue(ctx); err != nil {
		t.Fatal(err)
	}
	var tail string
	var observations int
	if err := db.QueryRow(ctx, `SELECT count(*),max(id::text) FROM undergrid.external_notification_candidates WHERE source_id=$1`, cfg.ID).Scan(&observations, &tail); err != nil {
		t.Fatal(err)
	}
	if observations != 55 {
		t.Fatal("controlled feed did not persist all observations", observations)
	}
	configPath := filepath.Join(t.TempDir(), "sources.json")
	raw, _ := json.Marshal([]SourceConfig{cfg})
	if err := os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERGRID_SOURCES_FILE", configPath)
	workerCtx, stop := context.WithCancel(ctx)
	stopped := make(chan error, 1)
	go func() { stopped <- RunWorker(workerCtx, worker.dsn, "development") }()
	t.Cleanup(func() {
		stop()
		select {
		case err := <-stopped:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(8 * time.Second):
			t.Error("tail worker failed to stop")
		}
	})
	deadline := time.Now().Add(8 * time.Second)
	var opportunity, event string
	for {
		err := db.QueryRow(ctx, `SELECT o.id,e.id FROM undergrid.opportunities o JOIN undergrid.events e ON e.aggregate_id=o.id AND e.type='opportunity.available' JOIN undergrid.inbox n ON n.event_id=e.id WHERE o.source_intent_id=$1 AND o.external_observation_id=$2`, intent["id"], tail).Scan(&opportunity, &event)
		if err == nil {
			var completed bool
			if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM undergrid_queue.river_job WHERE kind='external_notification_scan_v1' AND state='completed' AND args->>'intention_id'=$1 AND args->>'source_id'=$2 AND args->>'snapshot_hash'<>'' AND args->>'after_observation_id'<>'')`, intent["id"], cfg.ID).Scan(&completed); err != nil {
				t.Fatal(err)
			}
			if completed {
				break
			}
		} else if err != pgx.ErrNoRows {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("durable matching stopped before relevant observation beyond first50", tail)
		}
		time.Sleep(25 * time.Millisecond)
	}
	code, selected := owner.call("GET", "/v1/opportunities/"+opportunity, "", nil)
	if code != 200 || selected["observation_id"] != tail {
		t.Fatal("tail opportunity unavailable", code, selected)
	}
	cursor := initial["next_cursor"].(string)
	found := false
	for page := 0; page < 3 && !found; page++ {
		code, inbox := owner.call("GET", "/v1/inbox?cursor="+cursor, "", nil)
		if code != 200 {
			t.Fatal(code, inbox)
		}
		for _, rawEvent := range inbox["events"].([]any) {
			if rawEvent.(map[string]any)["event_id"] == event {
				found = true
			}
		}
		cursor = inbox["next_cursor"].(string)
	}
	if !found {
		t.Fatal("tail notification missing from real paged inbox", event)
	}
}

// This checks the genuine worker SQL-role boundary, not an HTTP exploit. The
// attempted malformed row is always rolled back, even against the RED schema.
func assertExternalOpportunityRevisionConstraint(t *testing.T, dsn, intent, observation string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	tag, err := tx.Exec(ctx, `INSERT INTO opportunities(id,recipient_agent_id,source_intent_id,kind,input_hash,evidence,external_observation_id,external_observation_hash,external_source_fingerprint)
 SELECT $1,i.agent_id,i.id,c.kind,$2,'{}',c.id,c.content_hash,c.source_fingerprint FROM intents i CROSS JOIN external_notification_candidates c WHERE i.id=$3 AND c.id=$4`, NewID(), NewID(), intent, observation)
	if err == nil {
		if tag.RowsAffected() != 1 {
			t.Fatal("constraint oracle did not reach an eligible observation")
		}
		t.Fatal("worker SQL accepted external opportunity without intention revision")
	}
	pgerr, ok := err.(*pgconn.PgError)
	if !ok || pgerr.Code != "23514" || pgerr.ConstraintName != "opportunities_external_revision_present" {
		t.Fatalf("expected revision check rejection, got %v", err)
	}
}

func TestExternalOpportunityWorkerAcceptsUppercaseSourceID(t *testing.T) {
	_, db, service, owner := externalNotificationFixture(t)
	ctx := context.Background()
	marker := "uppercase-" + NewID()
	input := intentFixture()
	input.Objective = marker
	code, intent := owner.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, intent)
	}
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"items":[{"id":"one","version":"1","url":"https://publisher.example/one","title":%q,"status":"active"}],"next_page":0}`, marker)
	}))
	defer publisher.Close()
	cfg := SourceConfig{ID: strings.ToUpper(NewID()), Kind: "capability_manifest", Endpoint: "https://publisher.example/capabilities", Publisher: "Controlled case publisher", AccessBasis: "fixture access", RepublicationBasis: "fixture redistribution", TombstoneBasis: "fixture tombstone", PublicPermissionNotice: "Permitted title and URL", Fields: []string{"canonical_url", "title"}}
	refresher := externalNotificationRefresher(t, service.dsn, cfg, publisher)
	if err := refresher.RefreshDue(ctx); err != nil {
		t.Fatal(err)
	}
	var snapshot string
	if err := db.QueryRow(ctx, externalSnapshotSQL, cfg.ID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	worker := externalNotificationWorker{service: service}
	if err := worker.Work(ctx, &river.Job[externalNotificationArgs]{Args: externalNotificationArgs{IntentionID: intent["id"].(string), Revision: 1, SourceID: cfg.ID, SnapshotHash: snapshot}}); err != nil {
		t.Fatal(err)
	}
	code, page := owner.call("GET", "/v1/intents/"+intent["id"].(string)+"/opportunities", "", nil)
	if code != 200 || len(page["items"].([]any)) != 1 {
		t.Fatal("accepted uppercase source ID lost relevant opportunity", code, page)
	}
}
