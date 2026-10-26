package network

import (
	"context"
	"testing"
	"time"
)

func ownerPlanProposalSpec(agent, principal string) PlanSpec {
	return PlanSpec{Objective: "Build a bounded dependent catalogue", RequiredPrincipals: []string{principal}, AcceptancePrincipals: []string{principal}, PublicationPrincipals: []string{principal}, Rights: RightsSpec{TemplateID: "test-contribution-v1", LicenseID: "CC0-1.0", Publication: "exact_manifest_approval"}, Tasks: []TaskSpec{
		{Key: "collect", AssigneeAgentID: agent, DependsOn: []string{}, OutputSchema: "source_catalog_v1", Checks: []string{"json_schema_v1"}, ProgressIntervalSeconds: 86400},
		{Key: "assemble", AssigneeAgentID: agent, DependsOn: []string{"collect"}, OutputSchema: "source_catalog_v1", Checks: []string{"json_schema_v1"}, ProgressIntervalSeconds: 86400},
	}, RespondBy: time.Now().UTC().Add(time.Hour)}
}

func TestOwnerPlanProposalContextLoadsCurrentParticipants(t *testing.T) {
	s, _ := ownerSwarmCreationFixture(t)
	owner, foreign := client(t, s), client(t, s)
	_, identity := owner.call("GET", "/v1/me", "", nil)
	code, room := owner.call("POST", "/v1/owner/agents/"+identity["agent_id"].(string)+"/swarms", "context-room", map[string]any{"objective": "Context selection from actual accepted memberships", "visibility": "restricted", "history_policy": "full", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	path := "/v1/owner/agents/" + identity["agent_id"].(string) + "/swarms/" + room["id"].(string) + "/plan-context"
	code, got := ownerCallMatch(owner, "GET", path, "", "", nil)
	if code != 200 {
		t.Fatalf("owner proposal context missing:%d %#v", code, got)
	}
	rows, ok := got["items"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("current accepted roster:%#v", got)
	}
	member := rows[0].(map[string]any)
	if member["agent_id"] != identity["agent_id"] || member["principal_id"] != identity["principal_id"] || member["role"] != "coordinator" {
		t.Fatalf("incorrect current participant:%#v", member)
	}
	if _, ok := member["profile"]; ok {
		t.Fatal("roster exposed private profile")
	}
	if _, ok := member["credentials"]; ok {
		t.Fatal("roster exposed credentials")
	}
	if code, out := ownerCallMatch(foreign, "GET", path, "", "", nil); code != 404 {
		t.Fatalf("foreign owner roster:%d %#v", code, out)
	}
}

func TestOwnerPlanProposalCreatesImmutableDependentTasksWithoutVotes(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	enableTestTerms(t, s)
	owner := client(t, s)
	_, identity := owner.call("GET", "/v1/me", "", nil)
	agent, principal := identity["agent_id"].(string), identity["principal_id"].(string)
	code, room := ownerCallMatch(owner, "POST", "/v1/owner/agents/"+agent+"/swarms", "proposal-room", "", map[string]any{"objective": "Owner-authored immutable plan", "visibility": "restricted", "history_policy": "full", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	path := "/v1/owner/agents/" + agent + "/swarms/" + room["id"].(string) + "/plans"
	spec := ownerPlanProposalSpec(agent, principal)
	code, first := ownerCallMatch(owner, "POST", path, "owner-proposal-first", room["etag"].(string), spec)
	if code != 201 {
		t.Fatalf("owner proposal entrypoint missing:%d %#v", code, first)
	}
	if first["state"] != "proposed" || first["version"] != float64(1) || len(first["tasks"].([]any)) != 2 {
		t.Fatalf("proposal result:%#v", first)
	}
	code, replayed := ownerCallMatch(owner, "POST", path, "owner-proposal-first", room["etag"].(string), spec)
	if code != 201 || replayed["id"] != first["id"] || replayed["content_hash"] != first["content_hash"] {
		t.Fatalf("own version advance broke replay:%d %#v", code, replayed)
	}
	var plans, tasks, offered, votes, events int
	var active *string
	if err := db.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM undergrid.plan_versions WHERE swarm_id=$1),(SELECT count(*) FROM undergrid.tasks WHERE plan_id=$2),(SELECT count(*) FROM undergrid.tasks WHERE plan_id=$2 AND state='offered'),(SELECT count(*) FROM undergrid.approvals WHERE plan_id=$2),(SELECT count(*) FROM undergrid.events WHERE aggregate_id=$2 AND type='plan.proposed'),(SELECT active_plan_id::text FROM undergrid.swarms WHERE id=$1)`, room["id"], first["id"]).Scan(&plans, &tasks, &offered, &votes, &events, &active); err != nil {
		t.Fatal(err)
	}
	if plans != 1 || tasks != 2 || offered != 2 || votes != 0 || events != 1 || active != nil {
		t.Fatalf("proposal implied consent/effects: plans=%d tasks=%d offered=%d votes=%d events=%d head=%v", plans, tasks, offered, votes, events, active)
	}
}

func TestPlanProposalReplayRejectsCurrentObserver(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	enableTestTerms(t, s)
	owner := planningClient(t, s)
	_, identity := owner.call("GET", "/v1/me", "", nil)
	agent, principal := identity["agent_id"].(string), identity["principal_id"].(string)
	code, room := owner.call("POST", "/v1/swarms", "observer-replay-room", map[string]any{"objective": "Current contributor replay guard", "visibility": "restricted", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	path := "/v1/swarms/" + room["id"].(string) + "/plans"
	spec := ownerPlanProposalSpec(agent, principal)
	code, first := owner.versioned("POST", path, "current-contributor-plan", room["etag"].(string), spec)
	if code != 201 {
		t.Fatal(code, first)
	}
	if code, out := owner.versioned("POST", path, "current-contributor-plan", room["etag"].(string), spec); code != 201 || out["id"] != first["id"] {
		t.Fatalf("unchanged contributor replay:%d %#v", code, out)
	}
	// Explicit authority-transition fixture: there is no owner role-edit route.
	// No fabricated proposal or approval row is inserted to complete this test.
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.memberships SET role='observer',version=version+1 WHERE swarm_id=$1 AND agent_id=$2", room["id"], agent); err != nil {
		t.Fatal(err)
	}
	code, out := owner.versioned("POST", path, "current-contributor-plan", room["etag"].(string), spec)
	if code != 403 {
		t.Fatalf("retained proposal bypassed current contributor authority:%d %#v", code, out)
	}
}
