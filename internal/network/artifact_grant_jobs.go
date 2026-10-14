package network

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

type inputGrantDeliveryArgs struct {
	GrantID string `json:"grant_id"`
}

func (inputGrantDeliveryArgs) Kind() string { return "artifact_input_grant_delivery_v1" }

type inputGrantDeliveryWorker struct {
	river.WorkerDefaults[inputGrantDeliveryArgs]
	dsn string
}

func enqueueInputGrantDelivery(ctx context.Context, tx pgx.Tx, id string) error {
	client, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		return err
	}
	_, err = client.InsertTx(ctx, tx, inputGrantDeliveryArgs{id}, &river.InsertOpts{MaxAttempts: 10, Queue: "artifact_verification"})
	return err
}
func (w *inputGrantDeliveryWorker) Work(ctx context.Context, job *river.Job[inputGrantDeliveryArgs]) error {
	db, err := pgx.Connect(ctx, w.dsn)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if _, err = db.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		return err
	}
	var agent, principal, event string
	err = db.QueryRow(ctx, `SELECT g.grant_recipient_agent_id,g.grant_recipient_principal_id,e.id FROM approvals g
		JOIN events e ON e.aggregate_id=g.id AND e.type='artifact.input_grant_created'
		WHERE g.id=$1 AND g.grant_state='active' AND g.grant_expires_at>clock_timestamp()
		AND input_grant_progress_audience_allowed(g.artifact_id,g.grant_recipient_agent_id,g.grant_recipient_principal_id)`, job.Args.GrantID).Scan(&agent, &principal, &event)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return deliverEvent(ctx, db, principal, agent, event, "/v1/artifact-input-grants/"+job.Args.GrantID)
}
