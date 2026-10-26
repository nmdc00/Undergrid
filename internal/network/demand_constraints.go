package network

import (
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Scope identifiers are exact declared labels. No geography, ontology, source
// verification or delivery commitment is inferred from a similar label.
type CoverageConstraint struct {
	Required []string `json:"required"`
	Excluded []string `json:"excluded"`
}
type FreshnessConstraint struct {
	MaxAgeSeconds int64 `json:"max_age_seconds"`
}
type DeliveryConstraint struct {
	NotBefore *time.Time `json:"not_before,omitempty"`
	DeliverBy *time.Time `json:"deliver_by,omitempty"`
}

func validConstraintLabels(values []string, max int, empty bool) bool {
	if len(values) > max || (!empty && len(values) == 0) {
		return false
	}
	seen := map[string]bool{}
	for _, v := range values {
		if v == "" || len(v) > 128 || !utf8.ValidString(v) || strings.TrimSpace(v) != v || seen[v] {
			return false
		}
		for _, r := range v {
			if unicode.IsControl(r) {
				return false
			}
		}
		seen[v] = true
	}
	return true
}
func (c Constraints) validate(now time.Time) error {
	if len(c.RequiredFields) > 50 || len(c.LicenseIDs) > 20 {
		return fail(422, "constraint_limit")
	}
	if c.Coverage != nil {
		if c.Coverage.Required == nil || c.Coverage.Excluded == nil || !validConstraintLabels(c.Coverage.Required, 50, true) || !validConstraintLabels(c.Coverage.Excluded, 50, true) || setsOverlap(c.Coverage.Required, c.Coverage.Excluded) {
			return fail(422, "invalid_coverage_constraint")
		}
	}
	if c.Freshness != nil && (c.Freshness.MaxAgeSeconds < 1 || c.Freshness.MaxAgeSeconds > 31536000) {
		return fail(422, "invalid_freshness_constraint")
	}
	if c.Delivery != nil {
		d := c.Delivery
		if (d.NotBefore == nil && d.DeliverBy == nil) || (d.NotBefore != nil && d.NotBefore.IsZero()) || (d.DeliverBy != nil && (!d.DeliverBy.After(now) || (d.NotBefore != nil && d.NotBefore.After(*d.DeliverBy)))) {
			return fail(422, "invalid_delivery_constraint")
		}
	}
	if c.InputSchemaFamilies != nil && !validConstraintLabels(*c.InputSchemaFamilies, 20, false) {
		return fail(422, "invalid_input_schema_constraint")
	}
	return nil
}
func setsOverlap(a, b []string) bool {
	for _, v := range a {
		if slices.Contains(b, v) {
			return true
		}
	}
	return false
}
func unionLabels(a, b []string) []string {
	out := append(append([]string{}, a...), b...)
	slices.Sort(out)
	return slices.Compact(out)
}
func intersectLabels(a, b []string) []string {
	out := []string{}
	for _, v := range a {
		if slices.Contains(b, v) {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
func restrictedCoverage(c *CoverageConstraint) bool {
	return c != nil && (len(c.Required) > 0 || len(c.Excluded) > 0)
}
func declaredLicenses(values []string) []string {
	out := []string{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
func typedConstraints(c Constraints) bool {
	return c.Coverage != nil || c.Freshness != nil || c.Delivery != nil || c.InputSchemaFamilies != nil
}
func laterBound(a, b *time.Time) *time.Time {
	if a == nil {
		return cloneBound(b)
	}
	if b == nil || a.After(*b) {
		return cloneBound(a)
	}
	return cloneBound(b)
}
func earlierBound(a, b *time.Time) *time.Time {
	if a == nil {
		return cloneBound(b)
	}
	if b == nil || a.Before(*b) {
		return cloneBound(a)
	}
	return cloneBound(b)
}
func cloneBound(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	copy := *v
	return &copy
}

// Similarity and protocol adapters share this deterministic policy. Missing a
// dimension requested by the other principal is unknown; explicit empty
// coverage lists mean no extra scope restriction. Contradictions dominate unknown.
func compareDemandConstraints(a, b IntentInput) Compatibility {
	e := Compatibility{Compatible: true, Status: "compatible", Reasons: []string{}, Algorithm: "hard-constraints-exact-v1"}
	ac, bc := a.HardConstraints, b.HardConstraints
	ac.LicenseIDs, bc.LicenseIDs = declaredLicenses(ac.LicenseIDs), declaredLicenses(bc.LicenseIDs)
	typed := typedConstraints(ac) || typedConstraints(bc)
	combined := Constraints{RequiredFields: unionLabels(ac.RequiredFields, bc.RequiredFields), LicenseIDs: intersectLabels(ac.LicenseIDs, bc.LicenseIDs)}
	add := func(dimension, status, reason string) {
		if typed {
			e.ConstraintEvidence = append(e.ConstraintEvidence, ConstraintAssessment{dimension, status, reason})
		}
		if status == "incompatible" {
			e.Compatible = false
			e.Status = "incompatible"
			e.Reasons = append(e.Reasons, reason)
		} else if status == "unknown" {
			e.Compatible = false
			if e.Status != "incompatible" {
				e.Status = "needs_clarification"
			}
			e.Reasons = append(e.Reasons, reason)
		}
	}
	if a.Output != b.Output {
		add("output", "incompatible", "output_schema_mismatch")
	} else {
		add("output", "aligned", "same_output_schema")
	}
	if len(ac.LicenseIDs) == 0 || len(bc.LicenseIDs) == 0 {
		add("license_ids", "unknown", "license_unspecified")
	} else if len(combined.LicenseIDs) == 0 {
		add("license_ids", "incompatible", "no_common_license")
	} else {
		add("license_ids", "aligned", "common_permitted_license")
	}
	add("required_fields", "aligned", "required_fields_can_be_combined")
	if ac.Coverage != nil || bc.Coverage != nil {
		if (ac.Coverage == nil && restrictedCoverage(bc.Coverage)) || (bc.Coverage == nil && restrictedCoverage(ac.Coverage)) {
			add("coverage", "unknown", "coverage_unspecified")
		} else {
			if ac.Coverage == nil {
				ac.Coverage = &CoverageConstraint{}
			}
			if bc.Coverage == nil {
				bc.Coverage = &CoverageConstraint{}
			}
			c := &CoverageConstraint{Required: unionLabels(ac.Coverage.Required, bc.Coverage.Required), Excluded: unionLabels(ac.Coverage.Excluded, bc.Coverage.Excluded)}
			if setsOverlap(c.Required, c.Excluded) {
				add("coverage", "incompatible", "required_coverage_excluded")
			} else {
				combined.Coverage = c
				add("coverage", "aligned", "coverage_requirements_can_be_combined")
			}
		}
	}
	if ac.Freshness != nil || bc.Freshness != nil {
		if ac.Freshness == nil || bc.Freshness == nil {
			add("freshness", "unknown", "freshness_unspecified")
		} else {
			combined.Freshness = &FreshnessConstraint{MaxAgeSeconds: min(ac.Freshness.MaxAgeSeconds, bc.Freshness.MaxAgeSeconds)}
			add("freshness", "aligned", "stricter_freshness_can_be_shared")
		}
	}
	if ac.Delivery != nil || bc.Delivery != nil {
		if (ac.Delivery != nil && ac.Delivery.DeliverBy != nil && !ac.Delivery.DeliverBy.After(time.Now())) || (bc.Delivery != nil && bc.Delivery.DeliverBy != nil && !bc.Delivery.DeliverBy.After(time.Now())) {
			add("delivery", "incompatible", "delivery_deadline_elapsed")
		} else if ac.Delivery == nil || bc.Delivery == nil {
			add("delivery", "unknown", "delivery_window_unspecified")
		} else {
			c := &DeliveryConstraint{NotBefore: laterBound(ac.Delivery.NotBefore, bc.Delivery.NotBefore), DeliverBy: earlierBound(ac.Delivery.DeliverBy, bc.Delivery.DeliverBy)}
			if c.NotBefore != nil && c.DeliverBy != nil && c.NotBefore.After(*c.DeliverBy) {
				add("delivery", "incompatible", "delivery_windows_disjoint")
			} else {
				combined.Delivery = c
				add("delivery", "aligned", "delivery_window_intersection")
			}
		}
	}
	if ac.InputSchemaFamilies != nil || bc.InputSchemaFamilies != nil {
		if ac.InputSchemaFamilies == nil || bc.InputSchemaFamilies == nil {
			add("input_schema_families", "unknown", "input_schema_unspecified")
		} else {
			common := intersectLabels(*ac.InputSchemaFamilies, *bc.InputSchemaFamilies)
			if len(common) == 0 {
				add("input_schema_families", "incompatible", "input_schema_options_disjoint")
			} else {
				combined.InputSchemaFamilies = &common
				add("input_schema_families", "aligned", "common_input_schema_options")
			}
		}
	}
	if e.Compatible {
		e.Reasons = []string{"same_output_schema", "common_permitted_license", "required_fields_can_be_combined"}
		if typed {
			e.CombinedConstraints = &combined
		}
	}
	return e
}
