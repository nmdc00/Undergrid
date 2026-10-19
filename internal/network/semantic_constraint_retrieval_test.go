package network

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Both exact and full-text top-20 sets would exclude the last UUID if hard
// eligibility were applied only after ranking. All inputs use actual API roles.
func TestSemanticRetrievalFiltersHardConstraintsBeforeCandidateLimit(t *testing.T) {
	s, _, workerDSN := semanticFixture(t)
	sourceOwner, candidateOwner := client(t, s), client(t, s)
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Constraint first retrieval " + NewID()
	code, source := sourceOwner.call("POST", "/v1/intents", "constraint-source", input)
	if code != 201 {
		t.Fatal(code, source)
	}
	input.HardConstraints.LicenseIDs = []string{"incompatible-license"}
	candidates := map[string]map[string]any{}
	t.Cleanup(func() {
		for id, row := range candidates {
			code, result := candidateOwner.versioned("PATCH", "/v1/intents/"+id, "retire-"+id, row["etag"].(string), map[string]any{"state": "withdrawn"})
			if code != 200 {
				t.Errorf("retire fixture demand: %d %v", code, result)
			}
		}
		code, result := sourceOwner.versioned("PATCH", "/v1/intents/"+source["id"].(string), "retire-source", source["etag"].(string), map[string]any{"state": "withdrawn"})
		if code != 200 {
			t.Errorf("retire source: %d %v", code, result)
		}
	})
	ids := []string{}
	for i := 0; i < 25; i++ {
		code, row := candidateOwner.call("POST", "/v1/intents", fmt.Sprintf("constraint-candidate-%d", i), input)
		if code != 201 {
			t.Fatal(code, row)
		}
		id := row["id"].(string)
		ids = append(ids, id)
		candidates[id] = row
	}
	sort.Strings(ids)
	eligible := ids[len(ids)-1]
	input.HardConstraints.LicenseIDs = []string{"CC0-1.0"}
	code, row := candidateOwner.versioned("PATCH", "/v1/intents/"+eligible, "constraint-compatible", candidates[eligible]["etag"].(string), map[string]any{"revision": input})
	if code != 200 {
		t.Fatal(code, row)
	}
	candidates[eligible] = row
	threshold := 0.8
	s.semantic = &semanticProvider{hash: "constraint-retrieval-fixture", config: semanticConfig{MinimumSimilarity: &threshold}}
	worker, err := pgx.Connect(context.Background(), workerDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close(context.Background())
	if _, err = worker.Exec(context.Background(), "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	if err = s.scanIntentCandidate(context.Background(), worker, source["id"].(string), "", false); err != nil {
		t.Fatal(err)
	}
	code, result := sourceOwner.call("GET", "/v1/intents/"+source["id"].(string)+"/opportunities", "", nil)
	if code != 200 {
		t.Fatal(code, result)
	}
	for _, value := range result["items"].([]any) {
		if value.(map[string]any)["candidate_intent_id"] == eligible {
			return
		}
	}
	t.Fatal("compatible demand crowded out by incompatible top-ranked candidates", result)
}
