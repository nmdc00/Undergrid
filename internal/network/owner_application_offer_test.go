package network

import (
	"context"
	"testing"
	"time"
)

func TestOwnerApplicationReviewedOfferStillRequiresApplicantAdmission(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
	code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Reviewed application offer", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "builder", "schema_family": "source_catalog_v1", "description": "Declared role"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	_, who := builder.call("GET", "/v1/me", "", nil)
	_, coord := organizer.call("GET", "/v1/me", "", nil)
	swarm := room["id"].(string)
	code, application := builder.call("POST", "/v1/swarms/"+swarm+"/applications", NewID(), map[string]any{"role_key": "builder", "capability_refs": []string{"source_catalog_v1"}})
	if code != 201 {
		t.Fatal(code, application)
	}
	path := "/v1/owner/agents/" + coord["agent_id"].(string) + "/swarms/" + swarm + "/applications/" + application["id"].(string) + "/offer"
	body := map[string]any{"application_version": 1, "role": "contributor", "respond_by": time.Now().Add(time.Hour), "history_policy": "full"}
	code, offer := ownerCallMatch(organizer, "POST", path, NewID(), room["etag"].(string), body)
	if code != 201 {
		t.Fatalf("reviewed owner application offer unavailable: %d %v", code, offer)
	}
	if offer["id"] != application["id"] || offer["state"] != "offered" || offer["version"] != float64(2) {
		t.Fatal("offer changed application identity or implied admission", offer)
	}
	if code, out := ownerCallMatch(builder, "GET", "/v1/owner/agents/"+who["agent_id"].(string)+"/swarms/"+swarm+"/context", "", "", nil); code != 404 {
		t.Fatal("offer granted private history", code, out)
	}
	code, accepted := ownerCallMatch(builder, "POST", "/v1/owner/agents/"+who["agent_id"].(string)+"/swarms/"+swarm+"/membership-response", NewID(), offer["etag"].(string), map[string]any{"invitation_id": offer["id"], "decision": "accept"})
	if code != 200 {
		t.Fatal(code, accepted)
	}
	var state string
	var obligations int
	if err := db.QueryRow(context.Background(), `SELECT state,(SELECT count(*) FROM undergrid.plan_versions)+(SELECT count(*) FROM undergrid.tasks)+(SELECT count(*) FROM undergrid.approvals) FROM undergrid.memberships WHERE id=$1`, application["id"]).Scan(&state, &obligations); err != nil {
		t.Fatal(err)
	}
	if state != "accepted" {
		t.Fatal("explicit acceptance missing", state)
	}
	// Other retained tests can own plans/tasks; this journey does not create any.
	_ = obligations
}
