package network

import (
	"encoding/hex"
	"encoding/json"
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
	if !input.processingGrant.matches(input, input.storedRevision) || q == nil || !p.config.AllowPrivateProjections || q.ProfileHash != p.hash || q.validate(input.Visibility) != nil {
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

// A startup client is not continuing operator authority. Both public and
// private observations recheck the current trusted file before disclosure and
// before persistence. Missing/replaced grants decline old work; configuration
// read failures stay retryable without sending text or publishing observations.
func (s *Service) semanticProjectionCurrent(input IntentInput) (bool, error) {
	if s.semantic == nil {
		return false, nil
	}
	current, err := semanticConfiguration(s.environment)
	if err != nil {
		return false, fail(503, "semantic_policy_unavailable")
	}
	if current == nil || current.hash != s.semantic.hash {
		return false, nil
	}
	_, _, allowed := current.selectedProjection(input, "")
	return allowed, nil
}

// This authority is minted only by explicit validated create/full-revision
// commands. Retaining query data or a historical grant does not renew it.
type semanticProcessingGrant struct {
	Revision       int    `json:"revision"`
	ProfileHash    string `json:"profile_hash"`
	ProjectionHash string `json:"projection_hash"`
}

func newSemanticProcessingGrant(input IntentInput, revision int) *semanticProcessingGrant {
	if input.PrivateSemanticQuery == nil {
		return nil
	}
	return &semanticProcessingGrant{revision, input.PrivateSemanticQuery.ProfileHash, digest([]byte(input.PrivateSemanticQuery.Projection))}
}
func (g *semanticProcessingGrant) matches(input IntentInput, revision int) bool {
	q := input.PrivateSemanticQuery
	return g != nil && q != nil && revision > 0 && g.Revision == revision && g.ProfileHash == q.ProfileHash && g.ProjectionHash == digest([]byte(q.Projection))
}
func decodeStoredIntent(raw []byte, revision int) (IntentInput, error) {
	var document intentRevisionDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return IntentInput{}, err
	}
	input := document.IntentInput
	input.processingGrant = document.SemanticProcessingGrant
	input.storedRevision = revision
	return input, nil
}
