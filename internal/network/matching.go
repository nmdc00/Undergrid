package network

import (
	"slices"
	"strings"
)

type Compatibility struct {
	Compatible            bool     `json:"compatible"`
	Status                string   `json:"status"`
	Reasons               []string `json:"reasons"`
	Algorithm             string   `json:"algorithm"`
	Similarity            *float64 `json:"similarity,omitempty"`
	SemanticProfileHash   string   `json:"semantic_profile_hash,omitempty"`
	Model                 string   `json:"model,omitempty"`
	ModelRevision         string   `json:"model_revision,omitempty"`
	SourceRevisionHash    string   `json:"source_revision_hash,omitempty"`
	CandidateRevisionHash string   `json:"candidate_revision_hash,omitempty"`
	TextRank              float64  `json:"text_rank,omitempty"`
}

func compareDemandConstraints(a, b IntentInput) Compatibility {
	e := Compatibility{Status: "compatible", Compatible: true, Reasons: []string{}, Algorithm: "hard-constraints-exact-v1"}
	reject := func(status, reason string) Compatibility {
		e.Compatible = false
		e.Status = status
		e.Reasons = append(e.Reasons, reason)
		return e
	}
	if a.Output != b.Output {
		return reject("incompatible", "output_schema_mismatch")
	}
	if len(a.HardConstraints.LicenseIDs) == 0 || len(b.HardConstraints.LicenseIDs) == 0 {
		return reject("needs_clarification", "license_unspecified")
	}
	overlap := false
	for _, license := range a.HardConstraints.LicenseIDs {
		if slices.Contains(b.HardConstraints.LicenseIDs, license) {
			overlap = true
			break
		}
	}
	if !overlap {
		return reject("incompatible", "no_common_license")
	}
	e.Reasons = []string{"same_output_schema", "common_permitted_license", "required_fields_can_be_combined"}
	return e
}

func CompareDemand(a, b IntentInput) Compatibility {
	e := compareDemandConstraints(a, b)
	if !e.Compatible {
		return e
	}
	text := func(i IntentInput) string {
		if i.PublicSummary != "" {
			return strings.ToLower(strings.TrimSpace(i.PublicSummary))
		}
		return strings.ToLower(strings.TrimSpace(i.Objective))
	}
	if text(a) != text(b) {
		e.Compatible = false
		e.Status = "needs_clarification"
		e.Reasons = []string{"semantic_comparison_pending"}
		return e
	}
	e.Reasons = append([]string{"same_declared_outcome"}, e.Reasons...)
	return e
}
