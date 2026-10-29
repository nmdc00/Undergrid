package network

import (
	"bytes"
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/riverqueue/river"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSwarmArchiveTerminalLifecycleAndMemberExit(t *testing.T) {
	f := newSuccessionTestRoom(t)
	code, current := f.old.call("GET", f.path+"/context", "", nil)
	if code != 200 {
		t.Fatal(code, current)
	}
	before := current["swarm"].(map[string]any)["etag"].(string)
	code, archived := f.old.versioned("POST", f.path+"/archive", "archive-stalled-room", before, map[string]any{})
	if code != 200 || archived["state"] != "archived" {
		t.Fatalf("current coordinator could not archive reviewed room: %d %v", code, archived)
	}
	code, replay := f.old.versioned("POST", f.path+"/archive", "archive-stalled-room", before, map[string]any{})
	if code != 200 || string(canonical(replay)) != string(canonical(archived)) {
		t.Fatal("archive replay changed result", code, replay, archived)
	}
	for _, c := range []testClient{f.old, f.successor, f.peer} {
		if code, out := c.call("GET", f.path+"/context", "", nil); code != 200 {
			t.Fatal("archive removed current authorized history", code, out)
		}
	}
	if code, out := f.successor.call("POST", f.path+"/messages", "after-archive", map[string]any{"kind": "text", "body": "must not reactivate archived collaboration"}); code != 409 {
		t.Fatal("archived room accepted new work", code, out)
	}
	for _, c := range []testClient{f.old, f.successor} {
		if code, out := c.call("POST", f.path+"/withdraw", "exit-archived-room", map[string]any{}); code != 200 {
			t.Fatal("archived member could not exit", code, out)
		}
		if code, out := c.call("GET", f.path+"/context", "", nil); code != 404 {
			t.Fatal("withdrawn member retained restricted history", code, out)
		}
	}
	var state string
	var events int
	if err := f.db.QueryRow(context.Background(), "SELECT state FROM undergrid.swarms WHERE id=$1", f.roomID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='swarm.archived'", f.roomID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if state != "archived" || events != 1 {
		t.Fatal("archive exit/replay changed terminal transition", state, events)
	}
}

func TestSwarmArchiveRequiresCurrentCoordinatorScopeAndReviewedVersion(t *testing.T) {
	for _, scenario := range []string{"member_not_coordinator", "foreign", "revoked", "missing_version", "stale_version"} {
		t.Run(scenario, func(t *testing.T) {
			f := newSuccessionTestRoom(t)
			_, current := f.old.call("GET", f.path+"/context", "", nil)
			etag := current["swarm"].(map[string]any)["etag"].(string)
			actor := f.old
			want := 409
			switch scenario {
			case "member_not_coordinator":
				actor = f.peer
				want = 403
			case "foreign":
				actor = successionClient(t, f.s)
				want = 404
			case "revoked":
				if code, out := actor.call("DELETE", "/v1/credentials/current", "", nil); code != 200 {
					t.Fatal(code, out)
				}
				want = 401
			case "missing_version":
				etag = ""
				want = 428
			case "stale_version":
				etag = "\"99999999\""
			}
			code, out := actor.versioned("POST", f.path+"/archive", "denied-archive", etag, map[string]any{})
			if code != want {
				t.Fatal(scenario, code, out)
			}
			var state string
			if err := f.db.QueryRow(context.Background(), "SELECT state FROM undergrid.swarms WHERE id=$1", f.roomID).Scan(&state); err != nil {
				t.Fatal(err)
			}
			if state != "active" {
				t.Fatal("denied archive changed room", state)
			}
		})
	}
	t.Run("missing_scope", func(t *testing.T) {
		s, db := coordinatorFixture(t)
		enableTestTerms(t, s)
		actor := collaborationClient(t, s)
		code, room := actor.call("POST", "/v1/swarms", "narrow-coordinator", map[string]any{"objective": "Do not infer archive authority from office", "visibility": "restricted", "recruitment_roles": []any{}})
		if code != 201 {
			t.Fatal(code, room)
		}
		code, out := actor.versioned("POST", "/v1/swarms/"+room["id"].(string)+"/archive", "missing-archive-scope", room["etag"].(string), map[string]any{})
		if code != 403 {
			t.Fatal("coordinator office granted missing scope", code, out)
		}
		var events int
		if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='swarm.archived'", room["id"]).Scan(&events); err != nil || events != 0 {
			t.Fatal(events, err)
		}
	})
}

