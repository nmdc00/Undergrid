package network

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The browser selects an owned agent for attribution, never a credential or a
// principal. Existing domain commands retain invitation and room authority.
func (s *Service) ownerParticipation(r *http.Request, tx pgx.Tx, principal string) (int, any, error) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 5 || parts[0] != "v1" || parts[1] != "owner" || parts[2] != "agents" || !validID(parts[3]) {
		return 0, nil, fail(404, "not_found")
	}
	recruitment := len(parts) == 5 && parts[4] == "recruitment"
	recruitmentItem := len(parts) == 6 && parts[4] == "recruitment" && validID(parts[5])
	creation := len(parts) == 5 && parts[4] == "swarms"
	collection := len(parts) == 5 && parts[4] == "participation"
	invitation := len(parts) == 6 && parts[4] == "invitations" && validID(parts[5])
	context := len(parts) == 7 && parts[4] == "swarms" && validID(parts[5]) && parts[6] == "context"
	inviteMember := len(parts) == 7 && parts[4] == "swarms" && validID(parts[5]) && parts[6] == "invitations"
	offer := len(parts) == 9 && parts[4] == "swarms" && validID(parts[5]) && parts[6] == "applications" && validID(parts[7]) && parts[8] == "offer"
	applicationCollection := len(parts) == 5 && parts[4] == "applications"
	application := len(parts) == 7 && parts[4] == "swarms" && validID(parts[5]) && parts[6] == "applications"
	message := len(parts) == 7 && parts[4] == "swarms" && validID(parts[5]) && parts[6] == "messages"
	response := len(parts) == 7 && parts[4] == "swarms" && validID(parts[5]) && parts[6] == "membership-response"
	if !creation && !collection && !invitation && !context && !response && !inviteMember && !message && !application && !applicationCollection && !offer && !recruitment && !recruitmentItem {
		return 0, nil, fail(404, "not_found")
	}
	if application {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			return 0, nil, fail(405, "method_not_allowed")
		}
	} else {
		mutation := creation || response || inviteMember || message || offer
		if (mutation && r.Method != http.MethodPost) || (!mutation && r.Method != http.MethodGet) {
			return 0, nil, fail(405, "method_not_allowed")
		}
	}

	a, err := lockOwnerAgent(r.Context(), tx, principal, parts[3])
	if err != nil {
		return 0, nil, err
	}
	if recruitment || recruitmentItem {
		opportunity := ""
		if recruitmentItem {
			opportunity = parts[5]
		}
		return s.ownerRecruitmentReview(r, tx, a, opportunity)
	}
	if creation {
		return s.ownerCreateSwarm(r, tx, a)
	}
	if offer {
		return s.ownerApplicationOffer(r, tx, a, parts[5], parts[7])
	}
	if inviteMember {
		return s.ownerInviteMember(r, tx, a, parts[5])
	}
	if applicationCollection {
		return ownerApplicationPage(r, tx, a, "")
	}
	if application && r.Method == http.MethodGet {
		return ownerApplicationPage(r, tx, a, parts[5])
	}
	if application {
		if r.Header.Get("If-Match") == "" {
			return 0, nil, fail(428, "if_match_required")
		}
		command := r.Clone(r.Context())
		command.URL.Path = "/v1/swarms/" + parts[5] + "/applications"
		a.Scopes = []string{"swarms:apply"}
		return s.swarmCommand(command, tx, a)
	}
	if message {
		return s.ownerRoomMessage(r, tx, a, parts[5])
	}
	if collection {
		return ownerParticipationPage(r, tx, a)
	}
	command := r.Clone(r.Context())
	if invitation {
		command.URL.Path = "/v1/invitations/" + parts[5]
		return invitationPacket(command, tx, a)
	}
	command.URL.Path = "/v1/swarms/" + parts[5] + "/" + parts[6]
	a.Scopes = []string{"swarms:read", "members:respond"}
	return s.swarmCommand(command, tx, a)
}

