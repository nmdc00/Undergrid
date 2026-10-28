package network

import "testing"

// This gate requires a fresh controlled population, because a bounded public
// sample cannot promise to include arbitrary newly registered principals.
func TestCapabilityFitBoundsDistinctPublicAgentDeclarations(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	owner := client(t, s)
	input := intentFixture()
	input.Watch.Enabled = false
	code, source := owner.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, source)
	}
	id := source["id"].(string)
	t.Cleanup(func() { retireClarificationFixture(t, db, owner, id) })
	public := func(actor testClient) {
		t.Helper()
		code, out := actor.call("POST", "/v1/agents", NewID(), map[string]any{"name": "Public declared schema", "scopes": []string{"intents:read"}, "profile": map[string]any{"public": true, "accept_invitations": false, "recruitment": false, "capabilities": []string{"source_catalog_v1"}}})
		if code != 201 {
			t.Fatal(code, out)
		}
	}
	// Passive public declarations are discoverable without recruitment consent.
	public(owner)
	first, second := client(t, s), client(t, s)
	public(first)
	public(first)
	public(second)
	_, identity := owner.call("GET", "/v1/me", "", nil)
	check := func(want int, truncated bool) {
		t.Helper()
		code, out := owner.call("GET", "/v1/intents/"+id+"/capability-fit", "", nil)
		if code != 200 {
			t.Fatal(code, out)
		}
		sample := out["agents"].(map[string]any)
		rows := sample["items"].([]any)
		if len(rows) != want || sample["truncated"] != truncated || sample["limit"] != float64(20) {
			t.Fatalf("bounded public sample: %v", out)
		}
		seen := map[string]bool{}
		for _, raw := range rows {
			row := raw.(map[string]any)
			principal := row["principal_id"].(string)
			if principal == identity["principal_id"] || seen[principal] {
				t.Fatal("own or duplicate principal", row)
			}
			seen[principal] = true
			if row["schema_support"] != "declared" || row["hard_status"] != "unknown" {
				t.Fatal("passive declaration misrepresented", row)
			}
		}
	}
	check(2, false)
	for range 19 {
		public(client(t, s))
	}
	check(20, true)
}