func archiveArtifactRoom(t *testing.T, x artifactScenario) {
	t.Helper()
	path := "/v1/swarms/" + x.room["id"].(string)
	code, current := x.actors[0].call("GET", path+"/context", "", nil)
	if code != 200 {
		t.Fatal(code, current)
	}
	if code, out := x.actors[0].versioned("POST", path+"/archive", "archive-artifact-room", current["swarm"].(map[string]any)["etag"].(string), map[string]any{}); code != 200 {
		t.Fatal(code, out)
	}
}

func TestSwarmArchivePreservesAcknowledgedVerificationButBlocksCanonicalWork(t *testing.T) {
	x := newArtifactScenario(t, "swarms:archive", "artifacts:grant")
	data := []byte(`{"sources":[{"source_id":"archive-fixture","source_url":"https://example.org/"}]}`)
	// Issue both generations before sealing moves the assignment to submitted.
	preissued := x.upload(t, data, "prearchive-quarantine")
	upload := x.upload(t, data, "prearchive-upload")
	path := "/v1/artifacts/" + upload["id"].(string)
	code, sealed := x.actors[2].call("POST", path+"/finalize", "prearchive-seal", x.finalBody(data))
	if code != 200 {
		t.Fatal(code, sealed)
	}
	var args artifactEvaluationArgs
	if err := x.db.QueryRow(context.Background(), "SELECT args FROM undergrid_queue.river_job WHERE kind='artifact_evaluation_v1' AND args->>'artifact_id'=$1 ORDER BY id LIMIT 1", upload["id"]).Scan(&args); err != nil {
		t.Fatal(err)
	}
	archiveArtifactRoom(t, x)
	worker := &artifactEvaluationWorker{service: New(x.workerDSN, "development", nil)}
	worker.service.artifacts = x.s.artifacts
	for range 2 {
		if err := worker.Work(context.Background(), &river.Job[artifactEvaluationArgs]{Args: args}); err != nil {
			t.Fatal(err)
		}
	}
	var state string
	var count int
	if err := x.db.QueryRow(context.Background(), "SELECT state FROM undergrid.artifacts WHERE id=$1", upload["id"]).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := x.db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.evaluations WHERE artifact_id=$1", upload["id"]).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if state != "verified" || count != 1 {
		t.Fatal("acknowledged fixed verification lost or duplicated", state, count)
	}
	for _, entry := range []struct {
		client testClient
		path   string
	}{
		{x.actors[2], "/v1/tasks/" + x.task["id"].(string) + "/progress"},
		{x.actors[2], "/v1/tasks/" + x.task["id"].(string) + "/progress-evidence"},
		{x.actors[0], "/v1/tasks/" + x.task["id"].(string) + "/progress-evidence-review"},
		{x.actors[0], path + "/acceptances"},
		{x.actors[0], path + "/publication-approvals"},
		{x.actors[0], path + "/publish"},
		{x.actors[2], path + "/finalize"},
		{x.actors[2], path + "/input-grants"},
	} {
		if code, out := entry.client.call("POST", entry.path, "archived-write-"+NewID(), map[string]any{}); code != 409 {
			t.Fatal("archived canonical write accepted", entry.path, code, out)
		}
	}
	if code, out := x.actors[2].call("POST", "/v1/artifacts/uploads", NewID(), map[string]any{"swarm_id": x.room["id"], "plan_id": x.plan["id"], "task_id": x.task["id"], "claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data)}); code != 409 {
		t.Fatal("archived room issued a new upload", code, out)
	}
	if code, out := x.actors[3].call("GET", path, "", nil); code != 404 {
		t.Fatal("archived private artifact leaked", code, out)
	}
	if code, out := x.actors[2].call("GET", path, "", nil); code != 200 {
		t.Fatal("archive erased current private history", code, out)
	}
	req, _ := http.NewRequest("PUT", preissued["upload_url"].(string), bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	response, err := x.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 409 {
		t.Fatal("preissued upload capability survived archive", response.StatusCode)
	}
	var taskState string
	if err := x.db.QueryRow(context.Background(), "SELECT state FROM undergrid.tasks WHERE id=$1", x.task["id"]).Scan(&taskState); err != nil {
		t.Fatal(err)
	}
	if taskState == "accepted_output" {
		t.Fatal("fixed verification manufactured completion")
	}
}

func TestSwarmArchivePreservesExistingPublicLicensedReuse(t *testing.T) {
	x := newArtifactScenario(t, "swarms:archive")
	data := []byte(`{"sources":[{"source_id":"published-archive-fixture","source_url":"https://example.org/"}]}`)
	upload := x.upload(t, data, "public-upload")
	path := "/v1/artifacts/" + upload["id"].(string)
	code, artifact := x.actors[2].call("POST", path+"/finalize", "seal", x.finalBody(data))
	if code != 200 {
		t.Fatal(code, artifact)
	}
	worker := New(x.workerDSN, "development", nil)
	worker.artifacts = x.s.artifacts
	if err := worker.evaluateArtifact(context.Background(), artifactEvaluationArgs{ArtifactID: upload["id"].(string), Generation: upload["generation"].(string), ManifestHash: artifact["manifest_hash"].(string)}); err != nil {
		t.Fatal(err)
	}
	var evaluation string
	if err := x.db.QueryRow(context.Background(), "SELECT id FROM undergrid.evaluations WHERE artifact_id=$1", upload["id"]).Scan(&evaluation); err != nil {
		t.Fatal(err)
	}
	for _, c := range x.actors[:2] {
		if code, out := c.call("POST", path+"/acceptances", "accept", map[string]any{"evaluation_id": evaluation, "artifact_sha256": digest(data), "plan_hash": x.plan["content_hash"]}); code != 200 {
			t.Fatal(code, out)
		}
	}
	for _, c := range x.actors[:3] {
		if code, out := c.call("POST", path+"/publication-approvals", "consent", map[string]any{"manifest_hash": artifact["manifest_hash"], "license_id": "CC0-1.0"}); code != 200 {
			t.Fatal(code, out)
		}
	}
	if code, out := x.actors[0].call("POST", path+"/publish", "publish", map[string]any{"manifest_hash": artifact["manifest_hash"]}); code != 200 {
		t.Fatal(code, out)
	}
	archiveArtifactRoom(t, x)
	req := httptest.NewRequest("GET", path+"/content", nil)
	req.Header.Set("Authorization", "Bearer "+x.actors[3].token)
	w := httptest.NewRecorder()
	x.s.ServeHTTP(w, req)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), data) {
		t.Fatal("archive revoked existing public bytes", w.Code, w.Body.String())
	}
	if code, out := x.actors[3].call("POST", path+"/reuse-receipts", "reuse-after-archive", map[string]any{"artifact_sha256": digest(data), "use_kind": "schema_validation", "declaration": "Synthetic licensed reuse after archive"}); code != 201 {
		t.Fatal(code, out)
	}
}

