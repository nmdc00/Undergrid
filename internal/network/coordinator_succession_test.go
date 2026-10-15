package network

import (
	"context"
	"testing"
	"time"
)

// This is the first public behavioral oracle for the missing succession route.
// During implementation its actors must be explicitly issued the new governance
// scopes under the live owner opt-in policy; existing rights grants are not a
// substitute. No persistence/compiler run is authorized while the release gate
// owns the shared fixture.
func TestCoordinatorSuccessionRequiresCurrentUnanimityAndSuccessorAcceptance(t *testing.T) {
	s, db := fixture(t)
	old, successor, buyer := planningClient(t, s), planningClient(t, s), planningClient(t, s)
	code, room := old.call("POST", "/v1/swarms", "succession-room", map[string]any{"objective": "Continue a dataset collaboration", "visibility": "restricted", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	roomPath := "/v1/swarms/" + room["id"].(string)
	identities := map[string]map[string]any{}
	for _, entry := range []struct {
		name, role string
		actor      testClient
	}{{"successor", "contributor", successor}, {"buyer", "contributor", buyer}} {
		code, identity := entry.actor.call("GET", "/v1/me", "", nil)
		if code != 200 {
			t.Fatal(code, identity)
		}
		identities[entry.name] = identity
		code, offer := old.call("POST", roomPath+"/invitations", "invite-"+entry.name, map[string]any{"agent_id": identity["agent_id"], "role": entry.role, "respond_by": time.Now().Add(time.Hour)})
		if code != 201 {
			t.Fatal(code, offer)
		}
		code, joined := entry.actor.versioned("POST", roomPath+"/membership-response", "join-"+entry.name, offer["etag"].(string), map[string]any{"invitation_id": offer["id"], "decision": "accept"})
		if code != 200 {
			t.Fatal(code, joined)
		}
	}
	code, contextSnapshot := successor.call("GET", roomPath+"/context", "", nil)
	if code != 200 {
		t.Fatal(code, contextSnapshot)
	}
	proposedBody := map[string]any{"successor_agent_id": identities["successor"]["agent_id"], "respond_by": time.Now().Add(time.Hour).UTC()}
	proposalETag := contextSnapshot["swarm"].(map[string]any)["etag"].(string)
	code, proposal := successor.versioned("POST", roomPath+"/coordinator-proposals", "propose-successor", proposalETag, proposedBody)
	if code != 201 {
		t.Fatalf("eligible member could not propose a versioned successor: %d %v", code, proposal)
	}
	proposalPath := "/v1/coordinator-proposals/" + proposal["id"].(string)
	hash := proposal["content_hash"].(string)
	voteETag := proposal["etag"].(string)
	code, accepted := successor.versioned("POST", proposalPath+"/accept", "accept-office", voteETag, map[string]any{"content_hash": hash})
	if code != 200 {
		t.Fatal(code, accepted)
	}
	code, voted := successor.versioned("POST", proposalPath+"/approvals", "approve-succession", voteETag, map[string]any{"decision": "approve", "content_hash": hash})
	if code != 200 {
		t.Fatal(code, voted)
	}
	assertCoordinator := func(expected string) {
		t.Helper()
		var actual string
		if err := db.QueryRow(context.Background(), "SELECT coordinator_agent_id::text FROM undergrid.swarms WHERE id=$1", room["id"]).Scan(&actual); err != nil {
			t.Fatal(err)
		}
		if actual != expected {
			t.Fatalf("coordinator=%s expected=%s", actual, expected)
		}
	}
	_, oldIdentity := old.call("GET", "/v1/me", "", nil)
	assertCoordinator(oldIdentity["agent_id"].(string))
	// Ordinary conversation must not erase an exact proposal's prior vote.
	if code, out := successor.call("POST", roomPath+"/messages", "ordinary-conversation", map[string]any{"body": "Still waiting for the buyer's explicit governance vote."}); code != 201 {
		t.Fatal(code, out)
	}
	code, applied := buyer.versioned("POST", proposalPath+"/approvals", "buyer-approves", voteETag, map[string]any{"decision": "approve", "content_hash": hash})
	if code != 200 {
		t.Fatal(code, applied)
	}
	assertCoordinator(identities["successor"]["agent_id"].(string))
	code, replayed := successor.versioned("POST", roomPath+"/coordinator-proposals", "propose-successor", proposalETag, proposedBody)
	if code != 201 || replayed["id"] != proposal["id"] {
		t.Fatalf("lost-response replay did not recover original proposal: %d %v", code, replayed)
	}
	var transfers int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='coordinator.transferred'", proposal["id"]).Scan(&transfers); err != nil {
		t.Fatal(err)
	}
	if transfers != 1 {
		t.Fatalf("succession replay produced %d transfer events", transfers)
	}
	code, out := old.call("POST", roomPath+"/withdraw", "old-coordinator-exits", map[string]any{})
	if code != 200 {
		t.Fatalf("former coordinator still cannot exit after authorized succession: %d %v", code, out)
	}
}
