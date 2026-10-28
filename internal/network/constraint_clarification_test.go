package network

import (
	"context"
	"strings"
	"testing"
)

func TestConstraintAssessmentReportsUnknownHardEvidenceWithoutInventingMatches(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	owner := client(t, s)
	source := intentFixture()
	source.Objective = "Clarify declared catalogue coverage " + NewID()
	source.HardConstraints.Coverage = &CoverageConstraint{Required: []string{"region:eu"}, Excluded: []string{}}
	source.Watch.Enabled = false // Explicit read is distinct from ongoing monitoring.
	code, created := owner.call("POST", "/v1/intents", "clarification-source", source)
	if code != 201 {
		t.Fatalf("source fixture: %d %v", code, created)
	}
	id := created["id"].(string)
	ids := map[string]string{}
	privateSecret := "UNPUBLISHED candidate objective " + NewID()
	for _, kind := range []string{"unknown", "aligned-prose", "incompatible", "private"} {
		candidateOwner := client(t, s)
		candidate := intentFixture()
		candidate.Objective = source.Objective
		candidate.Visibility = "redacted_public"
		candidate.PublicSummary = source.Objective
		if kind != "unknown" {
			candidate.HardConstraints.Coverage = &CoverageConstraint{Required: []string{"region:eu"}, Excluded: []string{}}
		}
		if kind == "aligned-prose" {
			candidate.PublicSummary = "A different declared outcome " + NewID()
		}
		if kind == "incompatible" {
			candidate.HardConstraints.LicenseIDs = []string{"ODC-BY-1.0"}
		}
		if kind == "private" {
			candidate.Visibility = "private"
			candidate.PublicSummary = ""
			candidate.Objective = privateSecret
		}
		code, out := candidateOwner.call("POST", "/v1/intents", "clarification-candidate-"+kind, candidate)
		if code != 201 {
			t.Fatalf("%s fixture: %d %v", kind, code, out)
		}
		ids[kind] = out["id"].(string)
	}
	snapshot := func() [5]int {
		t.Helper()
		var v [5]int
		err := db.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM undergrid.opportunities),(SELECT count(*) FROM undergrid.events),(SELECT count(*) FROM undergrid.inbox),(SELECT count(*) FROM undergrid_queue.river_job),(SELECT count(*) FROM undergrid.memberships)`).Scan(&v[0], &v[1], &v[2], &v[3], &v[4])
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	before := snapshot()
	var ownerResult map[string]any
	for _, transport := range []string{"owner", "agent"} {
		t.Run(transport, func(t *testing.T) {
			path := "/v1/intents/" + id + "/constraint-assessments"
			if transport == "owner" {
				path = "/v1/owner/intents/" + id + "/constraint-assessments"
			}
			code, report := owner.call("GET", path, "", nil)
			if code != 200 {
				t.Fatalf("current hard-evidence report unavailable: %d %v", code, report)
			}
			if report["source_intent_id"] != id || report["source_version"] != float64(1) || report["source_revision"] != float64(1) || report["scope"] != "registered_demand_hard_evidence_v1" {
				t.Fatalf("unbound report: %v", report)
			}
			rows, ok := report["items"].([]any)
			if !ok || len(rows) != 3 || report["candidate_limit"] != float64(20) || report["truncated"] != false {
				t.Fatalf("bounded permitted reports: %v", report)
			}
			byID := map[string]map[string]any{}
			for _, raw := range rows {
				row := raw.(map[string]any)
				byID[row["candidate_intent_id"].(string)] = row
			}
			unknown, aligned, incompatible := byID[ids["unknown"]], byID[ids["aligned-prose"]], byID[ids["incompatible"]]
			if unknown == nil || unknown["hard_status"] != "unknown" || unknown["clarification_required"] != true || unknown["outcome_comparison"] != "same_declared_outcome" {
				t.Fatalf("missing coverage counted as alignment: %v", unknown)
			}
			evidence, ok := unknown["constraint_evidence"].([]any)
			found := false
			if ok {
				for _, raw := range evidence {
					row := raw.(map[string]any)
					if row["dimension"] == "coverage" && row["status"] == "unknown" && row["reason"] == "coverage_unspecified" {
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("actual missing dimension absent: %v", unknown)
			}
			if aligned == nil || aligned["hard_status"] != "aligned" || aligned["clarification_required"] != false || aligned["outcome_comparison"] != "not_evaluated" {
				t.Fatalf("prose difference invented missing hard evidence: %v", aligned)
			}
			if incompatible == nil || incompatible["hard_status"] != "incompatible" || incompatible["clarification_required"] != false {
				t.Fatalf("license conflict lost: %v", incompatible)
			}
			if byID[ids["private"]] != nil || strings.Contains(string(canonical(report)), privateSecret) {
				t.Fatal("private declaration disclosed", report)
			}
			if transport == "owner" {
				ownerResult = report
			} else if ownerResult != nil && string(canonical(ownerResult)) != string(canonical(report)) {
				t.Fatal("adapters diverged", ownerResult, report)
			}
		})
	}
	if after := snapshot(); after != before {
		t.Fatalf("read created opportunities/notifications/jobs/admission: before=%v after=%v", before, after)
	}
}

// Opportunity identities and dismissal suppression include these exact evidence
// bytes. A richer read report must not revise the published legacy evidence.
func TestConstraintReportsPreserveLegacyMatchingEvidenceBytes(t *testing.T) {
	a, b := intentFixture(), intentFixture()
	const want = `{"compatible":true,"status":"compatible","reasons":["same_declared_outcome","same_output_schema","common_permitted_license","required_fields_can_be_combined"],"algorithm":"hard-constraints-exact-v1"}`
	if got := string(canonical(CompareDemand(a, b))); got != want {
		t.Fatalf("legacy evidence changed: %s", got)
	}
	_ = assessDemandConstraints(a, b, true)
	if got := string(canonical(CompareDemand(a, b))); got != want {
		t.Fatalf("report evaluation mutated legacy evidence: %s", got)
	}
}
