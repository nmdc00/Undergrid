package network

import (
	"encoding/hex"
	"errors"
	"strings"
	"unicode/utf8"
)

// This is an explicit external-processing grant for one revision/profile. It
// neither publishes the query nor grants access to the private intention.
type PrivateSemanticQuery struct {
	Projection                    string `json:"projection"`
	ProfileHash                   string `json:"profile_hash"`
	AcknowledgeExternalProcessing bool   `json:"acknowledge_external_processing"`
}

func (q PrivateSemanticQuery) validate(visibility string) error {
	hash, err := hex.DecodeString(q.ProfileHash)
	if (visibility != "private" && visibility != "restricted") || !q.AcknowledgeExternalProcessing || utf8.RuneCountInString(strings.TrimSpace(q.Projection)) < 3 || len(q.Projection) > 1000 || err != nil || len(hash) != 32 || strings.ToLower(q.ProfileHash) != q.ProfileHash {
		return fail(422, "invalid_private_semantic_query")
	}
	return nil
}
func validateSemanticConsent(input IntentInput, environment string) error {
	if input.PrivateSemanticQuery == nil {
		return nil
	}
	provider, err := semanticConfiguration(environment)
	if err != nil || provider == nil {
		return fail(503, "semantic_policy_unavailable")
	}
	if !provider.config.AllowPrivateProjections {
		return fail(422, "private_semantic_queries_disabled")
	}
	if input.PrivateSemanticQuery.ProfileHash != provider.hash {
		return fail(409, "semantic_profile_changed")
	}
	return nil
}
func matchingPolicy(environment string) (int, any, error) {
	p, err := semanticConfiguration(environment)
	if err != nil {
		return 0, nil, fail(503, "semantic_policy_unavailable")
	}
	if p == nil {
		return 200, map[string]any{"enabled": false}, nil
	}
	return 200, map[string]any{"enabled": true, "profile_hash": p.hash, "provider_id": p.config.ProviderID, "model": p.config.Model, "model_revision": p.config.ModelRevision, "dimensions": p.config.Dimensions, "permission_evidence": p.config.PermissionEvidence, "privacy_policy_sha256": p.config.PrivacyPolicySHA256, "allow_public_projections": p.config.AllowPublicProjections, "allow_private_projections": p.config.AllowPrivateProjections, "default_minimum_similarity": *p.config.MinimumSimilarity}, nil
}

// The hash binds only the explicitly selected provider input. Public projection
// hashes preserve their existing complete-public-projection identity.
func (p *semanticProvider) selectedProjection(input IntentInput, publicProjection string) (text, hash string, ok bool) {
	if input.Visibility == "redacted_public" && p.config.AllowPublicProjections {
		return input.PublicSummary, digest([]byte(publicProjection)), true
	}
	q := input.PrivateSemanticQuery
	if q == nil || !p.config.AllowPrivateProjections || q.ProfileHash != p.hash || q.validate(input.Visibility) != nil {
		return "", "", false
	}
	return q.Projection, digest([]byte(q.Projection)), true
}

// Audit policy choices without copying the selected private text into events.
func intentMatchingFacts(input IntentInput) map[string]any {
	var profile any
	if input.PrivateSemanticQuery != nil {
		profile = input.PrivateSemanticQuery.ProfileHash
	}
	return map[string]any{"semantic_minimum_similarity": input.SemanticMinimumSimilarity, "private_semantic_profile_hash": profile}
}

// Workers holding an older startup profile cannot outlive the operator's
// current private-processing policy. Temporary configuration failures retry;
// revoked or replaced grants simply decline this observation.
func (s *Service) privateConsentCurrent(input IntentInput) (bool, error) {
	if input.PrivateSemanticQuery == nil {
		return true, nil
	}
	err := validateSemanticConsent(input, s.environment)
	if err == nil {
		return true, nil
	}
	var p problem
	if errors.As(err, &p) && p.Status != 503 {
		return false, nil
	}
	return false, err
}
