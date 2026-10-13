package network

import (
	"context"
	"testing"
	"time"
)

// A state/epoch heartbeat remains wire compatible, but does not prove useful
// work and therefore cannot renew the execution clock.
func TestTaskProgressHeartbeatDoesNotExtendDeadline(t *testing.T) {
	x := newArtifactScenario(t)
	path := "/v1/tasks/" + x.task["id"].(string)
	_, before := x.actors[2].call("GET", path, "", nil)
	if before["progress_deadline"] == nil {
		t.Fatal("accepted task has no execution deadline", before)
	}
	code, heartbeat := x.actors[2].versioned("POST", path+"/progress", "heartbeat", before["etag"].(string), map[string]any{
		"state": "in_progress", "claim_epoch": before["claim_epoch"],
	})
	if code != 200 {
		t.Fatal(code, heartbeat)
	}
	_, after := x.actors[2].call("GET", path, "", nil)
	if after["progress_deadline"] != before["progress_deadline"] || after["cure_deadline"] != before["cure_deadline"] {
		t.Fatalf("untrusted heartbeat moved a task clock: before=%v after=%v", before, after)
	}
}

// Fixed-verifier evidence is bound to this accepted task, plan and claim epoch.
// A designated acceptance principal reviews that exact report separately from
// final artifact acceptance or publication.
func TestTaskProgressEvidenceReviewLifecycle(t *testing.T) {
	x := newArtifactScenario(t)
	// Add a current room member who is deliberately outside this evidence's
	// audience, so the report read test proves audience filtering within a room.
	roomPath := "/v1/swarms/" + x.room["id"].(string)
	_, outsider := x.actors[3].call("GET", "/v1/me", "", nil)
	code, invitation := x.actors[0].call("POST", roomPath+"/invitations", "progress-audience-invite", map[string]any{
		"agent_id": outsider["agent_id"], "role": "contributor", "respond_by": time.Now().Add(time.Hour),
	})
	if code != 201 {
		t.Fatal(code, invitation)
	}
	if code, joined := x.actors[3].versioned("POST", roomPath+"/membership-response", "progress-audience-join", invitation["etag"].(string), map[string]any{
		"invitation_id": invitation["id"], "decision": "accept",
	}); code != 200 {
		t.Fatal(code, joined)
	}
	data := []byte(`{"sources":[{"source_id":"progress-fixture","source_url":"https://example.org/"}]}`)
	upload := x.upload(t, data, "progress-upload", "progress")
	artifactID := upload["id"].(string)
	artifactPath := "/v1/artifacts/" + artifactID
	code, artifact := x.actors[2].call("POST", artifactPath+"/finalize", "progress-seal", x.finalBody(data))
	if code != 200 {
		t.Fatal(code, artifact)
	}
	x.startWorker(t)
	evaluation := x.waitEvaluation(t, artifactID)
	if evaluation["result"] != "passed" {
		t.Fatal("fixed verifier did not pass fixture", evaluation)
	}
	taskPath := "/v1/tasks/" + x.task["id"].(string)
	if _, err := x.db.Exec(context.Background(), `UPDATE undergrid.tasks SET state='at_risk',version=version+1,
		progress_deadline=clock_timestamp()-interval '1 second',cure_deadline=clock_timestamp()+interval '47 hours' WHERE id=$1`, x.task["id"]); err != nil {
		t.Fatal(err)
	}
	_, current := x.actors[2].call("GET", taskPath, "", nil)
	code, heartbeat := x.actors[2].versioned("POST", taskPath+"/progress", "at-risk-heartbeat", current["etag"].(string), map[string]any{
		"state": "in_progress", "claim_epoch": current["claim_epoch"],
	})
	if code != 200 || heartbeat["state"] != "at_risk" || heartbeat["progress_deadline"] != current["progress_deadline"] || heartbeat["cure_deadline"] != current["cure_deadline"] {
		t.Fatalf("heartbeat changed at-risk cure: %d before=%v after=%v", code, current, heartbeat)
	}
	_, current = x.actors[2].call("GET", taskPath, "", nil)
	body := map[string]any{
		"claim_epoch": current["claim_epoch"], "artifact_id": artifactID,
		"artifact_sha256": digest(data), "evaluation_id": evaluation["id"],
		"report":                 "Verified the fixture schema and source-reference fields.",
		"audience_principal_ids": []string{x.principals[0], x.principals[1], x.principals[2]},
		"reviewer_principal_id":  x.principals[0],
	}

	code, evidence := x.actors[2].versioned("POST", taskPath+"/progress-evidence", "progress-report", current["etag"].(string), body)
	if code != 201 {
		t.Fatalf("submit progress evidence: %d %v", code, evidence)
	}
	if evidence["artifact_id"] != artifactID || evidence["evaluation_id"] != evaluation["id"] || evidence["report_sha256"] != digest([]byte(body["report"].(string))) {
		t.Fatalf("evidence lost exact bindings: %v", evidence)
	}
	if code, replay := x.actors[2].versioned("POST", taskPath+"/progress-evidence", "progress-report", current["etag"].(string), body); code != 201 || replay["id"] != evidence["id"] {
		t.Fatalf("same-key report retry did not return the original evidence: %d %v", code, replay)
	}
	wrongArtifact := cloneObject(body)
	wrongArtifact["artifact_id"] = NewID()
	if code, _ = x.actors[2].versioned("POST", taskPath+"/progress-evidence", "wrong-artifact", current["etag"].(string), wrongArtifact); code != 404 {
		t.Fatalf("unbound artifact accepted: %d", code)
	}
	wrongEvaluation := cloneObject(body)
	wrongEvaluation["evaluation_id"] = NewID()
	if code, _ = x.actors[2].versioned("POST", taskPath+"/progress-evidence", "wrong-evaluation", current["etag"].(string), wrongEvaluation); code != 409 {
		t.Fatalf("unbound evaluation accepted: %d", code)
	}
	wrongEpoch := cloneObject(body)
	wrongEpoch["claim_epoch"] = current["claim_epoch"].(float64) + 1
	if code, _ = x.actors[2].versioned("POST", taskPath+"/progress-evidence", "wrong-epoch", current["etag"].(string), wrongEpoch); code != 409 {
		t.Fatalf("stale claim epoch accepted: %d", code)
	}
	wrongAudience := cloneObject(body)
	wrongAudience["audience_principal_ids"] = []string{x.principals[0], x.principals[2], NewID()}
	if code, _ = x.actors[2].versioned("POST", taskPath+"/progress-evidence", "wrong-audience", current["etag"].(string), wrongAudience); code != 422 {
		t.Fatalf("unlisted audience accepted: %d", code)
	}
	if code, _ = x.actors[3].call("GET", taskPath+"/progress-evidence/"+evidence["id"].(string), "", nil); code != 404 {
		t.Fatalf("non-audience member read private report: %d", code)
	}
	if code, denied := x.actors[1].call("POST", taskPath+"/progress-evidence-review", "wrong-reviewer", map[string]any{
		"evidence_id": evidence["id"], "decision": "accept", "report_sha256": evidence["report_sha256"],
	}); code != 403 {
		t.Fatalf("different acceptance principal reviewed evidence: %d %v", code, denied)
	}
	if code, report := x.actors[0].call("GET", taskPath+"/progress-evidence/"+evidence["id"].(string), "", nil); code != 200 || report["report"] != body["report"] {
		t.Fatalf("named audience could not recover report: %d %v", code, report)
	}
	if code, _ = x.actors[0].call("POST", artifactPath+"/acceptances", "progress-is-not-output", map[string]any{
		"evaluation_id": evaluation["id"], "artifact_sha256": digest(data), "plan_hash": x.plan["content_hash"],
	}); code != 409 {
		t.Fatalf("progress artifact entered final output acceptance: %d", code)
	}
	if code, _ = x.actors[0].call("POST", artifactPath+"/publication-approvals", "progress-is-not-public", map[string]any{
		"manifest_hash": artifact["manifest_hash"], "license_id": "CC0-1.0",
	}); code != 409 {
		t.Fatalf("progress artifact entered publication approvals: %d", code)
	}
	if code, _ = x.actors[2].call("POST", taskPath+"/progress-evidence-review", "self-review", map[string]any{
		"evidence_id": evidence["id"], "decision": "accept", "report_sha256": evidence["report_sha256"],
	}); code != 403 {
		t.Fatalf("assignee reviewed own evidence: %d", code)
	}
	if _, err := x.db.Exec(context.Background(), "UPDATE undergrid.tasks SET cure_deadline=clock_timestamp()-interval '1 second' WHERE id=$1", x.task["id"]); err != nil {
		t.Fatal(err)
	}
	if code, expired := x.actors[0].call("POST", taskPath+"/progress-evidence-review", "expired-review", map[string]any{
		"evidence_id": evidence["id"], "decision": "accept", "report_sha256": evidence["report_sha256"],
	}); code != 409 {
		t.Fatalf("expired cure window was accepted: %d %v", code, expired)
	}
	var expiredApprovals int
	if err := x.db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.approvals WHERE progress_evidence_id=$1", evidence["id"]).Scan(&expiredApprovals); err != nil {
		t.Fatal(err)
	}
	if expiredApprovals != 0 {
		t.Fatalf("expired evidence review left an approval: %d", expiredApprovals)
	}
	if _, err := x.db.Exec(context.Background(), "UPDATE undergrid.tasks SET cure_deadline=clock_timestamp()+interval '1 hour' WHERE id=$1", x.task["id"]); err != nil {
		t.Fatal(err)
	}
	type reviewResult struct {
		code int
		body map[string]any
	}
	reviews := make(chan reviewResult, 2)
	for _, key := range []string{"review-progress-a", "review-progress-b"} {
		go func(key string) {
			c, out := x.actors[0].call("POST", taskPath+"/progress-evidence-review", key, map[string]any{
				"evidence_id": evidence["id"], "decision": "accept", "report_sha256": evidence["report_sha256"],
			})
			reviews <- reviewResult{c, out}
		}(key)
	}
	var reviewed map[string]any
	var acceptedReviews, deniedReviews int
	for range 2 {
		result := <-reviews
		if result.code == 200 {
			acceptedReviews++
			reviewed = result.body
		} else if result.code == 409 {
			deniedReviews++
		} else {
			t.Fatalf("racing review result: %d %v", result.code, result.body)
		}
	}
	if acceptedReviews != 1 || deniedReviews != 1 || reviewed["state"] != "in_progress" || reviewed["credited_progress_evidence_id"] != evidence["id"] {
		t.Fatalf("named reviewer race did not credit once: accepted=%d denied=%d result=%v", acceptedReviews, deniedReviews, reviewed)
	}
	_, credited := x.actors[2].call("GET", taskPath, "", nil)
	if credited["progress_deadline"] == current["progress_deadline"] || credited["cure_deadline"] != nil {
		t.Fatalf("accepted evidence did not renew only the progress clock: before=%v after=%v", current, credited)
	}
	rejectedBody := cloneObject(body)
	rejectedBody["report"] = "A separate observation that the reviewer does not credit."
	code, rejectedEvidence := x.actors[2].versioned("POST", taskPath+"/progress-evidence", "separate-progress-report", credited["etag"].(string), rejectedBody)
	if code != 201 {
		t.Fatalf("submit second report for rejection test: %d %v", code, rejectedEvidence)
	}
	deadlineBeforeRejection := credited["progress_deadline"]
	code, rejected := x.actors[0].call("POST", taskPath+"/progress-evidence-review", "reject-progress", map[string]any{
		"evidence_id": rejectedEvidence["id"], "decision": "reject", "report_sha256": rejectedEvidence["report_sha256"],
	})
	if code != 200 || rejected["state"] != "in_progress" || rejected["credited_progress_evidence_id"] != evidence["id"] || rejected["progress_deadline"] != deadlineBeforeRejection {
		t.Fatalf("rejected evidence changed credited progress: %d before=%v after=%v", code, credited, rejected)
	}
	// Progress review is not final-output acceptance or publication.
	if credited["submitted_artifact_id"] != nil || credited["state"] != "in_progress" {
		t.Fatalf("progress review changed final task submission state: artifact=%v task=%v", artifact, credited)
	}
	var artifactState string
	var finalApprovals, publicationEvents int
	if err := x.db.QueryRow(context.Background(), `SELECT a.state,
		(SELECT count(*) FROM undergrid.approvals WHERE artifact_id=a.id AND purpose='acceptance'),
		(SELECT count(*) FROM undergrid.events WHERE aggregate_id=a.id AND type='artifact.published')
		FROM undergrid.artifacts a WHERE a.id=$1`, artifactID).Scan(&artifactState, &finalApprovals, &publicationEvents); err != nil {
		t.Fatal(err)
	}
	if artifactState != "verified" || finalApprovals != 0 || publicationEvents != 0 {
		t.Fatalf("progress review was conflated with output acceptance/publication: state=%s acceptances=%d publications=%d", artifactState, finalApprovals, publicationEvents)
	}
	// A distinct approval command or re-submission cannot buy a second interval.
	deadline := credited["progress_deadline"]
	if code, _ = x.actors[0].call("POST", taskPath+"/progress-evidence-review", "second-review", map[string]any{
		"evidence_id": evidence["id"], "decision": "accept", "report_sha256": evidence["report_sha256"],
	}); code != 409 {
		t.Fatalf("already credited evidence reviewed twice: %d", code)
	}
	_, afterReplay := x.actors[2].call("GET", taskPath, "", nil)
	if afterReplay["progress_deadline"] != deadline {
		t.Fatalf("evidence replay renewed deadline: before=%v after=%v", deadline, afterReplay["progress_deadline"])
	}
	_, currentAfterReview := x.actors[2].call("GET", taskPath, "", nil)
	if code, duplicate := x.actors[2].versioned("POST", taskPath+"/progress-evidence", "duplicate-progress-report", currentAfterReview["etag"].(string), body); code != 409 {
		t.Fatalf("identical credited evidence was accepted again: %d %v", code, duplicate)
	}
	var linkedArtifact, linkedEvaluation, reportHash, creditedEvidence string
	var epoch int64
	if err := x.db.QueryRow(context.Background(), `SELECT e.artifact_id::text,e.evaluation_id::text,e.report_sha256,e.claim_epoch,t.credited_progress_evidence_id::text
		FROM undergrid.task_progress_evidence e JOIN undergrid.tasks t ON t.id=e.task_id WHERE e.id=$1`, evidence["id"]).Scan(&linkedArtifact, &linkedEvaluation, &reportHash, &epoch, &creditedEvidence); err != nil {
		t.Fatal(err)
	}
	if linkedArtifact != artifactID || linkedEvaluation != evaluation["id"] || reportHash != evidence["report_sha256"] || epoch != int64(current["claim_epoch"].(float64)) || creditedEvidence != evidence["id"] {
		t.Fatalf("normalized evidence links diverged: artifact=%s evaluation=%s hash=%s epoch=%d credited=%s", linkedArtifact, linkedEvaluation, reportHash, epoch, creditedEvidence)
	}
	// A validly evaluated artifact from the previous claim epoch is stale even
	// when its task, plan and hashes still match. Bump authority after all
	// current-epoch lifecycle assertions so this check cannot credit evidence.
	if _, err := x.db.Exec(context.Background(), "UPDATE undergrid.tasks SET claim_epoch=claim_epoch+1,version=version+1 WHERE id=$1", x.task["id"]); err != nil {
		t.Fatal(err)
	}
	_, staleCurrent := x.actors[2].call("GET", taskPath, "", nil)
	staleEvidence := cloneObject(body)
	staleEvidence["claim_epoch"] = staleCurrent["claim_epoch"]
	if code, stale := x.actors[2].versioned("POST", taskPath+"/progress-evidence", "stale-artifact-epoch", staleCurrent["etag"].(string), staleEvidence); code != 409 {
		t.Fatalf("previous-epoch artifact was accepted as current progress: %d %v", code, stale)
	}
	// New handler reads persisted evidence binding and the exact renewed clock.
	reconnected := x.actors[2]
	reconnected.s = New(x.s.dsn, "development", nil)
	_, recovered := reconnected.call("GET", taskPath, "", nil)
	if recovered["credited_progress_evidence_id"] != evidence["id"] || recovered["progress_deadline"] != deadline {
		t.Fatalf("task evidence state lost on handler recovery: %v", recovered)
	}
}

func cloneObject(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
