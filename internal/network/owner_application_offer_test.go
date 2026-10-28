package network

import (
	"context"
	"testing"
	"time"
)

func TestOwnerApplicationReviewedOfferStillRequiresApplicantAdmission(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
	observeOwnerApplicationContract(t, &organizer, &builder)
	code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Reviewed application offer", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "builder", "schema_family": "source_catalog_v1", "description": "Declared role"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	_, who := builder.call("GET", "/v1/me", "", nil)
	_, coord := organizer.call("GET", "/v1/me", "", nil)
	swarm := room["id"].(string)
	code, application := ownerCallMatch(builder, "POST", "/v1/owner/agents/"+who["agent_id"].(string)+"/swarms/"+swarm+"/applications", NewID(), room["etag"].(string), map[string]any{"role_key": "builder", "capability_refs": []string{"source_catalog_v1"}})
	if code != 201 {
		t.Fatal(code, application)
	}
	for _, view := range []struct {
		c    testClient
		path string
	}{
		{builder, "/v1/owner/agents/" + who["agent_id"].(string) + "/applications"},
		{organizer, "/v1/owner/agents/" + coord["agent_id"].(string) + "/swarms/" + swarm + "/applications"},
	} {
		if status, out := ownerCallMatch(view.c, "GET", view.path, "", "", nil); status != 200 {
			t.Fatal(status, out)
		}
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
	if err := db.QueryRow(context.Background(), `SELECT state,(SELECT count(*) FROM undergrid.plan_versions p WHERE p.swarm_id=m.swarm_id)+(SELECT count(*) FROM undergrid.tasks t JOIN undergrid.plan_versions p ON p.id=t.plan_id WHERE p.swarm_id=m.swarm_id)+(SELECT count(*) FROM undergrid.approvals a JOIN undergrid.plan_versions p ON p.id=a.plan_id WHERE p.swarm_id=m.swarm_id) FROM undergrid.memberships m WHERE m.id=$1`, application["id"]).Scan(&state, &obligations); err != nil {
		t.Fatal(err)
	}
	if state != "accepted" || obligations != 0 {
		t.Fatal("explicit acceptance missing", state)
	}
}

func TestOwnerApplicationOfferPinsReviewsAndRecoversExactly(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	organizer, builder, foreign := collaborationClient(t, s), collaborationClient(t, s), client(t, s)
	code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Pinned application offer reviews", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "builder", "schema_family": "source_catalog_v1", "description": "Declared role"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	_, target := builder.call("GET", "/v1/me", "", nil)
	_, coord := organizer.call("GET", "/v1/me", "", nil)
	swarm := room["id"].(string)
	code, application := builder.call("POST", "/v1/swarms/"+swarm+"/applications", NewID(), map[string]any{"role_key": "builder", "capability_refs": []string{"source_catalog_v1"}})
	if code != 201 {
		t.Fatal(code, application)
	}
	path := "/v1/owner/agents/" + coord["agent_id"].(string) + "/swarms/" + swarm + "/applications/" + application["id"].(string) + "/offer"
	body := map[string]any{"application_version": 1, "role": "contributor", "respond_by": time.Now().Add(time.Hour), "history_policy": "full"}
	for _, c := range []struct {
		actor   testClient
		tag     string
		version int
		history string
		want    int
	}{{organizer, "", 1, "full", 428}, {organizer, etag(999), 1, "full", 409}, {organizer, room["etag"].(string), 999, "full", 409}, {organizer, room["etag"].(string), 1, "partial", 422}, {foreign, room["etag"].(string), 1, "full", 404}} {
		candidate := map[string]any{"application_version": c.version, "role": body["role"], "respond_by": body["respond_by"], "history_policy": c.history}
		if code, out := ownerCallMatch(c.actor, "POST", path, NewID(), c.tag, candidate); code != c.want {
			t.Fatal("offer review precondition", code, c.want, out)
		}
	}
	key := NewID()
	code, offer := ownerCallMatch(organizer, "POST", path, key, room["etag"].(string), body)
	if code != 201 {
		t.Fatal(code, offer)
	}
	code, replay := ownerCallMatch(organizer, "POST", path, key, room["etag"].(string), body)
	if code != 201 || string(canonical(replay)) != string(canonical(offer)) {
		t.Fatal("offer receipt changed", code, replay, offer)
	}
	if code, out := ownerCallMatch(organizer, "POST", path, NewID(), room["etag"].(string), body); code != 409 {
		t.Fatal("stale application review reused", code, out)
	}
	if _, err := db.Exec(context.Background(), `UPDATE undergrid.agents SET status='revoked' WHERE id=$1`, target["agent_id"]); err != nil {
		t.Fatal(err)
	}
	if code, out := ownerCallMatch(organizer, "POST", path, key, room["etag"].(string), body); code != 409 {
		t.Fatal("owner receipt bypassed current applicant authority", code, out)
	}
	var state string
	var offered int
	if err := db.QueryRow(context.Background(), `SELECT state,(SELECT count(*) FROM undergrid.events WHERE aggregate_id=m.id AND type='membership.offered') FROM undergrid.memberships m WHERE id=$1`, application["id"]).Scan(&state, &offered); err != nil {
		t.Fatal(err)
	}
	if state != "offered" || offered != 1 {
		t.Fatal("offer retries duplicated admission/effects", state, offered)
	}
}
