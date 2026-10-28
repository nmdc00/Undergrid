package network

import (
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

// Applicant identity and coordinator authority are resolved before this query.
// Membership claims are explicit submissions, never foreign profiles or history.
func ownerApplicationPage(r *http.Request, tx pgx.Tx, a actor, swarm string) (int, any, error) {
	after := r.URL.Query().Get("after")
	if after != "" && !validID(after) {
		return 0, nil, fail(400, "invalid_cursor")
	}
	if swarm != "" {
		if err := roomAuthority(r.Context(), tx, a, swarm, true); err != nil {
			return 0, nil, err
		}
	}
	rows, err := tx.Query(r.Context(), `SELECT m.id,m.swarm_id,m.agent_id,m.principal_id,m.state,m.version,m.respond_by,m.role_key,m.application_claims,s.objective,s.visibility,s.version
 FROM memberships m JOIN swarms s ON s.id=m.swarm_id
 WHERE m.origin='application' AND m.state IN ('applied','offered') AND m.respond_by>clock_timestamp() AND s.state='active'
 AND (($2='' AND m.agent_id=$1) OR ($2<>'' AND m.swarm_id=NULLIF($2,'')::uuid))
 AND ($3='' OR m.id>NULLIF($3,'')::uuid) ORDER BY m.id LIMIT 101`, a.AgentID, swarm, after)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	items := []any{}
	last, next := "", ""
	for rows.Next() {
		var id, room, agent, principal, state, objective, visibility string
		var version, roomVersion int64
		var deadline time.Time
		var role *string
		var claims []byte
		if err = rows.Scan(&id, &room, &agent, &principal, &state, &version, &deadline, &role, &claims, &objective, &visibility, &roomVersion); err != nil {
			return 0, nil, err
		}
		if len(items) == 100 {
			next = last
			break
		}
		last = id
		items = append(items, map[string]any{"id": id, "swarm_id": room, "agent_id": agent, "principal_id": principal, "state": state, "version": version, "etag": etag(version), "respond_by": deadline, "role_key": role, "application_claims": json.RawMessage(claims), "objective": objective, "visibility": visibility, "swarm_version": roomVersion, "history_policy": "full", "content_trust": "untrusted"})
	}
	if err = rows.Err(); err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"items": items, "next_after": next}, nil
}
