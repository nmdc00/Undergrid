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

// A repair coordinator must commit independently recoverable work before any
// provider call. One rejected projection cannot abort the other owners' repair.
func TestSemanticRepairQueuesIndependentRevisionPinnedWorkBeforeProviderIO(t *testing.T) {
	s, db, workerDSN := semanticFixture(t)
	t.Setenv("UNDERGRID_EMBEDDING_API_KEY", "")
	badSummary := "Rejected repair projection " + NewID()
	firstSummary := "Environmental regulation inventory " + NewID()
	secondSummary := "Climate citation references " + NewID()
	var requests atomic.Int64
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid", 400)
			return
		}
		text, _ := in["input"].(string)
		if text == badSummary {
			http.Error(w, "controlled rejection", 422)
			return
		}
		if text != firstSummary && text != secondSummary {
			http.Error(w, "outside this fixture", 422)
			return
		}
		vector := []float64{1, 0, 0}
		if text == secondSummary {
			vector = []float64{0.98, 0.1, 0}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": in["model"], "model_revision": in["model_revision"], "input_sha256": digest([]byte(text)), "embedding": vector})
	}))
	defer provider.Close()
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	path := filepath.Join(dir, "semantic.json")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"endpoint": provider.URL, "provider_id": "repair-fixture", "model": "repair-fixture", "model_revision": "r1", "dimensions": 3, "permission_evidence": "controlled repair fixture only", "privacy_policy_sha256": strings.Repeat("b", 64), "allow_public_projections": true, "allow_private_projections": false, "test_only": true, "tls_root_ca_file": ca}
	if err := os.WriteFile(path, canonical(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERGRID_SEMANTIC_CONFIG_FILE", path)
	a, b := client(t, s), client(t, s)
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = badSummary
	code, bad := a.call("POST", "/v1/intents", "repair-rejected", input)
	if code != 201 {
		t.Fatal(code, bad)
	}
	input.PublicSummary = firstSummary
	code, first := a.call("POST", "/v1/intents", "repair-first", input)
	if code != 201 {
		t.Fatal(code, first)
	}
	input.PublicSummary = secondSummary
	code, second := b.call("POST", "/v1/intents", "repair-second", input)
	if code != 201 {
		t.Fatal(code, second)
	}
	t.Cleanup(func() {
		for _, item := range []struct {
			owner testClient
			row   map[string]any
		}{{a, bad}, {a, first}, {b, second}} {
			id := item.row["id"].(string)
			code, out := item.owner.versioned("PATCH", "/v1/intents/"+id, "retire-repair-"+id, item.row["etag"].(string), map[string]any{"state": "withdrawn"})
			if code != 200 {
				t.Errorf("retire repair fixture: %d %v", code, out)
			}
		}
	})
	runner := New(workerDSN, "development", nil)
	var err error
	runner.semantic, err = semanticConfiguration("development")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for range 2 {
		if err := runner.ScanDueIntentions(ctx); err != nil {
			t.Fatalf("one rejected projection aborted coordinator: %v", err)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("repair coordinator performed provider I/O", requests.Load())
	}
	ids := []string{bad["id"].(string), first["id"].(string), second["id"].(string)}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM undergrid_queue.river_job WHERE kind='semantic_repair_v1' AND args->>'intent_id'=ANY($1::text[]) AND args->>'revision'='1' AND args->>'profile_hash'=$2`, ids, runner.semantic.hash).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatal("repair work missing or duplicate on coordinator replay", count)
	}
	// A later revision needs a distinct wakeup even while the first is queued.
	input.PublicSummary = firstSummary
	input.Objective = "Revised private objective never sent"
	code, changed := a.versioned("PATCH", "/v1/intents/"+first["id"].(string), "repair-revised", first["etag"].(string), map[string]any{"revision": input})
	if code != 200 {
		t.Fatal(code, changed)
	}
	first = changed
	if err := runner.ScanDueIntentions(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM undergrid_queue.river_job WHERE kind='semantic_repair_v1' AND args->>'intent_id'=$1 AND args->>'profile_hash'=$2`, first["id"], runner.semantic.hash).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("queued earlier revision absorbed new repair", count)
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- RunWorker(workerCtx, workerDSN, "development") }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(15 * time.Second):
			t.Error("repair worker shutdown exceeded bound")
		}
	}()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		code, result := a.call("GET", "/v1/intents/"+first["id"].(string)+"/opportunities", "", nil)
		if code != 200 {
			t.Fatal(code, result)
		}
		found := false
		for _, value := range result["items"].([]any) {
			if value.(map[string]any)["candidate_intent_id"] == second["id"] {
				found = true
			}
		}
		var completed int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM undergrid_queue.river_job WHERE kind='semantic_repair_v1' AND state='completed' AND ((args->>'intent_id'=$1 AND args->>'revision'='2') OR (args->>'intent_id'=$2 AND args->>'revision'='1'))`, first["id"], second["id"]).Scan(&completed); err != nil {
			t.Fatal(err)
		}
		if found && completed == 2 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("healthy repair work failed behind rejected projection")
}
