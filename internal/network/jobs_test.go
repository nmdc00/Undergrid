package network

import (
	"context"
	"testing"
	"time"
)

func TestCommittedIntentionRunsThroughDurableWorker(t *testing.T) {
	s, db := fixture(t)
	a, b := client(t, s), client(t, s)
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Worker catalog " + NewID()
	code, first := a.call("POST", "/v1/intents", "queued-first", input)
	if code != 201 {
		t.Fatal(code, first)
	}
	code, second := b.call("POST", "/v1/intents", "queued-second", input)
	if code != 201 {
		t.Fatal(code, second)
	}
	var queued int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE args->>'cause_id'=$1", first["id"]).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("transactional job missing %d %v", queued, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() { stopped <- RunWorker(ctx, s.dsn, "development") }()
	defer func() {
		cancel()
		select {
		case err := <-stopped:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(12 * time.Second):
			t.Error("worker did not stop")
		}
	}()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		code, page := a.call("GET", "/v1/intents/"+first["id"].(string)+"/opportunities", "", nil)
		if code != 200 {
			t.Fatal(code, page)
		}
		if len(page["items"].([]any)) == 1 {
			return
		}
		select {
		case err := <-stopped:
			t.Fatalf("worker exited: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatal("committed job did not produce opportunity")
}
