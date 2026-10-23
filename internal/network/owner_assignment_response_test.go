package network

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestOwnerAssignmentReviewAndResponsePersistOnlyAssignment(t *testing.T) {
	f := newOwnerAssignmentFixture(t)
	owner, foreign, db, agent, swarm, plan := f.owner, f.foreign, f.db, f.agent, f.swarm, f.plan
	task := plan["tasks"].([]any)[0].(map[string]any)
	var cases []map[string]any
	observe := func(r *http.Request, w *httptest.ResponseRecorder, body any) {
		if !strings.Contains(r.URL.Path, "/assignments") && !strings.Contains(r.URL.Path, "/assignment-response") {
			return
		}
		var response any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"method": r.Method, "path": r.URL.String(), "request": body, "response": response, "status": w.Code, "request_id": w.Header().Get("X-Request-ID"), "etag": w.Header().Get("ETag")})
	}
	owner.observe, foreign.observe = observe, observe
	defer func() {
		raw, err := json.Marshal(cases)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--owner-assignment-response")
		cmd.Stdin = bytes.NewReader(raw)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("owner assignment contract: %v %s", err, out)
		}
	}()
	base := "/v1/owner/agents/" + agent
	code, review := ownerCallMatch(owner, "GET", base+"/plans/"+plan["id"].(string)+"/assignments", "", "", nil)
	if code != 200 {
		t.Fatalf("owner assignment review unavailable: %d %v", code, review)
	}
	if len(review["items"].([]any)) != 1 {
		t.Fatal("assigned tasks", review)
	}
	path := base + "/tasks/" + task["id"].(string) + "/assignment-response"
	body := map[string]any{"decision": "accept", "plan_hash": plan["content_hash"], "acknowledge_responsibility_and_terms": true}
	for _, attack := range []map[string]any{
		{"decision": "accept", "plan_hash": plan["content_hash"]},
		{"decision": "accept", "plan_hash": strings.Repeat("0", 64), "acknowledge_responsibility_and_terms": true},
	} {
		if code, out := ownerCallMatch(owner, "POST", path, NewID(), task["etag"].(string), attack); code < 400 {
			t.Fatal("unchecked acceptance", code, out)
		}
	}
	if code, out := ownerCallMatch(foreign, "POST", path, NewID(), task["etag"].(string), body); code != 404 {
		t.Fatal("foreign assignment", code, out)
	}
	for _, match := range []string{"", "\"999\""} {
		if code, out := ownerCallMatch(owner, "POST", path, NewID(), match, body); code < 400 {
			t.Fatal("unchecked task version", code, out)
		}
	}
	// The owner also owns the registration listener; selecting a different owned
	// agent must not inherit this planner's assignment authority.
	_, owned := owner.call("GET", "/v1/agents", "", nil)
	for _, raw := range owned["items"].([]any) {
		other := raw.(map[string]any)["id"].(string)
		if other != agent {
			otherPath := "/v1/owner/agents/" + other + "/tasks/" + task["id"].(string) + "/assignment-response"
			if code, out := ownerCallMatch(owner, "POST", otherPath, NewID(), task["etag"].(string), body); code != 404 {
				t.Fatal("wrong owned agent", code, out)
			}
			break
		}
	}
	code, out := ownerCallMatch(owner, "POST", path, "owner-assignment-response", task["etag"].(string), body)
	if code != 200 || out["state"] != "accepted" {
		t.Fatal("assignment acceptance", code, out)
	}
	if code, out := ownerCallMatch(owner, "POST", path, "owner-assignment-response", task["etag"].(string), body); code != 200 {
		t.Fatal("replay", code, out)
	}
	var state string
	var votes int
	if err := db.QueryRow(context.Background(), "SELECT state,(SELECT count(*) FROM undergrid.approvals WHERE plan_id=$2) FROM undergrid.tasks WHERE id=$1", task["id"], plan["id"]).Scan(&state, &votes); err != nil || state != "accepted" || votes != 0 {
		t.Fatal("separate assignment effects", state, votes, err)
	}
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.memberships SET state='withdrawn' WHERE swarm_id=$1 AND agent_id=$2", swarm, agent); err != nil {
		t.Fatal(err)
	}
	if code, out := ownerCallMatch(owner, "POST", path, "owner-assignment-response", task["etag"].(string), body); code < 400 {
		t.Fatal("withdrawn replay", code, out)
	}
}

