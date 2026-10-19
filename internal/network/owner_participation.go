package network

import (
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
	collection := len(parts) == 5 && parts[4] == "participation"
	invitation := len(parts) == 6 && parts[4] == "invitations" && validID(parts[5])
	context := len(parts) == 7 && parts[4] == "swarms" && validID(parts[5]) && parts[6] == "context"
	response := len(parts) == 7 && parts[4] == "swarms" && validID(parts[5]) && parts[6] == "membership-response"
	if !collection && !invitation && !context && !response {
		return 0, nil, fail(404, "not_found")
	}
	if (response && r.Method != http.MethodPost) || (!response && r.Method != http.MethodGet) {
		return 0, nil, fail(405, "method_not_allowed")
	}
	a, err := lockOwnerAgent(r.Context(), tx, principal, parts[3])
	if err != nil {
		return 0, nil, err
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
