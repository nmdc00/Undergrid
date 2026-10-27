package network

import (
	"context"
	"testing"
	"time"
)

// The proposed replacement must never inherit an accepted plan's authority or
// invalidate its commitments merely because the replacement is rejected/expired.
func TestOwnerPlanUnacceptedRevisionPreservesAcceptedCommitments(t *testing.T) {
	for _, outcome := range []string{"rejected", "expired"} {
		t.Run(outcome, func(t *testing.T) {
			s, db := ownerSwarmCreationFixture(t)
			enableTestTerms(t, s)
			owner := planningClient(t, s)
			_, identity := owner.call("GET", "/v1/me", "", nil)
			agent, principal := identity["agent_id"].(string), identity["principal_id"].(string)
			base := "/v1/owner/agents/" + agent
			code, room := ownerCallMatch(owner, "POST", base+"/swarms", "lifecycle-room", "", map[string]any{"objective": "Preserve accepted work across unaccepted revisions", "visibility": "restricted", "history_policy": "full", "recruitment_roles": []any{}})
			if code != 201 {
				t.Fatal(code, room)
			}
			swarm := room["id"].(string)
			proposalPath := base + "/swarms/" + swarm + "/plans"
			spec := ownerPlanProposalSpec(agent, principal)
			code, first := ownerCallMatch(owner, "POST", proposalPath, "accepted-original", room["etag"].(string), spec)
			if code != 201 {
				t.Fatal(code, first)
			}
			vote := map[string]any{"decision": "approve", "content_hash": first["content_hash"], "version": first["version"], "acknowledge_contribution_terms": true}
			if code, out := ownerCallMatch(owner, "POST", base+"/plans/"+first["id"].(string)+"/approvals", "original-vote", "", vote); code != 200 {
				t.Fatal(code, out)
			}
			for _, raw := range first["tasks"].([]any) {
				task := raw.(map[string]any)
				body := map[string]any{"decision": "accept", "plan_hash": first["content_hash"], "acknowledge_responsibility_and_terms": true}
				if code, out := ownerCallMatch(owner, "POST", base+"/tasks/"+task["id"].(string)+"/assignment-response", NewID(), task["etag"].(string), body); code != 200 {
					t.Fatal(code, out)
				}
			}
			ctx := context.Background()
			snapshot := func() string {
				t.Helper()
				var raw string
				if err := db.QueryRow(ctx, `SELECT jsonb_build_object('head',s.active_plan_id,'plan',to_jsonb(p),'tasks',(SELECT jsonb_agg(to_jsonb(t) ORDER BY t.id) FROM undergrid.tasks t WHERE t.plan_id=p.id))::text FROM undergrid.swarms s JOIN undergrid.plan_versions p ON p.id=s.active_plan_id WHERE s.id=$1`, swarm).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				return raw
			}
			before := snapshot()
			var initialState string
			if err := db.QueryRow(ctx, "SELECT state FROM undergrid.plan_versions WHERE id=$1", first["id"]).Scan(&initialState); err != nil || initialState != "accepted" {
				t.Fatal(initialState, err)
			}
			code, current := ownerCallMatch(owner, "GET", base+"/swarms/"+swarm+"/plan-context", "", "", nil)
			if code != 200 {
				t.Fatal(code, current)
			}
			parent := int64(1)
			spec.ParentVersion = &parent
			spec.Objective = "An independently decided replacement that never becomes accepted"
			spec.RespondBy = time.Now().UTC().Add(2 * time.Minute)
			code, next := ownerCallMatch(owner, "POST", proposalPath, "unaccepted-replacement", current["swarm"].(map[string]any)["etag"].(string), spec)
			if code != 201 {
				t.Fatal(code, next)
			}
			if got := snapshot(); got != before {
				t.Fatalf("proposal changed accepted commitments: before=%s after=%s", before, got)
			}
			if outcome == "rejected" {
				body := map[string]any{"decision": "reject", "content_hash": next["content_hash"], "version": next["version"]}
				for range 2 {
					if code, out := ownerCallMatch(owner, "POST", base+"/plans/"+next["id"].(string)+"/approvals", "reject-replacement", "", body); code != 200 {
						t.Fatal(code, out)
					}
				}
			} else {
				// Exercise the production sweep clock argument without rewriting deadlines,
				// plan specifications or product state. Old progress deadlines are 24h away.
				for range 2 {
					if err := s.SweepCoordinationDeadlines(ctx, spec.RespondBy.Add(time.Second)); err != nil {
						t.Fatal(err)
					}
				}
			}
			if got := snapshot(); got != before {
				t.Fatalf("%s revision changed accepted commitments: before=%s after=%s", outcome, before, got)
			}
			var state string
			var approved, rejected, events int
			if err := db.QueryRow(ctx, `SELECT p.state,(SELECT count(*) FROM undergrid.approvals WHERE plan_id=p.id AND decision='approve'),(SELECT count(*) FROM undergrid.approvals WHERE plan_id=p.id AND decision='reject'),(SELECT count(*) FROM undergrid.events WHERE aggregate_id=p.id AND type=$2) FROM undergrid.plan_versions p WHERE p.id=$1`, next["id"], "plan."+outcome).Scan(&state, &approved, &rejected, &events); err != nil {
				t.Fatal(err)
			}
			expectedRejections := 0
			if outcome == "rejected" {
				expectedRejections = 2
			}
			if state != outcome || approved != 0 || rejected != expectedRejections || events != 1 {
				t.Fatalf("terminal revision effects: state=%s approved=%d rejected=%d events=%d", state, approved, rejected, events)
			}
			late := map[string]any{"decision": "approve", "content_hash": next["content_hash"], "version": next["version"], "acknowledge_contribution_terms": true}
			if code, out := ownerCallMatch(owner, "POST", base+"/plans/"+next["id"].(string)+"/approvals", "late-replacement-vote", "", late); code != 409 {
				t.Fatalf("terminal revision accepted a new vote:%d %v", code, out)
			}
			if got := snapshot(); got != before {
				t.Fatalf("late denied vote changed accepted commitments: before=%s after=%s", before, got)
			}
		})
	}
}
