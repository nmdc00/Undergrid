package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type partialScenario struct {
	service   *Service
	db        *pgx.Conn
	worker    *Service
	owner     testClient
	input     IntentInput
	original  map[string]any
	path      string
	body      map[string]any
	config    SourceConfig
	refresher *SourceRefresher
	phase     *atomic.Int64
}

func partialScenarioFixture(t *testing.T, visibility string, configure ...func(*IntentInput)) *partialScenario {
	t.Helper()

	s, db, worker, owner := externalNotificationFixture(t)
	input := intentFixture()
	input.Visibility = visibility
	if visibility == "redacted_public" {
		input.PublicSummary = "Partial source catalog"
	}
	input.HardConstraints.RequiredFields = []string{"source_id", "source_url", "title"}
	input.Preferences = map[string]string{"region": "EU"}
	budget := json.RawMessage(`{"amount_minor":12345,"currency":"EUR","kind":"stated_budget"}`)
	input.Budget = &budget
	for _, change := range configure {
		change(&input)
	}
	code, original := owner.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, original)
	}
	phase := new(atomic.Int64)
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if phase.Load() == 2 {
			w.Write([]byte(`{"items":[],"next_page":0}`))
			return
		}
		fields := []string{"source_id"}
		if phase.Load() == 1 {
			fields = append(fields, "title")
		}
		rawFields, _ := json.Marshal(fields)
		fmt.Fprintf(w, `{"items":[{"id":"catalogue","version":"1","url":"https://publisher.example/catalogue","title":"Existing catalogue","status":"active","capability_claim":{"version":"capability-claim-v1","output":{"kind":"json_dataset","schema_family":"source_catalog_v1"},"provided_fields":%s,"license_ids":["CC0-1.0"]}}],"next_page":0}`, rawFields)
	}))
	t.Cleanup(publisher.Close)
	cfg := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example/capabilities", Publisher: "Controlled publisher", AccessBasis: "fixture access", RepublicationBasis: "fixture redistribution", TombstoneBasis: "fixture tombstone", PublicPermissionNotice: "Permitted public declaration", Fields: []string{"canonical_url", "title", "capability_claim"}}
	refresher := externalNotificationRefresher(t, worker.dsn, cfg, publisher)
	if err := refresher.RefreshDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := "/v1/owner/intents/" + original["id"].(string)
	code, leads := ownerCallMatch(owner, "GET", path+"/external-leads", "", "", nil)
	if code != 200 || len(leads["items"].([]any)) != 1 {
		t.Fatal(code, leads)
	}
	lead := leads["items"].([]any)[0].(map[string]any)
	code, report := ownerCallMatch(owner, "GET", path+"/external-fit-report?lead_id="+lead["id"].(string), "", "", nil)
	if code != 200 {
		t.Fatal(code, report)
	}
	body := map[string]any{"lead_id": lead["id"], "source_version": lead["source_version"], "projection_hash": lead["projection_hash"], "report_hash": report["report_hash"], "intention_revision": report["intention_revision"], "fulfilled_required_fields": []string{"source_id"}}
	return &partialScenario{s, db, worker, owner, input, original, path, body, cfg, refresher, phase}
}
func (x *partialScenario) refresh(t *testing.T) {
	t.Helper()
	if _, err := x.db.Exec(context.Background(), "UPDATE undergrid.external_sources SET next_refresh=now() WHERE id=$1", x.config.ID); err != nil {
		t.Fatal(err)
	}
	if err := x.refresher.RefreshDue(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestOwnerPartialSatisfactionPersistsUnmetRequirementsAndExactRetry(t *testing.T) {
	x := partialScenarioFixture(t, "private")
	owner, original, path, body := x.owner, x.original, x.path, x.body

	code, result := ownerCallMatch(owner, "POST", path+"/partial-satisfaction", "partial-current", original["etag"].(string), body)
	if code != 200 {
		t.Fatalf("missing owner partial-satisfaction behavior: %d %v", code, result)
	}
	if result["state"] != "active" || result["partial_satisfaction"] == nil {
		t.Fatal("partial fulfillment lost active watch or private audit", result)
	}
	remaining := result["hard_constraints"].(map[string]any)["required_fields"].([]any)
	if len(remaining) != 2 || remaining[0] != "source_url" || remaining[1] != "title" {
		t.Fatal("wrong unmet remainder", result)
	}
	code, replay := ownerCallMatch(owner, "POST", path+"/partial-satisfaction", "partial-current", original["etag"].(string), body)
	if code != 200 || replay["version"] != result["version"] {
		t.Fatal("exact retry did not survive its own revision", code, replay)
	}
	expected := x.input
	expected.HardConstraints.RequiredFields = []string{"source_url", "title"}
	var expectedFields map[string]any
	if err := json.Unmarshal(canonical(expected), &expectedFields); err != nil {
		t.Fatal(err)
	}
	for field, want := range expectedFields {
		if !reflect.DeepEqual(result[field], want) {
			t.Fatalf("unrequested constraint changed %s: got%v want%v", field, result[field], want)
		}
	}
	x.refresh(t)
	code, replay = ownerCallMatch(owner, "POST", path+"/partial-satisfaction", "partial-current", original["etag"].(string), body)
	if code != 200 || replay["version"] != result["version"] {
		t.Fatal("same-content source recheck invalidated exact retry", code, replay)
	}
	var revisions, events int
	if err := x.db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.intent_revisions WHERE intent_id=$1", original["id"]).Scan(&revisions); err != nil || revisions != 2 {
		t.Fatal(revisions, err)
	}
	if err := x.db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='intent.partially_satisfied'", original["id"]).Scan(&events); err != nil || events != 1 {
		t.Fatal(events, err)
	}

}

