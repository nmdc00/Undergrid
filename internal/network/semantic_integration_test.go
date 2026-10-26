package network

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Controlled TLS vectors prove the pipeline, never external provider qualification.
func TestCurrentReleaseSemanticParaphraseRuntime(t *testing.T) {
	s, _, worker, _ := externalNotificationFixture(t)
	t.Setenv("UNDERGRID_SOURCES_FILE", "")
	t.Setenv("UNDERGRID_EMBEDDING_API_KEY", "")
	summaries := []string{"Environmental regulation reference catalog", "Climate agency citation directory"}
	var mu sync.Mutex
	inputs := []string{}
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method", 405)
			return
		}
		var in struct {
			Model    string `json:"model"`
			Revision string `json:"model_revision"`
			Input    string `json:"input"`
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		d.DisallowUnknownFields()
		if err := d.Decode(&in); err != nil {
			http.Error(w, "shape", 400)
			return
		}
		mu.Lock()
		inputs = append(inputs, in.Input)
		mu.Unlock()
		if in.Model != "controlled-fixture" || in.Revision != "fixture-revision-1" {
			http.Error(w, "pin", 422)
			return
		}
		vector := []float64{1, 0, 0}
		if in.Input == summaries[1] {
			vector = []float64{0.98, 0.1, 0}
		}
		if in.Input != summaries[0] && in.Input != summaries[1] {
			http.Error(w, "unapproved text", 422)
			return
		}
		hash := sha256.Sum256([]byte(in.Input))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"model": in.Model, "model_revision": in.Revision, "input_sha256": hex.EncodeToString(hash[:]), "embedding": vector})
	}))
	defer provider.Close()
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"endpoint": provider.URL, "provider_id": "controlled-fixture", "model": "controlled-fixture", "model_revision": "fixture-revision-1", "dimensions": 3, "permission_evidence": "controlled TLS fixture, not provider qualification", "privacy_policy_sha256": strings.Repeat("d", 64), "allow_public_projections": true, "allow_private_projections": false, "minimum_similarity": 0.9999, "test_only": true, "tls_root_ca_file": ca}
	path := filepath.Join(dir, "semantic.json")
	if err := os.WriteFile(path, canonical(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERGRID_SEMANTIC_CONFIG_FILE", path)

	a, b := client(t, s), client(t, s)
	ids := []string{}
	for n, owner := range []testClient{a, b} {
		input := intentFixture()
		input.Visibility = "redacted_public"
		input.PublicSummary = summaries[n]
		input.Objective = "PRIVATE OBJECTIVE NEVER EMBED"
		code, out := owner.call("POST", "/v1/intents", NewID(), input)
		if code != 201 {
			t.Fatal(code, out)
		}
		ids = append(ids, out["id"].(string))
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
		code, page := a.call("GET", "/v1/intents/"+ids[0]+"/opportunities", "", nil)
		if code != 200 {
			t.Fatal(code, page)
		}
		for _, raw := range page["items"].([]any) {
			item := raw.(map[string]any)
			if item["candidate_intent_id"] != ids[1] {
				continue
			}
			evidence := item["evidence"].(map[string]any)
			if evidence["algorithm"] != "hard-constraints-hybrid-cosine-v1" || evidence["semantic_profile_hash"] == "" {
				t.Fatal("missing semantic profile-bound evidence", evidence)
			}
			mu.Lock()
			count := len(inputs)
			mu.Unlock()
			if count < 2 {
				t.Fatal("no actual provider calls", count)
			}
			return
		}
		if time.Now().After(deadline) {
			mu.Lock()
			defer mu.Unlock()
			t.Fatalf("semantic paraphrase result absent within five seconds; actual provider inputs=%v", inputs)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
func TestCurrentReleaseMatchingPolicyAndGroupEntrypoints(t *testing.T) {
	s, _, _, _ := externalNotificationFixture(t)
	owner := client(t, s)
	code, intent := owner.call("POST", "/v1/intents", NewID(), intentFixture())
	if code != 201 {
		t.Fatal(code, intent)
	}
	for _, path := range []string{"/v1/matching-policy", "/v1/intents/" + intent["id"].(string) + "/compatible-groups"} {
		t.Run(path, func(t *testing.T) {
			code, out := owner.call("GET", path, "", nil)
			if code != 200 {
				t.Fatalf("required current matching entrypoint: %d %v", code, out)
			}
		})
	}
}
