package network

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Roots retain the resulting invalidated claim epoch, not a reusable assignment.
// They are event facts; no contributor text supplies graph or authority changes.
type withdrawnTaskRoot struct {
	ID    string `json:"task_id"`
	Epoch int64  `json:"claim_epoch"`
}

// The caller holds the room lock. Plan-before-task locks keep this transition
// aligned with progress, artifact commands and accepted-plan replacement.
func (s *Service) withdrawAssignments(ctx context.Context, tx pgx.Tx, a actor, swarm string) error {
	rows, err := tx.Query(ctx, `SELECT p.id FROM plan_versions p WHERE p.swarm_id=$1 AND EXISTS(
 SELECT 1 FROM tasks t WHERE t.plan_id=p.id AND t.assigned_agent_id=$2
 AND t.state NOT IN ('accepted_output','abandoned','cancelled','withdrawn','open')) ORDER BY p.id FOR UPDATE`, swarm, a.AgentID)
	if err != nil {
		return err
	}
	plans := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		plans = append(plans, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, plan := range plans {
		rows, err = tx.Query(ctx, `UPDATE tasks SET state='withdrawn',claim_epoch=claim_epoch+1,version=version+1
 WHERE plan_id=$1 AND assigned_agent_id=$2 AND state NOT IN ('accepted_output','abandoned','cancelled','withdrawn','open') RETURNING id,version,claim_epoch`, plan, a.AgentID)
		if err != nil {
			return err
		}
		type changedRoot struct {
			withdrawnTaskRoot
			version int64
		}
		changed := []changedRoot{}
		for rows.Next() {
			var root changedRoot
			if err = rows.Scan(&root.ID, &root.version, &root.Epoch); err != nil {
				rows.Close()
				return err
			}
			changed = append(changed, root)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		slices.SortFunc(changed, func(a, b changedRoot) int { return strings.Compare(a.ID, b.ID) })
		roots := make([]withdrawnTaskRoot, 0, len(changed))
		for _, root := range changed {
			roots = append(roots, root.withdrawnTaskRoot)
			if err = s.roomEvent(ctx, tx, a, root.ID, root.version, "task.withdrawn", swarm, "/v1/tasks/"+root.ID); err != nil {
				return err
			}
		}
		if err = s.blockWithdrawnDependents(ctx, tx, a, swarm, plan, roots); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) blockWithdrawnDependents(ctx context.Context, tx pgx.Tx, a actor, swarm, plan string, roots []withdrawnTaskRoot) error {
	type node struct {
		id, key, state string
		version        int64
		blocked        bool
		spec           TaskSpec
	}
	rows, err := tx.Query(ctx, `SELECT id,task_key,state,version,dependency_blocked,specification FROM tasks WHERE plan_id=$1 ORDER BY id LIMIT 51 FOR UPDATE`, plan)
	if err != nil {
		return err
	}
	nodes := []node{}
	keys := map[string]string{}
	for rows.Next() {
		var n node
		var raw []byte
		if err = rows.Scan(&n.id, &n.key, &n.state, &n.version, &n.blocked, &raw); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(raw, &n.spec); err != nil {
			rows.Close()
			return err
		}
		nodes = append(nodes, n)
		keys[n.key] = n.id
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(nodes) > 50 {
		return fail(409, "dependency_graph_limit")
	}
	// Reverse adjacency is bounded by the sealed plan's 50 tasks / 100 edges.
	dependents := map[string][]string{}
	edges := 0
	for _, n := range nodes {
		for _, key := range n.spec.DependsOn {
			id, ok := keys[key]
			if !ok {
				return fail(409, "invalid_dependency_graph")
			}
			dependents[id] = append(dependents[id], n.id)
			edges++
		}
	}
	if edges > 100 {
		return fail(409, "dependency_graph_limit")
	}
	causes := map[string][]withdrawnTaskRoot{}
	for _, root := range roots {
		seen := map[string]bool{root.ID: true}
		queue := []string{root.ID}
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			for _, next := range dependents[id] {
				if seen[next] {
					continue
				}
				seen[next] = true
				causes[next] = append(causes[next], root)
				queue = append(queue, next)
			}
		}
	}
	for _, n := range nodes {
		if n.blocked || len(causes[n.id]) == 0 || slices.Contains([]string{"accepted_output", "withdrawn", "abandoned", "cancelled"}, n.state) {
			continue
		}
		if _, err = tx.Exec(ctx, `UPDATE tasks SET dependency_blocked=true,version=version+1 WHERE id=$1`, n.id); err != nil {
			return err
		}
		if err = s.roomEventFacts(ctx, tx, a, n.id, n.version+1, "task.dependency_blocked", swarm, "/v1/tasks/"+n.id, nil, map[string]any{"withdrawn_roots": causes[n.id]}); err != nil {
			return err
		}
	}
	return nil
}

// A durable withdrawal block cannot be cleared by completing an unrelated task
// or by a heartbeat. Current direct prerequisites also gate canonical work.
func requireTaskDependencies(ctx context.Context, tx pgx.Tx, t storedTask) error {
	if t.blocked {
		return fail(409, "task_dependency_blocked")
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tasks WHERE plan_id=$1 AND task_key=ANY($2) AND state<>'accepted_output'`, t.plan, t.spec.DependsOn).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return fail(409, "dependencies_unaccepted")
	}
	return nil
}
