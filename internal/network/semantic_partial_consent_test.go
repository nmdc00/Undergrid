package network

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
)

func controlledSemanticConsent(t *testing.T) (*semanticProvider, *atomic.Int64) {
	t.Helper()
	t.Setenv("UNDERGRID_EMBEDDING_API_KEY", "")
	calls := new(atomic.Int64)
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "shape", 400)
			return
		}
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"model": in["model"], "model_revision": in["model_revision"], "input_sha256": digest([]byte(in["input"].(string))), "embedding": []float64{1, 0, 0}})
	}))
	t.Cleanup(provider.Close)
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"endpoint": provider.URL, "provider_id": "consent-fixture", "model": "consent-fixture", "model_revision": "r1", "dimensions": 3, "permission_evidence": "controlled fixture, not qualification", "privacy_policy_sha256": strings.Repeat("e", 64), "allow_public_projections": true, "allow_private_projections": true, "minimum_similarity": 0.99, "test_only": true, "tls_root_ca_file": ca}
	path := filepath.Join(dir, "semantic.json")
	if err := os.WriteFile(path, canonical(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERGRID_SEMANTIC_CONFIG_FILE", path)
	p, err := semanticConfiguration("development")
	if err != nil {
		t.Fatal(err)
	}
	return p, calls
}

func TestPartialRevisionRetainsQueryWithoutRenewingExternalProcessingGrant(t *testing.T) {
	provider, calls := controlledSemanticConsent(t)
	x := partialScenarioFixture(t, "private", func(input *IntentInput) {
		input.PrivateSemanticQuery = &PrivateSemanticQuery{Projection: "Explicit selected private query", ProfileHash: provider.hash, AcknowledgeExternalProcessing: true}
	})
	x.worker.semantic = provider
	db, err := pgx.Connect(context.Background(), x.worker.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(context.Background())
	if _, err = db.Exec(context.Background(), "SET ROLE undergrid_worker;SET search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	id := x.original["id"].(string)
	if err = x.worker.ensureSemanticEmbedding(context.Background(), db, id); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("explicit creation grant did not process", calls.Load())
	}
	code, remainder := ownerCallMatch(x.owner, "POST", x.path+"/partial-satisfaction", "partial-consent", x.original["etag"].(string), x.body)
	if code != 200 {
		t.Fatal(code, remainder)
	}
	var raw []byte
	if err = x.db.QueryRow(context.Background(), "SELECT document FROM undergrid.intent_revisions WHERE intent_id=$1 AND revision=2", id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var retained IntentInput
	if err = json.Unmarshal(raw, &retained); err != nil {
		t.Fatal(err)
	}
	expected := x.input
	expected.HardConstraints.RequiredFields = []string{"source_url", "title"}
	if !reflect.DeepEqual(retained, expected) {
		t.Fatalf("partial must preserve all query/input fields: got%+v expected%+v", retained, expected)
	}
	if err = x.worker.ensureSemanticEmbedding(context.Background(), db, id); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("partial revision silently renewed disclosure consent: provider calls=%d", calls.Load())
	}
	var embedded bool
	if err = x.db.QueryRow(context.Background(), "SELECT embedding IS NOT NULL FROM undergrid.intent_revisions WHERE intent_id=$1 AND revision=2", id).Scan(&embedded); err != nil {
		t.Fatal(err)
	}
	if embedded {
		t.Fatal("unreviewed partial revision persisted semantic embedding")
	}
	code, renewed := x.owner.versioned("PATCH", "/v1/intents/"+id, "explicit-query-review", remainder["etag"].(string), map[string]any{"revision": retained})
	if code != 200 {
		t.Fatal(code, renewed)
	}
	if err = x.worker.ensureSemanticEmbedding(context.Background(), db, id); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("explicit full revision failed to renew grant", calls.Load())
	}
}
