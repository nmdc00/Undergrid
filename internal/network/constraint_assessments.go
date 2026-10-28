package network

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"
)

type demandConstraintReport struct {
	SourceIntentID    string                 `json:"source_intent_id"`
	SourceVersion     int64                  `json:"source_version"`
	SourceRevision    int                    `json:"source_revision"`
	Scope             string                 `json:"scope"`
	IdentityAssurance string                 `json:"identity_assurance"`
	CandidateLimit    int                    `json:"candidate_limit"`
	Truncated         bool                   `json:"truncated"`
	Items             []demandConstraintItem `json:"items"`
}
type demandConstraintItem struct {
	CandidateIntentID     string                 `json:"candidate_intent_id"`
	CandidateVersion      int64                  `json:"candidate_version"`
	PrincipalID           string                 `json:"principal_id"`
	PublicSummary         string                 `json:"public_summary"`
	Output                Output                 `json:"output"`
	HardConstraints       Constraints            `json:"hard_constraints"`
	HardStatus            string                 `json:"hard_status"`
	OutcomeComparison     string                 `json:"outcome_comparison"`
	ClarificationRequired bool                   `json:"clarification_required"`
	ConstraintEvidence    []ConstraintAssessment `json:"constraint_evidence"`
}

// A bounded current public-declaration report, not an opportunity or permission.
// Scope/ownership/state checks live here for every transport. Source state stays
// pinned through the read; public candidates use the existing disclosure boundary.
func (s *Service) constraintAssessments(r *http.Request, tx pgx.Tx, a actor, id string) (int, any, error) {
	if r.Method != http.MethodGet {
		return 0, nil, fail(405, "method_not_allowed")
	}
	if !slices.Contains(a.Scopes, "intents:read") {
		return 0, nil, insufficientScope(a, "intents:read")
	}
	if !validID(id) {
		return 0, nil, fail(404, "not_found")
	}
	report := demandConstraintReport{SourceIntentID: id, Scope: "registered_demand_hard_evidence_v1", IdentityAssurance: "common_control_unknown", CandidateLimit: 20, Items: []demandConstraintItem{}}
	var raw []byte
	err := tx.QueryRow(r.Context(), `SELECT r.document,i.version,i.current_revision FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1 AND i.principal_id=$2 AND i.agent_id=$3 AND i.state='active' AND i.expires_at>clock_timestamp() FOR SHARE OF i NOWAIT`, id, a.PrincipalID, a.AgentID).Scan(&raw, &report.SourceVersion, &report.SourceRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	var source IntentInput
	if err = json.Unmarshal(raw, &source); err != nil {
		return 0, nil, err
	}
	rows, err := tx.Query(r.Context(), `SELECT DISTINCT ON (principal_id) id,principal_id,version,projection FROM public_intents() WHERE principal_id<>$1 ORDER BY principal_id,id LIMIT $2`, a.PrincipalID, report.CandidateLimit+1)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item demandConstraintItem
		var projection []byte
		if err = rows.Scan(&item.CandidateIntentID, &item.PrincipalID, &item.CandidateVersion, &projection); err != nil {
			return 0, nil, err
		}
		if len(report.Items) == report.CandidateLimit {
			report.Truncated = true
			break
		}
		var candidate IntentInput
		if err = json.Unmarshal(projection, &candidate); err != nil {
			return 0, nil, err
		}
		evidence := assessDemandConstraints(source, candidate, true)
		item.PublicSummary = candidate.PublicSummary
		item.Output = candidate.Output
		item.HardConstraints = candidate.HardConstraints
		item.HardStatus = "aligned"
		if evidence.Status == "incompatible" {
			item.HardStatus = "incompatible"
		} else if !evidence.Compatible {
			item.HardStatus = "unknown"
		}
		item.OutcomeComparison = "not_evaluated"
		if declaredOutcome(source) == declaredOutcome(candidate) {
			item.OutcomeComparison = "same_declared_outcome"
		}
		item.ConstraintEvidence = evidence.ConstraintEvidence
		for _, dimension := range item.ConstraintEvidence {
			if dimension.Status == "unknown" {
				item.ClarificationRequired = true
			}
		}
		report.Items = append(report.Items, item)
	}
	if err = rows.Err(); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, report, nil
}
