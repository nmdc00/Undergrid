package network

import (
	"context"
	"reflect"
	"testing"
)

func acceptedOwnerExecutionTask(t *testing.T) (ownerAssignmentFixture, map[string]any) {
	t.Helper()
	f := newOwnerAssignmentFixture(t)
	base := "/v1/owner/agents/" + f.agent
	if code, out := ownerCallMatch(f.owner, "POST", base+"/plans/"+f.plan["id"].(string)+"/approvals", NewID(), "", map[string]any{"decision": "approve", "content_hash": f.plan["content_hash"], "version": f.plan["version"], "acknowledge_contribution_terms": true}); code != 200 {
		t.Fatal("normal exact plan/rights approval", code, out)
	}
	task := f.plan["tasks"].([]any)[0].(map[string]any)
	if code, out := ownerCallMatch(f.owner, "POST", base+"/tasks/"+task["id"].(string)+"/assignment-response", NewID(), task["etag"].(string), map[string]any{"decision": "accept", "plan_hash": f.plan["content_hash"], "acknowledge_responsibility_and_terms": true}); code != 200 {
		t.Fatal("normal separate assignment acceptance", code, out)
	}
	code, current := f.owner.call("GET", "/v1/tasks/"+task["id"].(string), "", nil)
	if code != 200 || current["state"] != "accepted" {
		t.Fatal("accepted fixture task", code, current)
	}
	return f, current
}
func TestOwnerTaskExecutionStartsWithoutExtendingProgressDeadline(t *testing.T) {
	f, task := acceptedOwnerExecutionTask(t)
	path := "/v1/owner/agents/" + f.agent + "/tasks/" + task["id"].(string) + "/progress"
	body := map[string]any{"state": "in_progress", "claim_epoch": task["claim_epoch"]}
	key := NewID()
	code, started := ownerCallMatch(f.owner, "POST", path, key, task["etag"].(string), body)
	if code != 200 {
		t.Fatalf("owner execution unavailable after normal activation: %d %v", code, started)
	}
	if started["state"] != "in_progress" || started["claim_epoch"] != task["claim_epoch"] || started["version"] != task["version"].(float64)+1 || !reflect.DeepEqual(started["progress_deadline"], task["progress_deadline"]) || !reflect.DeepEqual(started["cure_deadline"], task["cure_deadline"]) {
		t.Fatal("heartbeat changed authority or clocks", started, task)
	}
	if code, recovered := ownerCallMatch(f.owner, "POST", path, key, task["etag"].(string), body); code != 200 || !reflect.DeepEqual(started, recovered) {
		t.Fatal("lost-response exact recovery failed", code, recovered)
	}
	var events, artifacts, evidence, approvals int
	if err := f.db.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='task.progress_recorded'),(SELECT count(*) FROM undergrid.artifacts WHERE task_id=$1),(SELECT count(*) FROM undergrid.task_progress_evidence WHERE task_id=$1),(SELECT count(*) FROM undergrid.approvals WHERE plan_id=$2)`, task["id"], f.plan["id"]).Scan(&events, &artifacts, &evidence, &approvals); err != nil {
		t.Fatal(err)
	}
	if events != 1 || artifacts != 0 || evidence != 0 || approvals != 2 {
		t.Fatal("heartbeat fabricated effects", events, artifacts, evidence, approvals)
	}
}
