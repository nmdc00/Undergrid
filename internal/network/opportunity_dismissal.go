package network

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

func opportunityDismissalID(path string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "opportunities" || parts[3] != "dismiss" {
		return "", false
	}
	return parts[2], true
}

func (s *Service) dismissOpportunity(r *http.Request, tx pgx.Tx, a actor, id string) (int, any, error) {
	if r.Method != http.MethodPost {
		return 0, nil, fail(405, "method_not_allowed")
	}
	if !slices.Contains(a.Scopes, "intents:write") {
		return 0, nil, insufficientScope(a, "intents:write")
	}
	if !validID(id) {
		return 0, nil, fail(404, "not_found")
	}
	// Eligibility is checked before retained receipt recovery, including when the
	// row is already dismissed. Foreign rows never become a receipt oracle.
	var current bool
	if err := tx.QueryRow(r.Context(), "SELECT opportunity_dismissal_current($1)", id).Scan(&current); err != nil {
		return 0, nil, err
	}
	if !current {
		return 0, nil, fail(404, "not_found")
	}
	var kind, hash string
	var source, candidate, swarm, role *string
	err := tx.QueryRow(r.Context(), `SELECT kind,input_hash,source_intent_id::text,candidate_intent_id::text,source_swarm_id::text,role_key FROM opportunities WHERE id=$1 AND recipient_agent_id=$2 FOR UPDATE`, id, a.AgentID).Scan(&kind, &hash, &source, &candidate, &swarm, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	if kind == "shared_demand" {
		var raw, public []byte
		var sourceHash, candidateHash string
		if err = tx.QueryRow(r.Context(), `SELECT r.document,r.content_hash FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1`, *source).Scan(&raw, &sourceHash); err != nil {
			return 0, nil, err
		}
		if err = tx.QueryRow(r.Context(), "SELECT projection,content_hash FROM public_intents() WHERE id=$1", *candidate).Scan(&public, &candidateHash); err != nil {
			return 0, nil, err
		}
		var left, right IntentInput
		if err = json.Unmarshal(raw, &left); err != nil {
			return 0, nil, err
		}
		if err = json.Unmarshal(public, &right); err != nil {
			return 0, nil, err
		}
		evidence := CompareDemand(left, right)
		if !evidence.Compatible || hash != digest(canonical([]string{*source, sourceHash, *candidate, candidateHash, evidence.Algorithm})) {
			return 0, nil, fail(404, "not_found")
		}
	}
	if kind == "builder_recruitment" {
		var raw, profileRaw []byte
		if err = tx.QueryRow(r.Context(), "SELECT recruitment_roles FROM swarms WHERE id=$1", *swarm).Scan(&raw); err != nil {
			return 0, nil, err
		}
		if err = tx.QueryRow(r.Context(), "SELECT profile FROM agents WHERE id=$1", a.AgentID).Scan(&profileRaw); err != nil {
			return 0, nil, err
		}
		var roles []RecruitmentRole
		var profile AgentProfile
		if err = json.Unmarshal(raw, &roles); err != nil {
			return 0, nil, err
		}
		if err = json.Unmarshal(profileRaw, &profile); err != nil {
			return 0, nil, err
		}
		valid := false
		for _, offered := range roles {
			if offered.Key == *role && profile.Public && profile.Recruitment && slices.Contains(profile.Capabilities, offered.SchemaFamily) && hash == recruitmentInputHash(*swarm, offered, a.AgentID, profile) {
				valid = true
			}
		}
		if !valid {
			return 0, nil, fail(404, "not_found")
		}
	}
	return idempotent(r, tx, a, func() (int, any, error) {
		var input map[string]json.RawMessage
		if err := decode(r, &input); err != nil {
			return 0, nil, err
		}
		if input == nil || len(input) != 0 {
			return 0, nil, fail(400, "invalid_json")
		}
		tag, err := tx.Exec(r.Context(), "UPDATE opportunities SET state='dismissed' WHERE id=$1 AND recipient_agent_id=$2 AND state='suggested'", id, a.AgentID)
		if err != nil {
			return 0, nil, err
		}
		if tag.RowsAffected() == 1 {
			origin := "registered"
			if s.environment != "production" {
				origin = "release_test"
			}
			if _, err = tx.Exec(r.Context(), "INSERT INTO events(id,aggregate_kind,aggregate_id,version,principal_id,agent_id,type,facts,origin) VALUES($1,'opportunity',$2,2,$3,$4,'opportunity.dismissed','{}',$5)", NewID(), id, a.PrincipalID, a.AgentID, origin); err != nil {
				return 0, nil, err
			}
		}
		return 200, map[string]any{"id": id, "state": "dismissed"}, nil
	})
}

func (s *Service) ownerOpportunityDismissal(r *http.Request, tx pgx.Tx, principal string) (int, any, error) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 7 || parts[0] != "v1" || parts[1] != "owner" || parts[2] != "agents" || !validID(parts[3]) || parts[4] != "opportunities" || parts[6] != "dismiss" {
		return 0, nil, fail(404, "not_found")
	}
	a, err := lockOwnerAgent(r.Context(), tx, principal, parts[3])
	if err != nil {
		return 0, nil, err
	}
	a.Scopes = []string{"intents:write"}
	return s.dismissOpportunity(r, tx, a, parts[5])
}
