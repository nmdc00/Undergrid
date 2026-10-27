package network

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"
)

// Provenance is a command precondition, including for an already retained
// receipt. The authority helper holds current evidence locks until this domain
// transaction commits; adapters neither establish eligibility nor grant rights.
func (s *Service) requireCurrentRoomOrigin(r *http.Request, tx pgx.Tx, a actor, id string) error {
	if id == "" {
		return nil
	}
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
		var raw, public []byte
		var sourceHash, candidateHash string
		if err = tx.QueryRow(r.Context(), `SELECT r.document,r.content_hash FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1`, *source).Scan(&raw, &sourceHash); err != nil {
			return err
		}
		if err = tx.QueryRow(r.Context(), "SELECT projection,content_hash FROM public_intents() WHERE id=$1", *candidate).Scan(&public, &candidateHash); err != nil {
			return err
		}
		var left, right IntentInput
		if err = json.Unmarshal(raw, &left); err != nil {
			return err
		}
		if err = json.Unmarshal(public, &right); err != nil {
			return err
		}
		if provider != nil {
			if stored.SemanticProfileHash != provider.hash || !compareDemandConstraints(left, right).Compatible || stored.MinimumSimilarity == nil || *stored.MinimumSimilarity != provider.minimumSimilarity(left) || hash != demandOpportunityHash(*source, sourceHash, *candidate, candidateHash, stored) {
				return fail(404, "not_found")
			}
		} else {
			evidence := CompareDemand(left, right)
			if !evidence.Compatible || hash != demandOpportunityHash(*source, sourceHash, *candidate, candidateHash, evidence) {
				return fail(404, "not_found")
			}
		}
	}
	if kind == "builder_recruitment" {
		var raw, profileRaw []byte
		if err = tx.QueryRow(r.Context(), "SELECT recruitment_roles FROM swarms WHERE id=$1", *swarm).Scan(&raw); err != nil {
			return err
		}
		if err = tx.QueryRow(r.Context(), "SELECT profile FROM agents WHERE id=$1", a.AgentID).Scan(&profileRaw); err != nil {
			return err
		}
		var roles []RecruitmentRole
		var profile AgentProfile
		if err = json.Unmarshal(raw, &roles); err != nil {
			return err
		}
		if err = json.Unmarshal(profileRaw, &profile); err != nil {
			return err
		}
		for _, offered := range roles {
			if offered.Key == *role && profile.Public && profile.Recruitment && slices.Contains(profile.Capabilities, offered.SchemaFamily) && hash == recruitmentInputHash(*swarm, offered, a.AgentID, profile) {
				return nil
			}
		}
		return fail(404, "not_found")
	}
	return nil
}
