package network

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

type declaredConstraint struct {
	Constraint    string   `json:"constraint"`
	Requested     []string `json:"requested"`
	Declared      []string `json:"declared"`
	Status        string   `json:"status"`
	Reason        string   `json:"reason"`
	EvidenceField string   `json:"evidence_field"`
	EvidenceClass string   `json:"evidence_class"`
}

func compareExternalClaim(input IntentInput, claim *capabilityClaim) []declaredConstraint {
	rows := []declaredConstraint{}
	add := func(name string, want, have []string, status, reason, field string) {
		if want == nil {
			want = []string{}
		}
		if have == nil {
			have = []string{}
		}
		rows = append(rows, declaredConstraint{name, want, have, status, reason, field, "unverified_publisher_claim"})
	}
	output := capabilityOutput{}
	if claim != nil && claim.Output != nil {
		output = *claim.Output
	}
	for _, v := range []struct{ name, want, have, field string }{{"output_kind", input.Output.Kind, output.Kind, "capability_claim.output.kind"}, {"schema_family", input.Output.SchemaFamily, output.SchemaFamily, "capability_claim.output.schema_family"}} {
		status, reason := "unknown", "declaration_missing"
		have := []string{}
		if v.have != "" {
			have = []string{v.have}
			status, reason = "declared_incompatible", "declared_output_mismatch"
			if v.want == v.have {
				status, reason = "declared_aligned", "declared_output_equal"
			}
		}
		add(v.name, []string{v.want}, have, status, reason, v.field)
	}
	for _, required := range input.HardConstraints.RequiredFields {
		status, reason := "unknown", "field_set_missing"
		have := []string{}
		if claim != nil && claim.ProvidedFields != nil {
			have = *claim.ProvidedFields
			reason = "not_declared_in_partial_field_set"
			if slices.Contains(have, required) {
				status, reason = "declared_aligned", "required_field_declared"
			} else if claim.FieldsComplete {
				status, reason = "declared_incompatible", "missing_from_complete_declared_field_set"
			}
		}
		declared := []string{}
		if slices.Contains(have, required) {
			declared = []string{required}
		}
		add("required_field", []string{required}, declared, status, reason, "capability_claim.provided_fields")
	}
	licenses := []string{}
	if claim != nil {
		licenses = claim.LicenseIDs
	}
	status, reason := "unknown", "license_evidence_missing"
	if len(input.HardConstraints.LicenseIDs) > 0 && len(licenses) > 0 {
		status, reason = "declared_incompatible", "declared_license_options_disjoint"
		for _, license := range input.HardConstraints.LicenseIDs {
			if slices.Contains(licenses, license) {
				status, reason = "declared_aligned", "declared_license_option_intersection"
				break
			}
		}
	}
	add("license_ids", input.HardConstraints.LicenseIDs, licenses, status, reason, "capability_claim.license_ids")
	return rows
}
func externalFitIntentID(path string) (string, bool) {
	const prefix = "/v1/intents/"
	const suffix = "/external-fit-report"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	return id, validID(id)
}
func (s *Service) externalFitReport(r *http.Request, tx pgx.Tx, a actor, intent string) (int, any, error) {
	if r.Method != http.MethodGet {
		return 0, nil, fail(405, "method_not_allowed")
	}
	if len(r.URL.RawQuery) > 256 {
		return 0, nil, fail(400, "invalid_fit_query")
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 1 || len(query["lead_id"]) != 1 || !validID(query.Get("lead_id")) {
		return 0, nil, fail(400, "invalid_fit_query")
	}
	var revision int
	var version int64
	var document []byte
	err = tx.QueryRow(r.Context(), `SELECT i.current_revision,i.version,r.document FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision JOIN agents g ON g.id=i.agent_id WHERE i.id=$1 AND i.principal_id=$2 AND i.state='active' AND i.expires_at>clock_timestamp() AND g.status='active'`, intent, a.PrincipalID).Scan(&revision, &version, &document)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	var input IntentInput
	if err = json.Unmarshal(document, &input); err != nil {
		return 0, nil, err
	}
	var lead externalLead
	var rawClaim []byte
	err = tx.QueryRow(r.Context(), `SELECT * FROM external_fit_source($1,$2)`, intent, query.Get("lead_id")).Scan(&lead.ID, &lead.SourceID, &lead.SourceVersion, &lead.Origin, &lead.CanonicalURL, &lead.Publisher, &lead.PermissionNotice, &lead.Metadata, &lead.ObservedAt, &lead.CheckedAt, &rawClaim)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	if lead.Origin != "external_capability" {
		return 0, nil, fail(422, "capability_lead_required")
	}
	report := externalFitReportValue(intent, revision, input, lead, rawClaim)
	report["intention_version"] = version
	report["intention_etag"] = etag(version)
	return 200, report, nil
}

// The selected-source read and owner action use the same report calculation;
// partial-satisfaction retries supply the original immutable requirements.
func externalFitReportValue(intent string, revision int, input IntentInput, lead externalLead, rawClaim []byte) map[string]any {
	lead = externalLeadProjection(lead)
	claim, claimErr := parseCapabilityClaim(rawClaim)
	claimStatus := "declared"
	if claimErr != nil {
		claimStatus = "invalid"
		claim = nil
	} else if claim == nil {
		claimStatus = "missing"
	}
	constraints := compareExternalClaim(input, claim)
	assessment := "declared_aligned"
	for _, constraint := range constraints {
		if constraint.Status == "declared_incompatible" {
			assessment = "incompatible"
			break
		}
		if constraint.Status == "unknown" {
			assessment = "needs_clarification"
		}
	}
	requirementsHash := digest(canonical(struct {
		Output      Output
		Constraints Constraints
	}{input.Output, input.HardConstraints}))
	claimHash := digest(canonical(claim))
	report := map[string]any{"algorithm": "external-claim-constraints-v1", "intention_id": intent, "intention_revision": revision, "requirements_hash": requirementsHash, "lead": lead, "claim": claim, "claim_status": claimStatus, "claim_hash": claimHash, "constraints": constraints, "assessment": assessment, "identity_assurance": "unknown", "rights_assurance": "unknown", "verification": "unverified_publisher_claim"}
	report["report_hash"] = digest(canonical(map[string]any{"algorithm": report["algorithm"], "intention_id": intent, "revision": revision, "requirements_hash": requirementsHash, "projection_hash": lead.ProjectionHash, "claim_hash": claimHash, "claim_status": claimStatus, "constraints": constraints, "assessment": assessment}))
	return report
}
