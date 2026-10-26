package network

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
)

type roomOriginFixtureState struct {
	service, worker                  *Service
	db                               *pgx.Conn
	owner, peer                      testClient
	agent, principal, candidateAgent string
	input                            IntentInput
	source, candidate                map[string]any
	origin                           string
}

func roomOriginFixture(t *testing.T) *roomOriginFixtureState {
	t.Helper()
	s, db, worker, owner := externalNotificationFixture(t)
	t.Setenv("UNDERGRID_SEMANTIC_CONFIG_FILE", "")
	code, registered := owner.call("POST", "/v1/agents", NewID(), map[string]any{"name": "Room provenance owner", "scopes": []string{"intents:read", "intents:write", "inbox:read", "swarms:create"}})
	if code != 201 {
		t.Fatal(code, registered)
	}
	owner.token = registered["token"].(string)
	code, identity := owner.call("GET", "/v1/me", "", nil)
	if code != 200 {
		t.Fatal(code, identity)
	}
	peer := client(t, s)
	_, peerIdentity := peer.call("GET", "/v1/me", "", nil)
	input := intentFixture()
	input.Objective = "Current room provenance " + NewID()
	code, source := owner.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, source)
	}
	public := input
	public.Visibility = "redacted_public"
	public.PublicSummary = input.Objective
	code, candidate := peer.call("POST", "/v1/intents", NewID(), public)
	if code != 201 {
		t.Fatal(code, candidate)
	}
	if err := worker.ScanDueIntentions(context.Background()); err != nil {
		t.Fatal(err)
	}
	code, page := owner.call("GET", "/v1/intents/"+source["id"].(string)+"/opportunities", "", nil)
	if code != 200 {
		t.Fatal(code, page)
	}
	origin := ""
	for _, raw := range page["items"].([]any) {
		row := raw.(map[string]any)
		if row["candidate_intent_id"] == candidate["id"] {
			origin = row["id"].(string)
		}
	}
	if origin == "" {
		t.Fatal("actual current shared-demand origin missing", page)
	}
	return &roomOriginFixtureState{s, worker, db, owner, peer, identity["agent_id"].(string), identity["principal_id"].(string), peerIdentity["agent_id"].(string), input, source, candidate, origin}
}
func (x *roomOriginFixtureState) body() map[string]any {
	return map[string]any{"objective": "Reviewed room from current need", "visibility": "restricted", "history_policy": "full", "origin_opportunity_id": x.origin, "recruitment_roles": []RecruitmentRole{{Key: "builder", SchemaFamily: "source_catalog_v1", Description: "Build the declared source catalog"}}}
}
func (x *roomOriginFixtureState) create(t *testing.T, owner bool, key string, body map[string]any) (int, map[string]any) {
	t.Helper()
	if owner {
		return ownerCallMatch(x.owner, "POST", "/v1/owner/agents/"+x.agent+"/swarms", key, "", body)
	}
	input := map[string]any{}
	for k, v := range body {
		if k != "history_policy" {
			input[k] = v
		}
	}
	return x.owner.call("POST", "/v1/swarms", key, input)
}
func (x *roomOriginFixtureState) effects(t *testing.T) []int64 {
	t.Helper()
	out := make([]int64, 5)
	err := x.db.QueryRow(context.Background(), `SELECT
 (SELECT count(*) FROM undergrid.swarms WHERE creator_principal_id=$1),
 (SELECT count(*) FROM undergrid.memberships m JOIN undergrid.swarms s ON s.id=m.swarm_id WHERE s.creator_principal_id=$1),
 (SELECT count(*) FROM undergrid.events WHERE principal_id=$1 AND type='swarm.created'),
 (SELECT count(*) FROM undergrid.inbox i JOIN undergrid.events e ON e.id=i.event_id WHERE e.principal_id=$1 AND e.type='swarm.created'),
 (SELECT count(*) FROM undergrid_queue.river_job)`, x.principal).Scan(&out[0], &out[1], &out[2], &out[3], &out[4])
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestRoomOriginRejectsRetainedIneligibleSharedEvidenceBeforeCreation(t *testing.T) {
	for _, ownerLane := range []bool{true, false} {
		lane := "agent"
		if ownerLane {
			lane = "owner"
		}
		for _, change := range []string{"dismissed", "source-withdrawn", "candidate-withdrawn", "source-revised", "candidate-revised", "candidate-agent-revoked"} {
			t.Run(lane+"/"+change, func(t *testing.T) {
				x := roomOriginFixture(t)
				switch change {
				case "dismissed":
					code, out := x.owner.call("POST", "/v1/opportunities/"+x.origin+"/dismiss", NewID(), map[string]any{})
					if code != 200 {
						t.Fatal(code, out)
					}
				case "source-withdrawn":
					code, out := x.owner.versioned("PATCH", "/v1/intents/"+x.source["id"].(string), NewID(), x.source["etag"].(string), map[string]any{"state": "withdrawn"})
					if code != 200 {
						t.Fatal(code, out)
					}
				case "candidate-withdrawn":
					code, out := x.peer.versioned("PATCH", "/v1/intents/"+x.candidate["id"].(string), NewID(), x.candidate["etag"].(string), map[string]any{"state": "withdrawn"})
					if code != 200 {
						t.Fatal(code, out)
					}
				case "source-revised":
					input := x.input
					input.Objective = "Changed current source " + NewID()
					code, out := x.owner.versioned("PATCH", "/v1/intents/"+x.source["id"].(string), NewID(), x.source["etag"].(string), map[string]any{"revision": input})
					if code != 200 {
						t.Fatal(code, out)
					}
				case "candidate-revised":
					input := x.input
					input.Visibility = "redacted_public"
					input.PublicSummary = "Different current candidate " + NewID()
					code, out := x.peer.versioned("PATCH", "/v1/intents/"+x.candidate["id"].(string), NewID(), x.candidate["etag"].(string), map[string]any{"revision": input})
					if code != 200 {
						t.Fatal(code, out)
					}
				case "candidate-agent-revoked":
					if _, err := x.db.Exec(context.Background(), "UPDATE undergrid.agents SET status='revoked' WHERE id=$1", x.candidateAgent); err != nil {
						t.Fatal(err)
					}
				}
				before := x.effects(t)
				code, out := x.create(t, ownerLane, NewID(), x.body())
				if code != 404 {
					t.Errorf("retained %s origin accepted through %s command: %d %v", change, lane, code, out)
				}
				if after := x.effects(t); !reflect.DeepEqual(before, after) {
					t.Errorf("denied origin committed room/membership/event/inbox/jobs: before%v after%v", before, after)
				}
			})
		}
	}
}
func TestRoomOriginCurrentEligibilityPrecedesRetainedResponseRecovery(t *testing.T) {
	for _, ownerLane := range []bool{true, false} {
		lane := "agent"
		if ownerLane {
			lane = "owner"
		}
		t.Run(lane, func(t *testing.T) {
			x := roomOriginFixture(t)
			body := x.body()
			code, room := x.create(t, ownerLane, "origin-current-retry", body)
			if code != 201 {
				t.Fatal(code, room)
			}
			code, replay := x.create(t, ownerLane, "origin-current-retry", body)
			if code != 201 || !reflect.DeepEqual(room, replay) {
				t.Fatal("unchanged exact room retry failed", code, replay)
			}
			code, out := x.peer.versioned("PATCH", "/v1/intents/"+x.candidate["id"].(string), NewID(), x.candidate["etag"].(string), map[string]any{"state": "withdrawn"})
			if code != 200 {
				t.Fatal(code, out)
			}
			before := x.effects(t)
			code, out = x.create(t, ownerLane, "origin-current-retry", body)
			if code != 404 {
				t.Errorf("obsolete origin recovered retained room: %d %v", code, out)
			}
			if after := x.effects(t); !reflect.DeepEqual(before, after) {
				t.Errorf("stale origin recovery changed effects: %v %v", before, after)
			}
		})
	}
}
func TestRoomOriginVisibilityDoesNotInventSharedMonitoringRequirement(t *testing.T) {
	x := roomOriginFixture(t)
	// Controlled runtime state, not a browser product seed: the current GET must
	// remain visible before this positive provenance command is exercised.
	if _, err := x.db.Exec(context.Background(), "UPDATE undergrid.intents SET watch_enabled=false WHERE id=$1", x.source["id"]); err != nil {
		t.Fatal(err)
	}
	code, page := x.owner.call("GET", "/v1/intents/"+x.source["id"].(string)+"/opportunities", "", nil)
	if code != 200 {
		t.Fatal(code, page)
	}
	found := false
	for _, raw := range page["items"].([]any) {
		if raw.(map[string]any)["id"] == x.origin {
			found = true
		}
	}
	if !found {
		t.Fatal("fixture does not represent a visible current unwatched origin", page)
	}
	code, room := x.create(t, true, "unwatched-current-origin", x.body())
	if code != 201 {
		t.Fatal("visible shared origin must not require monitoring", code, room)
	}
	var savedOrigin string
	var roles []byte
	var members int
	if err := x.db.QueryRow(context.Background(), "SELECT origin_opportunity_id::text,recruitment_roles,(SELECT count(*) FROM undergrid.memberships WHERE swarm_id=s.id AND state='accepted') FROM undergrid.swarms s WHERE id=$1", room["id"]).Scan(&savedOrigin, &roles, &members); err != nil {
		t.Fatal(err)
	}
	var saved []RecruitmentRole
	if err := json.Unmarshal(roles, &saved); err != nil {
		t.Fatal(err)
	}
	if savedOrigin != x.origin || !reflect.DeepEqual(saved, x.body()["recruitment_roles"]) || members != 1 {
		t.Fatal("room provenance/roles or admission boundary changed", savedOrigin, saved, members)
	}
}
func TestRoomOriginForeignAndObjectiveOnlyControlsRemainDistinct(t *testing.T) {
	x := roomOriginFixture(t)
	foreign := client(t, x.service)
	_, identity := foreign.call("GET", "/v1/me", "", nil)
	code, out := ownerCallMatch(foreign, "POST", "/v1/owner/agents/"+identity["agent_id"].(string)+"/swarms", NewID(), "", x.body())
	if code != 404 {
		t.Fatal("foreign origin accepted", code, out)
	}
	body := x.body()
	delete(body, "origin_opportunity_id")
	code, room := x.create(t, true, "objective-only-retry", body)
	if code != 201 {
		t.Fatal(code, room)
	}
	before := x.effects(t)
	code, replay := x.create(t, true, "objective-only-retry", body)
	if code != 201 || !reflect.DeepEqual(room, replay) {
		t.Fatal("objective-only retry changed", code, replay)
	}
	if after := x.effects(t); !reflect.DeepEqual(before, after) {
		t.Fatal("objective-only retry duplicated effects", before, after)
	}
}
