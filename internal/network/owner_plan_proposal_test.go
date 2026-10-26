package network

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"

	"context"
	"github.com/jackc/pgx/v5"
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

func TestPlanProposalSelectedParticipantActivityGatesNewWriteNotOwnReceipt(t *testing.T) {
	for _, kind := range []string{"agent", "principal"} {
		t.Run(kind, func(t *testing.T) {
			s, db := ownerSwarmCreationFixture(t)
			enableTestTerms(t, s)
			proposer, peer := planningClient(t, s), collaborationClient(t, s)
			_, a := proposer.call("GET", "/v1/me", "", nil)
			_, b := peer.call("GET", "/v1/me", "", nil)
			code, room := proposer.call("POST", "/v1/swarms", "selected-activity-room", map[string]any{"objective": "Selected participant activity in new immutable proposals", "visibility": "restricted", "recruitment_roles": []any{}})
			if code != 201 {
				t.Fatal(code, room)
			}
			roomPath := "/v1/swarms/" + room["id"].(string)
			code, invitation := proposer.call("POST", roomPath+"/invitations", "selected-peer-offer", map[string]any{"agent_id": b["agent_id"], "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
			if code != 201 {
				t.Fatal(code, invitation)
			}
			if code, out := peer.versioned("POST", roomPath+"/membership-response", "selected-peer-accept", invitation["etag"].(string), map[string]any{"invitation_id": invitation["id"], "decision": "accept"}); code != 200 {
				t.Fatal(code, out)
			}
			spec := ownerPlanProposalSpec(b["agent_id"].(string), a["principal_id"].(string))
			spec.RequiredPrincipals = append(spec.RequiredPrincipals, b["principal_id"].(string))
			spec.AcceptancePrincipals = append(spec.AcceptancePrincipals, b["principal_id"].(string))
			spec.PublicationPrincipals = append(spec.PublicationPrincipals, b["principal_id"].(string))
			path := roomPath + "/plans"
			code, first := proposer.versioned("POST", path, "activity-first-plan", room["etag"].(string), spec)
			if code != 201 {
				t.Fatal(code, first)
			}
			// Explicit current-authority fixture; all room/plan/task product creation above
			// used normal scoped commands. Deactivation must not invent a second plan.
			table, id, status := "agents", b["agent_id"], "revoked"
			if kind == "principal" {
				table, id, status = "principals", b["principal_id"], "suspended"
			}
			if _, err := db.Exec(context.Background(), "UPDATE undergrid."+table+" SET status=$2 WHERE id=$1", id, status); err != nil {
				t.Fatal(err)
			}
			if code, recovered := proposer.versioned("POST", path, "activity-first-plan", room["etag"].(string), spec); code != 201 || recovered["id"] != first["id"] {
				t.Fatalf("unrelated participant change invalidated own committed receipt:%d %#v", code, recovered)
			}
			_, snapshot := proposer.call("GET", roomPath+"/context", "", nil)
			parent := int64(1)
			spec.ParentVersion = &parent
			spec.Objective = "New revision must recheck the selected participant"
			code, next := proposer.versioned("POST", path, "activity-new-revision", snapshot["swarm"].(map[string]any)["etag"].(string), spec)
			if code < 400 {
				t.Fatalf("new plan accepted inactive selected %s: status=%d id=%v", kind, code, next["id"])
			}
			var plans, tasks, votes int
			if err := db.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM undergrid.plan_versions WHERE swarm_id=$1),(SELECT count(*) FROM undergrid.tasks t JOIN undergrid.plan_versions p ON p.id=t.plan_id WHERE p.swarm_id=$1),(SELECT count(*) FROM undergrid.approvals a JOIN undergrid.plan_versions p ON p.id=a.plan_id WHERE p.swarm_id=$1)`, room["id"]).Scan(&plans, &tasks, &votes); err != nil {
				t.Fatal(err)
			}
			if plans != 1 || tasks != 2 || votes != 0 {
				t.Fatalf("denied revision effects: plans=%d tasks=%d approvals=%d", plans, tasks, votes)
			}
		})
	}
}

// All participant records are created through normal owner/agent commands. These
// focused fixtures isolate current identity changes from the future browser path.
func planProposalPeerRoom(t *testing.T, role string) (*Service, *pgx.Conn, testClient, testClient, map[string]any, map[string]any, map[string]any) {
	t.Helper()
	s, db := ownerSwarmCreationFixture(t)
	enableTestTerms(t, s)
	a, b := planningClient(t, s), collaborationClient(t, s)
	_, aid := a.call("GET", "/v1/me", "", nil)
	_, bid := b.call("GET", "/v1/me", "", nil)
	code, room := a.call("POST", "/v1/swarms", "peer-eligibility-room", map[string]any{"objective": "Current selected plan participants", "visibility": "restricted", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	path := "/v1/swarms/" + room["id"].(string)
	code, invite := a.call("POST", path+"/invitations", "peer-eligibility-offer", map[string]any{"agent_id": bid["agent_id"], "role": role, "respond_by": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, invite)
	}
	if code, out := b.versioned("POST", path+"/membership-response", "peer-eligibility-join", invite["etag"].(string), map[string]any{"invitation_id": invite["id"], "decision": "accept"}); code != 200 {
		t.Fatal(code, out)
	}
	return s, db, a, b, aid, bid, room
}

func TestPlanProposalNonAssignedPrincipalsNeedAnActiveEligibleMember(t *testing.T) {
	for _, purpose := range []string{"required", "publication"} {
		for _, alternate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/alternate-active-%t", purpose, alternate), func(t *testing.T) {
				role := "contributor"
				if purpose == "publication" {
					role = "observer"
				}
				_, db, a, b, aid, bid, room := planProposalPeerRoom(t, role)
				path := "/v1/swarms/" + room["id"].(string)
				if alternate {
					code, issued := b.call("POST", "/v1/agents", "alternate-active-member", map[string]any{"name": "Alternative eligible attribution", "scopes": []string{"members:respond", "swarms:read"}, "profile": map[string]any{"accept_invitations": true}})
					if code != 201 {
						t.Fatal(code, issued)
					}
					alternative := b
					alternative.token = issued["token"].(string)
					code, invite := a.call("POST", path+"/invitations", "alternative-eligible-offer", map[string]any{"agent_id": issued["id"], "role": role, "respond_by": time.Now().Add(time.Hour)})
					if code != 201 {
						t.Fatal(code, invite)
					}
					if code, out := alternative.versioned("POST", path+"/membership-response", "alternative-eligible-join", invite["etag"].(string), map[string]any{"invitation_id": invite["id"], "decision": "accept"}); code != 200 {
						t.Fatal(code, out)
					}
				}
				if _, err := db.Exec(context.Background(), "UPDATE undergrid.agents SET status='revoked' WHERE id=$1", bid["agent_id"]); err != nil {
					t.Fatal(err)
				}
				spec := ownerPlanProposalSpec(aid["agent_id"].(string), aid["principal_id"].(string))
				if purpose == "required" {
					spec.RequiredPrincipals = append(spec.RequiredPrincipals, bid["principal_id"].(string))
				} else {
					spec.PublicationPrincipals = append(spec.PublicationPrincipals, bid["principal_id"].(string))
				}
				code, result := a.versioned("POST", path+"/plans", "non-assigned-principal-proposal", room["etag"].(string), spec)
				if alternate {
					if code != 201 {
						t.Fatalf("another revoked agent incorrectly disqualified active accepted witness:%d %#v", code, result)
					}
				} else if code < 400 {
					t.Fatalf("%s principal without active eligible accepted agent entered plan:%d", purpose, code)
				}
			})
		}
	}
}

func TestPlanProposalParticipantContentionRetainsRetryableRecovery(t *testing.T) {
	s, db, a, _, aid, bid, room := planProposalPeerRoom(t, "contributor")
	spec := ownerPlanProposalSpec(bid["agent_id"].(string), aid["principal_id"].(string))
	spec.RequiredPrincipals = append(spec.RequiredPrincipals, bid["principal_id"].(string))
	path := "/v1/swarms/" + room["id"].(string) + "/plans"
	locker, err := pgx.Connect(context.Background(), db.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer locker.Close(context.Background())
	held, err := locker.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Rollback(context.Background())
	if _, err = held.Exec(context.Background(), "SELECT id FROM undergrid.principals WHERE id=$1 FOR UPDATE", bid["principal_id"]); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r := httptest.NewRequest("POST", path, bytes.NewReader(canonical(spec))).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+a.token)
	r.Header.Set("Idempotency-Key", "contended-plan-original")
	r.Header.Set("If-Match", room["etag"].(string))
	response := httptest.NewRecorder()
	started := time.Now()
	s.ServeHTTP(response, r)
	var result map[string]any
	if err = json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != 500 || result["error"].(map[string]any)["retryable"] != true || time.Since(started) > 2*time.Second {
		t.Fatalf("NOWAIT contention lost existing retryable failure contract: %d %s elapsed=%s", response.Code, response.Body, time.Since(started))
	}
	var plans int
	if err = db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.plan_versions WHERE swarm_id=$1", room["id"]).Scan(&plans); err != nil || plans != 0 {
		t.Fatalf("contended command committed plan:%d %v", plans, err)
	}
	if err = held.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	code, recovered := a.versioned("POST", path, "contended-plan-original", room["etag"].(string), spec)
	if code != 201 {
		t.Fatalf("same-key retry after releasing authority contention:%d %#v", code, recovered)
	}
	if err = db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.plan_versions WHERE swarm_id=$1", room["id"]).Scan(&plans); err != nil || plans != 1 {
		t.Fatalf("recovered command multiplicity:%d %v", plans, err)
	}
}
