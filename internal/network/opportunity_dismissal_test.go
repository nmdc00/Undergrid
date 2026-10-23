package network

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Uses real API and worker LOGINs. Separate fixture principals are not proof of
// independent beneficial ownership.
func TestOpportunityDismissalHTTPPersistsAndSuppressesUnchangedEvidence(t *testing.T) {
	s, db, worker, a := externalNotificationFixture(t)
	b := client(t, s)
	ctx := context.Background()
	workerDB, err := pgx.Connect(ctx, worker.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer workerDB.Close(ctx)
	if _, err = workerDB.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "dismissal catalogue " + NewID()
	code, source := a.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, source)
	}
	code, candidate := b.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, candidate)
	}
	if err := worker.scanIntentCandidate(ctx, workerDB, source["id"].(string), candidate["id"].(string), false); err != nil {
		t.Fatal(err)
	}
	code, page := a.call("GET", "/v1/intents/"+source["id"].(string)+"/opportunities", "", nil)
	if code != 200 || len(page["items"].([]any)) != 1 {
		t.Fatal(code, page)
	}
	id := page["items"].([]any)[0].(map[string]any)["id"].(string)
	path := "/v1/opportunities/" + id + "/dismiss"
	limited := a
	limited.token = "ug_" + NewID()
	if _, err = db.Exec(ctx, `INSERT INTO undergrid.credentials(id,agent_id,digest,scopes,audience,expires_at) SELECT $1,agent_id,$2,ARRAY['intents:read'],'undergrid-network',now()+interval '1 hour' FROM undergrid.intents WHERE id=$3`, NewID(), sha256Bytes(limited.token), source["id"]); err != nil {
		t.Fatal(err)
	}
	code, deniedScope := limited.call("POST", path, "read-only-dismissal", map[string]any{})
	if code != 403 {
		t.Fatal("read-only credential dismissed suggestion", code, deniedScope)
	}
	// Force failure after the state update: the entire command must roll back.
	triggerName := "dismiss_fault_" + strings.ReplaceAll(NewID(), "-", "")
	quoted := pgx.Identifier{"undergrid", triggerName}.Sanitize()
	if _, err = db.Exec(ctx, "CREATE FUNCTION "+quoted+"() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.type='opportunity.dismissed' AND NEW.aggregate_id::text='"+id+"' THEN RAISE EXCEPTION 'fixture dismissal event failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER "+pgx.Identifier{triggerName}.Sanitize()+" BEFORE INSERT ON undergrid.events FOR EACH ROW EXECUTE FUNCTION "+quoted+"()"); err != nil {
		t.Fatal(err)
	}
	removeFault := func() {
		if _, err := db.Exec(ctx, "DROP TRIGGER IF EXISTS "+pgx.Identifier{triggerName}.Sanitize()+" ON undergrid.events; DROP FUNCTION IF EXISTS "+quoted+"()"); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(removeFault)
	code, failed := a.call("POST", path, "dismiss-current", map[string]any{})
	if code != 500 {
		t.Fatal("expected atomic fault", code, failed)
	}
	var preserved string
	var receipts int
	if err = db.QueryRow(ctx, "SELECT state FROM undergrid.opportunities WHERE id=$1", id).Scan(&preserved); err != nil || preserved != "suggested" {
		t.Fatal("partial dismissal", preserved, err)
	}
	if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid.idempotency WHERE operation=$1 AND key='dismiss-current'", "POST "+path).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatal("partial receipt", receipts, err)
	}
	removeFault()
	for _, invalid := range []any{nil, map[string]any{"decision": "accept"}} {
		code, out := a.call("POST", path, NewID(), invalid)
		if code != 400 {
			t.Fatal("closed input accepted", code, out)
		}
	}
	code, result := a.call("POST", path, "dismiss-current", map[string]any{})
	if code != 200 || result["id"] != id || result["state"] != "dismissed" {
		t.Fatalf("missing authorized dismissal: %d %v", code, result)
	}
	code, replay := a.call("POST", path, "dismiss-current", map[string]any{})
	if code != 200 || replay["id"] != id || replay["state"] != "dismissed" {
		t.Fatal("lost dismissal receipt", code, replay)
	}
	// Both finite transports recover the same domain receipt; neither adapter
	// synthesizes dismissal authority or mutates the evidence independently.
	if err = s.ConfigureMCP("https://network.fixture.test"); err != nil {
		t.Fatal(err)
	}
	peer := mcpSDKClient(t, s, a.token)
	toolResult, err := peer.CallTool(ctx, &mcp.CallToolParams{Name: "opportunity_dismiss", Arguments: map[string]any{"id": id, "body": map[string]any{}, "idempotency_key": "dismiss-current"}})
	if err != nil || toolResult.IsError || mcpToolData(t, toolResult)["state"] != "dismissed" {
		t.Fatalf("MCP dismissal recovery: %v %+v", err, toolResult)
	}
	if err = s.ConfigureA2ATransport("https://undergrid.test"); err != nil {
		t.Fatal(err)
	}
	command := a2aCommandBody(NewID(), "opportunity_dismiss", "dismiss-current", map[string]any{})
	commandArgs(command)["id"] = id
	code, task, _ := a2aCall(a, "POST", "/a2a/message:send", command)
	if code != 200 || task["task"] == nil || a2aLiveTaskResult(task)["state"] != "dismissed" {
		t.Fatal("A2A dismissal recovery", code, task)
	}
	_, identity := a.call("GET", "/v1/me", "", nil)
	// The actual application SQL identity cannot reopen or rewrite evidence even
	// after an authorized command has dismissed the row.
	apiDB, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer apiDB.Close(ctx)
	for _, mutation := range []string{"UPDATE opportunities SET state='suggested' WHERE id=$1", "UPDATE opportunities SET input_hash='forged' WHERE id=$1"} {
		tx, err := apiDB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, "SET LOCAL ROLE undergrid_app; SET LOCAL search_path=undergrid,pg_catalog"); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, "SELECT set_config('undergrid.principal_id',$1,true),set_config('undergrid.agent_id',$2,true)", identity["principal_id"], identity["agent_id"]); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, mutation, id)
		var denied *pgconn.PgError
		if !errors.As(err, &denied) || denied.Code != "42501" {
			t.Fatal("application mutation boundary", mutation, err)
		}
		if err = tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
	}
	code, foreign := b.call("POST", path, "dismiss-current", map[string]any{})
	if code != 404 {
		t.Fatal("foreign dismissal/replay", code, foreign)
	}
	if err := worker.scanIntentCandidate(ctx, workerDB, source["id"].(string), candidate["id"].(string), false); err != nil {
		t.Fatal(err)
	}
	var state string
	var opportunities, notifications int
	if err := db.QueryRow(ctx, "SELECT state FROM undergrid.opportunities WHERE id=$1", id).Scan(&state); err != nil || state != "dismissed" {
		t.Fatal(state, err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.opportunities WHERE source_intent_id=$1 AND candidate_intent_id=$2", source["id"], candidate["id"]).Scan(&opportunities); err != nil || opportunities != 1 {
		t.Fatal(opportunities, err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='opportunity.available'", id).Scan(&notifications); err != nil || notifications != 1 {
		t.Fatal("renotified unchanged evidence", notifications, err)
	}
	code, hidden := a.call("GET", "/v1/opportunities/"+id, "", nil)
	if code != 404 {
		t.Fatal("dismissed suggestion visible", code, hidden)
	}
	// A new eligible immutable revision creates a new suggestion rather than
	// resurrecting the dismissed evidence or granting a new receipt for it.
	input.Objective = "Changed private descriptive detail"
	code, revised := b.versioned("PATCH", "/v1/intents/"+candidate["id"].(string), "new-candidate-evidence", candidate["etag"].(string), map[string]any{"revision": input})
	if code != 200 {
		t.Fatal(code, revised)
	}
	if err := worker.scanIntentCandidate(ctx, workerDB, source["id"].(string), candidate["id"].(string), false); err != nil {
		t.Fatal(err)
	}
	code, stale := a.call("POST", path, "dismiss-current", map[string]any{})
	if code != 404 {
		t.Fatal("stale evidence replay", code, stale)
	}
	code, changed := a.call("GET", "/v1/intents/"+source["id"].(string)+"/opportunities", "", nil)
	if code != 200 || len(changed["items"].([]any)) != 1 {
		t.Fatal("changed eligible evidence not suggested", code, changed)
	}
	newID := changed["items"].([]any)[0].(map[string]any)["id"].(string)
	if newID == id {
		t.Fatal("dismissed evidence reopened")
	}
	path = "/v1/opportunities/" + newID + "/dismiss"
	code, result = a.call("POST", path, "dismiss-current", map[string]any{})
	if code != 200 {
		t.Fatal(code, result)
	}
	// Current source authority is required even for a retained success receipt.
	if _, err := db.Exec(ctx, "UPDATE undergrid.intents SET state='paused',watch_enabled=false WHERE id=$1", source["id"]); err != nil {
		t.Fatal(err)
	}
	code, denied := a.call("POST", path, "dismiss-current", map[string]any{})
	if code != 404 {
		t.Fatal("paused source replayed old receipt", code, denied)
	}
}

func TestOpportunityDismissalExternalKindsAndCurrentSourcePermission(t *testing.T) {
	for _, sourceKind := range []string{"github_requests", "capability_manifest"} {
		t.Run(sourceKind, func(t *testing.T) {
			s, db, worker, a := externalNotificationFixture(t)
			marker := "dismiss-external-" + NewID()
			input := intentFixture()
			input.Objective = marker
			code, intent := a.call("POST", "/v1/intents", NewID(), input)
			if code != 201 {
				t.Fatal(code, intent)
			}
			publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if sourceKind == "github_requests" {
					fmt.Fprintf(w, `[{"id":271,"html_url":"https://github.com/fixture/project/issues/271","title":%q,"updated_at":"2026-10-23T00:00:00Z","state":"open","labels":[{"name":"undergrid"}]}]`, marker)
					return
				}
				fmt.Fprintf(w, `{"items":[{"id":"lead","version":"1","url":"https://publisher.example/lead","title":%q,"status":"active"}],"next_page":0}`, marker)
			}))
			defer publisher.Close()
			cfg := SourceConfig{ID: NewID(), Kind: sourceKind, Endpoint: "https://publisher.example/feed", Publisher: "Controlled publisher", AccessBasis: "fixture access", RepublicationBasis: "fixture redistribution", TombstoneBasis: "fixture tombstone", PublicPermissionNotice: "Permitted title and URL", Fields: []string{"canonical_url", "title"}}
			if sourceKind == "github_requests" {
				cfg.Endpoint = "https://api.github.com/repos/fixture/project/issues"
				cfg.Labels = []string{"undergrid"}
			}
			// Both source adapters use their actual published JSON format.
			refresher := externalNotificationRefresher(t, worker.dsn, cfg, publisher)
			if err := refresher.RefreshDue(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := worker.ScanDueIntentions(context.Background()); err != nil {
				t.Fatal(err)
			}
			code, page := a.call("GET", "/v1/intents/"+intent["id"].(string)+"/opportunities", "", nil)
			if code != 200 || len(page["items"].([]any)) != 1 {
				t.Fatal(code, page)
			}
			id := page["items"].([]any)[0].(map[string]any)["id"].(string)
			_, me := a.call("GET", "/v1/me", "", nil)
			path := "/v1/owner/agents/" + me["agent_id"].(string) + "/opportunities/" + id + "/dismiss"
			foreign := client(t, s)
			code, out := ownerCallMatch(foreign, "POST", path, "foreign-dismiss", "", map[string]any{})
			if code != 404 {
				t.Fatal(code, out)
			}
			for range 2 {
				code, out = ownerCallMatch(a, "POST", path, "owner-dismiss", "", map[string]any{})
				if code != 200 || out["state"] != "dismissed" {
					t.Fatal(code, out)
				}
			}
			if _, err := db.Exec(context.Background(), "UPDATE undergrid.external_sources SET config=jsonb_set(config,'{PublicPermissionNotice}','\"\"') WHERE id=$1", cfg.ID); err != nil {
				t.Fatal(err)
			}
			code, out = ownerCallMatch(a, "POST", path, "owner-dismiss", "", map[string]any{})
			if code != 404 {
				t.Fatal("removed source permission replay", code, out)
			}
		})
	}
}

