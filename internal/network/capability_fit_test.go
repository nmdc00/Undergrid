package network

import (
	"context"
	"strings"
	"testing"
)

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

func TestCapabilityFitRechecksDisclosureAndCreatesNoEffects(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	owner, builder, foreign := client(t, s), collaborationClient(t, s), client(t, s)
	_, who := builder.call("GET", "/v1/me", "", nil)
	_, private := foreign.call("GET", "/v1/me", "", nil)
	input := intentFixture()
	input.Watch.Enabled = false
	code, source := owner.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, source)
	}
	id := source["id"].(string)
	t.Cleanup(func() { retireClarificationFixture(t, db, owner, id) })
	path := "/v1/owner/intents/" + id + "/capability-fit"
	if code, out := ownerCallMatch(foreign, "GET", path, "", "", nil); code != 404 {
		t.Fatal("foreign source readable", code, out)
	}
	const effects = `SELECT (SELECT count(*) FROM undergrid.opportunities)+(SELECT count(*) FROM undergrid.events)+(SELECT count(*) FROM undergrid.inbox)+(SELECT count(*) FROM undergrid_queue.river_job)+(SELECT count(*) FROM undergrid.swarms)+(SELECT count(*) FROM undergrid.memberships)+(SELECT count(*) FROM undergrid.approvals)+(SELECT count(*) FROM undergrid.tasks)`
	var before, after int
	if err := db.QueryRow(context.Background(), effects).Scan(&before); err != nil {
		t.Fatal(err)
	}
	read := func() map[string]any {
		t.Helper()
		code, out := ownerCallMatch(owner, "GET", path, "", "", nil)
		if code != 200 {
			t.Fatal(code, out)
		}
		if strings.Contains(string(canonical(out)), private["agent_id"].(string)) {
			t.Fatal("private agent exposed", out)
		}
		for _, key := range []string{`"endpoint"`, `"token"`, `"accept_invitations"`, `"recruitment"`, `"owner"`} {
			if strings.Contains(string(canonical(out)), key) {
				t.Fatal("unselected disclosure", key, out)
			}
		}
		return out
	}
	read()
	if err := db.QueryRow(context.Background(), effects).Scan(&after); err != nil || before != after {
		t.Fatal("read effects", before, after, err)
	}
	if _, err := db.Exec(context.Background(), `UPDATE undergrid.agents SET status='revoked' WHERE id=$1`, who["agent_id"]); err != nil {
		t.Fatal(err)
	}
	out := read()
	for _, raw := range out["agents"].(map[string]any)["items"].([]any) {
		if raw.(map[string]any)["agent_id"] == who["agent_id"] {
			t.Fatal("revoked declaration retained", out)
		}
	}
}
