package network

import (
	"bytes"
	"context"
	"crypto/sha256"
	"github.com/jackc/pgx/v5"
	"net/http/httptest"
	"testing"
	"time"
)

// Governance scopes are explicitly issued under the owner opt-in; existing
// rights-template approval alone must never confer succession authority.
func TestCoordinatorSuccessionRequiresCurrentUnanimityAndSuccessorAcceptance(t *testing.T) {
	s, db := coordinatorFixture(t)
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
	if code, out := successor.call("POST", roomPath+"/messages", "ordinary-conversation", map[string]any{"kind": "text", "body": "Still waiting for the buyer's explicit governance vote."}); code != 201 {
		t.Fatal(code, out)
	}
	code, applied := buyer.versioned("POST", proposalPath+"/approvals", "buyer-approves", voteETag, map[string]any{"decision": "approve", "content_hash": hash})
	if code != 200 {
		t.Fatal(code, applied)
	}
	assertCoordinator(identities["successor"]["agent_id"].(string))
	var oldRole string
	if err := db.QueryRow(context.Background(), "SELECT role FROM undergrid.memberships WHERE swarm_id=$1 AND agent_id=$2", room["id"], oldIdentity["agent_id"]).Scan(&oldRole); err != nil {
		t.Fatal(err)
	}
	if oldRole != "observer" {
		t.Fatalf("office-only predecessor acquired contribution/governance role: %s", oldRole)
	}
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
	s, db := coordinatorFixture(t)
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
	var oldRole string
	if err := f.db.QueryRow(context.Background(), "SELECT role FROM undergrid.memberships WHERE swarm_id=$1 AND agent_id=$2", f.roomID, f.oldID["agent_id"]).Scan(&oldRole); err != nil {
		t.Fatal(err)
	}
	if oldRole != "contributor" {
		t.Fatalf("required prior contributor lost role: %s", oldRole)
	}
}

