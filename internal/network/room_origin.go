package network

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// Provenance is a command precondition, including for an already retained
// receipt. The authority helper holds current evidence locks until this domain
// transaction commits; adapters neither establish eligibility nor grant rights.
func (s *Service) requireCurrentRoomOrigin(r *http.Request, tx pgx.Tx, a actor, id string) error {
	if id == "" {
		return nil
	}
	return s.requireCurrentOpportunityEvidence(r, tx, a, id)
}

// Shared current provenance; callers separately own read/action permission.
func (s *Service) requireCurrentOpportunityEvidence(r *http.Request, tx pgx.Tx, a actor, id string) error {
	if !validID(id) {
		return fail(422, "invalid_opportunity")
	}
	var kind, hash string
	var evidenceRaw []byte
	var source, candidate, swarm, role *string
	err := tx.QueryRow(r.Context(), `SELECT kind,input_hash,evidence,source_intent_id::text,candidate_intent_id::text,source_swarm_id::text,role_key FROM opportunities WHERE id=$1 AND recipient_agent_id=$2`, id, a.AgentID).Scan(&kind, &hash, &evidenceRaw, &source, &candidate, &swarm, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(404, "not_found")
	}
	if err != nil {
		return err
	}
	var stored Compatibility
	if err = json.Unmarshal(evidenceRaw, &stored); err != nil {
		return err
	}
	var provider *semanticProvider
	profile := ""
	if kind == "shared_demand" && stored.Algorithm == "hard-constraints-hybrid-cosine-v1" {
		provider, err = semanticConfiguration(s.environment)
		if err != nil {
			return fail(503, "semantic_policy_unavailable")
		}
		if provider == nil {
			return fail(404, "not_found")
		}
		profile = provider.hash
	}
	var current bool
	if err = tx.QueryRow(r.Context(), "SELECT room_origin_authority($1,$2)", id, profile).Scan(&current); err != nil {
		return err
	}
	if !current {
		return fail(404, "not_found")
	}
	if kind == "shared_demand" {
		return requireDemandOpportunityEvidence(r, tx, *source, *candidate, hash, stored, provider)
	}
	if kind == "builder_recruitment" {
		return requireRecruitmentOpportunityEvidence(r, tx, *swarm, *role, a.AgentID, hash)
	}
	return nil
}
