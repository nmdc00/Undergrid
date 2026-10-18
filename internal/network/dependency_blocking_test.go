package network

import (
	"context"
	"encoding/json"
	"github.com/riverqueue/river"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type dependencyScenario struct {
	s                             *Service
	db                            *pgx.Conn
	owner, departing, replacement testClient
	room, plan                    map[string]any
	tasks                         map[string]map[string]any
	agent, principal              map[string]string
	workerDSN                     string
}

func newDependencyScenario(t *testing.T) dependencyScenario {
	t.Helper()
	s, db := fixture(t)
	enableTestTerms(t, s)
	_, err := db.Exec(context.Background(), `DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='undergrid_dependency_api_runner') THEN CREATE ROLE undergrid_dependency_api_runner LOGIN NOSUPERUSER NOBYPASSRLS; END IF;
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='undergrid_dependency_worker_runner') THEN CREATE ROLE undergrid_dependency_worker_runner LOGIN NOSUPERUSER NOBYPASSRLS; END IF;
 END $$; GRANT undergrid_app TO undergrid_dependency_api_runner; GRANT undergrid_worker TO undergrid_dependency_worker_runner;`)
	if err != nil {
		t.Fatal(err)
	}
	s.dsn = networkFixtureRoleDSN(t, db, "undergrid_dependency_api_runner")
	f := dependencyScenario{s: s, db: db, tasks: map[string]map[string]any{}, agent: map[string]string{}, principal: map[string]string{}, workerDSN: networkFixtureRoleDSN(t, db, "undergrid_dependency_worker_runner")}
	f.owner = artifactClient(t, s)
	f.departing = artifactClient(t, s)
	f.replacement = artifactClient(t, s)
	for name, c := range map[string]testClient{"owner": f.owner, "departing": f.departing, "replacement": f.replacement} {
		code, me := c.call("GET", "/v1/me", "", nil)
		if code != 200 {
			t.Fatal(code, me)
		}
		f.agent[name] = me["agent_id"].(string)
		f.principal[name] = me["principal_id"].(string)
	}
	code, room := f.owner.call("POST", "/v1/swarms", "dependency-room", map[string]any{"objective": "Transitive dependency withdrawal", "visibility": "private", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	f.room = room
	f.admit(t, f.departing, "departing")
	specs := []any{}
	for _, task := range []struct {
		key, assignee string
		dependencies  []string
	}{{"root", "departing", []string{}}, {"root_two", "departing", []string{}}, {"middle", "owner", []string{"root", "root_two"}}, {"leaf", "owner", []string{"middle"}}, {"branch", "owner", []string{"root"}}, {"unrelated", "owner", []string{}}} {
		specs = append(specs, map[string]any{"key": task.key, "assignee_agent_id": f.agent[task.assignee], "depends_on": task.dependencies, "output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 86400})
	}
	_, snapshot := f.owner.call("GET", f.roomPath()+"/context", "", nil)
	principals := []string{f.principal["owner"], f.principal["departing"]}
	spec := map[string]any{"parent_version": nil, "objective": "Transitive dependency withdrawal", "required_principals": principals, "acceptance_principals": []string{f.principal["owner"]}, "publication_principals": principals, "rights": map[string]any{"template_id": "test-contribution-v1", "license_id": "CC0-1.0", "publication": "exact_manifest_approval"}, "tasks": specs, "respond_by": time.Now().Add(time.Hour)}
	code, plan := f.owner.versioned("POST", f.roomPath()+"/plans", "dependency-plan", snapshot["swarm"].(map[string]any)["etag"].(string), spec)
	if code != 201 {
		t.Fatal(code, plan)
	}
	f.plan = plan
	for _, c := range []testClient{f.owner, f.departing} {
		code, out := c.call("POST", "/v1/plans/"+plan["id"].(string)+"/approvals", "approve", map[string]any{"decision": "approve", "content_hash": plan["content_hash"]})
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	for _, summary := range plan["tasks"].([]any) {
		task := summary.(map[string]any)
		path := "/v1/tasks/" + task["id"].(string)
		_, full := f.owner.call("GET", path, "", nil)
		key := full["specification"].(map[string]any)["key"].(string)
		c := f.owner
		if full["assigned_agent_id"] == f.agent["departing"] {
			c = f.departing
		}
		code, out := c.versioned("POST", path+"/assignment-response", "accept", task["etag"].(string), map[string]any{"decision": "accept", "plan_hash": plan["content_hash"]})
		if code != 200 {
			t.Fatal(code, out)
		}
		f.tasks[key] = full
	}
	for key, task := range f.tasks {
		_, f.tasks[key] = f.owner.call("GET", "/v1/tasks/"+task["id"].(string), "", nil)
	}
	return f
}
func (f dependencyScenario) roomPath() string { return "/v1/swarms/" + f.room["id"].(string) }
func (f dependencyScenario) admit(t *testing.T, c testClient, name string) {
	t.Helper()
	code, invite := f.owner.call("POST", f.roomPath()+"/invitations", NewID(), map[string]any{"agent_id": f.agent[name], "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, invite)
	}
	code, out := c.versioned("POST", f.roomPath()+"/membership-response", NewID(), invite["etag"].(string), map[string]any{"invitation_id": invite["id"], "decision": "accept"})
	if code != 200 {
		t.Fatal(code, out)
	}
}

func TestWithdrawalDurablyBlocksTransitiveDependencies(t *testing.T) {
	f := newDependencyScenario(t)
	code, out := f.departing.call("POST", f.roomPath()+"/withdraw", "withdraw", map[string]any{})
	if code != 200 {
		t.Fatal(code, out)
	}
	for _, key := range []string{"middle", "leaf", "branch"} {
		code, task := f.owner.call("GET", "/v1/tasks/"+f.tasks[key]["id"].(string), "", nil)
		if code != 200 || task["dependency_blocked"] != true {
			t.Fatalf("withdrawal did not durably mark %s blocked: %d %v", key, code, task)
		}
		if task["state"] != f.tasks[key]["state"] || task["assigned_agent_id"] != f.tasks[key]["assigned_agent_id"] || task["claim_epoch"] != f.tasks[key]["claim_epoch"] {
			t.Fatalf("blocking replaced assignment: %v", task)
		}
	}
}

func (f dependencyScenario) task(t *testing.T, key string) map[string]any {
	t.Helper()
	code, out := f.owner.call("GET", "/v1/tasks/"+f.tasks[key]["id"].(string), "", nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	return out
}
func TestWithdrawalDependencyEvidenceRecoveryAndReplacement(t *testing.T) {
	f := newDependencyScenario(t)
	ctx := context.Background()
	beforePlan := f.plan["canonical_json"]
	code, out := f.departing.call("POST", f.roomPath()+"/withdraw", "withdraw", map[string]any{})
	if code != 200 {
		t.Fatal(code, out)
	}
	for range 2 {
		code, out = f.departing.call("POST", f.roomPath()+"/withdraw", "withdraw", map[string]any{})
		if code != 200 {
			t.Fatal("withdraw retry", code, out)
		}
	}
	for _, key := range []string{"root", "root_two"} {
		task := f.task(t, key)
		if task["state"] != "withdrawn" || task["claim_epoch"] != float64(2) {
			t.Fatalf("root not invalidated: %v", task)
		}
	}
	for _, key := range []string{"middle", "leaf", "branch"} {
		task := f.task(t, key)
		if task["dependency_blocked"] != true || task["version"] != f.tasks[key]["version"].(float64)+1 {
			t.Fatalf("not once blocked: %v", task)
		}
		code, out = f.owner.versioned("POST", "/v1/tasks/"+task["id"].(string)+"/progress", NewID(), task["etag"].(string), map[string]any{"state": "in_progress", "claim_epoch": task["claim_epoch"]})
		if code != 409 || out["error"].(map[string]any)["code"] != "task_dependency_blocked" {
			t.Fatalf("blocked progress %d %v", code, out)
		}
		var facts []byte
		var eventID string
		var count int
		if err := f.db.QueryRow(ctx, `SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='task.dependency_blocked'`, task["id"]).Scan(&count); err != nil || count != 1 {
			t.Fatalf("event count %d %v", count, err)
		}
		if err := f.db.QueryRow(ctx, `SELECT id,facts FROM undergrid.events WHERE aggregate_id=$1 AND type='task.dependency_blocked'`, task["id"]).Scan(&eventID, &facts); err != nil {
			t.Fatal(err)
		}
		var event struct {
			Roots []struct {
				ID    string `json:"task_id"`
				Epoch int64  `json:"claim_epoch"`
			} `json:"withdrawn_roots"`
		}
		if err := json.Unmarshal(facts, &event); err != nil {
			t.Fatal(err)
		}
		roots := map[string]bool{}
		for _, root := range event.Roots {
			if root.Epoch != 2 {
				t.Fatalf("wrong root epoch: %s", facts)
			}
			roots[root.ID] = true
		}
		expectedRoots := 2
		if key == "branch" {
			expectedRoots = 1
		}
		if len(roots) != expectedRoots || !roots[f.tasks["root"]["id"].(string)] || (key != "branch" && !roots[f.tasks["root_two"]["id"].(string)]) {
			t.Fatalf("lost causal root: %s", facts)
		}
		if err := f.db.QueryRow(ctx, `SELECT count(*) FROM undergrid_queue.river_job WHERE kind='event_fanout_v1' AND args->>'event_id'=$1`, eventID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("durable fanout %d %v", count, err)
		}
		w := fanoutWorker{dsn: f.workerDSN}
		for range 2 {
			if err := w.Work(ctx, &river.Job[fanoutArgs]{Args: fanoutArgs{EventID: eventID}}); err != nil {
				t.Fatal(err)
			}
		}
		if err := f.db.QueryRow(ctx, `SELECT count(*) FROM undergrid.inbox WHERE event_id=$1`, eventID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("private deduped fanout %d %v", count, err)
		}
	}
	if task := f.task(t, "unrelated"); task["dependency_blocked"] != false || task["version"] != f.tasks["unrelated"]["version"] {
		t.Fatalf("unrelated changed: %v", task)
	}
	for _, c := range []testClient{f.departing, f.replacement} {
		code, out = c.call("GET", "/v1/tasks/"+f.tasks["leaf"]["id"].(string), "", nil)
		if code != 404 {
			t.Fatalf("private dependency leaked %d %v", code, out)
		}
	}
	// Blocking is orthogonal to accepted work: no capacity is silently released.
	conn, err := pgx.Connect(ctx, f.s.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `SET LOCAL ROLE undergrid_app; SET LOCAL search_path=undergrid,pg_catalog`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `SELECT set_config('undergrid.principal_id',$1,true),set_config('undergrid.agent_id',$2,true)`, f.principal["owner"], f.agent["owner"])
	if err != nil {
		t.Fatal(err)
	}
	var load int
	if err = tx.QueryRow(ctx, "SELECT principal_task_load()").Scan(&load); err != nil || load != 4 {
		t.Fatalf("blocked workload released: %d %v", load, err)
	}
	if _, err = tx.Exec(ctx, `UPDATE tasks SET dependency_blocked=false WHERE id=$1`, f.tasks["leaf"]["id"]); err == nil {
		t.Fatal("application role cleared historical dependency blocking")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	restarted := New(f.workerDSN, "development", nil)
	if err = restarted.SweepCoordinationDeadlines(ctx, time.Now().Add(72*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"middle", "leaf", "branch"} {
		task := f.task(t, key)
		if task["state"] != "accepted" || task["progress_deadline"] != f.tasks[key]["progress_deadline"] || task["version"] != f.tasks[key]["version"].(float64)+1 {
			t.Fatalf("blocked task blamed or extended: %v", task)
		}
	}
	// Fresh process reads the same durable blocking contract.
	reader := f.owner
	reader.s = New(f.s.dsn, "development", nil)
	code, out = reader.call("GET", "/v1/tasks/"+f.tasks["leaf"]["id"].(string), "", nil)
	if code != 200 || out["dependency_blocked"] != true {
		t.Fatal(code, out)
	}
	code, contextOut := f.owner.call("GET", "/v1/agent-context", "", nil)
	if code != 200 {
		t.Fatal(code, contextOut)
	}
	blocked := 0
	for _, raw := range contextOut["assignments"].([]any) {
		if raw.(map[string]any)["dependency_blocked"] == true {
			blocked++
		}
	}
	if blocked != 3 {
		t.Fatalf("recovery lost blocks: %v", contextOut)
	}
	code, old := f.owner.call("GET", "/v1/plans/"+f.plan["id"].(string), "", nil)
	if code != 200 || old["canonical_json"] != beforePlan {
		t.Fatalf("withdrawal edited sealed plan: %d %v", code, old)
	}
	// The unadmitted replacement cannot see history or receive an assignment.
	f.admit(t, f.replacement, "replacement")
	_, snapshot := f.owner.call("GET", f.roomPath()+"/context", "", nil)
	spec := map[string]any{"parent_version": 1, "objective": "Explicit replacement after withdrawal", "required_principals": []string{f.principal["owner"], f.principal["replacement"]}, "acceptance_principals": []string{f.principal["owner"]}, "publication_principals": []string{f.principal["owner"], f.principal["replacement"]}, "rights": map[string]any{"template_id": "test-contribution-v1", "license_id": "CC0-1.0", "publication": "exact_manifest_approval"}, "tasks": []any{map[string]any{"key": "root", "assignee_agent_id": f.agent["replacement"], "depends_on": []string{}, "output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 86400}}, "respond_by": time.Now().Add(time.Hour)}
	code, next := f.owner.versioned("POST", f.roomPath()+"/plans", "replacement-plan", snapshot["swarm"].(map[string]any)["etag"].(string), spec)
	if code != 201 {
		t.Fatal(code, next)
	}
	for _, c := range []testClient{f.owner, f.replacement} {
		code, out = c.call("POST", "/v1/plans/"+next["id"].(string)+"/approvals", "replacement-approve", map[string]any{"decision": "approve", "content_hash": next["content_hash"]})
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	_, notAccepted := f.owner.call("GET", "/v1/plans/"+next["id"].(string), "", nil)
	if notAccepted["state"] != "proposed" {
		t.Fatalf("replacement silently accepted: %v", notAccepted)
	}
	nextTask := next["tasks"].([]any)[0].(map[string]any)
	nextPath := "/v1/tasks/" + nextTask["id"].(string)
	code, out = f.replacement.versioned("POST", nextPath+"/assignment-response", "accept-replacement", nextTask["etag"].(string), map[string]any{"decision": "accept", "plan_hash": next["content_hash"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	_, newPlan := f.owner.call("GET", "/v1/plans/"+next["id"].(string), "", nil)
	if newPlan["state"] != "accepted" {
		t.Fatal(newPlan)
	}
	_, newTask := f.replacement.call("GET", nextPath, "", nil)
	code, out = f.replacement.versioned("POST", nextPath+"/progress", "replacement-start", newTask["etag"].(string), map[string]any{"state": "in_progress", "claim_epoch": newTask["claim_epoch"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	oldLeaf := f.task(t, "leaf")
	if oldLeaf["state"] != "cancelled" || oldLeaf["claim_epoch"] != float64(2) || oldLeaf["dependency_blocked"] != true {
		t.Fatalf("replacement rewrote old claim/block: %v", oldLeaf)
	}
	code, out = f.departing.versioned("POST", nextPath+"/progress", "old-agent", newTask["etag"].(string), map[string]any{"state": "in_progress", "claim_epoch": 1})
	if code != 404 {
		t.Fatalf("old agent reached replacement: %d %v", code, out)
	}
	var approvals int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM undergrid.approvals WHERE plan_id=$1 AND purpose='rights' AND content_hash=$2`, f.plan["id"], f.plan["content_hash"]).Scan(&approvals); err != nil || approvals != 2 {
		t.Fatalf("old rights destroyed: %d %v", approvals, err)
	}
	if strings.Contains(newPlan["canonical_json"].(string), f.principal["departing"]) {
		t.Fatal("departing principal silently retained as new obligor")
	}
}

func TestWithdrawalSerializesWithDownstreamProgress(t *testing.T) {
	f := newDependencyScenario(t)
	ctx := context.Background()
	guard, err := pgx.Connect(ctx, f.db.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close(ctx)
	tx, err := guard.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM undergrid.swarms WHERE id=$1 FOR UPDATE`, f.room["id"]); err != nil {
		t.Fatal(err)
	}
	type result struct {
		code int
		out  map[string]any
	}
	withdrawal := make(chan result, 1)
	progress := make(chan result, 1)
	go func() {
		code, out := f.departing.call("POST", f.roomPath()+"/withdraw", "serialized-withdraw", map[string]any{})
		withdrawal <- result{code, out}
	}()
	wait := func(fragment string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			var count int
			if err := f.db.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE usename='undergrid_dependency_api_runner' AND cardinality(pg_blocking_pids(pid))>0 AND query LIKE $1`, "%"+fragment+"%").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count > 0 {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("actual request never waited on %s", fragment)
	}
	wait("SELECT swarm_member(id)")
	task := f.tasks["leaf"]
	go func() {
		code, out := f.owner.versioned("POST", "/v1/tasks/"+task["id"].(string)+"/progress", "serialized-progress", task["etag"].(string), map[string]any{"state": "in_progress", "claim_epoch": task["claim_epoch"]})
		progress <- result{code, out}
	}()
	wait("lock_plan_authorities")
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if result := <-withdrawal; result.code != 200 {
		t.Fatal(result.code, result.out)
	}
	if result := <-progress; result.code != 409 {
		t.Fatalf("withdrawal did not serialize downstream mutation: %d %v", result.code, result.out)
	}
	task = f.task(t, "leaf")
	if task["dependency_blocked"] != true || task["state"] != "accepted" {
		t.Fatal(task)
	}
	var count int
	if err = f.db.QueryRow(ctx, `SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='task.progress_recorded'`, task["id"]).Scan(&count); err != nil || count != 0 {
		t.Fatalf("concurrent denied progress persisted: %d %v", count, err)
	}
}

func TestWithdrawalDependencyBlockingPreservesVerifiedEvidence(t *testing.T) {
	x := newArtifactScenario(t)
	ctx := context.Background()
	_, builder := x.actors[2].call("GET", "/v1/me", "", nil)
	_, owner := x.actors[0].call("GET", "/v1/me", "", nil)
	_, snapshot := x.actors[0].call("GET", "/v1/swarms/"+x.room["id"].(string)+"/context", "", nil)
	spec := map[string]any{"parent_version": 1, "objective": "Preserve actual verified contribution after withdrawal", "required_principals": x.principals[:3], "acceptance_principals": x.principals[:2], "publication_principals": x.principals[:3], "rights": map[string]any{"template_id": "test-contribution-v1", "license_id": "CC0-1.0", "publication": "exact_manifest_approval"}, "tasks": []any{
		map[string]any{"key": "catalog", "assignee_agent_id": builder["agent_id"], "depends_on": []string{}, "output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 86400},
		map[string]any{"key": "assemble", "assignee_agent_id": owner["agent_id"], "depends_on": []string{"catalog"}, "output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 86400}}, "respond_by": time.Now().Add(time.Hour)}
	code, plan := x.actors[0].versioned("POST", "/v1/swarms/"+x.room["id"].(string)+"/plans", "evidence-plan", snapshot["swarm"].(map[string]any)["etag"].(string), spec)
	if code != 201 {
		t.Fatal(code, plan)
	}
	for _, c := range x.actors[:3] {
		code, out := c.call("POST", "/v1/plans/"+plan["id"].(string)+"/approvals", "approve-evidence-plan", map[string]any{"decision": "approve", "content_hash": plan["content_hash"]})
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	var downstream map[string]any
	for _, raw := range plan["tasks"].([]any) {
		summary := raw.(map[string]any)
		path := "/v1/tasks/" + summary["id"].(string)
		_, task := x.actors[0].call("GET", path, "", nil)
		c := x.actors[0]
		if task["assigned_agent_id"] == builder["agent_id"] {
			c = x.actors[2]
			x.task = task
		} else {
			downstream = task
		}
		code, out := c.versioned("POST", path+"/assignment-response", "accept-evidence-plan", summary["etag"].(string), map[string]any{"decision": "accept", "plan_hash": plan["content_hash"]})
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	x.plan = plan
	_, x.task = x.actors[2].call("GET", "/v1/tasks/"+x.task["id"].(string), "", nil)
	data := []byte(`{"sources":[{"source_id":"fixture","source_url":"https://example.org/reference"}]}`)
	upload := x.upload(t, data, "root-evidence")
	id := upload["id"].(string)
	code, out := x.actors[2].call("POST", "/v1/artifacts/"+id+"/finalize", "seal-root-evidence", x.finalBody(data))
	if code != 200 {
		t.Fatal(code, out)
	}
	x.startWorker(t)
	evaluation := x.waitEvaluation(t, id)
	if evaluation["result"] != "passed" {
		t.Fatalf("fixed verifier did not pass: %v", evaluation)
	}
	var artifactBefore, evaluationBefore []byte
	if err := x.db.QueryRow(ctx, `SELECT row_to_json(a)::text FROM undergrid.artifacts a WHERE id=$1`, id).Scan(&artifactBefore); err != nil {
		t.Fatal(err)
	}
	if err := x.db.QueryRow(ctx, `SELECT row_to_json(e)::text FROM undergrid.evaluations e WHERE id=$1`, evaluation["id"]).Scan(&evaluationBefore); err != nil {
		t.Fatal(err)
	}
	code, out = x.actors[2].call("POST", "/v1/swarms/"+x.room["id"].(string)+"/withdraw", "withdraw-verified-root", map[string]any{})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, blocked := x.actors[0].call("GET", "/v1/tasks/"+downstream["id"].(string), "", nil)
	if code != 200 || blocked["dependency_blocked"] != true {
		t.Fatal(code, blocked)
	}
	for _, purpose := range []string{"output", "progress"} {
		body := map[string]any{"swarm_id": x.room["id"], "plan_id": plan["id"], "task_id": downstream["id"], "claim_epoch": downstream["claim_epoch"], "media_type": "application/json", "size_bytes": len(data), "purpose": purpose}
		if purpose == "progress" {
			body["audience_principal_ids"] = x.principals[:2]
			body["reviewer_principal_id"] = x.principals[1]
		}
		code, out = x.actors[0].call("POST", "/v1/artifacts/uploads", NewID(), body)
		if code != 409 || out["error"].(map[string]any)["code"] != "task_dependency_blocked" {
			t.Fatalf("blocked %s upload %d %v", purpose, code, out)
		}
	}
	var artifactAfter, evaluationAfter []byte
	if err := x.db.QueryRow(ctx, `SELECT row_to_json(a)::text FROM undergrid.artifacts a WHERE id=$1`, id).Scan(&artifactAfter); err != nil {
		t.Fatal(err)
	}
	if err := x.db.QueryRow(ctx, `SELECT row_to_json(e)::text FROM undergrid.evaluations e WHERE id=$1`, evaluation["id"]).Scan(&evaluationAfter); err != nil {
		t.Fatal(err)
	}
	var pointer string
	var epoch int
	if err := x.db.QueryRow(ctx, `SELECT submitted_artifact_id::text,claim_epoch FROM undergrid.tasks WHERE id=$1`, x.task["id"]).Scan(&pointer, &epoch); err != nil || pointer != id || epoch != 2 {
		t.Fatalf("withdrawal changed canonical artifact pointer or retained old claim: pointer=%s epoch=%d err=%v", pointer, epoch, err)
	}
	if string(artifactBefore) != string(artifactAfter) || string(evaluationBefore) != string(evaluationAfter) {
		t.Fatal("withdrawal mutated sealed artifact or acknowledged evaluation")
	}
	code, out = x.actors[0].call("GET", "/v1/artifacts/"+id+"/evaluations", "", nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = x.actors[2].call("POST", "/v1/artifacts/"+id+"/finalize", "seal-root-evidence", x.finalBody(data))
	if code != 404 {
		t.Fatalf("withdrawn contributor replay accessed canonical artifact: %d %v", code, out)
	}
}
