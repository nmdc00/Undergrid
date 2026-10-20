package network

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestOwnerPlanReviewAndExactApprovalPersistSharedDomainEffects(t *testing.T) {
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
	base := "/v1/owner/agents/" + agent
	path := base + "/plans/" + plan["id"].(string)
	if code, out := ownerCallMatch(foreign, "GET", path, "", "", nil); code != 404 {
		t.Fatal("foreign review", code, out)
	}
	code, review := ownerCallMatch(owner, "GET", path, "", "", nil)
	if code != 200 || review["content_hash"] != plan["content_hash"] || review["canonical_json"] != plan["canonical_json"] {
		t.Fatalf("exact owner review unavailable: %d %v", code, review)
	}
	if review["contribution_template_hash"] != strings.Repeat("a", 64) || review["terms_url"] != "https://example.org/test-terms" {
		t.Fatal("sealed terms missing", review)
	}
	code, page := ownerCallMatch(owner, "GET", base+"/swarms/"+swarm+"/plans", "", "", nil)
	if code != 200 || len(page["items"].([]any)) != 1 {
		t.Fatal("plan discovery", code, page)
	}
	body := map[string]any{"decision": "approve", "content_hash": plan["content_hash"], "version": plan["version"], "acknowledge_contribution_terms": true}
	for _, attack := range []map[string]any{
		{"decision": "approve", "content_hash": strings.Repeat("0", 64), "version": plan["version"], "acknowledge_contribution_terms": true},
		{"decision": "approve", "content_hash": plan["content_hash"], "version": 999, "acknowledge_contribution_terms": true},
		{"decision": "approve", "content_hash": plan["content_hash"], "version": plan["version"]},
	} {
		if code, out := ownerCallMatch(owner, "POST", path+"/approvals", NewID(), "", attack); code < 400 {
			t.Fatal("unchecked approval", code, out)
		}
	}
	var before int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.approvals WHERE plan_id=$1", plan["id"]).Scan(&before); err != nil || before != 0 {
		t.Fatal("denial wrote approval", before, err)
	}
	if code, out := ownerCallMatch(foreign, "POST", path+"/approvals", NewID(), "", body); code != 404 {
		t.Fatal("foreign vote", code, out)
	}
	code, out := ownerCallMatch(owner, "POST", path+"/approvals", "owner-exact-plan", "", body)
	if code != 200 {
		t.Fatal("owner approval", code, out)
	}
	if code, out := ownerCallMatch(owner, "POST", path+"/approvals", "owner-exact-plan", "", body); code != 200 {
		t.Fatal("replay", code, out)
	}
	var votes, rights, assignments int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FILTER(WHERE purpose='plan'),count(*) FILTER(WHERE purpose='rights'),(SELECT count(*) FROM undergrid.tasks WHERE plan_id=$1 AND state<>'offered') FROM undergrid.approvals WHERE plan_id=$1 AND content_hash=$2 AND principal_id=$3 AND agent_id=$4`, plan["id"], plan["content_hash"], principal, agent).Scan(&votes, &rights, &assignments); err != nil || votes != 1 || rights != 1 || assignments != 0 {
		t.Fatal("authority effects", votes, rights, assignments, err)
	}
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.memberships SET state='withdrawn' WHERE swarm_id=$1 AND agent_id=$2", swarm, agent); err != nil {
		t.Fatal(err)
	}
	if code, out := ownerCallMatch(owner, "POST", path+"/approvals", "owner-exact-plan", "", body); code < 400 {
		t.Fatal("removed replay", code, out)
	}
	if code, out := ownerCallMatch(owner, "GET", path, "", "", nil); code < 400 {
		t.Fatal("removed review", code, out)
	}
}
