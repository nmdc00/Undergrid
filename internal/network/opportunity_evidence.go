package network

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"
)

// Callers own their different authority/state locks: dismissal requires a watch
// and accepts its dismissed receipt; room creation requires suggested provenance
// but does not require a shared-demand watch. Current evidence knowledge is shared.
func requireDemandOpportunityEvidence(r *http.Request, tx pgx.Tx, source, candidate, hash string, stored Compatibility, provider *semanticProvider) error {
	var raw, public []byte
	var sourceHash, candidateHash string
	if err := tx.QueryRow(r.Context(), `SELECT r.document,r.content_hash FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1`, source).Scan(&raw, &sourceHash); err != nil {
		return err
	}
	if err := tx.QueryRow(r.Context(), "SELECT projection,content_hash FROM public_intents() WHERE id=$1", candidate).Scan(&public, &candidateHash); err != nil {
		return err
	}
	var left, right IntentInput
	if err := json.Unmarshal(raw, &left); err != nil {
		return err
	}
	if err := json.Unmarshal(public, &right); err != nil {
		return err
	}
	if stored.Algorithm == "hard-constraints-hybrid-cosine-v1" {
		if provider == nil || stored.SemanticProfileHash != provider.hash || !compareDemandConstraints(left, right).Compatible || stored.MinimumSimilarity == nil || *stored.MinimumSimilarity != provider.minimumSimilarity(left) || hash != demandOpportunityHash(source, sourceHash, candidate, candidateHash, stored) {
			return fail(404, "not_found")
		}
		return nil
	}
	evidence := CompareDemand(left, right)
	if !evidence.Compatible || hash != demandOpportunityHash(source, sourceHash, candidate, candidateHash, evidence) {
		return fail(404, "not_found")
	}
	return nil
}

func requireRecruitmentOpportunityEvidence(r *http.Request, tx pgx.Tx, swarm, role, agent, hash string) error {
	var raw, profileRaw []byte
	if err := tx.QueryRow(r.Context(), "SELECT recruitment_roles FROM swarms WHERE id=$1", swarm).Scan(&raw); err != nil {
		return err
	}
	if err := tx.QueryRow(r.Context(), "SELECT profile FROM agents WHERE id=$1", agent).Scan(&profileRaw); err != nil {
		return err
	}
	var roles []RecruitmentRole
	var profile AgentProfile
	if err := json.Unmarshal(raw, &roles); err != nil {
		return err
	}
	if err := json.Unmarshal(profileRaw, &profile); err != nil {
		return err
	}
	for _, offered := range roles {
		if offered.Key == role && profile.Public && profile.Recruitment && slices.Contains(profile.Capabilities, offered.SchemaFamily) && hash == recruitmentInputHash(swarm, offered, agent, profile) {
			return nil
		}
	}
	return fail(404, "not_found")
}

// Both recruitment read channels and room provenance use the same locked
// current role/profile evidence. Callers select only owned recruitment records.
func requireCurrentRecruitmentOpportunityEvidence(r *http.Request, tx pgx.Tx, a actor, id, swarm, role, hash string) error {
	var current bool
	if err := tx.QueryRow(r.Context(), "SELECT room_origin_authority($1,$2)", id, "").Scan(&current); err != nil {
		return err
	}
	if !current {
		return fail(404, "not_found")
	}
	return requireRecruitmentOpportunityEvidence(r, tx, swarm, role, a.AgentID, hash)
}
