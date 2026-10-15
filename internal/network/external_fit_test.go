package network

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func seedExternalFitClaim(t *testing.T, db *pgx.Conn, claim any, permitted bool) (string, string) {
	t.Helper()
	fields := []string{"canonical_url", "title"}
	if permitted {
		fields = append(fields, "capability_claim")
	}
	source, lead := seedExternalReadLead(t, db, "external_capability", "Fixture publisher permits declared metadata", fields)
	raw, _ := json.Marshal(claim)
	if _, err := db.Exec(context.Background(), `UPDATE undergrid.external_observations SET metadata=jsonb_set(metadata,'{capability_claim}',$2::jsonb) WHERE id=$1`, lead, raw); err != nil {
		t.Fatal(err)
	}
	return source, lead
}
func declaredFitClaim() map[string]any {
	return map[string]any{"version": "capability-claim-v1", "output": map[string]any{"kind": "json_dataset", "schema_family": "source_catalog_v1"}, "provided_fields": []string{"source_id", "source_url"}, "fields_complete": true, "license_ids": []string{"CC0-1.0"}}
}
func TestExternalFitReportDeclaredEvidenceAndUnknowns(t *testing.T) {
	_, db, owner, intent := externalLeadFixture(t)
	source, lead := seedExternalFitClaim(t, db, declaredFitClaim(), true)
	path := "/v1/intents/" + intent + "/external-fit-report?lead_id=" + url.QueryEscape(lead)
	code, result := owner.call("GET", path, "", nil)
	if code != 200 {
		t.Fatalf("declared fit route unavailable %d %v", code, result)
	}
	if result["assessment"] != "declared_aligned" || result["algorithm"] != "external-claim-constraints-v1" || result["identity_assurance"] != "unknown" || result["rights_assurance"] != "unknown" || result["verification"] != "unverified_publisher_claim" {
		t.Fatalf("false or missing assurance %v", result)
	}
	raw, _ := json.Marshal(result)
	for _, forbidden := range []string{"PRIVATE", "secret-operator-endpoint", "execute me", "stated_budget"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("private/unselected material in report %s", raw)
		}
	}
	reportHash := result["report_hash"]
	code, ownerResult := owner.call("GET", "/v1/owner/intents/"+intent+"/external-fit-report?lead_id="+lead, "", nil)
	if code != 200 || ownerResult["report_hash"] != reportHash {
		t.Fatalf("owner report diverged %d %v", code, ownerResult)
	}
	if _, err := db.Exec(context.Background(), `UPDATE undergrid.external_sources SET config=jsonb_set(config,'{Fields}','["canonical_url","title"]') WHERE id=$1`, source); err != nil {
		t.Fatal(err)
	}
	code, result = owner.call("GET", path, "", nil)
	if code != 200 || result["assessment"] != "needs_clarification" {
		t.Fatalf("current claim permission ignored %d %v", code, result)
	}
	if result["report_hash"] == reportHash {
		t.Fatal("removed evidence left report hash unchanged")
	}
	if _, err := db.Exec(context.Background(), `UPDATE undergrid.external_sources SET config=config-'PublicPermissionNotice' WHERE id=$1`, source); err != nil {
		t.Fatal(err)
	}
	code, _ = owner.call("GET", path, "", nil)
	if code != 404 {
		t.Fatalf("revoked public source still assessable %d", code)
	}
}
func TestExternalFitReportRejectsProseInferenceAndContradictions(t *testing.T) {
	s, db, owner, intent := externalLeadFixture(t)
	for _, test := range []struct {
		name      string
		claim     any
		permitted bool
		want      string
	}{
		{"unselected typed claim", declaredFitClaim(), false, "needs_clarification"},
		{"absent typed claim", nil, true, "needs_clarification"},
		{"incompatible output", map[string]any{"version": "capability-claim-v1", "output": map[string]any{"kind": "csv_dataset", "schema_family": "other"}}, true, "incompatible"},
		{"partial missing fields", map[string]any{"version": "capability-claim-v1", "provided_fields": []string{"other"}, "fields_complete": false}, true, "needs_clarification"},
		{"complete missing fields", map[string]any{"version": "capability-claim-v1", "provided_fields": []string{"other"}, "fields_complete": true}, true, "incompatible"},
		{"license contradiction", map[string]any{"version": "capability-claim-v1", "license_ids": []string{"different-license"}}, true, "incompatible"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, lead := seedExternalFitClaim(t, db, test.claim, test.permitted)
			path := "/v1/intents/" + intent + "/external-fit-report?lead_id=" + lead
			code, result := owner.call("GET", path, "", nil)
			if code != 200 || result["assessment"] != test.want {
				t.Fatalf("assessment %d %v", code, result)
			}
			foreign := client(t, s)
			code, _ = foreign.call("GET", path, "", nil)
			if code != 404 {
				t.Fatalf("foreign report %d", code)
			}
		})
	}
}