func TestPartialSatisfactionRejectsInvalidSelectionsAndStalePinsWithoutEffects(t *testing.T) {
	x := partialScenarioFixture(t, "private")
	for _, tc := range []struct {
		name   string
		fields []string
	}{
		{"empty", nil}, {"duplicate", []string{"source_id", "source_id"}}, {"unknown", []string{"not_required"}}, {"all", []string{"source_id", "source_url", "title"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{}
			for k, v := range x.body {
				body[k] = v
			}
			body["fulfilled_required_fields"] = tc.fields
			code, out := ownerCallMatch(x.owner, "POST", x.path+"/partial-satisfaction", NewID(), x.original["etag"].(string), body)
			if code != 422 {
				t.Fatal(code, out)
			}
		})
	}
	for _, field := range []string{"projection_hash", "report_hash", "source_version", "intention_revision"} {
		t.Run(field, func(t *testing.T) {
			body := map[string]any{}
			for k, v := range x.body {
				body[k] = v
			}
			body[field] = strings.Repeat("a", 64)
			if field == "source_version" {
				body[field] = "changed"
			}
			if field == "intention_revision" {
				body[field] = 99
			}
			code, out := ownerCallMatch(x.owner, "POST", x.path+"/partial-satisfaction", NewID(), x.original["etag"].(string), body)
			if code != 409 {
				t.Fatal(code, out)
			}
		})
	}
	foreign := client(t, x.service)
	code, out := ownerCallMatch(foreign, "POST", x.path+"/partial-satisfaction", NewID(), x.original["etag"].(string), x.body)
	if code != 404 {
		t.Fatal("foreign owner", code, out)
	}
	code, out = ownerCallMatch(x.owner, "POST", x.path+"/partial-satisfaction", NewID(), `"9999"`, x.body)
	if code != 409 {
		t.Fatal("stale If-Match", code, out)
	}
	code, out = ownerCallMatch(x.owner, "POST", x.path+"/partial-satisfaction", NewID(), "", x.body)
	if code != 428 {
		t.Fatal("missing If-Match", code, out)
	}
	var head, revisions, events int
	if err := x.db.QueryRow(context.Background(), "SELECT current_revision,(SELECT count(*) FROM undergrid.intent_revisions WHERE intent_id=i.id),(SELECT count(*) FROM undergrid.events WHERE aggregate_id=i.id AND type='intent.partially_satisfied') FROM undergrid.intents i WHERE id=$1", x.original["id"]).Scan(&head, &revisions, &events); err != nil || head != 1 || revisions != 1 || events != 0 {
		t.Fatal("invalid decision mutated state", head, revisions, events, err)
	}
}