func TestSwarmArchiveAndSuccessionSerializeOneGovernanceOutcome(t *testing.T) {
	f := newSuccessionTestRoom(t)
	p := f.propose(t)
	path := "/v1/coordinator-proposals/" + p["id"].(string)
	for _, action := range []string{"accept", "approvals"} {
		body := map[string]any{"content_hash": p["content_hash"]}
		if action == "approvals" {
			body["decision"] = "approve"
		}
		if code, out := f.successor.versioned("POST", path+"/"+action, "prepare-"+action, p["etag"].(string), body); code != 200 {
			t.Fatal(code, out)
		}
	}
	_, current := f.old.call("GET", f.path+"/context", "", nil)
	roomETag := current["swarm"].(map[string]any)["etag"].(string)
	type response struct {
		code int
		body map[string]any
	}
	archived, voted := make(chan response, 1), make(chan response, 1)
	go func() {
		code, out := f.old.versioned("POST", f.path+"/archive", "archive-race", roomETag, map[string]any{})
		archived <- response{code, out}
	}()
	go func() {
		code, out := f.peer.versioned("POST", path+"/approvals", "last-vote-race", p["etag"].(string), map[string]any{"decision": "approve", "content_hash": p["content_hash"]})
		voted <- response{code, out}
	}()
	a, v := <-archived, <-voted
	var state, coordinator string
	if err := f.db.QueryRow(context.Background(), "SELECT state,coordinator_agent_id::text FROM undergrid.swarms WHERE id=$1", f.roomID).Scan(&state, &coordinator); err != nil {
		t.Fatal(err)
	}
	if a.code == 200 {
		if v.code != 409 || state != "archived" || coordinator != f.oldID["agent_id"] {
			t.Fatal("archive failed to exclude succession", a, v, state, coordinator)
		}
	} else if v.code == 200 {
		if a.code != 403 || state != "active" || coordinator != f.successorID["agent_id"] {
			t.Fatal("succession failed to revoke old coordinator archive", a, v, state, coordinator)
		}
	} else {
		t.Fatal("neither concurrent governance action completed", a, v)
	}
}

