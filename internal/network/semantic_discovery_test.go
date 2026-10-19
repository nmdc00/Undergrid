package network

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func semanticFixture(t *testing.T) (*Service, *pgx.Conn, string) {
	t.Helper()
	s, db := fixture(t)
	_, err := db.Exec(context.Background(), `DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='undergrid_semantic_api_runner') THEN CREATE ROLE undergrid_semantic_api_runner LOGIN NOSUPERUSER NOBYPASSRLS; END IF;
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='undergrid_semantic_worker_runner') THEN CREATE ROLE undergrid_semantic_worker_runner LOGIN NOSUPERUSER NOBYPASSRLS; END IF;
 END $$; GRANT undergrid_app TO undergrid_semantic_api_runner; GRANT undergrid_worker TO undergrid_semantic_worker_runner;`)
	if err != nil {
		t.Fatal(err)
	}
	s.dsn = networkFixtureRoleDSN(t, db, "undergrid_semantic_api_runner")
	workerDSN := networkFixtureRoleDSN(t, db, "undergrid_semantic_worker_runner")
	return s, db, workerDSN
}

// This first oracle uses only existing runtime entrypoints: a configured worker
// must discover meaningfully related public summaries with no lexical overlap.
// Controlled vectors prove pipeline behavior, never real-model relevance.
func TestSemanticParaphraseDiscoveryThroughConfiguredWorker(t *testing.T) {
	s, db, workerDSN := semanticFixture(t)
	var err error
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
	if err = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"endpoint": provider.URL, "provider_id": "controlled-fixture", "model": "controlled-fixture", "model_revision": "fixture-revision-1", "dimensions": 3, "permission_evidence": "controlled TLS fixture, not provider qualification", "privacy_policy_sha256": strings.Repeat("d", 64), "allow_public_projections": true, "allow_private_projections": false, "test_only": true, "tls_root_ca_file": ca}
	path := filepath.Join(dir, "semantic.json")
	if err = os.WriteFile(path, canonical(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERGRID_SEMANTIC_CONFIG_FILE", path)
	a, b := client(t, s), client(t, s)
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = summaries[0]
	input.Objective = "PRIVATE OBJECTIVE NEVER EMBED"
	input.Budget = rawJSON([]byte(`{"amount_minor":73519,"currency":"EUR","kind":"stated_budget"}`))
	code, first := a.call("POST", "/v1/intents", "semantic-first", input)
	if code != 201 {
		t.Fatal(code, first)
	}
	input.PublicSummary = summaries[1]
	input.Objective = "OTHER PRIVATE OBJECTIVE NEVER EMBED"
	code, second := b.call("POST", "/v1/intents", "semantic-second", input)
	if code != 201 {
		t.Fatal(code, second)
	}

	var cases []map[string]any
	observe := func(r *http.Request, w *httptest.ResponseRecorder, body any) {
		var response any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"method": r.Method, "path": r.URL.String(), "request": body, "response": response, "status": w.Code, "request_id": w.Header().Get("X-Request-ID"), "etag": w.Header().Get("ETag")})
	}
	a.observe, b.observe = observe, observe
	defer func() {
		raw, err := json.Marshal(cases)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--semantic-discovery")
		cmd.Stdin = bytes.NewReader(raw)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("actual semantic HTTP contract: %v %s", err, out)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- RunWorker(ctx, workerDSN, "development") }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(15 * time.Second):
			t.Error("semantic worker did not stop")
		}
	}()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		code, out := a.call("GET", "/v1/intents/"+first["id"].(string)+"/opportunities", "", nil)
		if code != 200 {
			t.Fatal(code, out)
		}
		for _, raw := range out["items"].([]any) {
			item := raw.(map[string]any)
			if item["candidate_intent_id"] == second["id"] {
				evidence := item["evidence"].(map[string]any)
				if evidence["algorithm"] != "hard-constraints-hybrid-cosine-v1" {
					t.Fatalf("semantic result has no pinned algorithm: %v", evidence)
				}

				if evidence["candidate_revision_hash"] != nil || evidence["candidate_projection_hash"] == nil {
					t.Fatalf("semantic evidence exposes a private document fingerprint instead of approved projection identity: %v", evidence)
				}
				mu.Lock()
				observedInputs := append([]string(nil), inputs...)
				mu.Unlock()
				if len(observedInputs) < 2 {
					t.Fatalf("no actual configured embeddings: %v", observedInputs)
				}
				for _, text := range observedInputs {
					if strings.Contains(text, "PRIVATE") || strings.Contains(text, "73519") {
						t.Fatalf("private content embedded: %q", text)
					}
				}

				var model, profile, inputHash string
				var dimensions int
				if err = db.QueryRow(context.Background(), "SELECT embedding_model,embedding_profile_hash,embedding_input_hash,embedding_dimensions FROM undergrid.intent_revisions WHERE intent_id=$1 AND revision=1", first["id"]).Scan(&model, &profile, &inputHash, &dimensions); err != nil {
					t.Fatal(err)
				}
				if model != "controlled-fixture" || len(profile) != 64 || inputHash != digest([]byte(summaries[0])) || dimensions != 3 {
					t.Fatal(model, profile, inputHash, dimensions)
				}
				writer, err := pgx.Connect(context.Background(), workerDSN)
				if err != nil {
					t.Fatal(err)
				}
				_, err = writer.Exec(context.Background(), "UPDATE undergrid.intent_revisions SET embedding_model='forged' WHERE intent_id=$1 AND revision=1", first["id"])
				writer.Close(context.Background())
				if err == nil {
					t.Fatal("worker overwrote completed embedding evidence")
				}
				// A new revision is still public, but its changed license is
				// incompatible. Old semantic evidence must not be re-presented
				// against the replacement projection, even before jobs catch up.
				next := input
				next.HardConstraints.LicenseIDs = []string{"proprietary"}
				code, changed := b.versioned("PATCH", "/v1/intents/"+second["id"].(string), "semantic-conflicting-revision", second["etag"].(string), map[string]any{"revision": next})
				if code != 200 {
					t.Fatal(code, changed)
				}
				code, current := a.call("GET", "/v1/intents/"+first["id"].(string)+"/opportunities", "", nil)
				if code != 200 {
					t.Fatal(code, current)
				}
				for _, raw := range current["items"].([]any) {
					if raw.(map[string]any)["candidate_intent_id"] == second["id"] {
						t.Fatalf("stale semantic evidence survived conflicting revision: %v", raw)
					}
				}
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("configured semantic discovery missed nonlexical paraphrase; approved endpoint inputs=%v", inputs)
}

// The same worker domain entrypoint used by the committed scan job must not
// hold authority during provider I/O or persist an observation after withdrawal.
func TestSemanticWithdrawalDuringProviderIOLeavesNoEmbeddingOrOpportunity(t *testing.T) {
	s, db, workerDSN := semanticFixture(t)
	a := client(t, s)
	t.Setenv("UNDERGRID_EMBEDDING_API_KEY", "")
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Approved revocation projection " + NewID()
	input.Objective = "PRIVATE MUST NOT BE SENT"
	code, created := a.call("POST", "/v1/intents", "revocation-source", input)
	if code != 201 {
		t.Fatal(code, created)
	}
	id := created["id"].(string)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string `json:"model"`
			Revision string `json:"model_revision"`
			Input    string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Input != input.PublicSummary {
			http.Error(w, "unapproved projection", 422)
			return
		}
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": body.Model, "model_revision": body.Revision, "input_sha256": digest([]byte(body.Input)), "embedding": []float64{1, 0, 0}})
	}))
	defer server.Close()
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	profile, err := newSemanticProvider(semanticConfig{Endpoint: server.URL, ProviderID: "controlled-revocation", Model: "controlled", ModelRevision: "1", Dimensions: 3, PermissionEvidence: "controlled fixture only", PrivacyPolicySHA256: strings.Repeat("e", 64), AllowPublicProjections: true, TestOnly: true, TLSRootCAFile: ca}, "development")
	if err != nil {
		t.Fatal(err)
	}
	worker := New(workerDSN, "development", nil)
	worker.semantic = profile
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	done := make(chan error, 1)
	completed := false
	go func() { done <- worker.scanIntentions(ctx, id) }()
	defer func() {
		unblock()
		cancel()
		if !completed {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("scan did not stop")
			}
		}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("provider not reached")
	}
	started := time.Now()
	code, withdrawn := a.versioned("PATCH", "/v1/intents/"+id, "withdraw-during-embedding", created["etag"].(string), map[string]any{"state": "withdrawn"})
	if code != 200 || time.Since(started) > 2*time.Second {
		t.Fatalf("authority blocked behind provider: %d %v", code, withdrawn)
	}
	unblock()
	select {
	case err = <-done:
		completed = true
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var state string
	var watch bool
	var embeddings, opportunities int
	if err = db.QueryRow(context.Background(), `SELECT state,watch_enabled,
 (SELECT count(*) FROM undergrid.intent_revisions WHERE intent_id=$1 AND embedding IS NOT NULL),
 (SELECT count(*) FROM undergrid.opportunities WHERE source_intent_id=$1 OR candidate_intent_id=$1)
 FROM undergrid.intents WHERE id=$1`, id).Scan(&state, &watch, &embeddings, &opportunities); err != nil {
		t.Fatal(err)
	}
	if state != "withdrawn" || watch || embeddings != 0 || opportunities != 0 {
		t.Fatal(state, watch, embeddings, opportunities)
	}
}
