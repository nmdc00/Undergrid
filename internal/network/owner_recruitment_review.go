package network

import (
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
)

const ownerRecruitmentCandidateLimit = 50

func (s *Service) ownerRecruitmentReview(r *http.Request, tx pgx.Tx, a actor, selected string) (int, any, error) {
	after := r.URL.Query().Get("after")
	if after != "" && !validID(after) {
		return 0, nil, fail(400, "invalid_cursor")
	}
	rows, err := tx.Query(r.Context(), `SELECT o.id FROM opportunities o JOIN swarms s ON s.id=o.source_swarm_id WHERE o.recipient_agent_id=$1 AND o.kind='builder_recruitment' AND o.state='suggested' AND s.state='active' AND s.visibility<>'private' AND ($2='' OR o.id=NULLIF($2,'')::uuid) AND ($3='' OR o.id>NULLIF($3,'')::uuid) ORDER BY o.id LIMIT $4`, a.AgentID, selected, after, ownerRecruitmentCandidateLimit+1)
	if err != nil {
		return 0, nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, nil, err
	}
	next := ""
	if len(ids) > ownerRecruitmentCandidateLimit {
		ids = ids[:ownerRecruitmentCandidateLimit]
		next = ids[len(ids)-1]
	}
	items := []any{}
	for _, id := range ids {
		if err = s.requireCurrentOpportunityEvidence(r, tx, a, id); err != nil {
			var denial problem
			if errors.As(err, &denial) && denial.Status == 404 {
				continue
			}
			return 0, nil, err
		}
		var swarm, roleKey, objective, visibility, hash string
		var roomVersion int64
		var rawRoles, evidence []byte
		if err = tx.QueryRow(r.Context(), `SELECT s.id,o.role_key,s.objective,s.visibility,s.version,s.recruitment_roles,o.input_hash,o.evidence FROM opportunities o JOIN swarms s ON s.id=o.source_swarm_id WHERE o.id=$1 AND o.recipient_agent_id=$2`, id, a.AgentID).Scan(&swarm, &roleKey, &objective, &visibility, &roomVersion, &rawRoles, &hash, &evidence); err != nil {
			return 0, nil, err
		}
		var roles []RecruitmentRole
		if err = json.Unmarshal(rawRoles, &roles); err != nil {
			return 0, nil, err
		}
		for _, role := range roles {
			if role.Key == roleKey {
				items = append(items, map[string]any{"id": id, "kind": "builder_recruitment", "origin": "registered_capability", "swarm_id": swarm, "objective": objective, "visibility": visibility, "swarm_version": roomVersion, "swarm_etag": etag(roomVersion), "role": role, "input_hash": hash, "evidence": json.RawMessage(evidence), "history_policy": "full", "identity_assurance": "common_control_unknown", "content_trust": "untrusted"})
				break
			}
		}
	}
	if selected != "" {
		if len(items) != 1 {
			return 0, nil, fail(404, "not_found")
		}
		return 200, items[0], nil
	}
	return 200, map[string]any{"items": items, "next_after": next, "candidate_limit": ownerRecruitmentCandidateLimit}, nil
}
