package network

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Ready numeric evidence must not wait for unrelated provider calls. Controlled
// vectors and held requests prove queue isolation, never provider relevance.
func TestCommittedSemanticEvidenceMatchesWhileOlderProviderWorkIsBlocked(t *testing.T) {
	s, db, workerDSN := semanticFixture(t)
	t.Setenv("UNDERGRID_EMBEDDING_API_KEY", "")
	sourceSummary := "Environmental legislation digest " + NewID()
	candidateSummary := "Climate citation directory " + NewID()
	oldSummaries := map[string]bool{}
	for i := 0; i < 4; i++ {
		oldSummaries[fmt.Sprintf("Held provider input %d %s", i, NewID())] = true
	}
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	seen := map[string]bool{}
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Model    string `json:"model"`
			Revision string `json:"model_revision"`
			Input    string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid", 400)
			return
		}
		if oldSummaries[in.Input] {
			mu.Lock()
			first := !seen[in.Input]
			seen[in.Input] = true
			mu.Unlock()
			if first {
				entered <- struct{}{}
			}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		} else if in.Input != sourceSummary && in.Input != candidateSummary {
			http.Error(w, "outside this controlled fixture", 422)
			return
		}
		vector := []float64{1, 0, 0}
		if in.Input == candidateSummary {
			vector = []float64{0.98, 0.1, 0}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": in.Model, "model_revision": in.Revision, "input_sha256": digest([]byte(in.Input)), "embedding": vector})
	}))
	defer provider.Close()
	defer once.Do(func() { close(release) })
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	configPath := filepath.Join(dir, "semantic.json")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"endpoint": provider.URL, "provider_id": "queue-fixture", "model": "queue-fixture", "model_revision": "r1", "dimensions": 3, "permission_evidence": "controlled queue fixture, not provider qualification", "privacy_policy_sha256": strings.Repeat("c", 64), "allow_public_projections": true, "allow_private_projections": false, "minimum_similarity": 0.9999, "test_only": true, "tls_root_ca_file": ca}
	if err := os.WriteFile(configPath, canonical(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERGRID_SEMANTIC_CONFIG_FILE", configPath)
	a, b := client(t, s), client(t, s)
	input := intentFixture()
	input.Visibility = "redacted_public"
	oldIDs := []string{}
	all := map[string]map[string]any{}
	for summary := range oldSummaries {
		input.PublicSummary = summary
		code, row := a.call("POST", "/v1/intents", NewID(), input)
		if code != 201 {
			t.Fatal(code, row)
		}
		id := row["id"].(string)
		oldIDs = append(oldIDs, id)
		all[id] = row
	}
	input.PublicSummary = sourceSummary
	threshold := 0.99
	input.SemanticMinimumSimilarity = &threshold
	code, source := a.call("POST", "/v1/intents", "ready-source", input)
	if code != 201 {
		t.Fatal(code, source)
	}
	all[source["id"].(string)] = source
	input.PublicSummary = candidateSummary
	input.SemanticMinimumSimilarity = nil
	code, candidate := b.call("POST", "/v1/intents", "ready-candidate", input)
	if code != 201 {
		t.Fatal(code, candidate)
	}
	t.Cleanup(func() {
		for id, row := range all {
			code, out := a.versioned("PATCH", "/v1/intents/"+id, "retire-ready-"+id, row["etag"].(string), map[string]any{"state": "withdrawn"})
			if code != 200 {
				t.Errorf("retire ready fixture: %d %v", code, out)
			}
		}
		code, out := b.versioned("PATCH", "/v1/intents/"+candidate["id"].(string), "retire-ready-candidate", candidate["etag"].(string), map[string]any{"state": "withdrawn"})
		if code != 200 {
			t.Errorf("retire candidate: %d %v", code, out)
		}
	})
	// Persist both vectors through the real worker observation transaction. No
	// scan or synthetic opportunity is inserted before the normal worker starts.
	runner := New(workerDSN, "development", nil)
	var err error
	runner.semantic, err = semanticConfiguration("development")
	if err != nil {
		t.Fatal(err)
	}
	writer, err := pgx.Connect(context.Background(), workerDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close(context.Background())
	if _, err = writer.Exec(context.Background(), "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{source["id"].(string), candidate["id"].(string)} {
		if err := runner.ensureSemanticEmbedding(context.Background(), writer, id); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- RunWorker(ctx, workerDSN, "development") }()
	defer func() {
		once.Do(func() { close(release) })
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(15 * time.Second):
			t.Error("worker shutdown exceeded bound")
		}
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(8 * time.Second):
			t.Fatal("older provider work did not occupy foreground capacity")
		}
	}
	deadline := time.Now().Add(8 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		code, result := a.call("GET", "/v1/intents/"+source["id"].(string)+"/opportunities", "", nil)
		if code != 200 {
			t.Fatal(code, result)
		}
		for _, value := range result["items"].([]any) {
			if value.(map[string]any)["candidate_intent_id"] == candidate["id"] {
				found = true
			}
		}
		if found {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !found {
		t.Fatal("committed semantic evidence starved behind older provider work")
	}
	once.Do(func() { close(release) })
	deadline = time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		var embedded int
		if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.intent_revisions WHERE intent_id=ANY($1::uuid[]) AND embedding IS NOT NULL", oldIDs).Scan(&embedded); err != nil {
			t.Fatal(err)
		}
		if embedded == 4 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("older retained provider work did not recover after release")
}
