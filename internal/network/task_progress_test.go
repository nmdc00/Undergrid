package network

import (
	"bytes"
	"context"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

func newPrivateProgressGrantScenario(t *testing.T) privateGrantScenario {
	t.Helper()
	return newPrivateGrantScenarioWithSourcePurpose(t, true, "progress")
}

func insertLegacyProgressInputGrants(t *testing.T, x privateGrantScenario, scope InputGrantScope) ([]string, []string) {
	t.Helper()
	scope.Purpose = "assembly"
	ids := make([]string, 0, 2)
	eventIDs := make([]string, 0, 2)
	tx, err := x.source.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	for _, i := range []int{1, 2} {
		meCode, me := x.source.actors[i].call("GET", "/v1/me", "", nil)
		if meCode != 200 {
			t.Fatal(meCode, me)
		}
		grantID, eventID := NewID(), NewID()
		var policyVersion int64
		if err = tx.QueryRow(context.Background(), "SELECT undergrid.current_policy_version($1::uuid)", x.source.principals[i]).Scan(&policyVersion); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(context.Background(), `INSERT INTO undergrid.approvals(id,principal_id,agent_id,purpose,plan_id,artifact_id,content_hash,decision,policy_version,
			grant_recipient_agent_id,grant_recipient_principal_id,grant_target_task_id,grant_target_plan_id,grant_scope,grant_expires_at,grant_survives_withdrawal,grant_state)
			VALUES($1,$2,$3,'assembly_grant',$4,$5,$6,'approve',$7,$8,$9,$10,$11,$12,$13,$14,'active')`,
			grantID, x.source.principals[i], me["agent_id"], x.source.plan["id"], x.input["id"], digest(canonical(scope)), policyVersion,
			scope.RecipientAgent, scope.RecipientPrincipal, scope.TargetTaskID, x.target.plan["id"], canonical(scope), scope.ExpiresAt, scope.SurviveWithdrawal); err != nil {
			t.Fatal(err)
		}
		facts := canonical(map[string]any{"grant_id": grantID, "artifact_id": x.input["id"]})
		if _, err = tx.Exec(context.Background(), `INSERT INTO undergrid.events(id,aggregate_kind,aggregate_id,version,principal_id,agent_id,type,facts,origin)
			VALUES($1,'artifact_grant',$2,1,$3,$4,'artifact.input_grant_created',$5,'release_test')`, eventID, grantID, x.source.principals[i], me["agent_id"], facts); err != nil {
			t.Fatal(err)
		}
		if err = enqueueInputGrantDelivery(context.Background(), tx, grantID); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, grantID)
		eventIDs = append(eventIDs, eventID)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	return ids, eventIDs
}

// Progress artifacts keep their named audience when separate source-rights
// approvals exist. The direct legacy rows exercise recovery/read predicates.
func TestProgressArtifactInputGrantsRespectImmutableAudience(t *testing.T) {
	x := newPrivateProgressGrantScenario(t)
	scope := x.scope
	scope.Purpose = "assembly"
	query := `SELECT count(*) FROM undergrid_queue.river_job WHERE kind='artifact_input_grant_delivery_v1'`
	var jobsBefore, jobsAfter int
	if err := x.source.db.QueryRow(context.Background(), query).Scan(&jobsBefore); err != nil {
		t.Fatal(err)
	}
	code, denied := x.source.actors[1].call("POST", "/v1/artifacts/"+x.input["id"].(string)+"/input-grants", "outside-progress-audience", scope)
	if code != 409 || denied["error"].(map[string]any)["code"] != "progress_input_audience_required" {
		t.Fatalf("out-of-audience progress grant was not denied: %d %v", code, denied)
	}
	var approvals, events int
	if err := x.source.db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.approvals WHERE artifact_id=$1 AND purpose='assembly_grant'", x.input["id"]).Scan(&approvals); err != nil {
		t.Fatal(err)
	}
	if err := x.source.db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE type='artifact.input_grant_created' AND facts->>'artifact_id'=$1", x.input["id"]).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := x.source.db.QueryRow(context.Background(), query).Scan(&jobsAfter); err != nil {
		t.Fatal(err)
	}
	if approvals != 0 || events != 0 || jobsBefore != jobsAfter {
		t.Fatalf("denied grant left persistence or work: approvals=%d events=%d jobs=%d->%d", approvals, events, jobsBefore, jobsAfter)
	}

	legacyIDs, eventIDs := insertLegacyProgressInputGrants(t, x, scope)
	deadline := time.Now().Add(10 * time.Second)
	var completed int
	for time.Now().Before(deadline) {
		if err := x.source.db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid_queue.river_job
			WHERE kind='artifact_input_grant_delivery_v1' AND args->>'grant_id'=ANY($1::text[]) AND state='completed'`, legacyIDs).Scan(&completed); err != nil {
			t.Fatal(err)
		}
		if completed == len(legacyIDs) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if completed != len(legacyIDs) {
		t.Fatalf("legacy grant delivery jobs did not complete: %d/%d", completed, len(legacyIDs))
	}
	var delivered int
	if err := x.source.db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.inbox WHERE agent_id=$1 AND event_id=ANY($2::uuid[])", x.scope.RecipientAgent, eventIDs).Scan(&delivered); err != nil {
		t.Fatal(err)
	}
	grantCode, grant := x.target.actors[2].call("GET", "/v1/artifact-input-grants/"+legacyIDs[0], "", nil)
	contextCode, agentContext := x.target.actors[2].call("GET", "/v1/agent-context", "", nil)
	contextGrantIDs := []string{}
	if grants, ok := agentContext["input_grants"].([]any); ok {
		for _, raw := range grants {
			contextGrantIDs = append(contextGrantIDs, raw.(map[string]any)["id"].(string))
		}
	}
	inboxCode, inbox := x.target.actors[2].call("GET", "/v1/inbox", "", nil)
	inboxGrantEvents := 0
	if listed, ok := inbox["events"].([]any); ok {
		for _, raw := range listed {
			if raw.(map[string]any)["type"] == "artifact.input_grant_created" {
				inboxGrantEvents++
			}
		}
	}
	if delivered != 0 || grantCode != 404 || contextCode != 200 || inboxCode != 200 || len(contextGrantIDs) != 0 || inboxGrantEvents != 0 {
		t.Fatalf("legacy out-of-audience grant escaped filter: inbox_rows=%d GET=%d %v context=%d %v inbox=%d %v", delivered, grantCode, grant, contextCode, contextGrantIDs, inboxCode, inbox)
	}
	readCodes := []int{}
	for _, path := range []string{
		"/v1/artifacts/" + x.input["id"].(string),
		"/v1/artifacts/" + x.input["id"].(string) + "/evaluations",
		"/v1/artifacts/" + x.input["id"].(string) + "/content?purpose=assembly&task_id=" + x.scope.TargetTaskID,
	} {
		code, _ := x.target.actors[2].call("GET", path, "", nil)
		readCodes = append(readCodes, code)
	}
	if !slices.Equal(readCodes, []int{404, 404, 404}) {
		t.Fatalf("out-of-audience recipient read progress source: %v", readCodes)
	}

	// A currently accepted recipient already in the immutable audience may use
	// a separate complete set of exact source-rights approvals.
	audienceTask := x.source.plan["tasks"].([]any)[1].(map[string]any)
	scope.TargetTaskID = audienceTask["id"].(string)
	scope.TargetPlanHash = x.source.plan["content_hash"].(string)
	scope.TargetEpoch = int64(audienceTask["claim_epoch"].(float64))
	scope.RecipientAgent = audienceTask["assigned_agent_id"].(string)
	scope.RecipientPrincipal = x.source.principals[1]
	for _, i := range []int{1, 2} {
		if code, out := x.source.actors[i].call("POST", "/v1/artifacts/"+x.input["id"].(string)+"/input-grants", "in-audience-progress-grant", scope); code != 201 {
			t.Fatalf("in-audience source-right grant %d: %d %v", i, code, out)
		}
	}
	contentPath := "/v1/artifacts/" + x.input["id"].(string) + "/content?purpose=assembly&task_id=" + scope.TargetTaskID
	request := httptest.NewRequest("GET", contentPath, nil)
	request.Header.Set("Authorization", "Bearer "+x.source.actors[1].token)
	response := httptest.NewRecorder()
	x.source.s.ServeHTTP(response, request)
	if response.Code != 200 || !bytes.Equal(response.Body.Bytes(), x.inputData) {
		t.Fatalf("in-audience separately granted progress input unavailable: %d %s", response.Code, response.Body.String())
	}
}

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
	upload := x.uploadForAudience(t, data, "progress-upload", "progress", x.principals[:3])
	artifactID := upload["id"].(string)
	artifactPath := "/v1/artifacts/" + artifactID
	var storedAudience []string
	var storedReviewer string
	if err := x.db.QueryRow(context.Background(), `SELECT audience_principal_ids::text[],progress_reviewer_principal_id::text
		FROM undergrid.artifacts WHERE id=$1`, artifactID).Scan(&storedAudience, &storedReviewer); err != nil {
		t.Fatal(err)
	}
	expectedAudience := slices.Clone(x.principals[:3])
	slices.Sort(expectedAudience)
	if !slices.Equal(storedAudience, expectedAudience) || storedReviewer != x.principals[0] {
		t.Fatalf("progress artifact did not persist exact normalized audience/reviewer: %v %s", storedAudience, storedReviewer)
	}
	for _, suffix := range []string{"", "/evaluations", "/content"} {
		if code, _ := x.actors[3].call("GET", artifactPath+suffix, "", nil); code != 404 {
			t.Errorf("non-audience member read pending progress artifact %s: %d", suffix, code)
		}
	}
	code, artifact := x.actors[2].call("POST", artifactPath+"/finalize", "progress-seal", x.finalBody(data))
	if code != 200 {
		t.Fatal(code, artifact)
	}
	x.startWorker(t)
	evaluation := x.waitEvaluation(t, artifactID)
	if evaluation["result"] != "passed" {
		t.Fatal("fixed verifier did not pass fixture", evaluation)
	}
	// Wait until ordinary recipients have the worker's durable event before
	// asserting that a current room member outside the named audience has none.
	uploaderAgent := testAgentID(t, x.actors[2])
	outsiderAgent := testAgentID(t, x.actors[3])
	for _, eventType := range []string{"artifact.sealed", "artifact.evaluated"} {
		deliverFanoutEvent(t, x, eventType, artifactID)
		if countInboxResourceEvent(t, x.db, uploaderAgent, eventType, artifactPath) == 0 {
			t.Fatalf("event %s for %s was not delivered to named audience", eventType, artifactPath)
		}
		if countInboxResourceEvent(t, x.db, outsiderAgent, eventType, artifactPath) != 0 {
			t.Errorf("non-audience member received private progress artifact event %s", eventType)
		}
	}
	for _, suffix := range []string{"", "/evaluations", "/content"} {
		if code, _ := x.actors[3].call("GET", artifactPath+suffix, "", nil); code != 404 {
			t.Errorf("non-audience member read sealed progress artifact %s: %d", suffix, code)
		}
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
	reviewerAgent := testAgentID(t, x.actors[0])
	deliverFanoutEvent(t, x, "task.progress_evidence_submitted", evidence["id"].(string))
	if countInboxAggregateEvent(t, x.db, reviewerAgent, "task.progress_evidence_submitted", evidence["id"].(string)) == 0 {
		t.Fatal("progress submission event was not delivered to named reviewer")
	}
	if countInboxAggregateEvent(t, x.db, outsiderAgent, "task.progress_evidence_submitted", evidence["id"].(string)) != 0 {
		t.Error("non-audience member received progress report submission event")
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
	deliverFanoutEvent(t, x, "task.progress_evidence_accepted", x.task["id"].(string))
	if countInboxAggregateEvent(t, x.db, reviewerAgent, "task.progress_evidence_accepted", x.task["id"].(string)) == 0 {
		t.Fatal("progress acceptance event was not delivered to named reviewer")
	}
	if countInboxAggregateEvent(t, x.db, outsiderAgent, "task.progress_evidence_accepted", x.task["id"].(string)) != 0 {
		t.Error("non-audience member received progress report acceptance event")
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
	deliverFanoutEvent(t, x, "task.progress_evidence_rejected", rejectedEvidence["id"].(string))
	if countInboxAggregateEvent(t, x.db, reviewerAgent, "task.progress_evidence_rejected", rejectedEvidence["id"].(string)) == 0 {
		t.Fatal("progress rejection event was not delivered to named reviewer")
	}
	if countInboxAggregateEvent(t, x.db, outsiderAgent, "task.progress_evidence_rejected", rejectedEvidence["id"].(string)) != 0 {
		t.Error("non-audience member received progress report rejection event")
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

func testAgentID(t *testing.T, actor testClient) string {
	t.Helper()
	code, me := actor.call("GET", "/v1/me", "", nil)
	if code != 200 {
		t.Fatalf("read test agent identity: %d %v", code, me)
	}
	return me["agent_id"].(string)
}

func countInboxResourceEvent(t *testing.T, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, agent, eventType, resourcePath string) int {
	t.Helper()
	var count int
	err := db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.inbox i JOIN undergrid.events e ON e.id=i.event_id
		WHERE i.agent_id=$1 AND e.type=$2 AND e.facts->>'resource_path'=$3`, agent, eventType, resourcePath).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func deliverFanoutEvent(t *testing.T, x artifactScenario, eventType, aggregate string) {
	t.Helper()
	var eventID string
	if err := x.db.QueryRow(context.Background(), "SELECT id FROM undergrid.events WHERE type=$1 AND aggregate_id=$2 ORDER BY created_at DESC LIMIT 1", eventType, aggregate).Scan(&eventID); err != nil {
		t.Fatalf("load durable event %s/%s: %v", eventType, aggregate, err)
	}
	worker := fanoutWorker{dsn: x.workerDSN}
	if err := worker.Work(context.Background(), &river.Job[fanoutArgs]{Args: fanoutArgs{EventID: eventID}}); err != nil {
		t.Fatalf("run durable event fanout %s/%s: %v", eventType, aggregate, err)
	}
}

func countInboxAggregateEvent(t *testing.T, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, agent, eventType, aggregate string) int {
	t.Helper()
	var count int
	err := db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.inbox i JOIN undergrid.events e ON e.id=i.event_id
		WHERE i.agent_id=$1 AND e.type=$2 AND e.aggregate_id=$3`, agent, eventType, aggregate).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}
