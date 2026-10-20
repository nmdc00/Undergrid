package network

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Controlled numeric vectors prove pairwise grouping, never model relevance or
// independent beneficial owners. Every observation traverses the actual trusted
// HTTPS adapter and restricted worker write path before the application read.
func TestSemanticGroupsRequireEveryCurrentPairAboveSourceThreshold(t *testing.T) {
	s, _, workerDSN := semanticFixture(t)
	t.Setenv("UNDERGRID_EMBEDDING_API_KEY", "")
	summaries := []string{"Environmental catalog " + NewID(), "Climate directory " + NewID(), "Policy citation index " + NewID(), "Agency references " + NewID()}
	vectors := [][]float64{{1, 0, 0}, {0.94, 0.342, 0}, {0.94, -0.342, 0}, {1, 0, 0}}
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid", 400)
			return
		}
		selected, _ := in["input"].(string)
		index := -1
		for i, value := range summaries {
			if selected == value {
				index = i
				break
			}
		}
		if index < 0 {
			http.Error(w, "outside fixture", 422)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"model": in["model"], "model_revision": in["model_revision"], "input_sha256": digest([]byte(selected)), "embedding": vectors[index]})
	}))
	defer provider.Close()
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	configPath := filepath.Join(dir, "semantic.json")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"endpoint": provider.URL, "provider_id": "group-fixture", "model": "group-fixture", "model_revision": "r1", "dimensions": 3, "permission_evidence": "controlled group numeric fixture only", "privacy_policy_sha256": strings.Repeat("a", 64), "allow_public_projections": true, "allow_private_projections": false, "minimum_similarity": 0.9, "test_only": true, "tls_root_ca_file": ca}
	if err := os.WriteFile(configPath, canonical(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERGRID_SEMANTIC_CONFIG_FILE", configPath)
	owners := []testClient{client(t, s), client(t, s), client(t, s), client(t, s)}
	cases := []any{}
	owners[0].observe = func(r *http.Request, w *httptest.ResponseRecorder, _ any) {
		if r.Method != "GET" {
			return
		}
		var out any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"method": r.Method, "path": r.URL.Path, "status": w.Code, "response": out, "request_id": w.Header().Get("X-Request-ID")})
	}
	defer func() {
		cmd := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--compatible-groups")
		cmd.Stdin = bytes.NewReader(canonical(cases))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("actual semantic group contract: %v %s", err, out)
		}
	}()

	rows := []map[string]any{}
	for i, owner := range owners {
		input := intentFixture()
		input.Visibility = "redacted_public"
		input.PublicSummary = summaries[i]
		code, row := owner.call("POST", "/v1/intents", "group-semantic-create", input)
		if code != 201 {
			t.Fatal(code, row)
		}
		rows = append(rows, row)
	}
	t.Cleanup(func() {
		for i, row := range rows {
			code, out := owners[i].versioned("PATCH", "/v1/intents/"+row["id"].(string), "retire-group-semantic", row["etag"].(string), map[string]any{"state": "withdrawn"})
			if code != 200 {
				t.Errorf("retire group fixture %d %v", code, out)
			}
		}
	})
	runner := New(workerDSN, "development", nil)
	var err error
	runner.semantic, err = semanticConfiguration("development")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if err := runner.scanIntentions(context.Background(), row["id"].(string)); err != nil {
			t.Fatal(err)
		}
	}
	if err := runner.scanIntentions(context.Background(), rows[0]["id"].(string)); err != nil {
		t.Fatal(err)
	}
	path := "/v1/intents/" + rows[0]["id"].(string) + "/compatible-groups"
	code, page := owners[0].call("GET", path, "", nil)
	if code != 200 {
		t.Fatal(code, page)
	}
	groups := page["items"].([]any)
	if len(groups) != 2 {
		t.Fatalf("expected two nontransitive semantic views, got %d: %v", len(groups), page)
	}
	for _, value := range groups {
		group := value.(map[string]any)
		if group["comparison_mode"] != "pairwise_semantic_and_hard_constraints" {
			t.Fatal("semantic view mislabeled", group)
		}
		members := group["members"].([]any)
		if len(members) != 3 {
			t.Fatal("missing genuinely compatible third member", group)
		}
		seen := map[string]bool{}
		for _, value := range members {
			seen[value.(map[string]any)["intent_id"].(string)] = true
		}
		if !seen[rows[0]["id"].(string)] || !seen[rows[3]["id"].(string)] || (seen[rows[1]["id"].(string)] && seen[rows[2]["id"].(string)]) {
			t.Fatal("source-only similarity inferred transitive group", group)
		}
	}

	// A current revision without its own evidence cannot inherit a previous
	// member's vector. Withdrawal and visibility use the same current projection.
	replacement := intentFixture()
	replacement.Visibility = "redacted_public"
	replacement.PublicSummary = "Unembedded replacement " + NewID()
	code, revised := owners[2].versioned("PATCH", "/v1/intents/"+rows[2]["id"].(string), "revise-group-member", rows[2]["etag"].(string), map[string]any{"revision": replacement})
	if code != 200 {
		t.Fatal(code, revised)
	}
	rows[2] = revised
	code, page = owners[0].call("GET", path, "", nil)
	if code != 200 || len(page["items"].([]any)) != 1 {
		t.Fatal("stale cross-member evidence survived revision", code, page)
	}
	// A changed operator model cannot silently reuse old numeric evidence.
	config["model_revision"] = "r2"
	if err := os.WriteFile(configPath, canonical(config), 0600); err != nil {
		t.Fatal(err)
	}
	code, page = owners[0].call("GET", path, "", nil)
	if code != 200 || len(page["items"].([]any)) != 0 {
		t.Fatal("old profile survived configuration change", code, page)
	}
}
