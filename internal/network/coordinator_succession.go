package network

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"slices"
	"strings"
	"time"
)

type coordinatorSnapshot struct {
	CoordinatorAgentID string   `json:"coordinator_agent_id"`
	RequiredPrincipals []string `json:"required_principals"`
}
type coordinatorProposal struct {
	ID, SwarmID, Proposer, Successor, Previous, Hash, State string
	Version                                                 int64
	Snapshot                                                json.RawMessage
	RespondBy                                               time.Time
	AcceptedAt                                              *time.Time
}

func coordinatorRequest(path string) (room, proposal, action string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 4 && parts[0] == "v1" && parts[1] == "swarms" && validID(parts[2]) && parts[3] == "coordinator-proposals" {
		return parts[2], "", "propose", true
	}
	if (len(parts) == 3 || len(parts) == 4) && parts[0] == "v1" && parts[1] == "coordinator-proposals" && validID(parts[2]) {
		action = "read"
		if len(parts) == 4 {
			action = parts[3]
			if action != "approvals" && action != "accept" {
				return "", "", "", false
			}
		}
		return "", parts[2], action, true
	}
	return "", "", "", false
}

// The same finite scope contract serves the denial-only prelock read and the
// full authorized command. Preliminary checks never grant authority.
func coordinatorRequiredScope(method, action string) (string, error) {
	if method == "GET" && action == "read" {
		return "swarms:read", nil
	}
	if method == "POST" {
		switch action {
		case "propose":
			return "coordinator:propose", nil
		case "approvals":
			return "coordinator:approve", nil
		case "accept":
			return "coordinator:accept", nil
		}
	}
	return "", fail(405, "method_not_allowed")
}
func prelockCoordinatorRequest(ctx context.Context, tx pgx.Tx, a actor, method, path string) error {
	if method != "POST" {
		return nil
	}
	room, proposal, action, ok := coordinatorRequest(path)
	if !ok {
		return nil
	}
	scope, err := coordinatorRequiredScope(method, action)
	if err != nil {
		return err
	}
	preliminary := a
	if err = tx.QueryRow(ctx, "SELECT scopes FROM credentials WHERE id=$1 AND revoked_at IS NULL AND expires_at>clock_timestamp()", a.CredentialID).Scan(&preliminary.Scopes); errors.Is(err, pgx.ErrNoRows) {
		return fail(401, "invalid_credential")
	} else if err != nil {
		return err
	}
	if claims, found := ctx.Value(oauthResourceContextKey).(verifiedOAuth); found {
		preliminary.grantedScopes = append([]string(nil), preliminary.Scopes...)
		preliminary.Scopes = scopeIntersection(preliminary.Scopes, claims.Scopes)
	}
	if !slices.Contains(preliminary.Scopes, scope) {
		return insufficientScope(preliminary, scope)
	}
	if proposal != "" {
		if err := tx.QueryRow(ctx, "SELECT swarm_id::text FROM coordinator_proposals WHERE id=$1", proposal).Scan(&room); errors.Is(err, pgx.ErrNoRows) {
			return fail(404, "not_found")
		} else if err != nil {
			return err
		}
	}
	var result int
	if err := tx.QueryRow(ctx, "SELECT lock_coordinator_authorities($1,$2,$3)", room, a.CredentialID, scope).Scan(&result); err != nil {
		return err
	}
	switch result {
	case 1:
		return nil
	case 0:
		return fail(404, "not_found")
	case 2:
		return fail(409, "succession_roster_limit")
	case 3:
		return fail(401, "invalid_credential")
	case 5:
		return insufficientScope(preliminary, scope)
	default:
		return fail(409, "succession_authority_changed")
	}
}
func requireCoordinatorPolicy(ctx context.Context, tx pgx.Tx, principal string) (int64, string, error) {
	var raw []byte
	var version int64
	var hash string
	if err := tx.QueryRow(ctx, "SELECT policy,version,encode(sha256(convert_to(policy::text,'UTF8')),'hex') FROM principals WHERE id=$1", principal).Scan(&raw, &version, &hash); err != nil {
		return 0, "", err
	}
	var policy CoordinationPolicy
	if json.Unmarshal(raw, &policy) != nil || !policy.AllowCoordinatorSuccession || !policy.ExpiresAt.After(time.Now()) {
		return 0, "", fail(403, "coordinator_policy_required")
	}
	return version, hash, nil
}
func loadCoordinatorProposal(ctx context.Context, tx pgx.Tx, id string, lock bool) (coordinatorProposal, error) {
	var p coordinatorProposal
	q := "SELECT id,swarm_id,proposer_agent_id,successor_agent_id,previous_coordinator_agent_id,content_hash,state,version,snapshot,respond_by,accepted_at FROM coordinator_proposals WHERE id=$1"
	if lock {
		q += " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, q, id).Scan(&p.ID, &p.SwarmID, &p.Proposer, &p.Successor, &p.Previous, &p.Hash, &p.State, &p.Version, &p.Snapshot, &p.RespondBy, &p.AcceptedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = fail(404, "not_found")
	}
	return p, err
}
func coordinatorProposalResult(ctx context.Context, tx pgx.Tx, p coordinatorProposal) (any, error) {
	var snapshot coordinatorSnapshot
	if err := json.Unmarshal(p.Snapshot, &snapshot); err != nil {
		return nil, err
	}
	var current bool
	if err := tx.QueryRow(ctx, "SELECT snapshot=coordinator_snapshot(swarm_id,successor_agent_id) AND coordinator_consents_current(id) FROM coordinator_proposals WHERE id=$1", p.ID).Scan(&current); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, "SELECT principal_id,decision FROM coordinator_votes WHERE proposal_id=$1 ORDER BY principal_id", p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	votes := []any{}
	for rows.Next() {
		var principal, decision string
		if err = rows.Scan(&principal, &decision); err != nil {
			return nil, err
		}
		votes = append(votes, map[string]any{"principal_id": principal, "decision": decision})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	state := p.State
	if state == "proposed" && !p.RespondBy.After(time.Now()) {
		state = "expired"
	}
	return map[string]any{"id": p.ID, "swarm_id": p.SwarmID, "successor_agent_id": p.Successor, "previous_coordinator_agent_id": p.Previous, "state": state, "version": p.Version, "etag": etag(p.Version), "content_hash": p.Hash, "respond_by": p.RespondBy, "required_principals": snapshot.RequiredPrincipals, "successor_accepted": p.AcceptedAt != nil, "current": current, "votes": votes}, nil
}
func (s *Service) coordinatorCommand(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	room, proposal, action, ok := coordinatorRequest(r.URL.Path)
	if !ok {
		return 0, nil, fail(404, "not_found")
	}
	scope, scopeErr := coordinatorRequiredScope(r.Method, action)
	if scopeErr != nil {
		return 0, nil, scopeErr
	}
	if !slices.Contains(a.Scopes, scope) {
		return 0, nil, insufficientScope(a, scope)
	}
	var p coordinatorProposal
	var err error
	if proposal != "" {
		p, err = loadCoordinatorProposal(r.Context(), tx, proposal, false)
		if err != nil {
			return 0, nil, err
		}
		room = p.SwarmID
	}
	var member bool
	if err = tx.QueryRow(r.Context(), "SELECT swarm_member($1)", room).Scan(&member); err != nil {
		return 0, nil, err
	}
	if !member {
		return 0, nil, fail(404, "not_found")
	}
	if r.Method == "GET" {
		result, err := coordinatorProposalResult(r.Context(), tx, p)
		return 200, result, err
	}
	if err = roomAuthority(r.Context(), tx, a, room, false); err != nil {
		return 0, nil, err
	}
	var guarded bool
	if err = tx.QueryRow(r.Context(), "SELECT coordinator_authorities_current($1)", room).Scan(&guarded); err != nil {
		return 0, nil, err
	}
	if !guarded {
		return 0, nil, fail(409, "succession_authority_changed")
	}
	if action != "propose" {
		if _, _, err = requireCoordinatorPolicy(r.Context(), tx, a.PrincipalID); err != nil {
			return 0, nil, err
		}
		if action == "accept" && p.Successor != a.AgentID {
			return 0, nil, fail(403, "successor_agent_required")
		}
	}
	return idempotent(r, tx, a, func() (int, any, error) {
		if action == "propose" {
			return s.proposeCoordinator(r, tx, a, room)
		}
		p, err = loadCoordinatorProposal(r.Context(), tx, proposal, true)
		if err != nil {
			return 0, nil, err
		}
		if err = checkCoordinatorProposal(r, tx, p); err != nil {
			return 0, nil, err
		}
		var in struct {
			Hash     string
			Decision string
		}
		if action == "accept" {
			var body struct {
				Hash string `json:"content_hash"`
			}
			if err = decode(r, &body); err != nil {
				return 0, nil, err
			}
			in.Hash = body.Hash
		} else {
			var body struct {
				Hash     string `json:"content_hash"`
				Decision string `json:"decision"`
			}
			if err = decode(r, &body); err != nil {
				return 0, nil, err
			}
			in.Hash, in.Decision = body.Hash, body.Decision
		}
		if in.Hash != p.Hash {
			return 0, nil, fail(409, "proposal_hash_conflict")
		}
		switch action {
		case "accept":
			if p.AcceptedAt == nil {
				if _, err = tx.Exec(r.Context(), "UPDATE coordinator_proposals SET accepted_credential_id=$2,accepted_at=clock_timestamp() WHERE id=$1", p.ID, a.CredentialID); err != nil {
					return 0, nil, err
				}
				if err = s.roomEvent(r.Context(), tx, a, NewID(), 1, "coordinator.successor_accepted", room, "/v1/coordinator-proposals/"+p.ID); err != nil {
					return 0, nil, err
				}
			}
		case "approvals":
			if !slices.Contains([]string{"approve", "reject"}, in.Decision) {
				return 0, nil, fail(422, "invalid_decision")
			}
			var snap coordinatorSnapshot
			if err = json.Unmarshal(p.Snapshot, &snap); err != nil {
				return 0, nil, err
			}
			if !slices.Contains(snap.RequiredPrincipals, a.PrincipalID) {
				return 0, nil, fail(403, "not_required_approver")
			}
			version, hash, err := requireCoordinatorPolicy(r.Context(), tx, a.PrincipalID)
			if err != nil {
				return 0, nil, err
			}
			voteID := NewID()
			tag, err := tx.Exec(r.Context(), "INSERT INTO coordinator_votes(id,proposal_id,principal_id,agent_id,credential_id,policy_version,policy_hash,decision) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING", voteID, p.ID, a.PrincipalID, a.AgentID, a.CredentialID, version, hash, in.Decision)
			if err != nil {
				return 0, nil, err
			}
			if tag.RowsAffected() == 0 {
				var decision string
				if err = tx.QueryRow(r.Context(), "SELECT decision FROM coordinator_votes WHERE proposal_id=$1 AND principal_id=$2", p.ID, a.PrincipalID).Scan(&decision); err != nil {
					return 0, nil, err
				}
				if decision != in.Decision {
					return 0, nil, fail(409, "approval_requires_new_proposal")
				}
			} else {
				if err = s.roomEvent(r.Context(), tx, a, voteID, 1, "coordinator."+in.Decision, room, "/v1/coordinator-proposals/"+p.ID); err != nil {
					return 0, nil, err
				}
			}
			if in.Decision == "reject" {
				if _, err = tx.Exec(r.Context(), "UPDATE coordinator_proposals SET state='rejected',version=version+1 WHERE id=$1", p.ID); err != nil {
					return 0, nil, err
				}
			}
		}
		var applied bool
		if err = tx.QueryRow(r.Context(), "SELECT apply_coordinator_succession($1)", p.ID).Scan(&applied); err != nil {
			return 0, nil, err
		}
		if applied {
			if err = s.roomEvent(r.Context(), tx, a, p.ID, p.Version+1, "coordinator.transferred", room, "/v1/coordinator-proposals/"+p.ID); err != nil {
				return 0, nil, err
			}
			if err = enqueueRecruitment(r.Context(), tx, room); err != nil {
				return 0, nil, err
			}
		}
		p, err = loadCoordinatorProposal(r.Context(), tx, p.ID, false)
		if err != nil {
			return 0, nil, err
		}
		result, err := coordinatorProposalResult(r.Context(), tx, p)
		return 200, result, err
	})
}
func checkCoordinatorProposal(r *http.Request, tx pgx.Tx, p coordinatorProposal) error {
	if r.Header.Get("If-Match") == "" {
		return fail(428, "if_match_required")
	}
	if r.Header.Get("If-Match") != etag(p.Version) {
		return fail(409, "version_conflict")
	}
	if p.State != "proposed" || !p.RespondBy.After(time.Now()) {
		return fail(409, "proposal_not_current")
	}
	var current bool
	if err := tx.QueryRow(r.Context(), "SELECT snapshot=coordinator_snapshot(swarm_id,successor_agent_id) AND coordinator_consents_current(id) FROM coordinator_proposals WHERE id=$1", p.ID).Scan(&current); err != nil {
		return err
	}
	if !current {
		return fail(409, "proposal_authority_changed")
	}
	return nil
}
func (s *Service) proposeCoordinator(r *http.Request, tx pgx.Tx, a actor, room string) (int, any, error) {
	var version int64
	var previous string
	if err := tx.QueryRow(r.Context(), "SELECT version,coordinator_agent_id FROM swarms WHERE id=$1", room).Scan(&version, &previous); err != nil {
		return 0, nil, err
	}
	if r.Header.Get("If-Match") == "" {
		return 0, nil, fail(428, "if_match_required")
	}
	if r.Header.Get("If-Match") != etag(version) {
		return 0, nil, fail(409, "version_conflict")
	}
	var in struct {
		Successor string    `json:"successor_agent_id"`
		RespondBy time.Time `json:"respond_by"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if !validID(in.Successor) || in.Successor == previous || !in.RespondBy.After(time.Now()) || in.RespondBy.After(time.Now().Add(7*24*time.Hour)) {
		return 0, nil, fail(422, "invalid_successor_proposal")
	}
	var eligible bool
	if err := tx.QueryRow(r.Context(), "SELECT coordinator_successor_eligible($1,$2)", room, in.Successor).Scan(&eligible); err != nil {
		return 0, nil, err
	}
	if !eligible {
		return 0, nil, fail(422, "successor_not_eligible")
	}
	var planCurrent, partiesCurrent bool
	if err := tx.QueryRow(r.Context(), "SELECT active_plan_id IS NULL OR EXISTS(SELECT 1 FROM plan_versions p WHERE p.id=s.active_plan_id AND p.state='accepted') FROM swarms s WHERE s.id=$1", room).Scan(&planCurrent); err != nil {
		return 0, nil, err
	}
	if !planCurrent {
		return 0, nil, fail(409, "accepted_plan_not_current")
	}
	if err := tx.QueryRow(r.Context(), "SELECT NOT EXISTS(SELECT 1 FROM unnest(succession_required_principals($1)) required(id) WHERE NOT EXISTS(SELECT 1 FROM memberships m WHERE m.swarm_id=$1 AND m.principal_id=required.id AND m.state='accepted' AND m.role<>'observer'))", room).Scan(&partiesCurrent); err != nil {
		return 0, nil, err
	}
	if !partiesCurrent {
		return 0, nil, fail(409, "required_party_not_current")
	}
	var raw []byte
	if err := tx.QueryRow(r.Context(), "SELECT coordinator_snapshot($1,$2)", room, in.Successor).Scan(&raw); err != nil {
		return 0, nil, err
	}
	var snapshot any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return 0, nil, err
	}
	id := NewID()
	respondBy := in.RespondBy.UTC()
	hash := digest(canonical(map[string]any{"proposal_id": id, "swarm_id": room, "proposer_agent_id": a.AgentID, "respond_by": respondBy, "snapshot": snapshot}))
	if _, err := tx.Exec(r.Context(), "INSERT INTO coordinator_proposals(id,swarm_id,proposer_agent_id,successor_agent_id,previous_coordinator_agent_id,snapshot,content_hash,respond_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", id, room, a.AgentID, in.Successor, previous, raw, hash, respondBy); err != nil {
		return 0, nil, err
	}
	if err := s.roomEvent(r.Context(), tx, a, id, 1, "coordinator.proposed", room, "/v1/coordinator-proposals/"+id); err != nil {
		return 0, nil, err
	}
	p, err := loadCoordinatorProposal(r.Context(), tx, id, false)
	if err != nil {
		return 0, nil, err
	}
	result, err := coordinatorProposalResult(r.Context(), tx, p)
	return 201, result, err
}
