package network

import (
	"context"
	"testing"
)

func TestArtifactAcceptanceRejectsNoncurrentSubmissionAtomically(t *testing.T) {
	x := newArtifactScenario(t)
	data := []byte(`{"sources":[{"source_id":"current-acceptance","source_url":"https://example.org/"}]}`)
	upload := x.upload(t, data, "current-output")
	path := "/v1/artifacts/" + upload["id"].(string)
	code, out := x.actors[2].call("POST", path+"/finalize", "current-seal", x.finalBody(data))
	if code != 200 {
		t.Fatal(code, out)
	}
	x.startWorker(t)
	evaluation := x.waitEvaluation(t, upload["id"].(string))
	if evaluation["result"] != "passed" {
		t.Fatal(evaluation)
	}
	body := map[string]any{"evaluation_id": evaluation["id"], "artifact_sha256": digest(data), "plan_hash": x.plan["content_hash"]}
	code, out = x.actors[0].call("POST", path+"/acceptances", "first-current-vote", body)
	if code != 200 {
		t.Fatal(code, out)
	}
	ctx := context.Background()
	// Explicit fault/recovery fixture: the ordinary API lifecycle above is
	// intact. A missing persisted pointer must not turn exact artifact consent
	// into a false task completion event or append another approval.
	if _, err := x.db.Exec(ctx, "UPDATE undergrid.tasks SET submitted_artifact_id=NULL WHERE id=$1", x.task["id"]); err != nil {
		t.Fatal(err)
	}
	var version int64
	if err := x.db.QueryRow(ctx, "SELECT version FROM undergrid.tasks WHERE id=$1", x.task["id"]).Scan(&version); err != nil {
		t.Fatal(err)
	}
	code, out = x.actors[1].call("POST", path+"/acceptances", "second-current-vote", body)
	if code != 409 {
		t.Fatalf("noncurrent submission accepted: %d %v", code, out)
	}
	var state string
	var currentVersion int64
	var approvals, events int
	if err := x.db.QueryRow(ctx, `SELECT state,version,
	 (SELECT count(*) FROM undergrid.approvals WHERE artifact_id=$2 AND purpose='acceptance'),
	 (SELECT count(*) FROM undergrid.events WHERE aggregate_id=t.id AND type='task.output_accepted')
	 FROM undergrid.tasks t WHERE id=$1`, x.task["id"], upload["id"]).Scan(&state, &currentVersion, &approvals, &events); err != nil {
		t.Fatal(err)
	}
	if state != "submitted" || currentVersion != version || approvals != 1 || events != 0 {
		t.Fatalf("denied acceptance left effects: state=%s version=%d/%d approvals=%d events=%d", state, currentVersion, version, approvals, events)
	}
	if _, err := x.db.Exec(ctx, "UPDATE undergrid.tasks SET submitted_artifact_id=$2 WHERE id=$1", x.task["id"], upload["id"]); err != nil {
		t.Fatal(err)
	}
	code, out = x.actors[1].call("POST", path+"/acceptances", "second-current-vote", body)
	if code != 200 {
		t.Fatal(code, out)
	}
	for range 2 {
		code, out = x.actors[1].call("POST", path+"/acceptances", "second-current-vote", body)
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	if err := x.db.QueryRow(ctx, `SELECT state,version,
	 (SELECT count(*) FROM undergrid.approvals WHERE artifact_id=$2 AND purpose='acceptance'),
	 (SELECT count(*) FROM undergrid.events WHERE aggregate_id=t.id AND type='task.output_accepted')
	 FROM undergrid.tasks t WHERE id=$1`, x.task["id"], upload["id"]).Scan(&state, &currentVersion, &approvals, &events); err != nil {
		t.Fatal(err)
	}
	if state != "accepted_output" || currentVersion != version+1 || approvals != 2 || events != 1 {
		t.Fatalf("restored current acceptance did not complete once: state=%s version=%d/%d approvals=%d events=%d", state, currentVersion, version, approvals, events)
	}
}
