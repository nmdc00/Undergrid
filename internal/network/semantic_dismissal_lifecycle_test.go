package network

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestSemanticDismissalSurvivesWorkerRestartAndProfileCutover(t *testing.T) {
	originalProfile, _ := controlledSemanticConsent(t)
	s, db, worker, _ := externalNotificationFixture(t)
	t.Setenv("UNDERGRID_SOURCES_FILE", "")
	a, b := client(t, s), client(t, s)
	ids := []string{}
	created := []map[string]any{}
	inputs := []IntentInput{}
	for n, owner := range []testClient{a, b} {
		input := intentFixture()
		input.Visibility = "redacted_public"
		input.PublicSummary = []string{"Environmental regulation reference catalog", "Climate agency citation directory"}[n]
		code, out := owner.call("POST", "/v1/intents", NewID(), input)
		if code != 201 {
			t.Fatal(code, out)
		}
		ids = append(ids, out["id"].(string))
		created = append(created, out)
		inputs = append(inputs, input)
	}
	run := func(phase func()) {
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- RunWorker(ctx, worker.dsn, "development") }()
		defer func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(12 * time.Second):
				t.Error("semantic lifecycle worker failed bounded shutdown")
			}
		}()
		phase()
	}
	currentSuggestion := func() map[string]any {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			code, page := a.call("GET", "/v1/intents/"+ids[0]+"/opportunities", "", nil)
			if code != 200 {
				t.Fatal(code, page)
			}
			for _, raw := range page["items"].([]any) {
				item := raw.(map[string]any)
				if item["candidate_intent_id"] == ids[1] && item["evidence"].(map[string]any)["algorithm"] == "hard-constraints-hybrid-cosine-v1" {
					return item
				}
			}
			if time.Now().After(deadline) {
				t.Fatal("current semantic suggestion missing within existing bound", page)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	var oldID string
	var oldScan, dismissedAt time.Time
	run(func() {
		item := currentSuggestion()
		oldID = item["id"].(string)
		code, out := a.call("POST", "/v1/opportunities/"+oldID+"/dismiss", "runtime-dismiss", map[string]any{})
		if code != 200 {
			t.Fatal(code, out)
		}
		if err := db.QueryRow(context.Background(), "SELECT last_scan FROM undergrid.intents WHERE id=$1", ids[0]).Scan(&oldScan); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(context.Background(), "SELECT created_at FROM undergrid.events WHERE aggregate_id=$1 AND type='opportunity.dismissed'", oldID).Scan(&dismissedAt); err != nil {
			t.Fatal(err)
		}
	})
	// Pause/resume keeps the immutable input unchanged but commits real new scan
	// wakeups. A restarted worker must complete them without resurfacing dismissal.
	code, paused := a.versioned("PATCH", "/v1/intents/"+ids[0], "pause-dismissed-watch", created[0]["etag"].(string), map[string]any{"state": "paused"})
	if code != 200 {
		t.Fatal(code, paused)
	}
	code, resumed := a.versioned("PATCH", "/v1/intents/"+ids[0], "resume-dismissed-watch", paused["etag"].(string), map[string]any{"state": "active"})
	if code != 200 {
		t.Fatal(code, resumed)
	}
	run(func() {
		deadline := time.Now().Add(5 * time.Second)
		for {
			var scanned time.Time
			if err := db.QueryRow(context.Background(), "SELECT last_scan FROM undergrid.intents WHERE id=$1", ids[0]).Scan(&scanned); err != nil {
				t.Fatal(err)
			}
			if scanned.After(oldScan) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("restarted unchanged watch did not complete real new scan")
			}
			time.Sleep(20 * time.Millisecond)
		}
		code, page := a.call("GET", "/v1/intents/"+ids[0]+"/opportunities", "", nil)
		if code != 200 || len(page["items"].([]any)) != 0 {
			t.Fatal("unchanged dismissed evidence resurfaced", code, page)
		}
		code, out := a.call("POST", "/v1/opportunities/"+oldID+"/dismiss", "runtime-dismiss", map[string]any{})
		if code != 200 {
			t.Fatal("unchanged receipt lost after restart", code, out)
		}
	})
	var opportunities, events int
	var stillDismissedAt time.Time
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.opportunities WHERE source_intent_id=$1 AND candidate_intent_id=$2", ids[0], ids[1]).Scan(&opportunities); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*),min(created_at) FROM undergrid.events WHERE aggregate_id=$1 AND type='opportunity.dismissed'", oldID).Scan(&events, &stillDismissedAt); err != nil {
		t.Fatal(err)
	}
	if opportunities != 1 || events != 1 || !dismissedAt.Equal(stillDismissedAt) {
		t.Fatal("restart duplicated unchanged opportunity/receipt", opportunities, events)
	}
	configPath := os.Getenv("UNDERGRID_SEMANTIC_CONFIG_FILE")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err = json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	config["provider_id"] = "current-profile-cutover-fixture"
	if err = os.WriteFile(configPath, canonical(config), 0600); err != nil {
		t.Fatal(err)
	}
	currentProfile, err := semanticConfiguration("development")
	if err != nil || currentProfile.hash == originalProfile.hash {
		t.Fatal("profile fixture did not change", err)
	}
	code, out := a.call("POST", "/v1/opportunities/"+oldID+"/dismiss", "runtime-dismiss", map[string]any{})
	if code != 404 {
		t.Fatal("old profile receipt recovered", code, out)
	}
	for n, owner := range []testClient{a, b} {
		etag := created[n]["etag"].(string)
		if n == 0 {
			etag = resumed["etag"].(string)
		}
		code, out := owner.versioned("PATCH", "/v1/intents/"+ids[n], NewID(), etag, map[string]any{"revision": inputs[n]})
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	run(func() {
		item := currentSuggestion()
		if item["id"] == oldID || item["evidence"].(map[string]any)["semantic_profile_hash"] != currentProfile.hash {
			t.Fatal("profile change did not produce new pinned suggestion", item)
		}
		code, out := a.call("POST", "/v1/opportunities/"+item["id"].(string)+"/dismiss", "new-profile-dismiss", map[string]any{})
		if code != 200 {
			t.Fatal(code, out)
		}
	})
	var sameSourceInput bool
	if err = db.QueryRow(context.Background(), "SELECT (SELECT content_hash FROM undergrid.intent_revisions WHERE intent_id=$1 AND revision=1)=(SELECT content_hash FROM undergrid.intent_revisions WHERE intent_id=$1 AND revision=2)", ids[0]).Scan(&sameSourceInput); err != nil {
		t.Fatal(err)
	}
	if !sameSourceInput {
		t.Fatal("profile-cutover fixture changed source content instead of profile identity")
	}
}
