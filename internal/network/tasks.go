package network

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

type storedTask struct {
	id, plan, agent, state string
	spec                   TaskSpec
	version, epoch         int64
	response               time.Time
	progress               *time.Time
	artifact               *string
}

func loadTask(ctx context.Context, tx pgx.Tx, id string, lock bool) (storedTask, error) {
	var t storedTask
	var raw []byte
	query := "SELECT id,plan_id,assigned_agent_id,state,specification,version,claim_epoch,response_deadline,progress_deadline,submitted_artifact_id FROM tasks WHERE id=$1"
	if lock {
		query += " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, query, id).Scan(&t.id, &t.plan, &t.agent, &t.state, &raw, &t.version, &t.epoch, &t.response, &t.progress, &t.artifact)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, fail(404, "not_found")
	}
	if err != nil {
		return t, err
	}
	err = json.Unmarshal(raw, &t.spec)
	return t, err
}
func taskResult(ctx context.Context, tx pgx.Tx, id string) (int, any, error) {
	t, err := loadTask(ctx, tx, id, false)
	if err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"id": t.id, "plan_id": t.plan, "assigned_agent_id": t.agent, "state": t.state, "specification": t.spec, "version": t.version, "etag": etag(t.version), "claim_epoch": t.epoch, "response_deadline": t.response, "progress_deadline": t.progress, "submitted_artifact_id": t.artifact}, nil
}
func (s *Service) updateTask(r *http.Request, tx pgx.Tx, a actor, id, action string) (int, any, error) {
	t, err := loadTask(r.Context(), tx, id, false)
	if err != nil {
		return 0, nil, err
	}
	p, err := loadPlan(r.Context(), tx, t.plan, true)
	if err != nil {
		return 0, nil, err
	}
	t, err = loadTask(r.Context(), tx, id, true)
	if err != nil {
		return 0, nil, err
	}
	// Immutable plan approvals authorize the sealed wording, including after
	// a service restart loads a newer template under an existing ID.
	template, enabled := s.templates[p.sealed.Plan.Rights.TemplateID]
	if !enabled || template.SHA256 != p.sealed.TemplateHash {
		return 0, nil, fail(409, "plan_terms_expired_or_changed")
	}
	if t.agent != a.AgentID {
		return 0, nil, fail(403, "assignee_required")
	}
	if r.Header.Get("If-Match") == "" {
		return 0, nil, fail(428, "if_match_required")
	}
	if r.Header.Get("If-Match") != etag(t.version) {
		return 0, nil, fail(409, "version_conflict")
	}
	next, event := "", ""
	var deadline *time.Time
	if action == "assignment-response" {
		var in struct {
			Decision string `json:"decision"`
			PlanHash string `json:"plan_hash"`
		}
		if err = decode(r, &in); err != nil {
			return 0, nil, err
		}
		if !slices.Contains([]string{"accept", "decline"}, in.Decision) {
			return 0, nil, fail(422, "invalid_decision")
		}
		if in.PlanHash != p.hash || p.state != "proposed" || t.state != "offered" || !t.response.After(time.Now()) || !p.sealed.Plan.RespondBy.After(time.Now()) {
			return 0, nil, fail(409, "assignment_not_current")
		}
		next, event = "open", "task.assignment_declined"
		if in.Decision == "accept" {
			policy, _, err := s.requireRights(r.Context(), tx, a, p.sealed.Plan.Rights)
			if err != nil {
				return 0, nil, err
			}
			var load int
			if err = tx.QueryRow(r.Context(), "SELECT principal_task_load()").Scan(&load); err != nil {
				return 0, nil, err
			}
			if load >= policy.MaxActiveTasks {
				return 0, nil, fail(409, "principal_task_limit")
			}
			next, event = "accepted", "task.assignment_accepted"
		}
	} else {
		var in struct {
			State string `json:"state"`
			Epoch int64  `json:"claim_epoch"`
		}
		if err = decode(r, &in); err != nil {
			return 0, nil, err
		}
		if in.State != "in_progress" {
			return 0, nil, fail(422, "invalid_progress_state")
		}
		if in.Epoch != t.epoch || p.state != "accepted" || !slices.Contains([]string{"accepted", "in_progress", "changes_requested", "at_risk"}, t.state) {
			return 0, nil, fail(409, "task_not_current")
		}
		if t.spec.Deadline != nil && !t.spec.Deadline.After(time.Now()) {
			return 0, nil, fail(409, "task_deadline_passed")
		}
		if _, _, err = s.requireRights(r.Context(), tx, a, p.sealed.Plan.Rights); err != nil {
			return 0, nil, err
		}
		var blocked int
		if err = tx.QueryRow(r.Context(), "SELECT count(*) FROM tasks WHERE plan_id=$1 AND task_key=ANY($2) AND state<>'accepted_output'", p.id, t.spec.DependsOn).Scan(&blocked); err != nil {
			return 0, nil, err
		}
		if blocked != 0 {
			return 0, nil, fail(409, "dependencies_unaccepted")
		}
		next, event = "in_progress", "task.progress_recorded"
		when := time.Now().UTC().Add(time.Duration(t.spec.ProgressIntervalSeconds) * time.Second)
		deadline = &when
	}
	if _, err = tx.Exec(r.Context(), "UPDATE tasks SET state=$2,version=version+1,progress_deadline=$3 WHERE id=$1", id, next, deadline); err != nil {
		return 0, nil, err
	}
	if err = s.roomEvent(r.Context(), tx, a, id, t.version+1, event, p.swarm, "/v1/tasks/"+id); err != nil {
		return 0, nil, err
	}
	if action == "assignment-response" && next == "accepted" {
		if err = s.maybeAcceptPlan(r.Context(), tx, a, p); err != nil {
			return 0, nil, err
		}
	}
	return taskResult(r.Context(), tx, id)
}

// Pending obligations survive inbox retention and reconnects. RLS excludes
// assignments in rooms the agent can no longer access.
func assignmentSnapshot(ctx context.Context, tx pgx.Tx, a actor) ([]any, error) {
	rows, err := tx.Query(ctx, "SELECT t.id,t.plan_id,t.state,t.version,t.claim_epoch,p.content_hash FROM tasks t JOIN plan_versions p ON p.id=t.plan_id WHERE t.assigned_agent_id=$1 AND t.state IN ('offered','accepted','in_progress','submitted','changes_requested','at_risk') AND p.state IN ('proposed','accepted') ORDER BY t.id", a.AgentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, plan, state, hash string
		var version, epoch int64
		if err = rows.Scan(&id, &plan, &state, &version, &epoch, &hash); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"id": id, "plan_id": plan, "plan_hash": hash, "state": state, "etag": etag(version), "claim_epoch": epoch, "context_path": "/v1/tasks/" + id})
	}
	return items, rows.Err()
}
