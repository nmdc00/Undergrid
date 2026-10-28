package network

import (
	"strings"
)

type ConstraintAssessment struct {
	Dimension string `json:"dimension"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
}

type Compatibility struct {
	MinimumSimilarity       *float64               `json:"minimum_similarity,omitempty"`
	Compatible              bool                   `json:"compatible"`
	Status                  string                 `json:"status"`
	Reasons                 []string               `json:"reasons"`
	Algorithm               string                 `json:"algorithm"`
	Similarity              *float64               `json:"similarity,omitempty"`
	SemanticProfileHash     string                 `json:"semantic_profile_hash,omitempty"`
	Model                   string                 `json:"model,omitempty"`
	ModelRevision           string                 `json:"model_revision,omitempty"`
	SourceProjectionHash    string                 `json:"source_projection_hash,omitempty"`
	CandidateProjectionHash string                 `json:"candidate_projection_hash,omitempty"`
	TextRank                float64                `json:"text_rank,omitempty"`
	ConstraintEvidence      []ConstraintAssessment `json:"constraint_evidence,omitempty"`
	CombinedConstraints     *Constraints           `json:"combined_constraints,omitempty"`
}

func CompareDemand(a, b IntentInput) Compatibility {
	e := compareDemandConstraints(a, b)
	if !e.Compatible {
		return e
	}
	if declaredOutcome(a) != declaredOutcome(b) {
		e.Compatible = false
		e.Status = "needs_clarification"
		e.Reasons = []string{"semantic_comparison_pending"}
		return e
	}
	e.Reasons = append([]string{"same_declared_outcome"}, e.Reasons...)
	return e
}

// A private source stays within its authorized transaction; candidates use only
// their approved public projection. This compares declarations, not semantics.
func declaredOutcome(i IntentInput) string {
	if i.PublicSummary != "" {
		return strings.ToLower(strings.TrimSpace(i.PublicSummary))
	}
	return strings.ToLower(strings.TrimSpace(i.Objective))
}