func TestOpportunityDismissalRecruitmentDoesNotAdmitOrAcceptRights(t *testing.T) {
	s, db, worker, _ := externalNotificationFixture(t)
	organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
	role := RecruitmentRole{Key: "builder", SchemaFamily: "source_catalog_v1", Description: "Build catalog"}
	code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Dismissible opportunity", "visibility": "restricted", "recruitment_roles": []RecruitmentRole{role}})
	if code != 201 {
		t.Fatal(code, room)
	}
	_, me := builder.call("GET", "/v1/me", "", nil)
	workerDB, err := pgx.Connect(context.Background(), worker.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer workerDB.Close(context.Background())
	if _, err = workerDB.Exec(context.Background(), "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	if _, err = worker.recruitOne(context.Background(), workerDB, room["id"].(string), role, me["agent_id"].(string), me["principal_id"].(string)); err != nil {
		t.Fatal(err)
	}
	var id string
	if err = db.QueryRow(context.Background(), "SELECT id FROM undergrid.opportunities WHERE source_swarm_id=$1 AND recipient_agent_id=$2", room["id"], me["agent_id"]).Scan(&id); err != nil {
		t.Fatal(err)
	}
	path := "/v1/owner/agents/" + me["agent_id"].(string) + "/opportunities/" + id + "/dismiss"
	code, out := ownerCallMatch(builder, "POST", path, "dismiss-recruitment", "", map[string]any{})
	if code != 200 || out["state"] != "dismissed" {
		t.Fatal(code, out)
	}
	var memberships, approvals int
	if err = db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.memberships WHERE swarm_id=$1 AND agent_id=$2", room["id"], me["agent_id"]).Scan(&memberships); err != nil || memberships != 0 {
		t.Fatal(memberships, err)
	}
	if err = db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.approvals WHERE principal_id=$1", me["principal_id"]).Scan(&approvals); err != nil || approvals != 0 {
		t.Fatal(approvals, err)
	}
	if _, err = db.Exec(context.Background(), "UPDATE undergrid.swarms SET state='archived' WHERE id=$1", room["id"]); err != nil {
		t.Fatal(err)
	}
	code, out = ownerCallMatch(builder, "POST", path, "dismiss-recruitment", "", map[string]any{})
	if code != 404 {
		t.Fatal("archived recruitment replay", code, out)
	}
}
