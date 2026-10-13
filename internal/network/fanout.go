package network

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

type fanoutArgs struct {
	EventID string `json:"event_id"`
}

func (fanoutArgs) Kind() string { return "event_fanout_v1" }

type fanoutWorker struct {
	river.WorkerDefaults[fanoutArgs]
	dsn string
}

func enqueueFanout(ctx context.Context, tx pgx.Tx, id string) error {
	client, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		return err
	}
	_, err = client.InsertTx(ctx, tx, fanoutArgs{id}, &river.InsertOpts{MaxAttempts: 10, Queue: "coordination"})
	return err
}
func (w *fanoutWorker) Work(ctx context.Context, job *river.Job[fanoutArgs]) error {
	db, err := pgx.Connect(ctx, w.dsn)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if _, err = db.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		return err
	}
	var facts []byte
	var kind, aggregate string
	if err = db.QueryRow(ctx, "SELECT facts,type,aggregate_id FROM events WHERE id=$1", job.Args.EventID).Scan(&facts, &kind, &aggregate); err != nil {
		return err
	}
	var event struct {
		SwarmID string `json:"swarm_id"`
		Path    string `json:"resource_path"`
	}
	if err = json.Unmarshal(facts, &event); err != nil {
		return err
	}
	rows, err := db.Query(ctx, "SELECT agent_id,principal_id FROM memberships WHERE swarm_id=$1 AND CASE WHEN $2='membership.offered' THEN id=$3 AND state='offered' WHEN $2='membership.expired' THEN state='accepted' OR id=$3 ELSE state='accepted' END ORDER BY principal_id,agent_id", event.SwarmID, kind, aggregate)
	if err != nil {
		return err
	}
	type recipient struct{ agent, principal string }
	recipients := []recipient{}
	for rows.Next() {
		var target recipient
		if err = rows.Scan(&target.agent, &target.principal); err != nil {
			rows.Close()
			return err
		}
		recipients = append(recipients, target)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, target := range recipients {
		if err = deliverEvent(ctx, db, target.principal, target.agent, job.Args.EventID, event.Path); err != nil {
			return err
		}
	}
	return nil
}
func deliverEvent(ctx context.Context, db *pgx.Conn, principal, agent, event, path string) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var state string
	if err = tx.QueryRow(ctx, "SELECT status FROM principals WHERE id=$1 FOR UPDATE", principal).Scan(&state); err != nil {
		return err
	}
	if state != "active" {
		return nil
	}
	if err = tx.QueryRow(ctx, "SELECT status FROM agents WHERE id=$1 FOR UPDATE", agent).Scan(&state); err != nil {
		return err
	}
	if state != "active" {
		return nil
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM inbox WHERE agent_id=$1 AND event_id=$2)", agent, event).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	var sequence int64
	if err = tx.QueryRow(ctx, "UPDATE agents SET inbox_sequence=inbox_sequence+1 WHERE id=$1 RETURNING inbox_sequence", agent).Scan(&sequence); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO inbox(agent_id,sequence,event_id,resource_path) VALUES($1,$2,$3,$4)", agent, sequence, event, path); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
