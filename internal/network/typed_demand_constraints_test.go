package network

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
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
		{"elapsed_delivery", map[string]any{"delivery": map[string]any{"deliver_by": at.Add(-3 * time.Hour)}}, map[string]any{"delivery": map[string]any{"deliver_by": at.Add(-3 * time.Hour)}}, "incompatible"},
		{"blank_license_is_unknown", map[string]any{"license_ids": []string{""}}, map[string]any{"license_ids": []string{""}}, "needs_clarification"},
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

	var cases []map[string]any
	observe := func(r *http.Request, w *httptest.ResponseRecorder, body any) {
		var request, response any
		if err := json.Unmarshal(canonical(body), &request); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"method": r.Method, "path": r.URL.String(), "request": request, "response": response, "status": w.Code, "request_id": w.Header().Get("X-Request-ID"), "etag": w.Header().Get("ETag")})
	}
	a.observe, b.observe = observe, observe
	defer func() {
		raw, err := json.Marshal(cases)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--typed-demand-constraints")
		cmd.Stdin = bytes.NewReader(raw)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("actual typed constraint contract: %v %s", err, out)
		}
	}()
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

func TestTypedDemandConstraintValidationRejectsMalformedRangesWithoutCreatingIntent(t *testing.T) {
	s, db, _ := semanticFixture(t)
	a := client(t, s)
	at := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	cases := []struct {
		name  string
		value map[string]any
	}{
		{"coverage_self_conflict", map[string]any{"coverage": map[string]any{"required": []string{"region:eu"}, "excluded": []string{"region:eu"}}}},
		{"coverage_missing_explicit_lists", map[string]any{"coverage": map[string]any{}}},
		{"freshness_zero", map[string]any{"freshness": map[string]any{"max_age_seconds": 0}}},
		{"freshness_over_limit", map[string]any{"freshness": map[string]any{"max_age_seconds": 31536001}}},
		{"delivery_empty", map[string]any{"delivery": map[string]any{}}},
		{"delivery_reversed", map[string]any{"delivery": map[string]any{"not_before": at.Add(time.Hour), "deliver_by": at}}},
		{"delivery_past", map[string]any{"delivery": map[string]any{"deliver_by": at.Add(-2 * time.Hour)}}},
		{"input_empty", map[string]any{"input_schema_families": []string{}}},
		{"input_duplicate", map[string]any{"input_schema_families": []string{"source_catalog_v1", "source_catalog_v1"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := map[string]any{}
			if err := json.Unmarshal(canonical(intentFixture()), &body); err != nil {
				t.Fatal(err)
			}
			hc := body["hard_constraints"].(map[string]any)
			for k, v := range c.value {
				hc[k] = v
			}
			code, out := a.call("POST", "/v1/intents", "invalid-typed-"+c.name, body)
			if code != 422 {
				t.Fatal(code, out)
			}
		})
	}
	var count int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.intents i JOIN undergrid.principals p ON p.id=i.principal_id WHERE p.subject=$1", a.owner).Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid constraints created state", count, err)
	}
}

func TestTypedDemandConstraintCombinationLeavesOriginalInputsImmutable(t *testing.T) {
	at := time.Now().UTC().Add(time.Hour)
	constraints := map[string]any{"coverage": map[string]any{"required": []string{"region:us", "region:eu"}, "excluded": []string{}}, "freshness": map[string]any{"max_age_seconds": 300}, "delivery": map[string]any{"not_before": at, "deliver_by": at.Add(time.Hour)}, "input_schema_families": []string{"source_catalog_v1"}}
	a, b := intentWithTypedConstraints(t, constraints), intentWithTypedConstraints(t, constraints)
	beforeA, beforeB := string(canonical(a)), string(canonical(b))
	proof := CompareDemand(a, b)
	if !proof.Compatible || proof.CombinedConstraints == nil {
		t.Fatal(proof)
	}
	proof.CombinedConstraints.Coverage.Required[0] = "changed"
	proof.CombinedConstraints.Freshness.MaxAgeSeconds = 1
	*proof.CombinedConstraints.Delivery.NotBefore = at.Add(100 * time.Hour)
	(*proof.CombinedConstraints.InputSchemaFamilies)[0] = "changed"
	if string(canonical(a)) != beforeA || string(canonical(b)) != beforeB {
		t.Fatal("combined view mutated immutable input")
	}
}

func TestExplicitUnrestrictedCoverageDoesNotInventMissingHardRequirements(t *testing.T) {
	a := intentWithTypedConstraints(t, map[string]any{"coverage": map[string]any{"required": []string{}, "excluded": []string{}}})
	b := intentFixture()
	proof := CompareDemand(a, b)
	if !proof.Compatible {
		t.Fatalf("no coverage restriction became a blocker: %+v", proof)
	}
	fields := []string{"source_id", "source_url"}
	claim := &capabilityClaim{Version: "capability-claim-v1", Output: &capabilityOutput{Kind: "json_dataset", SchemaFamily: "source_catalog_v1"}, ProvidedFields: &fields, FieldsComplete: true, LicenseIDs: []string{"CC0-1.0"}}
	for _, row := range compareExternalClaim(a, claim) {
		if row.Status == "unknown" {
			t.Fatal("empty coverage invented missing publisher evidence", row)
		}
	}
}
func TestTypedDeliveryEligibilityExpiresAtReadWithoutWaitingForWorker(t *testing.T) {
	s, db, workerDSN := semanticFixture(t)
	a, b := client(t, s), client(t, s)
	by := time.Now().UTC().Add(3 * time.Second).Truncate(time.Microsecond)
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Delivery eligibility " + NewID()
	raw := map[string]any{}
	if err := json.Unmarshal(canonical(input), &raw); err != nil {
		t.Fatal(err)
	}
	raw["hard_constraints"].(map[string]any)["delivery"] = map[string]any{"deliver_by": by}
	code, first := a.call("POST", "/v1/intents", "delivery-read-first", raw)
	if code != 201 {
		t.Fatal(code, first)
	}
	code, second := b.call("POST", "/v1/intents", "delivery-read-second", raw)
	if code != 201 {
		t.Fatal(code, second)
	}
	worker := New(workerDSN, "development", nil)
	if err := worker.scanIntentions(context.Background(), first["id"].(string)); err != nil {
		t.Fatal(err)
	}
	path := "/v1/intents/" + first["id"].(string) + "/opportunities"
	code, page := a.call("GET", path, "", nil)
	if code != 200 || len(page["items"].([]any)) != 1 {
		t.Fatal("pre-deadline suggestion absent", code, page)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		var elapsed bool
		if err := db.QueryRow(ctx, "SELECT clock_timestamp()>$1::timestamptz", by).Scan(&elapsed); err != nil {
			t.Fatal(err)
		}
		if elapsed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	code, page = a.call("GET", path, "", nil)
	if code != 200 {
		t.Fatal(code, page)
	}
	for _, row := range page["items"].([]any) {
		if row.(map[string]any)["candidate_intent_id"] == second["id"] {
			t.Fatal("elapsed delivery remained eligible without a worker sweep", row)
		}
	}
}
