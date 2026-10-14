package network

import (
	"context"
	"strings"
	"testing"
	"time"
)

func planningClient(t *testing.T, s *Service) testClient {
	t.Helper()
	c := client(t, s)
	code, current := c.call("GET", "/v1/owner/policy", "", nil)
	if code != 200 {
		t.Fatal(code, current)
	}
	code, out := c.versioned("POST", "/v1/owner/policy", "", current["etag"].(string), map[string]any{"template_ids": []string{"test-contribution-v1"}, "license_ids": []string{"CC0-1.0"}, "task_categories": []string{"json_dataset"}, "max_active_tasks": 5, "expires_at": time.Now().Add(time.Hour)})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = c.call("POST", "/v1/agents", "planner-agent", map[string]any{"name": "planner", "scopes": []string{"intents:read", "intents:write", "inbox:read", "swarms:read", "swarms:create", "swarms:apply", "members:invite", "members:respond", "members:withdraw", "messages:write", "plans:propose", "plans:approve", "rights:accept", "tasks:accept", "tasks:progress"}, "profile": map[string]any{"public": true, "accept_invitations": true, "recruitment": true, "capabilities": []string{"source_catalog_v1"}}})
	if code != 201 {
		t.Fatal(code, out)
	}
	c.token = out["token"].(string)
	return c
}
func enableTestTerms(t *testing.T, s *Service) {
	t.Helper()
	if err := s.SetContributionTemplates([]ContributionTemplate{{ID: "test-contribution-v1", SHA256: strings.Repeat("a", 64), LicenseIDs: []string{"CC0-1.0"}, TermsURL: "https://example.org/test-terms", ApprovalEvidence: "fixture only", TestOnly: true}}); err != nil {
		t.Fatal(err)
	}
}