func coordinatorFixture(t *testing.T) (*Service, *pgx.Conn) {
	t.Helper()
	s, db := fixture(t)
	if _, err := db.Exec(context.Background(), `DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='undergrid_coordinator_api_runner') THEN CREATE ROLE undergrid_coordinator_api_runner LOGIN NOSUPERUSER NOBYPASSRLS;END IF;END $$;GRANT undergrid_app TO undergrid_coordinator_api_runner`); err != nil {
		t.Fatal(err)
	}
	s.dsn = networkFixtureRoleDSN(t, db, "undergrid_coordinator_api_runner")
	return s, db
}
func TestCoordinatorHelperFailsClosedWithoutAuthorityGuard(t *testing.T) {
	f := newSuccessionTestRoom(t)
	p := f.propose(t)
	path := "/v1/coordinator-proposals/" + p["id"].(string)
	for _, voter := range []testClient{f.successor, f.peer} {
		code, out := voter.versioned("POST", path+"/approvals", "before-accept", p["etag"].(string), map[string]any{"content_hash": p["content_hash"], "decision": "approve"})
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, f.s.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET LOCAL ROLE undergrid_app;SET LOCAL search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(f.successor.token))
	var principal, agent, credential string
	if err = tx.QueryRow(ctx, "SELECT principal_id,agent_id,credential_id FROM identify_token($1)", digest[:]).Scan(&principal, &agent, &credential); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "SELECT set_config('undergrid.principal_id',$1,true),set_config('undergrid.agent_id',$2,true)", principal, agent); err != nil {
		t.Fatal(err)
	}
	// Explicit SQL boundary fixture: all genuine HTTP votes exist; the app's
	// acceptance field is populated without invoking the transition or guard.
	if _, err = tx.Exec(ctx, "UPDATE coordinator_proposals SET accepted_credential_id=$2,accepted_at=clock_timestamp() WHERE id=$1", p["id"], credential); err != nil {
		t.Fatal(err)
	}
	var denied bool
	if err = tx.QueryRow(ctx, "SELECT coordinator_authorities_current($1) IS FALSE", f.roomID).Scan(&denied); err != nil {
		t.Fatal(err)
	}
	if !denied {
		t.Error("unset guard returned nullable/truthy authority instead of false")
	}
	var applied bool
	if err = tx.QueryRow(ctx, "SELECT apply_coordinator_succession($1)", p["id"]).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Error("restricted app applied coordinator transfer without authority guards")
	}
}
func TestCoordinatorScopeDenialDoesNotWaitOnPeerAuthority(t *testing.T) {
	for _, mode := range []string{"api_token", "oauth_narrow_scope"} {
		t.Run(mode, func(t *testing.T) {
			f := newSuccessionTestRoom(t)
			var token string
			if mode == "api_token" {
				code, issued := f.peer.call("POST", "/v1/agents", "read-only-member", map[string]any{"name": "read-only member", "scopes": []string{"swarms:read", "members:respond"}, "profile": map[string]any{"accept_invitations": true}})
				if code != 201 {
					t.Fatal(code, issued)
				}
				member := f.peer
				member.token = issued["token"].(string)
				_, identity := member.call("GET", "/v1/me", "", nil)
				code, offer := f.old.call("POST", f.path+"/invitations", "invite-read-only", map[string]any{"agent_id": identity["agent_id"], "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
				if code != 201 {
					t.Fatal(code, offer)
				}
				code, out := member.versioned("POST", f.path+"/membership-response", "join-read-only", offer["etag"].(string), map[string]any{"invitation_id": offer["id"], "decision": "accept"})
				if code != 200 {
					t.Fatal(code, out)
				}
				token = member.token
			} else {
				as := newTestOAuthAS(t)
				as.configure(t, f.s)
				wide := as.token(t, "coordinator-subject", "coordinator-client", "swarms:read coordinator:propose", time.Now().Add(time.Hour), "at+jwt", as.resource)
				code, out, _ := oauthCall(t, f.s, f.successor.owner, wide, "POST", "/v1/owner/agents/"+f.successorID["agent_id"].(string)+"/oauth-bindings", "bind-governance", "", map[string]any{"scopes": []string{"swarms:read", "coordinator:propose"}})
				if code != 201 {
					t.Fatal(code, out)
				}
				token = as.token(t, "coordinator-subject", "coordinator-client", "swarms:read", time.Now().Add(time.Hour), "at+jwt", as.resource)
			}
			ctx := context.Background()
			held, err := f.db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer held.Rollback(ctx)
			if _, err = held.Exec(ctx, "SELECT id FROM undergrid.principals WHERE id=$1 FOR UPDATE", f.oldID["principal_id"]); err != nil {
				t.Fatal(err)
			}
			bounded, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			req := httptest.NewRequest("POST", f.path+"/coordinator-proposals", bytes.NewReader(canonical(map[string]any{"successor_agent_id": f.successorID["agent_id"], "respond_by": time.Now().Add(time.Hour)}))).WithContext(bounded)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", "scope-denied")
			req.Header.Set("If-Match", `"1"`)
			response := httptest.NewRecorder()
			f.s.ServeHTTP(response, req)
			if response.Code != 403 {
				t.Fatalf("missing %s scope waited on peer authority: %d %s", mode, response.Code, response.Body.String())
			}
		})
	}
}

func TestCoordinatorSuccessionRetainsBindingTaskContributorOutsideAcceptedPlan(t *testing.T) {
	f := newSuccessionTestRoom(t)
	createPlan := func(key string, parent *int64, required []string, assignee map[string]any) map[string]any {
		t.Helper()
		code, current := f.old.call("GET", f.path+"/context", "", nil)
		if code != 200 {
			t.Fatal(code, current)
		}
		spec := map[string]any{"parent_version": parent, "objective": "Preserve binding assignments during governance succession", "required_principals": required, "acceptance_principals": []string{required[0]}, "publication_principals": required, "rights": map[string]any{"template_id": "test-contribution-v1", "license_id": "CC0-1.0", "publication": "exact_manifest_approval"}, "tasks": []any{map[string]any{"key": "catalog", "assignee_agent_id": assignee["agent_id"], "depends_on": []any{}, "output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 86400}}, "respond_by": time.Now().Add(time.Hour)}
		code, plan := f.old.versioned("POST", f.path+"/plans", key, current["swarm"].(map[string]any)["etag"].(string), spec)
		if code != 201 {
			t.Fatal(code, plan)
		}
		return plan
	}
	required := []string{f.successorID["principal_id"].(string), f.peerID["principal_id"].(string)}
	active := createPlan("current-plan", nil, required, f.successorID)
	for _, voter := range []testClient{f.successor, f.peer} {
		code, out := voter.call("POST", "/v1/plans/"+active["id"].(string)+"/approvals", "accept-current-plan", map[string]any{"decision": "approve", "content_hash": active["content_hash"]})
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	task := active["tasks"].([]any)[0].(map[string]any)
	code, out := f.successor.versioned("POST", "/v1/tasks/"+task["id"].(string)+"/assignment-response", "accept-current-task", task["etag"].(string), map[string]any{"decision": "accept", "plan_hash": active["content_hash"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	// This current accepted scope excludes the old coordinator. A separate
	// proposed revision binds that owner's explicit assignment acceptance.
	parent := int64(1)
	next := createPlan("next-plan", &parent, append(required, f.oldID["principal_id"].(string)), f.oldID)
	nextTask := next["tasks"].([]any)[0].(map[string]any)
	code, out = f.old.versioned("POST", "/v1/tasks/"+nextTask["id"].(string)+"/assignment-response", "bind-future-task", nextTask["etag"].(string), map[string]any{"decision": "accept", "plan_hash": next["content_hash"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	p := f.propose(t)
	path := "/v1/coordinator-proposals/" + p["id"].(string)
	etag := p["etag"].(string)
	code, out = f.successor.versioned("POST", path+"/accept", "accept-office", etag, map[string]any{"content_hash": p["content_hash"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	for _, voter := range []testClient{f.successor, f.peer} {
		code, out := voter.versioned("POST", path+"/approvals", "vote", etag, map[string]any{"decision": "approve", "content_hash": p["content_hash"]})
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	f.assertOldCoordinator(t)
	code, out = f.old.versioned("POST", path+"/approvals", "binding-contributor-approves", etag, map[string]any{"decision": "approve", "content_hash": p["content_hash"]})
	if code != 200 || out["state"] != "applied" {
		t.Fatal(code, out)
	}
}

func TestCoordinatorMemberCanNominateAnotherEligibleMember(t *testing.T) {
	f := newSuccessionTestRoom(t)
	code, current := f.successor.call("GET", f.path+"/context", "", nil)
	if code != 200 {
		t.Fatal(code, current)
	}
	code, p := f.successor.versioned("POST", f.path+"/coordinator-proposals", "nominate-peer", current["swarm"].(map[string]any)["etag"].(string), map[string]any{"successor_agent_id": f.peerID["agent_id"], "respond_by": time.Now().Add(time.Hour)})
	if code != 201 || p["successor_agent_id"] != f.peerID["agent_id"] {
		t.Fatalf("member could not nominate eligible peer under actual app RLS: %d %v", code, p)
	}
}
