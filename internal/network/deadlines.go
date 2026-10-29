package network

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

const taskCureWindow = 48 * time.Hour

type deadlineArgs struct{}

func (deadlineArgs) Kind() string { return "coordination_deadlines_v1" }

type deadlineWorker struct {
	river.WorkerDefaults[deadlineArgs]
	service *Service
}

func (w *deadlineWorker) Work(ctx context.Context, _ *river.Job[deadlineArgs]) error {
	return w.service.SweepCoordinationDeadlines(ctx, time.Now().UTC())
}

// SweepCoordinationDeadlines repairs persisted deadlines, including after a
// restart. A job carries no authority-bearing cached state. Each batch locks
// rooms in the same order as commands and rechecks rows under those locks.
func (s *Service) SweepCoordinationDeadlines(ctx context.Context, now time.Time) error {
	db, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if _, err = db.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		return err
	}
	rows, err := db.Query(ctx, `SELECT DISTINCT due.swarm_id FROM (
 SELECT swarm_id FROM memberships WHERE state IN ('applied','offered') AND respond_by<=$1
 UNION ALL SELECT swarm_id FROM plan_versions WHERE state='proposed' AND respond_by<=$1
 UNION ALL SELECT swarm_id FROM coordinator_proposals WHERE state='proposed' AND respond_by<=$1
 UNION ALL SELECT s.id FROM swarms s CROSS JOIN LATERAL (SELECT swarm_accepted_progress_at(s.id) AS since) activity
 WHERE s.state='active' AND activity.since <= $1::timestamptz-interval '30 days'
 AND (activity.since <= $1::timestamptz-interval '45 days' OR NOT EXISTS(
 SELECT 1 FROM events e WHERE e.aggregate_id=s.id AND e.type='swarm.inactivity_warning'
 AND (e.facts->>'inactivity_since_us')::bigint=(extract(epoch FROM activity.since)*1000000)::bigint))
 UNION ALL SELECT p.swarm_id FROM tasks t JOIN plan_versions p ON p.id=t.plan_id WHERE
 (t.state='offered' AND t.response_deadline<=$1) OR
 (p.state='accepted' AND NOT t.dependency_blocked AND ((t.state IN ('accepted','in_progress','changes_requested') AND t.progress_deadline<=$1) OR (t.state='at_risk' AND t.cure_deadline<=$1)))
 ) due JOIN swarms active_room ON active_room.id=due.swarm_id WHERE active_room.state='active' ORDER BY due.swarm_id LIMIT 100`, now)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.sweepRoomDeadlines(ctx, db, id, now); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) sweepRoomDeadlines(ctx context.Context, db *pgx.Conn, swarm string, now time.Time) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var coordinator, principal string
	// Lock event attribution before the room, matching authenticated commands.
	// A changed coordinator or archived room is skipped and safely rediscovered.
	if err = tx.QueryRow(ctx, `SELECT coordinator_agent::text,coordinator_principal::text FROM lock_swarm_deadline_room($1)`, swarm).Scan(&coordinator, &principal); errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	a := actor{PrincipalID: principal, AgentID: coordinator}
	type transition struct {
		id, kind, path string
		version        int64
	}
	changes := []transition{}
	// UPDATE locks and exact persisted predicates exclude renewed or accepted rows.
	rows, err := tx.Query(ctx, `UPDATE memberships SET state='expired',version=version+1 WHERE id IN (SELECT id FROM memberships WHERE swarm_id=$1 AND state IN ('applied','offered') AND respond_by<=$2 ORDER BY id LIMIT 100 FOR UPDATE) RETURNING id,version`, swarm, now)
	if err != nil {
		return err
	}
	for rows.Next() {
		var c transition
		if err = rows.Scan(&c.id, &c.version); err != nil {
			rows.Close()
			return err
		}
		c.kind = "membership.expired"
		c.path = "/v1/invitations/" + c.id
		changes = append(changes, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = tx.Query(ctx, `UPDATE coordinator_proposals SET state='expired',version=version+1 WHERE id IN(SELECT id FROM coordinator_proposals WHERE swarm_id=$1 AND state='proposed' AND respond_by<=$2 ORDER BY id LIMIT 100 FOR UPDATE) RETURNING id,version`, swarm, now)
	if err != nil {
		return err
	}
	for rows.Next() {
		var c transition
		if err = rows.Scan(&c.id, &c.version); err != nil {
			rows.Close()
			return err
		}
		c.kind = "coordinator.expired"
		c.path = "/v1/coordinator-proposals/" + c.id
		changes = append(changes, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// Lock due plans before their tasks, matching task response/progress commands.
	rows, err = tx.Query(ctx, `SELECT p.id FROM plan_versions p WHERE p.swarm_id=$1 AND ((p.state='proposed' AND p.respond_by<=$2) OR EXISTS(SELECT 1 FROM tasks t WHERE t.plan_id=p.id AND (
 (t.state='offered' AND t.response_deadline<=$2) OR (p.state='accepted' AND NOT t.dependency_blocked AND ((t.state IN ('accepted','in_progress','changes_requested') AND t.progress_deadline<=$2) OR (t.state='at_risk' AND t.cure_deadline<=$2)))
 ))) ORDER BY p.id LIMIT 100 FOR UPDATE`, swarm, now)
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
		tag, e := tx.Exec(ctx, "UPDATE plan_versions SET state='expired' WHERE id=$1 AND state='proposed' AND respond_by<=$2", plan, now)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 0 {
			changes = append(changes, transition{plan, "plan.expired", "/v1/plans/" + plan, 2})
		}
		rows, e = tx.Query(ctx, `UPDATE tasks t SET state=CASE WHEN p.state='expired' AND t.state<>'offered' THEN 'cancelled' WHEN t.state='offered' THEN 'open' WHEN t.state='at_risk' THEN 'abandoned' ELSE 'at_risk' END,
 version=t.version+1, claim_epoch=t.claim_epoch+CASE WHEN t.state IN ('offered','at_risk') OR p.state='expired' THEN 1 ELSE 0 END,
 cure_deadline=CASE WHEN t.state IN ('accepted','in_progress','changes_requested') AND p.state='accepted' THEN t.progress_deadline+$3::bigint*interval '1 second' ELSE NULL END
 FROM plan_versions p WHERE t.plan_id=p.id AND p.id=$1 AND (
 (p.state='expired' AND t.state IN ('offered','accepted')) OR (t.state='offered' AND t.response_deadline<=$2) OR
 (t.state IN ('accepted','in_progress','changes_requested') AND p.state='accepted' AND NOT t.dependency_blocked AND t.progress_deadline<=$2) OR
 (t.state='at_risk' AND p.state='accepted' AND NOT t.dependency_blocked AND t.cure_deadline<=$2)) RETURNING t.id,t.version,t.state`, plan, now, int64(taskCureWindow/time.Second))
		if e != nil {
			return e
		}
		for rows.Next() {
			var c transition
			var state string
			if e = rows.Scan(&c.id, &c.version, &state); e != nil {
				rows.Close()
				return e
			}
			c.kind = "task." + state
			if state == "open" {
				c.kind = "task.assignment_expired"
			}
			c.path = "/v1/tasks/" + c.id
			changes = append(changes, c)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
	}
	for _, c := range changes {
		if err = s.roomEvent(ctx, tx, a, c.id, c.version, c.kind, swarm, c.path); err != nil {
			return err
		}
	}
	if err := s.applySwarmInactivity(ctx, tx, a, swarm, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Room locking serializes the accepted-progress clock with reviews, output
// acceptance and explicit archival. Discovery carries no cached authority.
func (s *Service) applySwarmInactivity(ctx context.Context, tx pgx.Tx, attribution actor, swarm string, now time.Time) error {
	var state string
	var version int64
	var since time.Time
	if err := tx.QueryRow(ctx, "SELECT state,version,swarm_accepted_progress_at(id) FROM swarms WHERE id=$1", swarm).Scan(&state, &version, &since); err != nil {
		return err
	}
	if state != "active" || now.Before(since.Add(30*24*time.Hour)) {
		return nil
	}
	// The event principal identifies the room's accountable coordinator. The
	// explicit initiator prevents attributing a command or consent to that actor.
	facts := map[string]any{"initiated_by": "inactivity_policy", "inactivity_since_us": since.UnixMicro()}
	if !now.Before(since.Add(45 * 24 * time.Hour)) {
		return s.transitionSwarmArchive(ctx, tx, attribution, swarm, version+1, facts)
	}
	var warned bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM events WHERE aggregate_id=$1 AND type='swarm.inactivity_warning' AND (facts->>'inactivity_since_us')::bigint=$2)", swarm, since.UnixMicro()).Scan(&warned); err != nil {
		return err
	}
	if warned {
		return nil
	}
	if _, err := tx.Exec(ctx, "UPDATE swarms SET version=version+1 WHERE id=$1", swarm); err != nil {
		return err
	}
	return s.roomEventFacts(ctx, tx, attribution, swarm, version+1, "swarm.inactivity_warning", swarm, "/v1/swarms/"+swarm+"/context", nil, facts)
}
