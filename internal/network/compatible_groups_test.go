package network

import "testing"

// These are deterministic group-policy oracles, not provider qualification.
func TestCompatibleGroupsRejectTransitiveAndGlobalLicenseAssumptions(t *testing.T) {
	input := intentFixture()
	input.PublicSummary = "Same desired outcome"
	input.Visibility = "redacted_public"
	makeMember := func(id, principal string, licenses ...string) groupCandidate {
		v := input
		v.HardConstraints.LicenseIDs = licenses
		return groupCandidate{ID: id, PrincipalID: principal, Input: v}
	}
	source := makeMember("source", "owner", "A", "B")
	candidates := []groupCandidate{makeMember("one", "one-owner", "B", "C"), makeMember("two", "two-owner", "A", "C"), makeMember("duplicate", "one-owner", "A", "B", "C")}
	pairs := func(a, b groupCandidate) bool { return CompareDemand(a.Input, b.Input).Compatible }
	groups := computeCompatibleGroups(source, candidates, pairs)
	if len(groups) == 0 {
		t.Fatal("compatible pairs missing")
	}
	for _, g := range groups {
		seen := map[string]bool{}
		common := source.Input.HardConstraints.LicenseIDs
		for _, m := range g {
			if seen[m.PrincipalID] {
				t.Fatal("one principal counted twice", g)
			}
			seen[m.PrincipalID] = true
			common = intersectLabels(common, m.Input.HardConstraints.LicenseIDs)
			for _, other := range g {
				if m.ID != other.ID && !pairs(m, other) {
					t.Fatal("nonpairwise group", g)
				}
			}
		}
		if len(common) == 0 {
			t.Fatal("pairwise licenses treated as global license", g)
		}
	}
}
func TestCompatibleGroupsDoNotInferTransitivityOrIgnoreSourceOwner(t *testing.T) {
	input := intentFixture()
	member := func(id, principal string) groupCandidate {
		return groupCandidate{ID: id, PrincipalID: principal, Input: input}
	}
	source := member("a", "owner")
	candidates := []groupCandidate{member("b", "b-owner"), member("c", "c-owner"), member("a-other-agent", "owner")}
	pairs := func(a, b groupCandidate) bool { return a.ID == "a" || b.ID == "a" }
	groups := computeCompatibleGroups(source, candidates, pairs)
	if len(groups) != 2 {
		t.Fatal("expected separate compatible pair views", len(groups))
	}
	for _, g := range groups {
		if len(g) != 2 || g[0].ID != "a" || g[1].PrincipalID == "owner" {
			t.Fatal("transitive or common-principal group", g)
		}
	}
}

func TestCompatibleGroupsRequireGlobalInputSchemaAlternative(t *testing.T) {
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "One declared outcome"
	member := func(id string, options ...string) groupCandidate {
		v := input
		v.HardConstraints.InputSchemaFamilies = &options
		return groupCandidate{ID: id, PrincipalID: id + "-owner", Input: v}
	}
	source := member("source", "A", "B")
	candidates := []groupCandidate{member("one", "B", "C"), member("two", "A", "C")}
	groups := computeCompatibleGroups(source, candidates, func(a, b groupCandidate) bool { return CompareDemand(a.Input, b.Input).Compatible })
	if len(groups) == 0 {
		t.Fatal("valid pair views absent")
	}
	for _, group := range groups {
		common := *source.Input.HardConstraints.InputSchemaFamilies
		for _, m := range group {
			common = intersectLabels(common, *m.Input.HardConstraints.InputSchemaFamilies)
		}
		if len(common) == 0 {
			t.Fatal("pairwise overlap presented as globally feasible input", group)
		}
	}
}

func TestCompatibleGroupsDoNotCountExternalObservationsAsMembers(t *testing.T) {
	id := NewID()
	items := []any{
		map[string]any{"kind": "external_request", "observation_id": NewID()},
		map[string]any{"kind": "shared_demand", "candidate_intent_id": id},
		map[string]any{"kind": "external_capability", "observation_id": NewID()},
	}
	ids := groupOpportunityIDs(items)
	if len(ids) != 1 || ids[0] != id {
		t.Fatal("external observation counted as registered group member", ids)
	}
}
