package network

import (
	"context"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestIntentionSatisfactionIsTerminalAndReplayable(t *testing.T) {
	for _, owner := range []bool{false, true} {
		t.Run(map[bool]string{false: "agent", true: "owner"}[owner], func(t *testing.T) {
			s, db, _ := satisfactionFixture(t)
			a, b := client(t, s), client(t, s)
			code, created := a.call("POST", "/v1/intents", "create-satisfied", intentFixture())
			if code != 201 {
				t.Fatal(code, created)
			}
			id := created["id"].(string)
			path := "/v1/intents/" + id
			if owner {
				path = "/v1/owner/intents/" + id
			}
			body := map[string]any{"state": "satisfied"}
			code, done := revisionCall(a, "PATCH", path, "satisfy", created["etag"].(string), body)
			if code != 200 || done["state"] != "satisfied" {
				t.Fatalf("satisfaction not implemented: %d %v", code, done)
			}
			if done["watch"].(map[string]any)["enabled"] != false {
				t.Fatal("satisfied watch still enabled")
			}
			if code, replay := revisionCall(a, "PATCH", path, "satisfy", created["etag"].(string), body); code != 200 || replay["etag"] != done["etag"] {
				t.Fatal("satisfaction replay lost", code, replay)
			}
			for _, mutation := range []map[string]any{{"state": "satisfied"}, {"state": "active"}, {"state": "paused"}, {"state": "withdrawn"}, {"revision": revisionInput("reopen satisfied")}} {
				if code, out := revisionCall(a, "PATCH", path, NewID(), done["etag"].(string), mutation); code != 409 {
					t.Fatal("terminal intention changed", code, out)
				}
			}
			if code, _ := revisionCall(b, "PATCH", path, "foreign-satisfy", done["etag"].(string), body); code != 404 {
				t.Fatal("foreign terminal state disclosed", code)
			}
			var state string
			var watch bool
			var revisions, events int
			if err := db.QueryRow(context.Background(), "SELECT state,watch_enabled,(SELECT count(*) FROM undergrid.intent_revisions WHERE intent_id=$1),(SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='intent.satisfied') FROM undergrid.intents WHERE id=$1", id).Scan(&state, &watch, &revisions, &events); err != nil {
				t.Fatal(err)
			}
			if state != "satisfied" || watch || revisions != 1 || events != 1 {
				t.Fatalf("terminal effects/history invalid: %s %v %d %d", state, watch, revisions, events)
			}
		})
	}
}

func TestPublicIntentionSatisfactionRemovesMatchWithoutErasingEvidence(t *testing.T) {
	for _, initial := range []string{"active", "paused"} {
		t.Run(initial, func(t *testing.T) {
			s, db, worker := satisfactionFixture(t)
			a, b := client(t, s), client(t, s)
			input := intentFixture()
			input.Visibility = "redacted_public"
			input.PublicSummary = "Satisfaction catalog " + NewID()
			_, source := a.call("POST", "/v1/intents", "watch-satisfaction", input)
			_, candidate := b.call("POST", "/v1/intents", "candidate-satisfaction", input)
			id := candidate["id"].(string)
			if err := worker.scanIntentions(context.Background(), source["id"].(string)); err != nil {
				t.Fatal(err)
			}
			_, before := a.call("GET", "/v1/intents/"+source["id"].(string)+"/opportunities", "", nil)
			found := false
			for _, entry := range before["items"].([]any) {
				if entry.(map[string]any)["candidate_intent_id"] == id {
					found = true
				}
			}
			if !found {
				t.Fatal("candidate never matched")
			}
			current := candidate
			if initial == "paused" {
				code, paused := revisionCall(b, "PATCH", "/v1/intents/"+id, "pause-satisfaction", candidate["etag"].(string), map[string]any{"state": "paused"})
				if code != 200 {
					t.Fatal(code, paused)
				}
				current = paused
			}
			code, done := revisionCall(b, "PATCH", "/v1/intents/"+id, "finish-satisfaction", current["etag"].(string), map[string]any{"state": "satisfied"})
			if code != 200 || done["state"] != "satisfied" {
				t.Fatal("satisfaction denied", code, done)
			}
			if code, _ := a.call("GET", "/v1/intents/"+id, "", nil); code != 404 {
				t.Fatal("satisfied public candidate remains visible", code)
			}
			_, after := a.call("GET", "/v1/intents/"+source["id"].(string)+"/opportunities", "", nil)
			for _, entry := range after["items"].([]any) {
				if entry.(map[string]any)["candidate_intent_id"] == id {
					t.Fatal("satisfied candidate remained eligible")
				}
			}
			var evidence int
			if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.opportunities WHERE source_intent_id=$1 AND candidate_intent_id=$2", source["id"], id).Scan(&evidence); err != nil {
				t.Fatal(err)
			}
			if evidence != 1 {
				t.Fatal("satisfaction erased match history", evidence)
			}
		})
	}
}

func satisfactionFixture(t *testing.T) (*Service, *pgx.Conn, *Service) {
	t.Helper()
	s, db := fixture(t)
	if _, err := db.Exec(context.Background(), `DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='undergrid_satisfaction_api_runner') THEN CREATE ROLE undergrid_satisfaction_api_runner LOGIN NOSUPERUSER NOBYPASSRLS; END IF;
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='undergrid_satisfaction_worker_runner') THEN CREATE ROLE undergrid_satisfaction_worker_runner LOGIN NOSUPERUSER NOBYPASSRLS; END IF;
 END $$; GRANT undergrid_app TO undergrid_satisfaction_api_runner; GRANT undergrid_worker TO undergrid_satisfaction_worker_runner;`); err != nil {
		t.Fatal(err)
	}
	s.dsn = networkFixtureRoleDSN(t, db, "undergrid_satisfaction_api_runner")
	worker := New(networkFixtureRoleDSN(t, db, "undergrid_satisfaction_worker_runner"), "development", nil)
	return s, db, worker
}