func TestSwarmArchiveMCPA2AAndCurrentReceiptRecovery(t *testing.T) {
	for _, transport := range []string{"mcp", "a2a"} {
		t.Run(transport, func(t *testing.T) {
			f := newSuccessionTestRoom(t)
			if err := f.s.ConfigureMCP("https://network.fixture.test"); err != nil {
				t.Fatal(err)
			}
			if err := f.s.ConfigureA2ATransport("https://network.fixture.test"); err != nil {
				t.Fatal(err)
			}
			_, current := f.old.call("GET", f.path+"/context", "", nil)
			version := current["swarm"].(map[string]any)["etag"]
			if transport == "mcp" {
				peer := mcpSDKClient(t, f.s, f.old.token)
				result, err := peer.CallTool(context.Background(), &mcp.CallToolParams{Name: "swarm_archive", Arguments: map[string]any{"id": f.roomID, "if_match": version, "idempotency_key": NewID(), "body": map[string]any{}}})
				if err != nil {
					t.Fatal(err)
				}
				if result.IsError {
					t.Fatal(result)
				}
				if out := mcpToolData(t, result); out["state"] != "archived" {
					t.Fatal(out)
				}
				return
			}
			body := a2aCommandBody(NewID(), "swarm_archive", NewID(), map[string]any{})
			commandArgs(body)["id"], commandArgs(body)["if_match"] = f.roomID, version
			code, out, _ := a2aCall(f.old, "POST", "/a2a/message:send", body)
			if code != 200 || a2aLiveTaskResult(out)["state"] != "archived" {
				t.Fatal(code, out)
			}
			receipt := out["task"].(map[string]any)["id"].(string)
			code, recovered, _ := a2aCall(f.old, "GET", "/a2a/tasks/"+receipt, nil)
			if code != 200 {
				t.Fatal(code, recovered)
			}
			resource := recovered["artifacts"].([]any)[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["data"].(map[string]any)["resource"].(map[string]any)
			if resource["swarm"].(map[string]any)["state"] != "archived" {
				t.Fatal(recovered)
			}
			if code, out := f.old.call("POST", f.path+"/withdraw", NewID(), map[string]any{}); code != 200 {
				t.Fatal(code, out)
			}
			code, out, _ = a2aCall(f.old, "GET", "/a2a/tasks/"+receipt, nil)
			if code != 200 || out["artifacts"] != nil || out["history"] != nil {
				t.Fatal("withdrawn receipt must retain metadata without private room resources", code, out)
			}
		})
	}
}

func TestSwarmArchiveSerializesWithRecruitment(t *testing.T) {
	f := newSuccessionTestRoom(t)
	recipient := successionClient(t, f.s)
	_, identity := recipient.call("GET", "/v1/me", "", nil)
	_, current := f.old.call("GET", f.path+"/context", "", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := f.db.Exec(ctx, `DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='undergrid_coordinator_worker_runner') THEN CREATE ROLE undergrid_coordinator_worker_runner LOGIN NOSUPERUSER NOBYPASSRLS;END IF;END $$;GRANT undergrid_worker TO undergrid_coordinator_worker_runner`); err != nil {
		t.Fatal(err)
	}
	worker, err := pgx.Connect(ctx, networkFixtureRoleDSN(t, f.db, "undergrid_coordinator_worker_runner"))
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close(context.Background())
	if _, err = worker.Exec(ctx, "SET ROLE undergrid_worker;SET search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	held, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Rollback(context.Background())
	if _, err = held.Exec(ctx, "SELECT id FROM undergrid.principals WHERE id=$1 FOR UPDATE", f.oldID["principal_id"]); err != nil {
		t.Fatal(err)
	}
	role := RecruitmentRole{Key: "builder", SchemaFamily: "source_catalog_v1"}
	recruited := make(chan error, 1)
	go func() {
		_, err := f.s.recruitOne(ctx, worker, f.roomID, role, identity["agent_id"].(string), identity["principal_id"].(string))
		recruited <- err
	}()
	for {
		var blocked bool
		if _, err = held.Exec(ctx, "SELECT pg_stat_clear_snapshot()"); err != nil {
			t.Fatal(err)
		}
		if err = held.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND wait_event_type='Lock')", worker.PgConn().PID()).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(5 * time.Millisecond):
		}
	}
	archived := make(chan int, 1)
	go func() {
		code, _ := f.old.versioned("POST", f.path+"/archive", NewID(), current["swarm"].(map[string]any)["etag"].(string), map[string]any{})
		archived <- code
	}()
	if err = held.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if code := <-archived; code != 200 {
		t.Fatal("archive lost recruitment race", code)
	}
	if err = <-recruited; err != nil {
		t.Fatal(err)
	}
	var before, after int
	if err = f.db.QueryRow(ctx, "SELECT count(*) FROM undergrid.opportunities WHERE source_swarm_id=$1", f.roomID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.recruitOne(ctx, worker, f.roomID, role, identity["agent_id"].(string), identity["principal_id"].(string)); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(ctx, "SELECT count(*) FROM undergrid.opportunities WHERE source_swarm_id=$1", f.roomID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("archive emitted further recruitment", before, after)
	}
}
