package network

import (
	"context"
	"reflect"
	"testing"
	"time"
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

func TestOwnerTaskExecutionRejectsUnreviewedVersionEpochAndForeignActor(t *testing.T) {
	f, task := acceptedOwnerExecutionTask(t)
	path := "/v1/owner/agents/" + f.agent + "/tasks/" + task["id"].(string) + "/progress"
	body := map[string]any{"state": "in_progress", "claim_epoch": task["claim_epoch"]}
	for _, scenario := range []struct {
		name, match string
		actor       testClient
		body        map[string]any
		status      int
	}{
		{"missing version", "", f.owner, body, 428},
		{"stale version", "\"999\"", f.owner, body, 409},
		{"stale epoch", task["etag"].(string), f.owner, map[string]any{"state": "in_progress", "claim_epoch": task["claim_epoch"].(float64) + 1}, 409},
		{"invalid state", task["etag"].(string), f.owner, map[string]any{"state": "accepted_output", "claim_epoch": task["claim_epoch"]}, 422},
		{"foreign owner", task["etag"].(string), f.foreign, body, 404},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if code, out := ownerCallMatch(scenario.actor, "POST", path, NewID(), scenario.match, scenario.body); code != scenario.status {
				t.Fatal(code, out)
			}
		})
	}
	_, agents := f.owner.call("GET", "/v1/agents", "", nil)
	for _, raw := range agents["items"].([]any) {
		agent := raw.(map[string]any)["id"].(string)
		if agent == f.agent {
			continue
		}
		wrong := "/v1/owner/agents/" + agent + "/tasks/" + task["id"].(string) + "/progress"
		if code, out := ownerCallMatch(f.owner, "POST", wrong, NewID(), task["etag"].(string), body); code != 404 {
			t.Fatal("wrong owned agent inherited assignment", code, out)
		}
		break
	}
	var state string
	var events int
	if err := f.db.QueryRow(context.Background(), `SELECT state,(SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='task.progress_recorded') FROM undergrid.tasks WHERE id=$1`, task["id"]).Scan(&state, &events); err != nil || state != "accepted" || events != 0 {
		t.Fatal("rejected execution changed state", state, events, err)
	}
}

func TestOwnerTaskExecutionHeartbeatCannotCureOrBypassBlockedWork(t *testing.T) {
	for _, scenario := range []string{"at_risk", "expired_cure", "dependency_blocked"} {
		t.Run(scenario, func(t *testing.T) {
			f, task := acceptedOwnerExecutionTask(t)
			ctx := context.Background()
			query := `UPDATE undergrid.tasks SET state='at_risk',progress_deadline=clock_timestamp()-interval '1 hour',cure_deadline=clock_timestamp()+interval '1 hour' WHERE id=$1`
			if scenario == "expired_cure" {
				query = `UPDATE undergrid.tasks SET state='at_risk',progress_deadline=clock_timestamp()-interval '3 days',cure_deadline=clock_timestamp()-interval '1 minute' WHERE id=$1`
			}
			if scenario == "dependency_blocked" {
				query = `UPDATE undergrid.tasks SET dependency_blocked=true WHERE id=$1`
			}
			if _, err := f.db.Exec(ctx, query, task["id"]); err != nil {
				t.Fatal(err)
			}
			_, before := f.owner.call("GET", "/v1/tasks/"+task["id"].(string), "", nil)
			path := "/v1/owner/agents/" + f.agent + "/tasks/" + task["id"].(string) + "/progress"
			code, out := ownerCallMatch(f.owner, "POST", path, NewID(), before["etag"].(string), map[string]any{"state": "in_progress", "claim_epoch": before["claim_epoch"]})
			want := 409
			if scenario == "at_risk" {
				want = 200
			}
			if code != want {
				t.Fatal("execution bypassed progress guard", code, out)
			}
			_, after := f.owner.call("GET", "/v1/tasks/"+task["id"].(string), "", nil)
			if after["state"] != before["state"] || !reflect.DeepEqual(after["progress_deadline"], before["progress_deadline"]) || !reflect.DeepEqual(after["cure_deadline"], before["cure_deadline"]) {
				t.Fatal("heartbeat changed clocks or cured work", before, after)
			}
		})
	}
}

