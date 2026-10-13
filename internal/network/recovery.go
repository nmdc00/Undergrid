package network

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func membershipSnapshot(ctx context.Context, tx pgx.Tx, a actor) ([]any, []any, error) {
	pending, refs := []any{}, []any{}
	rows, err := tx.Query(ctx, "SELECT id,swarm_id,state,version,respond_by FROM memberships WHERE agent_id=$1 AND (state='accepted' OR (state='offered' AND respond_by>now())) ORDER BY id", a.AgentID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, swarm, state string
		var version int64
		var deadline *time.Time
		if err = rows.Scan(&id, &swarm, &state, &version, &deadline); err != nil {
			return nil, nil, err
		}
		if state == "accepted" {
			refs = append(refs, map[string]any{"id": swarm, "resource_path": "/v1/swarms/" + swarm + "/context"})
		} else {
			pending = append(pending, map[string]any{"kind": "invitation", "id": id, "swarm_id": swarm, "etag": etag(version), "respond_by": deadline, "resource_path": "/v1/invitations/" + id, "content_trust": "untrusted"})
		}
	}
	return pending, refs, rows.Err()
}
func invitationPacket(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/invitations/")
	if !validID(id) {
		return 0, nil, fail(404, "not_found")
	}
	var swarm, objective, visibility, role, state string
	var version int64
	var deadline *time.Time
	err := tx.QueryRow(r.Context(), "SELECT m.swarm_id,s.objective,s.visibility,m.role,m.state,m.version,m.respond_by FROM memberships m JOIN swarms s ON s.id=m.swarm_id WHERE m.id=$1 AND m.agent_id=$2", id, a.AgentID).Scan(&swarm, &objective, &visibility, &role, &state, &version, &deadline)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"id": id, "swarm_id": swarm, "objective": objective, "visibility": visibility, "role": role, "state": state, "version": version, "etag": etag(version), "respond_by": deadline, "history_policy": "full", "content_trust": "untrusted"}, nil
}
