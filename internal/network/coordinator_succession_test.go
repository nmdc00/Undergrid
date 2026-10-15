package network

import (
	"context"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

// Governance scopes are explicitly issued under the owner opt-in; existing
// rights-template approval alone must never confer succession authority.
func TestCoordinatorSuccessionRequiresCurrentUnanimityAndSuccessorAcceptance(t *testing.T) {
	s, db := fixture(t)
	enableTestTerms(t, s)
	old, successor, buyer := successionClient(t, s), successionClient(t, s), successionClient(t, s)
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

func successionClient(t *testing.T, s *Service) testClient {
	t.Helper()
	c := planningClient(t, s)
	code, current := c.call("GET", "/v1/owner/policy", "", nil)
	if code != 200 {
		t.Fatal(code, current)
	}
	policy := current["policy"].(map[string]any)
	policy["allow_coordinator_succession"] = true
	code, out := c.versioned("POST", "/v1/owner/policy", "enable-governance", current["etag"].(string), policy)
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = c.call("POST", "/v1/agents", "governance-agent", map[string]any{"name": "governance participant", "scopes": networkOAuthScopes(), "profile": map[string]any{"public": true, "accept_invitations": true, "recruitment": true, "capabilities": []string{"source_catalog_v1"}}})
	if code != 201 {
		t.Fatal(code, out)
	}
	c.token = out["token"].(string)
	return c
}

type successionTestRoom struct {
	s                          *Service
	db                         *pgx.Conn
	old, successor, peer       testClient
	roomID, path               string
	oldID, successorID, peerID map[string]any
}

func newSuccessionTestRoom(t *testing.T) successionTestRoom {
	t.Helper()
	s, db := fixture(t)
	enableTestTerms(t, s)
	f := successionTestRoom{s: s, db: db, old: successionClient(t, s), successor: successionClient(t, s), peer: successionClient(t, s)}
	_, f.oldID = f.old.call("GET", "/v1/me", "", nil)
	_, f.successorID = f.successor.call("GET", "/v1/me", "", nil)
	_, f.peerID = f.peer.call("GET", "/v1/me", "", nil)
	code, room := f.old.call("POST", "/v1/swarms", "test-succession-room", map[string]any{"objective": "Durable unanimous coordinator succession", "visibility": "restricted", "recruitment_roles": []RecruitmentRole{{Key: "builder", SchemaFamily: "source_catalog_v1"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	f.roomID = room["id"].(string)
	f.path = "/v1/swarms/" + f.roomID
	for _, entry := range []struct {
		client testClient
		id     map[string]any
		key    string
	}{{f.successor, f.successorID, "successor"}, {f.peer, f.peerID, "peer"}} {
		code, offer := f.old.call("POST", f.path+"/invitations", "offer-"+entry.key, map[string]any{"agent_id": entry.id["agent_id"], "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
		if code != 201 {
			t.Fatal(code, offer)
		}
		code, out := entry.client.versioned("POST", f.path+"/membership-response", "join-"+entry.key, offer["etag"].(string), map[string]any{"invitation_id": offer["id"], "decision": "accept"})
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	return f
}
func (f successionTestRoom) propose(t *testing.T) map[string]any {
	t.Helper()
	code, current := f.successor.call("GET", f.path+"/context", "", nil)
	if code != 200 {
		t.Fatal(code, current)
	}
	code, p := f.successor.versioned("POST", f.path+"/coordinator-proposals", "propose", current["swarm"].(map[string]any)["etag"].(string), map[string]any{"successor_agent_id": f.successorID["agent_id"], "respond_by": time.Now().Add(time.Hour).UTC()})
	if code != 201 {
		t.Fatal(code, p)
	}
	return p
}
func (f successionTestRoom) assertOldCoordinator(t *testing.T) {
	t.Helper()
	var coordinator string
	if err := f.db.QueryRow(context.Background(), "SELECT coordinator_agent_id::text FROM undergrid.swarms WHERE id=$1", f.roomID).Scan(&coordinator); err != nil {
		t.Fatal(err)
	}
	if coordinator != f.oldID["agent_id"] {
		t.Fatal("incomplete or stale consent transferred office", coordinator)
	}
}
func TestCoordinatorSuccessionRejectsChangedAuthority(t *testing.T) {
	for _, scenario := range []string{"member_withdrawn", "policy_revoked", "voter_credential_revoked", "principal_suspended"} {
		t.Run(scenario, func(t *testing.T) {
			f := newSuccessionTestRoom(t)
			p := f.propose(t)
			path := "/v1/coordinator-proposals/" + p["id"].(string)
			hash := p["content_hash"]
			etag := p["etag"].(string)
			code, out := f.successor.versioned("POST", path+"/accept", "accept", etag, map[string]any{"content_hash": hash})
			if code != 200 {
				t.Fatal(code, out)
			}
			code, out = f.successor.versioned("POST", path+"/approvals", "vote", etag, map[string]any{"content_hash": hash, "decision": "approve"})
			if code != 200 {
				t.Fatal(code, out)
			}
			switch scenario {
			case "member_withdrawn":
				code, out = f.peer.call("POST", f.path+"/withdraw", "leave", map[string]any{})
				if code != 200 {
					t.Fatal(code, out)
				}
			case "policy_revoked":
				code, current := f.successor.call("GET", "/v1/owner/policy", "", nil)
				if code != 200 {
					t.Fatal(code, current)
				}
				policy := current["policy"].(map[string]any)
				policy["allow_coordinator_succession"] = false
				code, out = f.successor.versioned("POST", "/v1/owner/policy", "revoke-governance", current["etag"].(string), policy)
				if code != 200 {
					t.Fatal(code, out)
				}
			case "voter_credential_revoked":
				code, out = f.successor.call("DELETE", "/v1/credentials/current", "", nil)
				if code != 200 {
					t.Fatal(code, out)
				}
			case "principal_suspended":
				// Administrative suspension is an explicit fixture transition; this
				// does not claim an implemented public administration command.
				if _, err := f.db.Exec(context.Background(), "UPDATE undergrid.principals SET status='suspended' WHERE id=$1", f.successorID["principal_id"]); err != nil {
					t.Fatal(err)
				}
			}
			caller := f.peer
			if scenario == "member_withdrawn" {
				caller = f.successor
			}
			code, out = caller.versioned("POST", path+"/approvals", "late-vote", etag, map[string]any{"content_hash": hash, "decision": "approve"})
			if code != 409 {
				t.Fatalf("stale %s consent accepted: %d %v", scenario, code, out)
			}
			f.assertOldCoordinator(t)
		})
	}
}
func TestCoordinatorSuccessionConcurrentVotesAndAcceptance(t *testing.T) {
	f := newSuccessionTestRoom(t)
	p := f.propose(t)
	path := "/v1/coordinator-proposals/" + p["id"].(string)
	etag := p["etag"].(string)
	hash := p["content_hash"]
	type result struct {
		code int
		body map[string]any
	}
	done := make(chan result, 3)
	for _, voter := range []testClient{f.successor, f.peer} {
		go func(voter testClient) {
			code, body := voter.versioned("POST", path+"/approvals", "concurrent-vote", etag, map[string]any{"content_hash": hash, "decision": "approve"})
			done <- result{code, body}
		}(voter)
	}
	go func() {
		code, body := f.successor.versioned("POST", path+"/accept", "concurrent-accept", etag, map[string]any{"content_hash": hash})
		done <- result{code, body}
	}()
	for range 3 {
		r := <-done
		if r.code != 200 {
			t.Fatal(r.code, r.body)
		}
	}
	var coordinator string
	var events int
	if err := f.db.QueryRow(context.Background(), "SELECT coordinator_agent_id::text FROM undergrid.swarms WHERE id=$1", f.roomID).Scan(&coordinator); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='coordinator.transferred'", p["id"]).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if coordinator != f.successorID["agent_id"] || events != 1 {
		t.Fatalf("concurrent transfer mismatch coordinator=%s events=%d", coordinator, events)
	}
}

func TestCoordinatorSuccessionRetainsAcceptedPlanContributorAndAcceptorConsent(t *testing.T) {
	f := newSuccessionTestRoom(t)
	code, current := f.old.call("GET", f.path+"/context", "", nil)
	if code != 200 {
		t.Fatal(code, current)
	}
	principals := []string{f.oldID["principal_id"].(string), f.successorID["principal_id"].(string), f.peerID["principal_id"].(string)}
	spec := map[string]any{"parent_version": nil, "objective": "Preserve current contributors and buyer consent", "required_principals": principals, "acceptance_principals": []string{principals[0]}, "publication_principals": principals, "rights": map[string]any{"template_id": "test-contribution-v1", "license_id": "CC0-1.0", "publication": "exact_manifest_approval"}, "tasks": []any{map[string]any{"key": "catalog", "assignee_agent_id": f.successorID["agent_id"], "depends_on": []any{}, "output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 86400}}, "respond_by": time.Now().Add(time.Hour)}
	code, plan := f.old.versioned("POST", f.path+"/plans", "accepted-scope", current["swarm"].(map[string]any)["etag"].(string), spec)
	if code != 201 {
		t.Fatal(code, plan)
	}
	planPath := "/v1/plans/" + plan["id"].(string)
	for _, actor := range []testClient{f.old, f.successor, f.peer} {
		code, out := actor.call("POST", planPath+"/approvals", "approve-current-plan", map[string]any{"decision": "approve", "content_hash": plan["content_hash"]})
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	task := plan["tasks"].([]any)[0].(map[string]any)
	code, out := f.successor.versioned("POST", "/v1/tasks/"+task["id"].(string)+"/assignment-response", "accept-current-work", task["etag"].(string), map[string]any{"decision": "accept", "plan_hash": plan["content_hash"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, acceptedPlan := f.old.call("GET", planPath, "", nil)
	if code != 200 || acceptedPlan["state"] != "accepted" {
		t.Fatal(code, acceptedPlan)
	}
	p := f.propose(t)
	proposalPath := "/v1/coordinator-proposals/" + p["id"].(string)
	etag := p["etag"].(string)
	code, out = f.successor.versioned("POST", proposalPath+"/accept", "accept-office", etag, map[string]any{"content_hash": p["content_hash"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	for _, actor := range []testClient{f.successor, f.peer} {
		code, out := actor.versioned("POST", proposalPath+"/approvals", "vote-succession", etag, map[string]any{"decision": "approve", "content_hash": p["content_hash"]})
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	f.assertOldCoordinator(t)
	// Office alone supplies no veto, but this owner is an actual named current
	// contributor/acceptor. The implementation cannot silently omit its vote.
	code, out = f.old.versioned("POST", proposalPath+"/approvals", "required-acceptor-consents", etag, map[string]any{"decision": "approve", "content_hash": p["content_hash"]})
	if code != 200 || out["state"] != "applied" {
		t.Fatal(code, out)
	}
}