func ownerParticipationPage(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	after := r.URL.Query().Get("after")
	if after != "" && !validID(after) {
		return 0, nil, fail(400, "invalid_cursor")
	}
	rows, err := tx.Query(r.Context(), `SELECT id,swarm_id,state,version,respond_by FROM memberships
 WHERE agent_id=$1 AND (state='accepted' OR (state='offered' AND respond_by>clock_timestamp()))
 AND ($2='' OR id>NULLIF($2,'')::uuid) ORDER BY id LIMIT 101`, a.AgentID, after)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	pending, rooms := []any{}, []any{}
	count, last, next := 0, "", ""
	for rows.Next() {
		var id, swarm, state string
		var version int64
		var deadline *time.Time
		if err = rows.Scan(&id, &swarm, &state, &version, &deadline); err != nil {
			return 0, nil, err
		}
		if count == 100 {
			next = last
			break
		}
		count++
		last = id
		if state == "accepted" {
			rooms = append(rooms, map[string]any{"id": swarm, "resource_path": "/v1/owner/agents/" + a.AgentID + "/swarms/" + swarm + "/context"})
		} else {
			pending = append(pending, map[string]any{"kind": "invitation", "id": id, "swarm_id": swarm, "etag": etag(version), "respond_by": deadline, "content_trust": "untrusted", "resource_path": "/v1/owner/agents/" + a.AgentID + "/invitations/" + id})
		}
	}
	return 200, map[string]any{"pending": pending, "swarms": rooms, "next_after": next, "limit": 100}, rows.Err()
}

// The owner session is the authority for this explicit decision; it does not
// expand any credential's scopes. Full-history disclosure is acknowledged here,
// while shared swarm commands own creation validation, quota and durable effects.
func (s *Service) ownerCreateSwarm(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	var input struct {
		Objective           string            `json:"objective"`
		Visibility          string            `json:"visibility"`
		HistoryPolicy       string            `json:"history_policy"`
		RecruitmentRoles    []RecruitmentRole `json:"recruitment_roles"`
		OriginOpportunityID string            `json:"origin_opportunity_id,omitempty"`
	}
	if err := decode(r, &input); err != nil {
		return 0, nil, err
	}
	if input.HistoryPolicy != "full" || (input.Visibility != "private" && input.Visibility != "restricted") {
		return 0, nil, fail(422, "nonpublic_swarm_and_full_history_required")
	}
	if input.RecruitmentRoles == nil {
		input.RecruitmentRoles = []RecruitmentRole{}
	}
	command := r.Clone(r.Context())
	command.URL.Path = "/v1/swarms"
	command.Body = io.NopCloser(bytes.NewReader(canonical(SwarmInput{Objective: input.Objective, Visibility: input.Visibility, RecruitmentRoles: input.RecruitmentRoles, OriginOpportunityID: input.OriginOpportunityID})))
	a.Scopes = []string{"swarms:create"}
	return s.swarmCommand(command, tx, a)
}

// Reviewed room version and coordinator membership are checked under the same
// room lock as the shared invitation mutation. Owner attribution grants neither
// admission nor any assignment, rights or payment authority.
func (s *Service) ownerInviteMember(r *http.Request, tx pgx.Tx, a actor, swarm string) (int, any, error) {
	if err := roomAuthority(r.Context(), tx, a, swarm, true); err != nil {
		return 0, nil, err
	}
	a.Scopes = []string{"members:invite"}
	return idempotent(r, tx, a, func() (int, any, error) {
		var version int64
		if err := tx.QueryRow(r.Context(), "SELECT version FROM swarms WHERE id=$1 FOR UPDATE", swarm).Scan(&version); err != nil {
			return 0, nil, err
		}
		if r.Header.Get("If-Match") == "" {
			return 0, nil, fail(428, "if_match_required")
		}
		if r.Header.Get("If-Match") != etag(version) {
			return 0, nil, fail(409, "version_conflict")
		}
		var input struct {
			AgentID       string    `json:"agent_id"`
			Role          string    `json:"role"`
			RespondBy     time.Time `json:"respond_by"`
			HistoryPolicy string    `json:"history_policy"`
		}
		if err := decode(r, &input); err != nil {
			return 0, nil, err
		}
		if input.HistoryPolicy != "full" {
			return 0, nil, fail(422, "full_history_required")
		}
		command := r.Clone(r.Context())
		command.Body = io.NopCloser(bytes.NewReader(canonical(map[string]any{"agent_id": input.AgentID, "role": input.Role, "respond_by": input.RespondBy})))
		return s.invite(command, tx, a, swarm)
	})
}
