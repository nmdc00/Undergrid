package network

import (
	"context"
	"fmt"
	"testing"
)

func TestWatchResumeRestoresSavedPreference(t *testing.T) {
	for _, owner := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("owner=%v/enabled=%v", owner, enabled), func(t *testing.T) {
				s, db, _ := satisfactionFixture(t)
				a := client(t, s)
				input := intentFixture()
				input.Watch.Enabled = enabled
				code, created := a.call("POST", "/v1/intents", "create-watch", input)
				if code != 201 {
					t.Fatal(code, created)
				}
				id := created["id"].(string)
				path := "/v1/intents/" + id
				if owner {
					path = "/v1/owner/intents/" + id
				}
				code, paused := revisionCall(a, "PATCH", path, "pause-watch", created["etag"].(string), map[string]any{"state": "paused"})
				if code != 200 || paused["watch"].(map[string]any)["enabled"] != false {
					t.Fatal("pause did not stop monitoring", code, paused)
				}
				code, resumed := revisionCall(a, "PATCH", path, "resume-watch", paused["etag"].(string), map[string]any{"state": "active"})
				if code != 200 || resumed["watch"].(map[string]any)["enabled"] != enabled {
					t.Fatal("resume lost saved preference", code, resumed)
				}
				code, replayed := revisionCall(a, "PATCH", path, "resume-watch", paused["etag"].(string), map[string]any{"state": "active"})
				if code != 200 || replayed["etag"] != resumed["etag"] {
					t.Fatal("resume replay changed result", code, replayed)
				}
				var watch bool
				var revisions, events int
				err := db.QueryRow(context.Background(), `SELECT watch_enabled,(SELECT count(*) FROM undergrid.intent_revisions WHERE intent_id=$1),(SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='intent.active') FROM undergrid.intents WHERE id=$1`, id).Scan(&watch, &revisions, &events)
				if err != nil || watch != enabled || revisions != 1 || events != 1 {
					t.Fatal("invalid resume effects", watch, revisions, events, err)
				}
			})
		}
	}
}

func TestWatchResumeUsesRevisionSavedWhilePaused(t *testing.T) {
	s, db, _ := satisfactionFixture(t)
	a := client(t, s)
	input := intentFixture()
	input.Watch.Enabled = false
	code, created := a.call("POST", "/v1/intents", "create-revised-watch", input)
	if code != 201 {
		t.Fatal(code, created)
	}
	id := created["id"].(string)
	path := "/v1/intents/" + id
	code, paused := revisionCall(a, "PATCH", path, "pause-revised-watch", created["etag"].(string), map[string]any{"state": "paused"})
	if code != 200 {
		t.Fatal(code, paused)
	}
	input.Watch.Enabled = true
	code, revised := revisionCall(a, "PATCH", path, "revise-paused-watch", paused["etag"].(string), map[string]any{"revision": input})
	if code != 200 || revised["state"] != "paused" || revised["watch"].(map[string]any)["enabled"] != false {
		t.Fatal("revision activated paused watch", code, revised)
	}
	code, resumed := revisionCall(a, "PATCH", path, "resume-revised-watch", revised["etag"].(string), map[string]any{"state": "active"})
	if code != 200 || resumed["watch"].(map[string]any)["enabled"] != true {
		t.Fatal("resume ignored latest revision preference", code, resumed)
	}
	var revisions int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.intent_revisions WHERE intent_id=$1", id).Scan(&revisions); err != nil || revisions != 2 {
		t.Fatal("resume changed revision history", revisions, err)
	}
}
