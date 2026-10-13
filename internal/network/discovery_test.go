package network

import (
	"context"
	"encoding/json"
	"testing"
)

func TestStandingIntentDiscoversIndependentPublicDemand(t *testing.T) {
	s, db := fixture(t)
	a, b := client(t, s), client(t, s)
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Continuously maintained source catalog " + NewID()
	code, first := a.call("POST", "/v1/intents", "first", input)
	if code != 201 {
		t.Fatal(code, first)
	}
	// No invented participant: an empty network returns an active watch and no match.
	code, empty := a.call("GET", "/v1/intents/"+first["id"].(string)+"/opportunities", "", nil)
	if code != 200 {
		t.Fatal(code, empty)
	}
	if len(empty["items"].([]any)) != 0 {
		t.Fatal("fake liquidity", empty)
	}
	input.Objective = "Private descriptive detail"
	budget := []byte(`{"amount_minor":99999,"currency":"EUR","kind":"stated_budget"}`)
	input.Budget = rawJSON(budget)
	code, second := b.call("POST", "/v1/intents", "second", input)
	if code != 201 {
		t.Fatal(code, second)
	}
	if err := s.ScanDueIntentions(context.Background()); err != nil {
		t.Fatal(err)
	}
	code, found := a.call("GET", "/v1/intents/"+first["id"].(string)+"/opportunities", "", nil)
	if code != 200 {
		t.Fatal(code, found)
	}
	items := found["items"].([]any)
	if len(items) != 1 {
		t.Fatal(found)
	}
	item := items[0].(map[string]any)
	if item["candidate_intent_id"] != second["id"] || item["origin"] != "registered_intent" || item["kind"] != "shared_demand" {
		t.Fatal(item)
	}
	code, public := a.call("GET", "/v1/intents/"+second["id"].(string), "", nil)
	if code != 200 || public["budget"] != nil || public["objective"] != nil {
		t.Fatal("projection leak", code, public)
	}
	// Re-running the worker must not duplicate either opportunity or notification.
	if err := s.ScanDueIntentions(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.opportunities WHERE source_intent_id=$1", first["id"]).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	// This is separate test ownership, not a claim of independent beneficial control.
}

func rawJSON(b []byte) *json.RawMessage { raw := json.RawMessage(b); return &raw }

func TestMatchingHardConstraintsCannotBeOverridden(t *testing.T) {
	a, b := intentFixture(), intentFixture()
	b.HardConstraints.LicenseIDs = []string{"proprietary"}
	e := CompareDemand(a, b)
	if e.Compatible || e.Status != "incompatible" {
		t.Fatal(e)
	}
	b.HardConstraints.LicenseIDs = nil
	e = CompareDemand(a, b)
	if e.Compatible || e.Status != "needs_clarification" {
		t.Fatal(e)
	}
	b = a
	e = CompareDemand(a, b)
	if !e.Compatible || len(e.Reasons) == 0 {
		t.Fatal(e)
	}
}