type ownerAssignmentFixture struct {
	owner, foreign testClient
	db             *pgx.Conn
	agent, swarm   string
	plan           map[string]any
}

func newOwnerAssignmentFixture(t *testing.T) ownerAssignmentFixture {
	t.Helper()
	s, db := ownerSwarmCreationFixture(t)
	enableTestTerms(t, s)
	owner, foreign := planningClient(t, s), planningClient(t, s)
	_, who := owner.call("GET", "/v1/me", "", nil)
	agent, principal := who["agent_id"].(string), who["principal_id"].(string)
	code, room := owner.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Owner reviewed work", "visibility": "private"})
	if code != 201 {
		t.Fatal(code, room)
	}
	swarm := room["id"].(string)
	spec := map[string]any{"objective": "Owner reviewed work", "required_principals": []string{principal}, "acceptance_principals": []string{principal}, "publication_principals": []string{principal}, "rights": map[string]any{"template_id": "test-contribution-v1", "license_id": "CC0-1.0", "publication": "exact_manifest_approval"}, "tasks": []any{map[string]any{"key": "catalog", "assignee_agent_id": agent, "depends_on": []string{}, "output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 86400}}, "respond_by": time.Now().Add(time.Hour)}
	code, plan := owner.versioned("POST", "/v1/swarms/"+swarm+"/plans", NewID(), room["etag"].(string), spec)
	if code != 201 {
		t.Fatal(code, plan)
	}

	return ownerAssignmentFixture{owner, foreign, db, agent, swarm, plan}
}

func TestOwnerAssignmentDeclineExpiryAndAtomicFanoutFailure(t *testing.T) {
	for _, scenario := range []string{"decline", "expired", "queue_failure"} {
		t.Run(scenario, func(t *testing.T) {
			f := newOwnerAssignmentFixture(t)
			task := f.plan["tasks"].([]any)[0].(map[string]any)
			path := "/v1/owner/agents/" + f.agent + "/tasks/" + task["id"].(string) + "/assignment-response"
			decision := "accept"
			want := 200
			expectedState := "accepted"
			if scenario == "decline" {
				decision = "decline"
				expectedState = "open"
			}
			if scenario == "expired" {
				want = 409
				expectedState = "offered"
				if _, err := f.db.Exec(context.Background(), "UPDATE undergrid.tasks SET response_deadline=now()-interval '1 second' WHERE id=$1", task["id"]); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "queue_failure" {
				want = 500
				expectedState = "offered"
				name := "owner_assignment_fault_" + strings.ReplaceAll(NewID(), "-", "")
				quoted := pgx.Identifier{"undergrid", name}.Sanitize()
				sql := "CREATE FUNCTION " + quoted + "() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='event_fanout_v1' AND EXISTS(SELECT 1 FROM undergrid.events e WHERE e.id::text=NEW.args->>'event_id' AND e.aggregate_id::text='" + task["id"].(string) + "' AND e.type='task.assignment_accepted') THEN RAISE EXCEPTION 'fixture assignment fanout failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER " + pgx.Identifier{name}.Sanitize() + " BEFORE INSERT ON undergrid_queue.river_job FOR EACH ROW EXECUTE FUNCTION " + quoted + "()"
				if _, err := f.db.Exec(context.Background(), sql); err != nil {
					t.Fatal(err)
				}
				defer f.db.Exec(context.Background(), "DROP TRIGGER "+pgx.Identifier{name}.Sanitize()+" ON undergrid_queue.river_job; DROP FUNCTION "+quoted+"()")
			}
			var beforeSequence int64
			if err := f.db.QueryRow(context.Background(), "SELECT sequence FROM undergrid.swarms WHERE id=$1", f.swarm).Scan(&beforeSequence); err != nil {
				t.Fatal(err)
			}
			body := map[string]any{"decision": decision, "plan_hash": f.plan["content_hash"], "acknowledge_responsibility_and_terms": true}
			code, out := ownerCallMatch(f.owner, "POST", path, "assignment-"+scenario, task["etag"].(string), body)
			if code != want {
				t.Fatal(code, out)
			}
			var state string
			var version, sequence int64
			var events, commands, votes int
			err := f.db.QueryRow(context.Background(), `SELECT t.state,t.version,s.sequence,(SELECT count(*) FROM undergrid.events WHERE aggregate_id=t.id AND type LIKE 'task.assignment_%'),(SELECT count(*) FROM undergrid.idempotency WHERE actor_id=$2 AND key=$3),(SELECT count(*) FROM undergrid.approvals WHERE plan_id=t.plan_id) FROM undergrid.tasks t JOIN undergrid.plan_versions p ON p.id=t.plan_id JOIN undergrid.swarms s ON s.id=p.swarm_id WHERE t.id=$1`, task["id"], f.agent, "assignment-"+scenario).Scan(&state, &version, &sequence, &events, &commands, &votes)
			expectedEffects := 0
			expectedVersion := int64(1)
			if want == 200 {
				expectedEffects = 1
				expectedVersion = 2
			}
			if err != nil || state != expectedState || version != expectedVersion || events != expectedEffects || commands != expectedEffects || votes != 0 || sequence != beforeSequence {
				t.Fatal("atomic assignment effects", state, version, sequence, beforeSequence, events, commands, votes, err)
			}
		})
	}
}

func TestOwnerAssignmentAuthorityRecheckedAfterCanonicalWait(t *testing.T) {
	for _, change := range []string{"agent_revoked", "owner_suspended", "membership_withdrawn"} {
		t.Run(change, func(t *testing.T) {
			s, db := ownerSwarmCreationFixture(t)
			enableTestTerms(t, s)
			owner := planningClient(t, s)
			_, identity := owner.call("GET", "/v1/me", "", nil)
			code, room := owner.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Current owner authority", "visibility": "private", "recruitment_roles": []any{}})
			if code != 201 {
				t.Fatal(code, room)
			}
			principal, agent := identity["principal_id"].(string), identity["agent_id"].(string)
			spec := map[string]any{"objective": "Current owner authority", "required_principals": []string{principal}, "acceptance_principals": []string{principal}, "publication_principals": []string{principal}, "rights": map[string]any{"template_id": "test-contribution-v1", "license_id": "CC0-1.0", "publication": "exact_manifest_approval"}, "tasks": []any{map[string]any{"key": "catalog", "assignee_agent_id": agent, "depends_on": []string{}, "output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 86400}}, "respond_by": time.Now().Add(time.Hour)}
			code, plan := owner.versioned("POST", "/v1/swarms/"+room["id"].(string)+"/plans", NewID(), room["etag"].(string), spec)
			if code != 201 {
				t.Fatal(code, plan)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			guardian, err := pgx.Connect(ctx, db.Config().ConnString())
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			hold, err := guardian.Begin(ctx)
			if err != nil {
				guardian.Close(ctx)
				cancel()
				t.Fatal(err)
			}
			var guardianPID int
			if err = hold.QueryRow(ctx, "SELECT pg_backend_pid() FROM undergrid.principals WHERE id=$1 FOR UPDATE", principal).Scan(&guardianPID); err != nil {
				hold.Rollback(ctx)
				guardian.Close(ctx)
				cancel()
				t.Fatal(err)
			}
			done := make(chan struct{})
			started := false
			t.Cleanup(func() {
				cancel()
				cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
				defer stop()
				hold.Rollback(cleanup)
				guardian.Close(cleanup)
				if started {
					select {
					case <-done:
					case <-cleanup.Done():
						t.Error("authority-wait request did not cancel")
					}
				}
			})
			type result struct {
				code int
				body map[string]any
				err  error
			}
			response := make(chan result, 1)
			key := NewID()
			started = true
			go func() {
				defer close(done)
				path := "/v1/owner/agents/" + agent + "/tasks/" + plan["tasks"].([]any)[0].(map[string]any)["id"].(string) + "/assignment-response"
				r := httptest.NewRequest("POST", path, bytes.NewReader(canonical(map[string]any{"decision": "accept", "plan_hash": plan["content_hash"], "acknowledge_responsibility_and_terms": true}))).WithContext(ctx)
				r.Header.Set("X-Test-Owner", owner.owner)
				r.Header.Set("Idempotency-Key", key)
				r.Header.Set("If-Match", plan["tasks"].([]any)[0].(map[string]any)["etag"].(string))
				w := httptest.NewRecorder()
				s.ServeHTTP(w, r)
				var out map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &out)
				response <- result{w.Code, out, err}
			}()
			deadline := time.Now().Add(3 * time.Second)
			for {
				var blocked bool
				err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND $1::integer=ANY(pg_blocking_pids(pid)) AND query='SELECT lock_plan_authorities($1)')`, guardianPID).Scan(&blocked)
				if err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("request never waited on canonical principal locks")
				}
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(20 * time.Millisecond):
				}
			}
			if change == "agent_revoked" {
				_, err = db.Exec(ctx, "UPDATE undergrid.agents SET status='revoked' WHERE id=$1", agent)
			} else if change == "membership_withdrawn" {
				_, err = db.Exec(ctx, "UPDATE undergrid.memberships SET state='withdrawn' WHERE swarm_id=$1 AND agent_id=$2", room["id"], agent)
			} else {
				_, err = hold.Exec(ctx, "UPDATE undergrid.principals SET status='suspended' WHERE id=$1", principal)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = hold.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case out := <-response:
				want := 403
				if change == "membership_withdrawn" {
					want = 404
				}
				if out.err != nil || out.code != want {
					t.Fatal("authority lost during wait", out)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			var votes, commands int
			if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM undergrid.approvals WHERE plan_id=$1),(SELECT count(*) FROM undergrid.idempotency WHERE actor_id=$2 AND key=$3)`, plan["id"], agent, key).Scan(&votes, &commands); err != nil || votes != 0 || commands != 0 {
				t.Fatal("lost authority persisted effects", votes, commands, err)
			}
		})
	}
}

func TestOwnerAssignmentReviewFiltersCurrentAgent(t *testing.T) {
	f := newOwnerAssignmentFixture(t)
	_, peer := f.foreign.call("GET", "/v1/me", "", nil)
	roomPath := "/v1/swarms/" + f.swarm
	code, invitation := f.owner.call("POST", roomPath+"/invitations", NewID(), map[string]any{"agent_id": peer["agent_id"], "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, invitation)
	}
	code, out := f.foreign.versioned("POST", roomPath+"/membership-response", NewID(), invitation["etag"].(string), map[string]any{"invitation_id": invitation["id"], "decision": "accept"})
	if code != 200 {
		t.Fatal(code, out)
	}
	_, identity := f.owner.call("GET", "/v1/me", "", nil)
	_, context := f.owner.call("GET", roomPath+"/context", "", nil)
	specification := f.plan["specification"].(map[string]any)
	specification["parent_version"] = f.plan["version"]
	specification["required_principals"] = []any{identity["principal_id"], peer["principal_id"]}
	tasks := []any{}
	for _, assigned := range []string{f.agent, peer["agent_id"].(string)} {
		tasks = append(tasks, map[string]any{"key": "catalog-" + assigned, "assignee_agent_id": assigned, "depends_on": []string{}, "output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 86400})
	}
	specification["tasks"] = tasks
	code, plan := f.owner.versioned("POST", roomPath+"/plans", NewID(), context["swarm"].(map[string]any)["etag"].(string), specification)
	if code != 201 {
		t.Fatal(code, plan)
	}
	for _, actor := range []struct {
		client testClient
		agent  string
	}{{f.owner, f.agent}, {f.foreign, peer["agent_id"].(string)}} {
		code, page := ownerCallMatch(actor.client, "GET", "/v1/owner/agents/"+actor.agent+"/plans/"+plan["id"].(string)+"/assignments", "", "", nil)
		if code != 200 || len(page["items"].([]any)) != 1 {
			t.Fatal("own assignment page", code, page)
		}
		task := page["items"].([]any)[0].(map[string]any)
		if task["assigned_agent_id"] != actor.agent {
			t.Fatal("peer assignment leaked", task)
		}
	}
}
