package network

import (
	"bytes"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"strings"
)

func ownerAssignmentRoute(path string) bool {
	p := strings.Split(strings.Trim(path, "/"), "/")
	return len(p) == 7 && p[0] == "v1" && p[1] == "owner" && p[2] == "agents" && validID(p[3]) && validID(p[5]) && (p[4] == "plans" && p[6] == "assignments" || p[4] == "tasks" && p[6] == "assignment-response")
}

func (s *Service) ownerAssignmentResponse(r *http.Request, tx pgx.Tx, principal string) (int, any, error) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	review := parts[4] == "plans"
	if review && r.Method != http.MethodGet || !review && r.Method != http.MethodPost {
		return 0, nil, fail(405, "method_not_allowed")
	}
	a, err := lockOwnerAgent(r.Context(), tx, principal, parts[3])
	if err != nil {
		return 0, nil, err
	}
	a.Scopes = []string{"swarms:read", "tasks:accept", "rights:accept"}
	planID := parts[5]
	if !review {
		task, err := loadTask(r.Context(), tx, parts[5], false)
		if err != nil {
			return 0, nil, err
		}
		// Recheck assignment attribution before returning any retained command result.
		if task.agent != a.AgentID {
			return 0, nil, fail(403, "assignee_required")
		}
		planID = task.plan
	}
	p, err := loadPlan(r.Context(), tx, planID, false)
	if err != nil {
		return 0, nil, err
	}
	if err = roomAuthority(r.Context(), tx, a, p.swarm, false); err != nil {
		return 0, nil, err
	}
	if review {
		rows, err := tx.Query(r.Context(), "SELECT id FROM tasks WHERE plan_id=$1 AND assigned_agent_id=$2 ORDER BY task_key LIMIT 50", p.id, a.AgentID)
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
		items := []any{}
		for _, id := range ids {
			_, result, err := taskResult(r.Context(), tx, id, a.PrincipalID)
			if err != nil {
				return 0, nil, err
			}
			items = append(items, result)
		}
		return 200, map[string]any{"items": items}, nil
	}
	// Keep the owner command body (including its human acknowledgement) in the
	// recovery fingerprint; only its shared domain payload is translated.
	return idempotent(r, tx, a, func() (int, any, error) {
		var in struct {
			Decision    string `json:"decision"`
			PlanHash    string `json:"plan_hash"`
			Acknowledge bool   `json:"acknowledge_responsibility_and_terms"`
		}
		if err := decode(r, &in); err != nil {
			return 0, nil, err
		}
		if in.Decision == "accept" && !in.Acknowledge {
			return 0, nil, fail(422, "assignment_acknowledgement_required")
		}
		command := r.Clone(r.Context())
		command.Body = io.NopCloser(bytes.NewReader(canonical(map[string]string{"decision": in.Decision, "plan_hash": in.PlanHash})))
		return s.updateTask(command, tx, a, parts[5], "assignment-response")
	})
}
