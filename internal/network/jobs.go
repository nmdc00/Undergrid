package network

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

type scanArgs struct {
	CauseID string `json:"cause_id"`
}

func (scanArgs) Kind() string { return "intention_scan_v1" }

type scanWorker struct {
	river.WorkerDefaults[scanArgs]
	service *Service
}

func (w *scanWorker) Work(ctx context.Context, job *river.Job[scanArgs]) error {
	if job.Args.CauseID != "repair" {
		db, err := pgx.Connect(ctx, w.service.dsn)
		if err != nil {
			return err
		}
		defer db.Close(ctx)
		if _, err = db.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
			return err
		}
		if _, err = db.Exec(ctx, "UPDATE intents SET next_scan=now() WHERE state='active' AND watch_enabled"); err != nil {
			return err
		}
	}
	if err := w.service.ScanDueIntentions(ctx); err != nil {
		return err
	}
	return w.service.ScanRecruitment(ctx, "")
}
func enqueueScan(ctx context.Context, tx pgx.Tx, cause string) error {
	client, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		return err
	}
	_, err = client.InsertTx(ctx, tx, scanArgs{cause}, &river.InsertOpts{MaxAttempts: 10})
	return err
}
func RunWorker(ctx context.Context, dsn, environment string) error {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return err
	}
	config.MaxConns = 5
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET ROLE undergrid_worker")
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return err
	}
	defer pool.Close()
	workers := river.NewWorkers()
	river.AddWorker(workers, &scanWorker{service: New(dsn, environment, nil)})
	river.AddWorker(workers, &fanoutWorker{dsn: dsn})
	river.AddWorker(workers, &recruitWorker{service: New(dsn, environment, nil)})
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: "undergrid_queue", SoftStopTimeout: 5 * time.Second, Workers: workers, Queues: map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 2}}, PeriodicJobs: []*river.PeriodicJob{river.NewPeriodicJob(river.PeriodicInterval(15*time.Minute), func() (river.JobArgs, *river.InsertOpts) { return scanArgs{"repair"}, nil }, &river.PeriodicJobOpts{RunOnStart: true})}})
	if err != nil {
		return err
	}
	workerContext, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	if err = client.Start(workerContext); err != nil {
		return err
	}
	<-ctx.Done()
	// Drain briefly, then River cancels running job contexts before the outer
	// deadline. This leaves time to record retriable work before closing the pool.
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return client.Stop(shutdown)
}
