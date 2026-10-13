package network

import (
	"slices"
	"strings"
)

type Compatibility struct {
	Compatible bool     `json:"compatible"`
	Status     string   `json:"status"`
	Reasons    []string `json:"reasons"`
	Algorithm  string   `json:"algorithm"`
}

func CompareDemand(a, b IntentInput) Compatibility {
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
	text := func(i IntentInput) string {
		if i.PublicSummary != "" {
			return strings.ToLower(strings.TrimSpace(i.PublicSummary))
		}
		return strings.ToLower(strings.TrimSpace(i.Objective))
	}
	if text(a) != text(b) {
		return reject("needs_clarification", "semantic_comparison_pending")
	}
	e.Reasons = []string{"same_declared_outcome", "same_output_schema", "common_permitted_license", "required_fields_can_be_combined"}
	return e
}
