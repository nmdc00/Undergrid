package network

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func intentWithTypedConstraints(t *testing.T, values map[string]any) IntentInput {
	t.Helper()
	base := intentFixture()
	raw := map[string]any{}
	if err := json.Unmarshal(canonical(base), &raw); err != nil {
		t.Fatal(err)
	}
	constraints := raw["hard_constraints"].(map[string]any)
	for k, v := range values {
		constraints[k] = v
	}
	var result IntentInput
	if err := json.Unmarshal(canonical(raw), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestTypedDemandConstraintsDoNotTreatUnknownOrConflictingEvidenceAsAligned(t *testing.T) {
	at := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	coverage := func(required, excluded []string) map[string]any {
		return map[string]any{"required": required, "excluded": excluded}
	}
	cases := []struct {
		name string
		a, b map[string]any
		want string
	}{
		{"coverage_exclusion", map[string]any{"coverage": coverage([]string{"region:eu"}, []string{})}, map[string]any{"coverage": coverage([]string{"region:us"}, []string{"region:eu"})}, "incompatible"},
		{"coverage_unspecified", map[string]any{"coverage": coverage([]string{"region:eu"}, []string{})}, nil, "needs_clarification"},
		{"freshness_unspecified", map[string]any{"freshness": map[string]any{"max_age_seconds": 60}}, nil, "needs_clarification"},
		{"delivery_disjoint", map[string]any{"delivery": map[string]any{"not_before": at.Add(time.Hour)}}, map[string]any{"delivery": map[string]any{"deliver_by": at}}, "incompatible"},
		{"input_family_disjoint", map[string]any{"input_schema_families": []string{"raw_catalog_v1"}}, map[string]any{"input_schema_families": []string{"source_catalog_v1"}}, "incompatible"},
		{"input_family_unspecified", map[string]any{"input_schema_families": []string{"source_catalog_v1"}}, nil, "needs_clarification"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, b := intentWithTypedConstraints(t, c.a), intentWithTypedConstraints(t, c.b)
			got := CompareDemand(a, b)
			if got.Compatible || got.Status != c.want {
				t.Fatalf("typed constraint ignored: %+v want %s", got, c.want)
			}
		})
	}
}
func TestTypedDemandConstraintHTTPPersistsAndExplainsCombinedRequirements(t *testing.T) {
	s, _, workerDSN := semanticFixture(t)
	a, b := client(t, s), client(t, s)
	at := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	body := map[string]any{}
	if err := json.Unmarshal(canonical(intentFixture()), &body); err != nil {
		t.Fatal(err)
	}
	body["visibility"] = "redacted_public"
	body["public_summary"] = "Typed declaration match " + NewID()
	hc := body["hard_constraints"].(map[string]any)
	hc["coverage"] = map[string]any{"required": []string{"region:eu"}, "excluded": []string{}}
	hc["freshness"] = map[string]any{"max_age_seconds": 300}
	hc["delivery"] = map[string]any{"not_before": at, "deliver_by": at.Add(4 * time.Hour)}
	hc["input_schema_families"] = []string{"raw_catalog_v1", "source_catalog_v1"}
	code, first := a.call("POST", "/v1/intents", "typed-first", body)
	if code != 201 {
		t.Fatalf("typed intention rejected: %d %v", code, first)
	}
	hc["coverage"] = map[string]any{"required": []string{"region:us"}, "excluded": []string{}}
	hc["freshness"] = map[string]any{"max_age_seconds": 60}
	hc["delivery"] = map[string]any{"not_before": at.Add(time.Hour), "deliver_by": at.Add(3 * time.Hour)}
	hc["input_schema_families"] = []string{"source_catalog_v1"}
	code, second := b.call("POST", "/v1/intents", "typed-second", body)
	if code != 201 {
		t.Fatal(code, second)
	}
	worker := New(workerDSN, "development", nil)
	if err := worker.scanIntentions(context.Background(), first["id"].(string)); err != nil {
		t.Fatal(err)
	}
	code, found := a.call("GET", "/v1/intents/"+first["id"].(string)+"/opportunities", "", nil)
	if code != 200 {
		t.Fatal(code, found)
	}
	var proof map[string]any
	for _, row := range found["items"].([]any) {
		item := row.(map[string]any)
		if item["candidate_intent_id"] == second["id"] {
			proof = item["evidence"].(map[string]any)
		}
	}
	if proof == nil {
		t.Fatal("aligned typed demand not found", found)
	}
	combined, ok := proof["combined_constraints"].(map[string]any)
	if !ok {
		t.Fatal("combined requirements absent", proof)
	}
	if combined["freshness"].(map[string]any)["max_age_seconds"] != float64(60) {
		t.Fatal(combined)
	}
	coverage := combined["coverage"].(map[string]any)["required"].([]any)
	if len(coverage) != 2 || coverage[0] != "region:eu" || coverage[1] != "region:us" {
		t.Fatal(combined)
	}
	delivery := combined["delivery"].(map[string]any)
	if delivery["not_before"] != at.Add(time.Hour).Format(time.RFC3339) || delivery["deliver_by"] != at.Add(3*time.Hour).Format(time.RFC3339) {
		t.Fatal(combined)
	}
	families := combined["input_schema_families"].([]any)
	if len(families) != 1 || families[0] != "source_catalog_v1" {
		t.Fatal(combined)
	}
}
func TestExternalDeclarationCannotSilentlyIgnoreTypedDemandRequirements(t *testing.T) {
	input := intentWithTypedConstraints(t, map[string]any{"freshness": map[string]any{"max_age_seconds": 60}})
	fields := []string{"source_id", "source_url"}
	claim := &capabilityClaim{Version: "capability-claim-v1", Output: &capabilityOutput{Kind: "json_dataset", SchemaFamily: "source_catalog_v1"}, ProvidedFields: &fields, FieldsComplete: true, LicenseIDs: []string{"CC0-1.0"}}
	for _, row := range compareExternalClaim(input, claim) {
		if row.Constraint == "freshness" && row.Status == "unknown" {
			return
		}
	}
	t.Fatal("publisher v1 claim incorrectly appears complete without freshness evidence")
}