func TestExactPlanApprovalAndIndependentAssignmentAcceptance(t *testing.T) {
	s, db := fixture(t)
	enableTestTerms(t, s)
	a, b := planningClient(t, s), planningClient(t, s)
	_, aid := a.call("GET", "/v1/me", "", nil)
	_, bid := b.call("GET", "/v1/me", "", nil)
	code, room := a.call("POST", "/v1/swarms", "plan-room", map[string]any{"objective": "Build a catalog", "visibility": "restricted", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	path := "/v1/swarms/" + room["id"].(string)
	code, invite := a.call("POST", path+"/invitations", "invite", map[string]any{"agent_id": bid["agent_id"], "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, invite)
	}
	code, out := b.versioned("POST", path+"/membership-response", "join", invite["etag"].(string), map[string]any{"invitation_id": invite["id"], "decision": "accept"})
	if code != 200 {
		t.Fatal(code, out)
	}
	_, snapshot := a.call("GET", path+"/context", "", nil)
	principals := []any{aid["principal_id"], bid["principal_id"]}
	spec := map[string]any{"parent_version": nil, "objective": "Build a catalog", "required_principals": principals, "acceptance_principals": principals, "publication_principals": principals, "rights": map[string]any{"template_id": "test-contribution-v1", "license_id": "CC0-1.0", "publication": "exact_manifest_approval"}, "tasks": []any{map[string]any{"key": "catalog", "assignee_agent_id": bid["agent_id"], "depends_on": []any{}, "output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 86400}}, "respond_by": time.Now().Add(time.Hour)}
	code, plan := a.versioned("POST", path+"/plans", "propose", snapshot["swarm"].(map[string]any)["etag"].(string), spec)
	if code != 201 {
		t.Fatal(code, plan)
	}
	code, recovered := b.call("GET", "/v1/agent-context", "", nil)
	if code != 200 || len(recovered["assignments"].([]any)) != 1 {
		t.Fatalf("assignment not recovered: %d %v", code, recovered)
	}
	planPath := "/v1/plans/" + plan["id"].(string)
	task := plan["tasks"].([]any)[0].(map[string]any)
	taskPath := "/v1/tasks/" + task["id"].(string)
	code, _ = a.call("POST", planPath+"/approvals", "wrong", map[string]any{"decision": "approve", "content_hash": strings.Repeat("0", 64)})
	if code != 409 {
		t.Fatalf("wrong plan hash %d", code)
	}
	type voteResult struct {
		code int
		body map[string]any
	}
	votes := make(chan voteResult, 2)
	for _, actor := range []testClient{a, b} {
		go func(actor testClient) {
			code, out := actor.call("POST", planPath+"/approvals", "approve", map[string]any{"decision": "approve", "content_hash": plan["content_hash"]})
			votes <- voteResult{code, out}
		}(actor)
	}
	for range 2 {
		result := <-votes
		if result.code != 200 {
			t.Fatal(result.code, result.body)
		}
	}

	code, waiting := a.call("GET", planPath, "", nil)
	if code != 200 || waiting["state"] != "proposed" {
		t.Fatalf("votes accepted assignment: %d %v", code, waiting)
	}
	code, out = b.versioned("POST", taskPath+"/assignment-response", "accept-work", task["etag"].(string), map[string]any{"decision": "accept", "plan_hash": plan["content_hash"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, accepted := a.call("GET", planPath, "", nil)
	if code != 200 || accepted["state"] != "accepted" {
		t.Fatal(code, accepted)
	}
	// Observe normalized persistence independently of the response projection.
	var persistedState, persistedHash, activePlan, assignedAgent string
	var approvals, rightsApprovals, acceptanceEvents int
	if err := db.QueryRow(context.Background(), `SELECT p.state,p.content_hash,s.active_plan_id::text,t.assigned_agent_id::text,
	 (SELECT count(*) FROM undergrid.approvals WHERE plan_id=p.id AND purpose='plan' AND decision='approve'),
	 (SELECT count(*) FROM undergrid.approvals WHERE plan_id=p.id AND purpose='rights' AND decision='approve'),
	 (SELECT count(*) FROM undergrid.events WHERE aggregate_id=p.id AND type='plan.accepted')
	 FROM undergrid.plan_versions p JOIN undergrid.swarms s ON s.id=p.swarm_id
	 JOIN undergrid.tasks t ON t.plan_id=p.id WHERE p.id=$1`, plan["id"]).Scan(&persistedState, &persistedHash, &activePlan, &assignedAgent, &approvals, &rightsApprovals, &acceptanceEvents); err != nil {
		t.Fatal(err)
	}
	if persistedState != "accepted" || persistedHash != plan["content_hash"] || activePlan != plan["id"] || assignedAgent != bid["agent_id"] || approvals != 2 || rightsApprovals != 2 || acceptanceEvents != 1 {
		t.Fatalf("normalized acceptance contract: state=%s hash=%s head=%s agent=%s approvals=%d rights=%d events=%d", persistedState, persistedHash, activePlan, assignedAgent, approvals, rightsApprovals, acceptanceEvents)
	}
	_, currentTask := b.call("GET", taskPath, "", nil)
	code, out = b.versioned("POST", taskPath+"/progress", "start", currentTask["etag"].(string), map[string]any{"state": "in_progress", "claim_epoch": currentTask["claim_epoch"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, _ = b.versioned("POST", taskPath+"/progress", "stale", currentTask["etag"].(string), map[string]any{"state": "in_progress", "claim_epoch": currentTask["claim_epoch"]})
	if code != 409 {
		t.Fatalf("stale task update %d", code)
	}
	if digest([]byte(plan["canonical_json"].(string))) != plan["content_hash"] {
		t.Fatal("published canonical hash mismatch")
	}
	code, replayed := a.versioned("POST", path+"/plans", "propose", snapshot["swarm"].(map[string]any)["etag"].(string), spec)
	if code != 201 || replayed["id"] != plan["id"] {
		t.Fatal(code, replayed)
	}
	// Restart with changed legal wording under the same ID: an old approval
	// cannot authorize progress against a different template digest.
	restarted := New(s.dsn, "development", nil)
	if err := restarted.SetContributionTemplates([]ContributionTemplate{{ID: "test-contribution-v1", SHA256: strings.Repeat("b", 64), LicenseIDs: []string{"CC0-1.0"}, TermsURL: "https://example.org/test-terms", ApprovalEvidence: "changed fixture only", TestOnly: true}}); err != nil {
		t.Fatal(err)
	}
	reconnected := b
	reconnected.s = restarted
	_, restartedTask := reconnected.call("GET", taskPath, "", nil)
	code, out = reconnected.versioned("POST", taskPath+"/progress", "changed-template", restartedTask["etag"].(string), map[string]any{"state": "in_progress", "claim_epoch": restartedTask["claim_epoch"]})
	if code != 409 || out["error"].(map[string]any)["code"] != "plan_terms_expired_or_changed" {
		t.Fatalf("changed template authorized old approval: %d %v", code, out)
	}
	_, afterDenied := reconnected.call("GET", taskPath, "", nil)
	if afterDenied["etag"] != restartedTask["etag"] || afterDenied["progress_deadline"] != restartedTask["progress_deadline"] {
		t.Fatalf("denied template change mutated work: before=%v after=%v", restartedTask, afterDenied)
	}
	_, policy := b.call("GET", "/v1/owner/policy", "", nil)
	code, out = b.versioned("POST", "/v1/owner/policy", "", policy["etag"].(string), map[string]any{"template_ids": []string{}, "license_ids": []string{}, "task_categories": []string{}, "max_active_tasks": 0, "expires_at": time.Now().Add(time.Hour)})
	if code != 200 {
		t.Fatal(code, out)
	}
	_, currentTask = b.call("GET", taskPath, "", nil)
	code, out = b.versioned("POST", taskPath+"/progress", "revoked-policy", currentTask["etag"].(string), map[string]any{"state": "in_progress", "claim_epoch": currentTask["claim_epoch"]})
	if code != 403 {
		t.Fatalf("revoked policy authorized progress: %d %v", code, out)
	}
	code, out = b.call("POST", path+"/withdraw", "leave", map[string]any{})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = b.call("POST", planPath+"/approvals", "approve", map[string]any{"decision": "approve", "content_hash": plan["content_hash"]})
	if code != 404 {
		t.Fatalf("withdrawn agent replay disclosed private plan: %d %v", code, out)
	}
	_, currentTask = a.call("GET", taskPath, "", nil)
	if currentTask["state"] != "withdrawn" || currentTask["claim_epoch"] != float64(2) {
		t.Fatalf("withdrawal retained assignment authority: %v", currentTask)
	}

}

func TestTemplateProductionBoundary(t *testing.T) {
	s := New("", "production", nil)
	if err := s.SetContributionTemplates([]ContributionTemplate{{ID: "test", SHA256: strings.Repeat("a", 64), LicenseIDs: []string{"CC0-1.0"}, TermsURL: "https://example.org/terms", ApprovalEvidence: "fixture", TestOnly: true}}); err == nil {
		t.Fatal("production accepted fixture rights")
	}
}

func TestPlanDependencyContract(t *testing.T) {
	principal, agent := NewID(), NewID()
	p := PlanSpec{Objective: "catalog", RequiredPrincipals: []string{principal}, AcceptancePrincipals: []string{principal}, RespondBy: time.Now().Add(time.Hour), Tasks: []TaskSpec{{Key: "a", AssigneeAgentID: agent, OutputSchema: "source_catalog_v1", Checks: []string{"json_schema_v1"}, ProgressIntervalSeconds: 60}}}
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	p.Tasks[0].DependsOn = []string{"a"}
	if err := p.validate(); err == nil {
		t.Fatal("self-dependency accepted")
	}
	p.Tasks[0].DependsOn = []string{"missing"}
	if err := p.validate(); err == nil {
		t.Fatal("missing dependency accepted")
	}
}
