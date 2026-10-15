package network

import (
	"context"
	"github.com/riverqueue/river"
	"testing"
	"time"
)

func TestObsoleteWatchPairDoesNotWaitOnOwner(t *testing.T) {
	for _, scenario := range []string{"withdrawn_watcher", "private_candidate"} {
		t.Run(scenario, func(t *testing.T) {
			s, db := fixture(t)
			a, b := client(t, s), client(t, s)
			input := intentFixture()
			input.Visibility = "redacted_public"
			input.PublicSummary = "Obsolete pair " + NewID()
			code, watcher := a.call("POST", "/v1/intents", "obsolete-watch", input)
			if code != 201 {
				t.Fatal(code, watcher)
			}
			if scenario == "private_candidate" {
				input.Visibility = "private"
			}
			code, candidate := b.call("POST", "/v1/intents", "obsolete-candidate", input)
			if code != 201 {
				t.Fatal(code, candidate)
			}
			if scenario == "withdrawn_watcher" {
				code, changed := a.versioned("PATCH", "/v1/intents/"+watcher["id"].(string), "withdraw-before-pair", watcher["etag"].(string), map[string]any{"state": "withdrawn"})
				if code != 200 {
					t.Fatal(code, changed)
				}
			}
			_, owner := a.call("GET", "/v1/me", "", nil)
			ctx := context.Background()
			held, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer held.Rollback(ctx)
			if _, err = held.Exec(ctx, "SELECT id FROM undergrid.principals WHERE id=$1 FOR UPDATE", owner["principal_id"]); err != nil {
				t.Fatal(err)
			}
			bounded, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			worker := watchRefreshWorker{service: s}
			if err = worker.Work(bounded, &river.Job[watchRefreshArgs]{Args: watchRefreshArgs{WatcherID: watcher["id"].(string), CandidateID: candidate["id"].(string)}}); err != nil {
				t.Fatalf("obsolete pair waited on unrelated authority instead of completing without effects: %v", err)
			}
			var effects int
			if err = held.QueryRow(ctx, "SELECT count(*) FROM undergrid.opportunities WHERE source_intent_id=$1 AND candidate_intent_id=$2", watcher["id"], candidate["id"]).Scan(&effects); err != nil {
				t.Fatal(err)
			}
			if effects != 0 {
				t.Fatalf("obsolete pair produced %d opportunities", effects)
			}
		})
	}
}