func TestOwnerTaskExecutionRechecksAuthorityBeforeReceiptRecovery(t *testing.T) {
	for _, scenario := range []string{"withdrawn", "revoked"} {
		t.Run(scenario, func(t *testing.T) {
			f, task := acceptedOwnerExecutionTask(t)
			path := "/v1/owner/agents/" + f.agent + "/tasks/" + task["id"].(string) + "/progress"
			key := NewID()
			body := map[string]any{"state": "in_progress", "claim_epoch": task["claim_epoch"]}
			if code, out := ownerCallMatch(f.owner, "POST", path, key, task["etag"].(string), body); code != 200 {
				t.Fatal(code, out)
			}
			want := 404
			if scenario == "withdrawn" {
				if _, err := f.db.Exec(context.Background(), `UPDATE undergrid.memberships SET state='withdrawn' WHERE swarm_id=$1 AND agent_id=$2`, f.swarm, f.agent); err != nil {
					t.Fatal(err)
				}
			} else {
				want = 403
				if _, err := f.db.Exec(context.Background(), `UPDATE undergrid.agents SET status='revoked' WHERE id=$1`, f.agent); err != nil {
					t.Fatal(err)
				}
			}
			if code, out := ownerCallMatch(f.owner, "POST", path, key, task["etag"].(string), body); code != want {
				t.Fatal("revoked receipt remained accessible", code, out)
			}
		})
	}
}

func TestTaskExecutionPolicyWithdrawalBlocksOwnerAndAgentReceiptRecovery(t *testing.T) {
	for _, channel := range []string{"owner", "agent"} {
		t.Run(channel, func(t *testing.T) {
			f, task := acceptedOwnerExecutionTask(t)
			path := "/v1/tasks/" + task["id"].(string) + "/progress"
			if channel == "owner" {
				path = "/v1/owner/agents/" + f.agent + "/tasks/" + task["id"].(string) + "/progress"
			}
			key := NewID()
			body := map[string]any{"state": "in_progress", "claim_epoch": task["claim_epoch"]}
			call := func() (int, map[string]any) {
				if channel == "owner" {
					return ownerCallMatch(f.owner, "POST", path, key, task["etag"].(string), body)
				}
				return f.owner.versioned("POST", path, key, task["etag"].(string), body)
			}
			if code, out := call(); code != 200 {
				t.Fatal(code, out)
			}
			_, policy := f.owner.call("GET", "/v1/owner/policy", "", nil)
			// Normal owner policy revision removes permission, not a privileged SQL fault.
			if code, out := f.owner.versioned("POST", "/v1/owner/policy", NewID(), policy["etag"].(string), map[string]any{"template_ids": []string{}, "license_ids": []string{}, "task_categories": []string{}, "max_active_tasks": 5, "expires_at": time.Now().Add(time.Hour)}); code != 200 {
				t.Fatal(code, out)
			}
			if code, out := call(); code != 403 {
				t.Fatal("withdrawn policy recovered execution receipt", code, out)
			}
		})
	}
}

func TestOwnerTaskExecutionRequiresAcceptedPlan(t *testing.T) {
	f := newOwnerAssignmentFixture(t)
	offered := f.plan["tasks"].([]any)[0].(map[string]any)
	code, task := f.owner.call("GET", "/v1/tasks/"+offered["id"].(string), "", nil)
	if code != 200 || task["state"] != "offered" || task["claim_epoch"].(float64) < 1 {
		t.Fatal("current unaccepted fixture", code, task)
	}
	path := "/v1/owner/agents/" + f.agent + "/tasks/" + task["id"].(string) + "/progress"
	if code, out := ownerCallMatch(f.owner, "POST", path, NewID(), task["etag"].(string), map[string]any{"state": "in_progress", "claim_epoch": task["claim_epoch"]}); code != 409 {
		t.Fatal("unaccepted plan started execution", code, out)
	}
}
