package network

import "testing"

func TestCapabilityFitReadsPublicBuilderDeclarationsWithoutInventingAuthority(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	owner, builder := client(t, s), collaborationClient(t, s)
	code, identity := builder.call("GET", "/v1/me", "", nil)
	if code != 200 {
		t.Fatal(code, identity)
	}
	input := intentFixture()
	input.Watch.Enabled = false
	input.HardConstraints.Coverage = &CoverageConstraint{Required: []string{"region:eu"}, Excluded: []string{}}
	code, source := owner.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, source)
	}
	id := source["id"].(string)
	t.Cleanup(func() { retireClarificationFixture(t, db, owner, id) })
	for _, ownerSession := range []bool{false, true} {
		name, prefix := "agent", "/v1/intents/"
		if ownerSession {
			name, prefix = "owner", "/v1/owner/intents/"
		}
		t.Run(name, func(t *testing.T) {
			var status int
			var report map[string]any
			path := prefix + id + "/capability-fit"
			if ownerSession {
				status, report = ownerCallMatch(owner, "GET", path, "", "", nil)
			} else {
				status, report = owner.call("GET", path, "", nil)
			}
			if status != 200 {
				t.Fatalf("current capability fit unavailable: %d %v", status, report)
			}
			if report["source_intent_id"] != id || report["source_version"] != float64(1) || report["identity_assurance"] != "common_control_unknown" {
				t.Fatal("unbound capability report", report)
			}
			agents, ok := report["agents"].(map[string]any)
			if !ok {
				t.Fatal("missing bounded public-agent sample", report)
			}
			rows, ok := agents["items"].([]any)
			if !ok {
				t.Fatal("missing agent items", agents)
			}
			found := false
			for _, raw := range rows {
				row := raw.(map[string]any)
				if row["agent_id"] != identity["agent_id"] {
					continue
				}
				found = true
				if row["origin"] != "registered_capability" || row["assurance"] != "self_declared" || row["schema_support"] != "declared" || row["hard_status"] != "unknown" {
					t.Fatal("invented complete capability fit", row)
				}
			}
			if !found {
				t.Fatal("current public builder absent", report)
			}
		})
	}
}