func TestPartialSatisfactionCurrentSourceAndActorRequiredForRetry(t *testing.T) {
	x := partialScenarioFixture(t, "private")
	path := x.path + "/partial-satisfaction"
	code, out := ownerCallMatch(x.owner, "POST", path, "source-retry", x.original["etag"].(string), x.body)
	if code != 200 {
		t.Fatal(code, out)
	}
	x.phase.Store(1)
	x.refresh(t)
	code, out = ownerCallMatch(x.owner, "POST", path, "source-retry", x.original["etag"].(string), x.body)
	if code != 409 {
		t.Fatal("changed claim with same public version", code, out)
	}
	x.phase.Store(0)
	x.refresh(t)
	code, out = ownerCallMatch(x.owner, "POST", path, "source-retry", x.original["etag"].(string), x.body)
	if code != 200 {
		t.Fatal("restored same source", code, out)
	}
	if _, err := x.db.Exec(context.Background(), "UPDATE undergrid.external_sources SET config=jsonb_set(config,'{PublicPermissionNotice}','\"\"') WHERE id=$1", x.config.ID); err != nil {
		t.Fatal(err)
	}
	code, out = ownerCallMatch(x.owner, "POST", path, "source-retry", x.original["etag"].(string), x.body)
	if code != 404 {
		t.Fatal("revoked permission replay", code, out)
	}
	if _, err := x.db.Exec(context.Background(), "UPDATE undergrid.agents SET status='revoked' WHERE id=(SELECT agent_id FROM undergrid.intents WHERE id=$1)", x.original["id"]); err != nil {
		t.Fatal(err)
	}
	code, out = ownerCallMatch(x.owner, "POST", path, "source-retry", x.original["etag"].(string), x.body)
	if code != 403 {
		t.Fatal("revoked attribution replay", code, out)
	}
}

func TestPartialSatisfactionAuditPrivateUnforgeableAndHistoricalAfterGenericRevision(t *testing.T) {
	for _, visibility := range []string{"redacted_public", "restricted"} {
		t.Run(visibility, func(t *testing.T) {
			x := partialScenarioFixture(t, visibility)
			code, result := ownerCallMatch(x.owner, "POST", x.path+"/partial-satisfaction", "private-audit", x.original["etag"].(string), x.body)
			if code != 200 {
				t.Fatal(code, result)
			}
			audit := result["partial_satisfaction"]
			latestETag := result["etag"].(string)
			other := client(t, x.service)
			if visibility == "restricted" {
				_, identity := other.call("GET", "/v1/me", "", nil)
				grant := map[string]any{"principal_id": identity["principal_id"], "fields": []string{"objective", "output", "hard_constraints", "budget"}, "expires_at": time.Now().Add(time.Hour)}
				code, out := ownerCallMatch(x.owner, "POST", x.path+"/grants", "audit-grant", result["etag"].(string), grant)
				if code != 201 {
					t.Fatal(code, out)
				}
				latestETag = out["etag"].(string)
			}
			if visibility == "restricted" {
				_, latest := ownerCallMatch(x.owner, "GET", x.path, "", "", nil)
				latestETag = latest["etag"].(string)
			}
			code, projection := other.call("GET", "/v1/intents/"+x.original["id"].(string), "", nil)
			if code != 200 || projection["partial_satisfaction"] != nil || strings.Contains(string(canonical(projection)), "command_digest") {
				t.Fatal("private provenance leaked", code, projection)
			}
			forged := map[string]any{}
			if err := json.Unmarshal(canonical(x.input), &forged); err != nil {
				t.Fatal(err)
			}
			forged["partial_satisfaction"] = audit
			code, out := x.owner.call("POST", "/v1/intents", "forge-create", forged)
			if code != 400 {
				t.Fatal("forged creation", code, out)
			}
			code, out = x.owner.versioned("PATCH", "/v1/intents/"+x.original["id"].(string), "forge-revision", latestETag, map[string]any{"revision": forged})
			if code != 400 {
				t.Fatal("forged revision", code, out)
			}
			next := x.input
			next.Objective = "Later owner clarification"
			next.HardConstraints.RequiredFields = []string{"title"}
			code, out = x.owner.versioned("PATCH", "/v1/intents/"+x.original["id"].(string), "ordinary-revision", latestETag, map[string]any{"revision": next})
			if code != 200 || !reflect.DeepEqual(out["partial_satisfaction"], audit) {
				t.Fatal("server audit lost or rewritten", code, out)
			}
			code, out = ownerCallMatch(x.owner, "POST", x.path+"/partial-satisfaction", "private-audit", x.original["etag"].(string), x.body)
			if code != 409 {
				t.Fatal("old receipt survived unrelated revision", code, out)
			}
		})
	}
}

