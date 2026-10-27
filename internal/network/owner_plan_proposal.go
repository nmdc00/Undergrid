package network

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

func ownerPlanProposalRoute(r *http.Request) bool {
	p := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	return len(p) == 7 && p[0] == "v1" && p[1] == "owner" && p[2] == "agents" && p[4] == "swarms" && (p[6] == "plan-context" || p[6] == "plans" && r.Method == http.MethodPost)
}

// An owner chooses an attribution agent, never a bearer credential. Proposal
// authority and retained-result recovery remain in the shared swarm command.
func (s *Service) ownerPlanProposal(r *http.Request, tx pgx.Tx, principal string) (int, any, error) {
	p := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if !validID(p[3]) || !validID(p[5]) {
		return 0, nil, fail(404, "not_found")
	}
	contextRead := p[6] == "plan-context"
	if contextRead && r.Method != http.MethodGet || !contextRead && r.Method != http.MethodPost {
		return 0, nil, fail(405, "method_not_allowed")
	}
	a, err := lockOwnerAgent(r.Context(), tx, principal, p[3])
	if err != nil {
		return 0, nil, err
	}
	if contextRead {
		return ownerPlanContext(r, tx, a, p[5])
	}
	command := r.Clone(r.Context())
	command.URL.Path = "/v1/swarms/" + p[5] + "/plans"
	a.Scopes = []string{"plans:propose"}
	return s.swarmCommand(command, tx, a)
}

func ownerPlanContext(r *http.Request, tx pgx.Tx, a actor, swarm string) (int, any, error) {
	if err := roomAuthority(r.Context(), tx, a, swarm, false); err != nil {
		return 0, nil, err
	}
	after := r.URL.Query().Get("after")
	if after != "" && !validID(after) {
		return 0, nil, fail(400, "invalid_cursor")
	}
	var version int64
	var state string
	if err := tx.QueryRow(r.Context(), "SELECT version,state FROM swarms WHERE id=$1", swarm).Scan(&version, &state); err != nil {
		return 0, nil, err
	}
	var latest any
	var id, hash, planState string
	var planVersion int64
	err := tx.QueryRow(r.Context(), "SELECT id,version,state,content_hash FROM plan_versions WHERE swarm_id=$1 ORDER BY version DESC LIMIT 1", swarm).Scan(&id, &planVersion, &planState, &hash)
	if err == nil {
		latest = map[string]any{"id": id, "version": planVersion, "state": planState, "content_hash": hash}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, err
	}
	rows, err := tx.Query(r.Context(), `SELECT id,agent_id,principal_id,role FROM memberships WHERE swarm_id=$1 AND state='accepted' AND ($2='' OR id>NULLIF($2,'')::uuid) ORDER BY id LIMIT 101`, swarm, after)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	items := []any{}
	next, last := "", ""
	for rows.Next() {
		var member, agent, principal, role string
		if err = rows.Scan(&member, &agent, &principal, &role); err != nil {
			return 0, nil, err
		}
		if len(items) == 100 {
			next = last
			break
		}
		items = append(items, map[string]any{"id": member, "agent_id": agent, "principal_id": principal, "role": role})
		last = member
	}
	return 200, map[string]any{"swarm": map[string]any{"id": swarm, "version": version, "etag": etag(version), "state": state}, "latest_plan": latest, "items": items, "next_after": next, "limit": 100}, rows.Err()
}
