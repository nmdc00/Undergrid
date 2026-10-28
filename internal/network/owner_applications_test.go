package network

import (
	"context"
	"encoding/json"
	"testing"
)

func TestOwnerApplicationSubmitsReviewedRoleWithoutAdmission(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
	code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Owner application role review", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "catalog-builder", "schema_family": "source_catalog_v1", "description": "Build the declared catalogue"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	_, identity := builder.call("GET", "/v1/me", "", nil)
	agent := identity["agent_id"].(string)
	swarm := room["id"].(string)
	path := "/v1/owner/agents/" + agent + "/swarms/" + swarm + "/applications"
	body := map[string]any{"role_key": "catalog-builder", "capability_refs": []string{"source_catalog_v1"}}
	code, application := ownerCallMatch(builder, "POST", path, NewID(), room["etag"].(string), body)
	if code != 201 {
		t.Fatalf("reviewed owner application unavailable: %d %v", code, application)
	}
	if application["state"] != "applied" || application["version"] != float64(1) {
		t.Fatal("application implies admission", application)
	}
	var applied, accepted int
	var currentDeadline bool
	if err := db.QueryRow(context.Background(), `SELECT count(*) FILTER(WHERE state='applied'),count(*) FILTER(WHERE state='accepted'),bool_and(respond_by>clock_timestamp()) FROM undergrid.memberships WHERE swarm_id=$1 AND agent_id=$2`, swarm, agent).Scan(&applied, &accepted, &currentDeadline); err != nil {
		t.Fatal(err)
	}
	if applied != 1 || accepted != 0 || !currentDeadline {
		t.Fatal("wrong application persistence", applied, accepted, currentDeadline)
	}
	if code, out := ownerCallMatch(builder, "GET", "/v1/owner/agents/"+agent+"/swarms/"+swarm+"/context", "", "", nil); code != 404 {
		t.Fatal("application exposed private transcript", code, out)
	}
}

func TestAgentApplicationRetainsDeclaredClaimsInsteadOfDiscardingThem(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
	code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Retain explicit application claims", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "catalog-builder", "schema_family": "source_catalog_v1", "description": "Build the declared catalogue"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	code, application := builder.call("POST", "/v1/swarms/"+room["id"].(string)+"/applications", NewID(), map[string]any{"role_key": "catalog-builder", "capability_refs": []string{"source_catalog_v1", "unverified-reference"}})
	if code != 201 {
		t.Fatal(code, application)
	}
	var raw []byte
	if err := db.QueryRow(context.Background(), `SELECT COALESCE(to_jsonb(m)->'application_claims','null'::jsonb) FROM undergrid.memberships m WHERE id=$1`, application["id"]).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	if claims == nil || claims["schema"] != "application-claims-v1" || claims["role_key"] != "catalog-builder" || string(canonical(claims["capability_refs"])) != `["source_catalog_v1","unverified-reference"]` {
		t.Fatal("application dropped its unverified proposal claims", string(raw))
	}
}
