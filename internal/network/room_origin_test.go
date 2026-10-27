package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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

// Controlled HTTPS data exercises both real adapters and restricted worker writes;
// it is not source or provider qualification.
func TestRoomOriginExternalKindsRequireCurrentSourceBeforeReceiptRecovery(t *testing.T) {
	for _, kind := range []string{"github_requests", "capability_manifest"} {
		t.Run(kind, func(t *testing.T) {
			s, db, worker, _ := externalNotificationFixture(t)
			owner := collaborationClient(t, s)
			_, me := owner.call("GET", "/v1/me", "", nil)
			input := intentFixture()
			input.Objective = "Room external provenance " + NewID()
			code, source := owner.call("POST", "/v1/intents", NewID(), input)
			if code != 201 {
				t.Fatal(code, source)
			}
			publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "github_requests" {
					fmt.Fprintf(w, `[{"id":271,"html_url":"https://github.com/fixture/project/issues/271","title":%q,"updated_at":"2026-10-23T00:00:00Z","state":"open","labels":[{"name":"undergrid"}]}]`, input.Objective)
					return
				}
				fmt.Fprintf(w, `{"items":[{"id":"lead","version":"1","url":"https://publisher.example/lead","title":%q,"status":"active"}],"next_page":0}`, input.Objective)
			}))
			defer publisher.Close()
			cfg := SourceConfig{ID: NewID(), Kind: kind, Endpoint: "https://publisher.example/feed", Publisher: "Controlled publisher", AccessBasis: "fixture access", RepublicationBasis: "fixture redistribution", TombstoneBasis: "fixture tombstone", PublicPermissionNotice: "Permitted title and URL", Fields: []string{"canonical_url", "title"}}
			if kind == "github_requests" {
				cfg.Endpoint = "https://api.github.com/repos/fixture/project/issues"
				cfg.Labels = []string{"undergrid"}
			}
			refresher := externalNotificationRefresher(t, worker.dsn, cfg, publisher)
			if err := refresher.RefreshDue(context.Background()); err != nil {
				t.Fatal(err)
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
				if row["kind"] == "external_capability" || row["kind"] == "external_request" {
					origin = row["id"].(string)
				}
			}
			if origin == "" {
				t.Fatal("actual external origin absent", page)
			}
			x := &roomOriginFixtureState{service: s, worker: worker, db: db, owner: owner, agent: me["agent_id"].(string), principal: me["principal_id"].(string), origin: origin}
			body := x.body()
			code, room := x.create(t, true, "external-origin-retry", body)
			if code != 201 {
				t.Fatal("current external origin denied", code, room)
			}
			code, replay := x.create(t, true, "external-origin-retry", body)
			if code != 201 || !reflect.DeepEqual(room, replay) {
				t.Fatal("current external receipt failed", code, replay)
			}
			if _, err := db.Exec(context.Background(), `UPDATE undergrid.external_sources SET config=jsonb_set(config,'{PublicPermissionNotice}','""') WHERE id=$1`, cfg.ID); err != nil {
				t.Fatal(err)
			}
			before := x.effects(t)
			for _, key := range []string{"external-origin-retry", "external-origin-new"} {
				code, out := x.create(t, true, key, body)
				if code != 404 {
					t.Errorf("removed source permission accepted room or receipt: %d %v", code, out)
				}
			}
			if after := x.effects(t); !reflect.DeepEqual(before, after) {
				t.Fatal("ineligible external origin committed effects", before, after)
			}
		})
	}
}