func partialEffectSnapshot(t *testing.T, x *partialScenario) []int64 {
	t.Helper()
	out := make([]int64, 9)
	err := x.db.QueryRow(context.Background(), `SELECT current_revision,version,
 (SELECT count(*) FROM undergrid.intent_revisions WHERE intent_id=i.id),
 (SELECT count(*) FROM undergrid.events WHERE aggregate_id=i.id),
 (SELECT count(*) FROM undergrid.inbox b JOIN undergrid.events e ON e.id=b.event_id WHERE e.aggregate_id=i.id),
 (SELECT count(*) FROM undergrid_queue.river_job WHERE args->>'cause_id'=i.id::text),
 (SELECT count(*) FROM undergrid.idempotency WHERE operation=$2),
 (SELECT inbox_sequence FROM undergrid.agents WHERE id=i.agent_id),
 (SELECT count(*) FROM undergrid_queue.river_job WHERE args->>'candidate_id'=i.id::text)
 FROM undergrid.intents i WHERE id=$1`, x.original["id"], "POST "+x.path+"/partial-satisfaction").Scan(&out[0], &out[1], &out[2], &out[3], &out[4], &out[5], &out[6], &out[7], &out[8])
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestPartialSatisfactionNOWAITAndEnqueueFailureRollBackAllEffects(t *testing.T) {
	x := partialScenarioFixture(t, "private")
	ctx := context.Background()
	before := partialEffectSnapshot(t, x)
	lock, err := x.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, "SELECT id FROM undergrid.external_sources WHERE id=$1 FOR UPDATE", x.config.ID); err != nil {
		t.Fatal(err)
	}
	code, out := ownerCallMatch(x.owner, "POST", x.path+"/partial-satisfaction", "atomic-partial", x.original["etag"].(string), x.body)
	if code != 500 || out["error"].(map[string]any)["retryable"] != true {
		t.Fatal("source contention must permit exact retry", code, out)
	}
	if err = lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, partialEffectSnapshot(t, x)) {
		t.Fatal("contention left effects")
	}
	name := "partial_enqueue_" + strings.ReplaceAll(NewID(), "-", "")
	quoted := pgx.Identifier{"undergrid", name}.Sanitize()
	sql := "CREATE FUNCTION " + quoted + "() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='intention_watch_refresh_v1' AND NEW.args->>'candidate_id'='" + x.original["id"].(string) + "' THEN RAISE EXCEPTION 'fixture partial enqueue failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER " + pgx.Identifier{name}.Sanitize() + " BEFORE INSERT ON undergrid_queue.river_job FOR EACH ROW EXECUTE FUNCTION " + quoted + "()"
	if _, err = x.db.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		if _, err := x.db.Exec(ctx, "DROP TRIGGER IF EXISTS "+pgx.Identifier{name}.Sanitize()+" ON undergrid_queue.river_job; DROP FUNCTION IF EXISTS "+quoted+"()"); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(cleanup)
	code, out = ownerCallMatch(x.owner, "POST", x.path+"/partial-satisfaction", "atomic-partial", x.original["etag"].(string), x.body)
	if code != 500 {
		t.Fatal(code, out)
	}
	if !reflect.DeepEqual(before, partialEffectSnapshot(t, x)) {
		t.Fatal("enqueue failure left revision/event/inbox/receipt/job")
	}
	cleanup()
	code, out = ownerCallMatch(x.owner, "POST", x.path+"/partial-satisfaction", "atomic-partial", x.original["etag"].(string), x.body)
	if code != 200 {
		t.Fatal("exact retry after rollback", code, out)
	}
}

