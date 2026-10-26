package network

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Four earlier projections occupy both direct scan slots for two provider
// timeout waves. Fresh exact discovery must finish while provider work remains.
func TestSemanticSlowProviderPreservesLocalExactFiveSecondBound(t *testing.T) {
	s, db, worker, _ := externalNotificationFixture(t)
	t.Setenv("UNDERGRID_SOURCES_FILE", "")
	t.Setenv("UNDERGRID_EMBEDDING_API_KEY", "")
	var pending atomic.Int64
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "shape", 400)
			return
		}
		if strings.Contains(in["input"].(string), "PRIVATE") {
			t.Error("private ungranted content disclosed")
		}
		pending.Add(1)
		defer pending.Add(-1)
		<-r.Context().Done()
	}))
	defer provider.Close()
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"endpoint": provider.URL, "provider_id": "slow-fixture", "model": "slow-fixture", "model_revision": "r1", "dimensions": 3, "permission_evidence": "controlled fixture, not qualification", "privacy_policy_sha256": strings.Repeat("e", 64), "allow_public_projections": true, "allow_private_projections": false, "minimum_similarity": 0.99, "test_only": true, "tls_root_ca_file": ca}
	path := filepath.Join(dir, "semantic.json")
	if err := os.WriteFile(path, canonical(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERGRID_SEMANTIC_CONFIG_FILE", path)
	old := client(t, s)
	for range 4 {
		input := intentFixture()
		input.Visibility = "redacted_public"
		input.PublicSummary = "Slow unrelated public projection " + NewID()
		code, out := old.call("POST", "/v1/intents", NewID(), input)
		if code != 201 {
			t.Fatal(code, out)
		}
	}
	a, b := client(t, s), client(t, s)
	input := intentFixture()
	input.Objective = "Local exact need " + NewID()
	code, watch := a.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, watch)
	}
	input.Visibility = "redacted_public"
	input.PublicSummary = input.Objective
	input.Objective = "PRIVATE candidate objective never embed"
	code, candidate := b.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, candidate)
	}
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
			t.Error("worker shutdown exceeded existing bound")
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var scanned bool
		var opportunities, events, inbox int
		if err := db.QueryRow(context.Background(), "SELECT last_scan IS NOT NULL FROM undergrid.intents WHERE id=$1", watch["id"]).Scan(&scanned); err != nil {
			t.Fatal(err)
		}
		err := db.QueryRow(context.Background(), `SELECT count(*),count(e.id),count(b.event_id) FROM undergrid.opportunities o LEFT JOIN undergrid.events e ON e.aggregate_id=o.id AND e.type='opportunity.available' LEFT JOIN undergrid.inbox b ON b.event_id=e.id WHERE o.source_intent_id=$1 AND o.candidate_intent_id=$2`, watch["id"], candidate["id"]).Scan(&opportunities, &events, &inbox)
		if err != nil {
			t.Fatal(err)
		}
		if scanned && opportunities == 1 && events == 1 && inbox == 1 && pending.Load() > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("local exact deadline: scanned=%v opportunity/event/inbox=%d/%d/%d provider_pending=%d", scanned, opportunities, events, inbox, pending.Load())
		}
		time.Sleep(20 * time.Millisecond)
	}
}