func TestRoomOriginRecruitmentRequiresCurrentRoomAndRecipientProfile(t *testing.T) {
	for _, change := range []string{"archived-room", "private-room", "revoked-coordinator", "recipient-optout", "changed-role"} {
		t.Run(change, func(t *testing.T) {
			s, db, worker, _ := externalNotificationFixture(t)
			organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
			role := RecruitmentRole{Key: "builder", SchemaFamily: "source_catalog_v1", Description: "Build the declared catalog"}
			code, source := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Recruitment provenance " + NewID(), "visibility": "restricted", "recruitment_roles": []RecruitmentRole{role}})
			if code != 201 {
				t.Fatal(code, source)
			}
			_, me := builder.call("GET", "/v1/me", "", nil)
			workerDB, err := pgx.Connect(context.Background(), worker.dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer workerDB.Close(context.Background())
			if _, err = workerDB.Exec(context.Background(), "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
				t.Fatal(err)
			}
			if _, err = worker.recruitOne(context.Background(), workerDB, source["id"].(string), role, me["agent_id"].(string), me["principal_id"].(string)); err != nil {
				t.Fatal(err)
			}
			var origin string
			if err = db.QueryRow(context.Background(), "SELECT id FROM undergrid.opportunities WHERE source_swarm_id=$1 AND recipient_agent_id=$2", source["id"], me["agent_id"]).Scan(&origin); err != nil {
				t.Fatal(err)
			}
			x := &roomOriginFixtureState{service: s, worker: worker, db: db, owner: builder, agent: me["agent_id"].(string), principal: me["principal_id"].(string), origin: origin}
			body := x.body()
			code, room := x.create(t, true, "recruit-origin-retry", body)
			if code != 201 {
				t.Fatal(code, room)
			}
			switch change {
			case "archived-room":
				_, err = db.Exec(context.Background(), "UPDATE undergrid.swarms SET state='archived' WHERE id=$1", source["id"])
			case "private-room":
				_, err = db.Exec(context.Background(), "UPDATE undergrid.swarms SET visibility='private' WHERE id=$1", source["id"])
			case "revoked-coordinator":
				_, err = db.Exec(context.Background(), "UPDATE undergrid.agents SET status='revoked' WHERE id=(SELECT coordinator_agent_id FROM undergrid.swarms WHERE id=$1)", source["id"])
			case "recipient-optout":
				_, err = db.Exec(context.Background(), `UPDATE undergrid.agents SET profile=jsonb_set(profile,'{recruitment}','false') WHERE id=$1`, me["agent_id"])
			case "changed-role":
				_, err = db.Exec(context.Background(), `UPDATE undergrid.swarms SET recruitment_roles=jsonb_set(recruitment_roles,'{0,description}','"Changed role"') WHERE id=$1`, source["id"])
			}
			if err != nil {
				t.Fatal(err)
			}
			before := x.effects(t)
			for _, key := range []string{"recruit-origin-retry", "recruit-origin-new"} {
				code, out := x.create(t, true, key, body)
				if code != 404 {
					t.Errorf("stale recruitment %s accepted room or receipt: %d %v", change, code, out)
				}
			}
			if after := x.effects(t); !reflect.DeepEqual(before, after) {
				t.Fatal("stale recruitment committed effects", before, after)
			}
			var members int
			if err = db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.memberships WHERE swarm_id=$1 AND agent_id=$2", source["id"], me["agent_id"]).Scan(&members); err != nil || members != 0 {
				t.Fatal("origin admitted builder to source room", members, err)
			}
		})
	}
}