func TestPartialSatisfactionMCPA2AExactRetryAndCurrentResourceRecovery(t *testing.T) {
	x := partialScenarioFixture(t, "private")
	ctx := context.Background()
	if err := x.service.ConfigureMCP("https://network.fixture.test"); err != nil {
		t.Fatal(err)
	}
	peer := mcpSDKClient(t, x.service, x.owner.token)
	args := map[string]any{"id": x.original["id"], "body": x.body, "if_match": x.original["etag"], "idempotency_key": "adapter-partial"}
	result, err := peer.CallTool(ctx, &mcp.CallToolParams{Name: "intention_partial_satisfaction", Arguments: args})
	if err != nil || result.IsError {
		t.Fatalf("independent SDK partial: %v %+v", err, result)
	}
	partial := mcpToolData(t, result)
	if partial["state"] != "active" || partial["partial_satisfaction"] == nil {
		t.Fatal(partial)
	}
	if err = x.service.ConfigureA2ATransport("https://undergrid.test"); err != nil {
		t.Fatal(err)
	}
	command := a2aCommandBody(NewID(), "intention_partial_satisfaction", "adapter-partial", x.body)
	commandArgs(command)["id"] = x.original["id"]
	commandArgs(command)["if_match"] = x.original["etag"]
	code, out, _ := a2aCall(x.owner, "POST", "/a2a/message:send", command)
	if code != 200 || out["task"] == nil || a2aLiveTaskResult(out)["version"] != partial["version"] {
		t.Fatal("A2A exact domain receipt", code, out)
	}
	receipt := out["task"].(map[string]any)["id"].(string)
	code, recovered, _ := a2aCall(x.owner, "GET", "/a2a/tasks/"+receipt, nil)
	if code != 200 || recovered["artifacts"] == nil {
		t.Fatal("partial receipt failed current authorized resource recovery", code, recovered)
	}
	var revisions int
	if err = x.db.QueryRow(ctx, "SELECT count(*) FROM undergrid.intent_revisions WHERE intent_id=$1", x.original["id"]).Scan(&revisions); err != nil || revisions != 2 {
		t.Fatal("adapter duplicated fulfillment", revisions, err)
	}
}

func TestPartialSatisfactionRepeatedActionKeepsBoundedOwnerDeclaredHistory(t *testing.T) {
	x := partialScenarioFixture(t, "private")
	code, first := ownerCallMatch(x.owner, "POST", x.path+"/partial-satisfaction", "first-part", x.original["etag"].(string), x.body)
	if code != 200 {
		t.Fatal(code, first)
	}
	code, report := ownerCallMatch(x.owner, "GET", x.path+"/external-fit-report?lead_id="+x.body["lead_id"].(string), "", "", nil)
	if code != 200 {
		t.Fatal(code, report)
	}
	body := map[string]any{}
	for k, v := range x.body {
		body[k] = v
	}
	body["intention_revision"] = report["intention_revision"]
	body["report_hash"] = report["report_hash"]
	body["fulfilled_required_fields"] = []string{"source_url"}
	code, next := ownerCallMatch(x.owner, "POST", x.path+"/partial-satisfaction", "second-part", first["etag"].(string), body)
	if code != 200 {
		t.Fatal(code, next)
	}
	audit := next["partial_satisfaction"].(map[string]any)
	if audit["previous_fulfillment_revision"] != float64(2) || audit["basis_revision"] != float64(2) || audit["result_revision"] != float64(3) || audit["evidence_class"] != "owner_declared_partial_satisfaction" {
		t.Fatal("unbounded/incorrect provenance", audit)
	}
	if fields := next["hard_constraints"].(map[string]any)["required_fields"].([]any); len(fields) != 1 || fields[0] != "title" {
		t.Fatal(fields)
	}
	if next["rights_assurance"] != nil || next["verified"] != nil {
		t.Fatal("owner declaration upgraded assurance", next)
	}
}

