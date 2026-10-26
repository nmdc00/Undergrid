package network

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunningWorkerSemanticConfigFailurePreservesExactDiscoveryAndReads(t *testing.T) {
	_, _ = controlledSemanticConsent(t)
	s, db, worker, _ := externalNotificationFixture(t)
	t.Setenv("UNDERGRID_SOURCES_FILE", "")
	a, b := client(t, s), client(t, s)
	input := intentFixture()
	input.Objective = "Local exact config isolation " + NewID()
	var phase atomic.Int64
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"items":[{"id":"catalogue","version":"%d","url":"https://publisher.example/catalogue","title":%q,"status":"active"}],"next_page":0}`, phase.Load()+1, input.Objective)
	}))
	defer publisher.Close()
	source := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example/capabilities", Publisher: "Controlled publisher", AccessBasis: "fixture access", RepublicationBasis: "fixture redistribution", TombstoneBasis: "fixture tombstone", PublicPermissionNotice: "Permitted fixture title and URL", Fields: []string{"canonical_url", "title"}}
	refresher := externalNotificationRefresher(t, worker.dsn, source, publisher)
	if err := refresher.RefreshDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	code, watch := a.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, watch)
	}
	public := input
	public.Visibility = "redacted_public"
	public.PublicSummary = input.Objective
	code, first := b.call("POST", "/v1/intents", NewID(), public)
	if code != 201 {
		t.Fatal(code, first)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runWorkerWithSources(ctx, worker.dsn, "development", refresher) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(12 * time.Second):
			t.Error("config failure prevented bounded shutdown")
		}
	}()
	var before time.Time
	deadline := time.Now().Add(5 * time.Second)
	for {
		var scanned bool
		var matches int
		if err := db.QueryRow(context.Background(), "SELECT last_scan IS NOT NULL FROM undergrid.intents WHERE id=$1", watch["id"]).Scan(&scanned); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.opportunities WHERE source_intent_id=$1 AND candidate_intent_id=$2", watch["id"], first["id"]).Scan(&matches); err != nil {
			t.Fatal(err)
		}
		if scanned && matches == 1 {
			if err := db.QueryRow(context.Background(), "SELECT last_scan FROM undergrid.intents WHERE id=$1", watch["id"]).Scan(&before); err != nil {
				t.Fatal(err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("valid configured worker precondition did not complete local discovery")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The already-running worker loses semantic policy readability. It must not
	// reinterpret that failure as permission to process content or disable exact work.
	if err := os.WriteFile(os.Getenv("UNDERGRID_SEMANTIC_CONFIG_FILE"), []byte("not valid JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	code, page := a.call("GET", "/v1/intents/"+watch["id"].(string)+"/opportunities", "", nil)
	if code != 200 {
		t.Errorf("semantic config outage blocked current exact opportunity read: %d %v", code, page)
	}
	code, policy := a.call("GET", "/v1/matching-policy", "", nil)
	if code != 503 {
		t.Errorf("unavailable processing policy failed open: %d %v", code, policy)
	}
	phase.Store(1)
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.external_sources SET next_refresh=now() WHERE id=$1", source.ID); err != nil {
		t.Fatal(err)
	}
	if err := refresher.RefreshDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	third := client(t, s)
	code, next := third.call("POST", "/v1/intents", NewID(), public)
	if code != 201 {
		t.Fatal(code, next)
	}
	code, paused := a.versioned("PATCH", "/v1/intents/"+watch["id"].(string), "pause-config-outage", watch["etag"].(string), map[string]any{"state": "paused"})
	if code != 200 {
		t.Fatal(code, paused)
	}
	code, resumed := a.versioned("PATCH", "/v1/intents/"+watch["id"].(string), "resume-config-outage", paused["etag"].(string), map[string]any{"state": "active"})
	if code != 200 {
		t.Fatal(code, resumed)
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		var scanned time.Time
		var opportunities, events, inbox int
		if err := db.QueryRow(context.Background(), "SELECT last_scan FROM undergrid.intents WHERE id=$1", watch["id"]).Scan(&scanned); err != nil {
			t.Fatal(err)
		}
		err := db.QueryRow(context.Background(), `SELECT count(*),count(e.id),count(b.event_id) FROM undergrid.opportunities o LEFT JOIN undergrid.events e ON e.aggregate_id=o.id AND e.type='opportunity.available' LEFT JOIN undergrid.inbox b ON b.event_id=e.id WHERE o.source_intent_id=$1 AND o.candidate_intent_id=$2`, watch["id"], next["id"]).Scan(&opportunities, &events, &inbox)
		if err != nil {
			t.Fatal(err)
		}
		var external, externalEvents, externalInbox int
		err = db.QueryRow(context.Background(), `SELECT count(*),count(e.id),count(b.event_id) FROM undergrid.opportunities o LEFT JOIN undergrid.events e ON e.aggregate_id=o.id AND e.type='opportunity.available' LEFT JOIN undergrid.inbox b ON b.event_id=e.id JOIN undergrid.external_observations observation ON observation.id=o.external_observation_id WHERE o.source_intent_id=$1 AND o.kind='external_capability' AND observation.source_id=$2 AND o.evidence->>'source_version'='2'`, watch["id"], source.ID).Scan(&external, &externalEvents, &externalInbox)
		if err != nil {
			t.Fatal(err)
		}
		if scanned.After(before) && opportunities == 1 && events == 1 && inbox == 1 && external == 1 && externalEvents == 1 && externalInbox == 1 {
			break
		}

		if time.Now().After(deadline) {
			t.Errorf("semantic policy outage blocked exact watch: completed_new_scan=%v exact=%d/%d/%d externalv2=%d/%d/%d", scanned.After(before), opportunities, events, inbox, external, externalEvents, externalInbox)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	var newEmbedding bool
	var acceptedEmbeddingJobs int
	if err := db.QueryRow(context.Background(), "SELECT embedding IS NOT NULL FROM undergrid.intent_revisions WHERE intent_id=$1 AND revision=1", next["id"]).Scan(&newEmbedding); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE kind='semantic_embedding_v1' AND args->>'intent_id'=$1 AND state IN ('available','pending','running','retryable','scheduled')", next["id"].(string)).Scan(&acceptedEmbeddingJobs); err != nil {
		t.Fatal(err)
	}
	if newEmbedding || acceptedEmbeddingJobs < 1 {
		t.Fatal("unavailable processing policy must retain accepted pending/retryable work without embedding", newEmbedding, acceptedEmbeddingJobs)
	}
	code, page = a.call("GET", "/v1/intents/"+watch["id"].(string)+"/opportunities", "", nil)
	if code != 200 {
		t.Errorf("generic current exact read unavailable: %d %v", code, page)
	}
}