func TestRoomOriginSemanticEvidenceRemainsBoundToCurrentProcessingProfile(t *testing.T) {
	for _, visibility := range []string{"redacted_public", "private"} {
		t.Run(visibility, func(t *testing.T) {
			provider, _ := controlledSemanticConsent(t)
			s, db, worker, _ := externalNotificationFixture(t)
			worker.semantic = provider
			owner, peer := collaborationClient(t, s), collaborationClient(t, s)
			_, me := owner.call("GET", "/v1/me", "", nil)
			inputs := []IntentInput{intentFixture(), intentFixture()}
			inputs[0].Visibility = visibility
			inputs[0].PublicSummary = "Environmental regulation reference catalog"
			if visibility == "private" {
				inputs[0].PublicSummary = ""
				inputs[0].PrivateSemanticQuery = &PrivateSemanticQuery{Projection: "Environmental regulation reference catalog", ProfileHash: provider.hash, AcknowledgeExternalProcessing: true}
			}
			inputs[1].Visibility = "redacted_public"
			inputs[1].PublicSummary = "Climate agency citation directory"
			ids := []string{}
			for n, c := range []testClient{owner, peer} {
				code, out := c.call("POST", "/v1/intents", NewID(), inputs[n])
				if code != 201 {
					t.Fatal(code, out)
				}
				ids = append(ids, out["id"].(string))
			}
			conn, err := pgx.Connect(context.Background(), worker.dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close(context.Background())
			if _, err = conn.Exec(context.Background(), "SET ROLE undergrid_worker;SET search_path=undergrid,pg_catalog"); err != nil {
				t.Fatal(err)
			}
			for _, id := range ids {
				if err = worker.ensureSemanticEmbedding(context.Background(), conn, id); err != nil {
					t.Fatal(err)
				}
			}
			if err = worker.scanIntentCandidate(context.Background(), conn, ids[0], ids[1], true); err != nil {
				t.Fatal(err)
			}
			code, page := owner.call("GET", "/v1/intents/"+ids[0]+"/opportunities", "", nil)
			if code != 200 {
				t.Fatal(code, page)
			}
			origin := ""
			for _, raw := range page["items"].([]any) {
				row := raw.(map[string]any)
				if row["candidate_intent_id"] == ids[1] && row["evidence"].(map[string]any)["algorithm"] == "hard-constraints-hybrid-cosine-v1" {
					origin = row["id"].(string)
				}
			}
			if origin == "" {
				t.Fatal("genuine current semantic suggestion missing", page)
			}
			x := &roomOriginFixtureState{service: s, worker: worker, db: db, owner: owner, agent: me["agent_id"].(string), principal: me["principal_id"].(string), origin: origin}
			body := x.body()
			code, room := x.create(t, true, "semantic-origin-retry", body)
			if code != 201 {
				t.Fatal(code, room)
			}
			code, replay := x.create(t, true, "semantic-origin-retry", body)
			if code != 201 || !reflect.DeepEqual(room, replay) {
				t.Fatal("current semantic receipt failed", code, replay)
			}
			path := os.Getenv("UNDERGRID_SEMANTIC_CONFIG_FILE")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var cfg map[string]any
			if err = json.Unmarshal(raw, &cfg); err != nil {
				t.Fatal(err)
			}
			cfg["provider_id"] = "room-current-profile-cutover"
			if err = os.WriteFile(path, canonical(cfg), 0600); err != nil {
				t.Fatal(err)
			}
			before := x.effects(t)
			for _, key := range []string{"semantic-origin-retry", "semantic-origin-new"} {
				code, out := x.create(t, true, key, body)
				if code != 404 {
					t.Errorf("obsolete semantic processing profile accepted room or receipt: %d %v", code, out)
				}
			}
			if after := x.effects(t); !reflect.DeepEqual(before, after) {
				t.Fatal("obsolete semantic evidence committed effects", before, after)
			}
		})
	}
}

func TestRoomOriginCurrentForeignAuthorityMustRemainLockedThroughCreationAndReplay(t *testing.T) {
	for _, retained := range []bool{false, true} {
		name := "new"
		if retained {
			name = "retained"
		}
		t.Run(name, func(t *testing.T) {
			x := roomOriginFixture(t)
			body := x.body()
			if retained {
				code, out := x.create(t, true, "authority-locked-origin", body)
				if code != 201 {
					t.Fatal(code, out)
				}
			}
			// Use a dedicated administrator connection only to stage an uncommitted
			// revocation. The command itself still uses the restricted application role.
			adminConfig := x.db.Config().Copy()
			admin, err := pgx.ConnectConfig(context.Background(), adminConfig)
			if err != nil {
				t.Fatal(err)
			}
			defer admin.Close(context.Background())
			tx, err := admin.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			if _, err = tx.Exec(context.Background(), "UPDATE undergrid.agents SET status='revoked' WHERE id=$1", x.candidateAgent); err != nil {
				t.Fatal(err)
			}
			before := x.effects(t)
			code, out := x.create(t, true, "authority-locked-origin", body)
			if code != 500 || out["retryable"] != true {
				t.Errorf("foreign authority contention did not preserve retryable rollback: %d %v", code, out)
			}
			if after := x.effects(t); !reflect.DeepEqual(before, after) {
				t.Fatal("contended origin committed effects", before, after)
			}
			if err = tx.Rollback(context.Background()); err != nil {
				t.Fatal(err)
			}
			code, out = x.create(t, true, "authority-locked-origin", body)
			if code != 201 {
				t.Fatal("same-key retry after authority rollback failed", code, out)
			}
			changed := x.body()
			changed["objective"] = "A genuinely different reviewed objective"
			before = x.effects(t)
			code, out = x.create(t, true, "authority-locked-origin", changed)
			if code != 409 {
				t.Fatal("changed retained command did not conflict", code, out)
			}
			if after := x.effects(t); !reflect.DeepEqual(before, after) {
				t.Fatal("changed replay committed effects", before, after)
			}
		})
	}
}