func TestPartialSatisfactionLocksSourceUntilItsRevisionCommits(t *testing.T) {
	x := partialScenarioFixture(t, "private")
	ctx := context.Background()
	barrier, err := x.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback(ctx)
	gate := NewID()
	if _, err = barrier.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,61741))", gate); err != nil {
		t.Fatal(err)
	}
	name := "partial_barrier_" + strings.ReplaceAll(NewID(), "-", "")
	quoted := pgx.Identifier{"undergrid", name}.Sanitize()
	sql := "CREATE FUNCTION " + quoted + "() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.intent_id::text='" + x.original["id"].(string) + "' AND NEW.document ? 'partial_satisfaction' THEN PERFORM pg_advisory_xact_lock(hashtextextended('" + gate + "',61741)); END IF; RETURN NEW; END $$; CREATE TRIGGER " + pgx.Identifier{name}.Sanitize() + " BEFORE INSERT ON undergrid.intent_revisions FOR EACH ROW EXECUTE FUNCTION " + quoted + "()"
	// Use a separate admin connection: the barrier transaction remains open.
	admin, err := pgx.ConnectConfig(ctx, x.db.Config())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	if _, err = admin.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := x.db.Exec(ctx, "DROP TRIGGER IF EXISTS "+pgx.Identifier{name}.Sanitize()+" ON undergrid.intent_revisions; DROP FUNCTION IF EXISTS "+quoted+"()"); err != nil {
			t.Error(err)
		}
	})
	type response struct {
		code int
		body map[string]any
	}
	finished := make(chan response, 1)
	go func() {
		code, out := ownerCallMatch(x.owner, "POST", x.path+"/partial-satisfaction", "source-serialized", x.original["etag"].(string), x.body)
		finished <- response{code, out}
	}()
	cfg, err := pgx.ParseConfig(x.service.dsn)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err = admin.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE usename=$1 AND wait_event='advisory' AND query LIKE '%INSERT INTO intent_revisions%')", cfg.User).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("domain did not reach controlled post-source-lock revision barrier")
		}
		time.Sleep(10 * time.Millisecond)
	}
	revoke, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = revoke.Exec(ctx, "SET LOCAL lock_timeout='100ms'"); err != nil {
		t.Fatal(err)
	}
	_, err = revoke.Exec(ctx, "UPDATE undergrid.external_sources SET config=jsonb_set(config,'{PublicPermissionNotice}','\"\"') WHERE id=$1", x.config.ID)
	var locked *pgconn.PgError
	if !errors.As(err, &locked) || locked.Code != "55P03" {
		t.Fatal("source changed while authorized revision was in flight", err)
	}
	if err = revoke.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err = barrier.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-finished:
		if result.code != 200 {
			t.Fatal(result.code, result.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("revision did not complete after barrier release")
	}
	if _, err = admin.Exec(ctx, "UPDATE undergrid.external_sources SET config=jsonb_set(config,'{PublicPermissionNotice}','\"\"') WHERE id=$1", x.config.ID); err != nil {
		t.Fatal(err)
	}
	code, out := ownerCallMatch(x.owner, "POST", x.path+"/partial-satisfaction", "source-serialized", x.original["etag"].(string), x.body)
	if code != 404 {
		t.Fatal("committed source revocation replay", code, out)
	}
}
