package network

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

const discoveryQueue = "intention_discovery"
const discoveryRepairQueue = "intention_repair"

type scanArgs struct {
	CauseID string `json:"cause_id"`
}

func (scanArgs) Kind() string { return "intention_scan_v1" }

type scanWorker struct {
	river.WorkerDefaults[scanArgs]
	service *Service
}

func (w *scanWorker) Work(ctx context.Context, job *river.Job[scanArgs]) error {
	// Jobs committed before queue isolation retain their sweep and recruitment
	// effects while the new queues keep matching independent of recruitment.
	legacy := job.Queue == river.QueueDefault
	if !legacy && job.Args.CauseID != "repair" && job.Args.CauseID != "refresh" {
		if !validID(job.Args.CauseID) {
			return errors.New("invalid committed intention ID")
		}
		return w.service.scanIntentions(ctx, job.Args.CauseID)
	}
	if !legacy && job.Args.CauseID == "refresh" {
		return w.service.scanIntentions(ctx, "refresh")
	}
	if legacy && job.Args.CauseID != "repair" {
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
	if legacy {
		return w.service.ScanRecruitment(ctx, "")
	}
	return nil
}
func enqueueScan(ctx context.Context, tx pgx.Tx, cause string) error {
	client, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		return err
	}
	if _, err = client.InsertTx(ctx, tx, scanArgs{cause}, &river.InsertOpts{MaxAttempts: 10, Queue: discoveryQueue, Priority: 1}); err != nil {
		return err
	}
	if _, err = client.InsertTx(ctx, tx, watchRefreshArgs{CandidateID: cause}, &river.InsertOpts{Queue: watchFanoutQueue, Priority: 2, MaxAttempts: 10}); err != nil {
		return err
	}
	// At most one queued refresh follows a running sweep. Lock the queued row
	// until the originating command commits, so a fetch cannot consume its
	// wakeup before the new candidate is visible. Running or delayed/failed
	// sweeps never absorb another command's immediate wakeup.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(74932523)"); err != nil {
		return err
	}
	var queued int64
	err = tx.QueryRow(ctx, `SELECT id FROM undergrid_queue.river_job WHERE kind='intention_scan_v1' AND queue=$1 AND args->>'cause_id'='refresh' AND state='available' ORDER BY id LIMIT 1 FOR UPDATE`, discoveryRepairQueue).Scan(&queued)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	_, err = client.InsertTx(ctx, tx, scanArgs{"refresh"}, &river.InsertOpts{MaxAttempts: 10, Queue: discoveryRepairQueue, Priority: 4})
	return err
}

const sourceRefreshQueue = "permissioned_sources"

type sourceRefreshArgs struct {
	SourceID string `json:"source_id,omitempty"`
}

func (sourceRefreshArgs) Kind() string { return "permissioned_source_refresh_v1" }

type sourceRefreshWorker struct {
	river.WorkerDefaults[sourceRefreshArgs]
	refresher *SourceRefresher
}

func (w *sourceRefreshWorker) Work(ctx context.Context, job *river.Job[sourceRefreshArgs]) error {
	if job != nil && job.Args.SourceID != "" {
		return w.refresher.RefreshSource(ctx, job.Args.SourceID)
	}
	db, err := pgx.Connect(ctx, w.refresher.dsn)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if _, err = db.Exec(ctx, "SET ROLE undergrid_worker"); err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = w.refresher.syncApprovals(ctx, tx); err != nil {
		return err
	}
	client, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		return err
	}
	// Only an immediate/running wakeup absorbs another coordinator tick.
	// Completed history and delayed failures cannot suppress fresh work.
	for _, config := range w.refresher.configs {
		if _, err = client.InsertTx(ctx, tx, sourceRefreshArgs{SourceID: config.ID}, &river.InsertOpts{Queue: sourceRefreshQueue, Priority: 2, MaxAttempts: 5, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled}}}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func RunWorker(ctx context.Context, dsn, environment string) error {
	sources, err := sourceConfiguration(dsn)
	if err != nil {
		return err
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return err
	}
	// Queue fetchers, completion and periodic/leader maintenance must
	// retain pool capacity even while another queue has a backlog.
	config.MaxConns = 24
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
	river.AddWorker(workers, &sourceRefreshWorker{refresher: sources})
	service := New(dsn, environment, nil)
	if err = service.ConfigureArtifactsFromEnvironment(); err != nil {
		return err
	}
	river.AddWorker(workers, &deadlineWorker{service: service})
	river.AddWorker(workers, &artifactEvaluationWorker{service: service})
	river.AddWorker(workers, &inputGrantDeliveryWorker{dsn: dsn})
	river.AddWorker(workers, &scanWorker{service: service})
	river.AddWorker(workers, &watchRefreshWorker{service: service})
	river.AddWorker(workers, &externalNotificationWorker{service: service})
	river.AddWorker(workers, &fanoutWorker{dsn: dsn})
	river.AddWorker(workers, &recruitWorker{service: New(dsn, environment, nil)})
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: "undergrid_queue", SoftStopTimeout: 5 * time.Second, Workers: workers, Queues: map[string]river.QueueConfig{
		river.QueueDefault:             {MaxWorkers: 2},
		sourceRefreshQueue:             {MaxWorkers: 2},
		externalNotificationQueue:      {MaxWorkers: 1},
		externalFreshNotificationQueue: {MaxWorkers: 2, FetchCooldown: 100 * time.Millisecond, FetchPollInterval: 100 * time.Millisecond},
		externalFreshMatchQueue:        {MaxWorkers: 4, FetchCooldown: 100 * time.Millisecond, FetchPollInterval: 100 * time.Millisecond},
		externalReadyMatchQueue:        {MaxWorkers: 2, FetchCooldown: 100 * time.Millisecond, FetchPollInterval: 100 * time.Millisecond},
		externalNotificationMatchQueue: {MaxWorkers: 4, FetchCooldown: 100 * time.Millisecond, FetchPollInterval: 100 * time.Millisecond},
		"artifact_verification":        {MaxWorkers: 2},
		"coordination":                 {MaxWorkers: 2},
		"coordination_deadlines":       {MaxWorkers: 2},
		discoveryQueue:                 {MaxWorkers: 2},
		discoveryRepairQueue:           {MaxWorkers: 1},
		// Fanout continuations cannot wait behind their own pair jobs. Both
		// queues bound wakeup latency even when LISTEN/NOTIFY is coalesced.
		// Eight bounded pair slots amortize durable completion latency under
		// backlog; idle polling stays at 100ms to avoid busy polling.
		watchRefreshQueue: {MaxWorkers: 8, FetchCooldown: time.Millisecond, FetchPollInterval: 100 * time.Millisecond},
		watchFanoutQueue:  {MaxWorkers: 2, FetchCooldown: 100 * time.Millisecond, FetchPollInterval: 100 * time.Millisecond},
	}, PeriodicJobs: []*river.PeriodicJob{river.NewPeriodicJob(river.PeriodicInterval(time.Minute), func() (river.JobArgs, *river.InsertOpts) {
		return sourceRefreshArgs{}, &river.InsertOpts{Queue: sourceRefreshQueue, Priority: 1, MaxAttempts: 5}
	}, &river.PeriodicJobOpts{RunOnStart: true}), river.NewPeriodicJob(river.PeriodicInterval(time.Minute), func() (river.JobArgs, *river.InsertOpts) {

		return deadlineArgs{}, &river.InsertOpts{MaxAttempts: 10, Queue: "coordination_deadlines", Priority: 1}
	}, &river.PeriodicJobOpts{RunOnStart: true}), river.NewPeriodicJob(river.PeriodicInterval(15*time.Minute), func() (river.JobArgs, *river.InsertOpts) {
		return scanArgs{"repair"}, &river.InsertOpts{Queue: discoveryRepairQueue, Priority: 4}
	}, &river.PeriodicJobOpts{RunOnStart: true}), river.NewPeriodicJob(river.PeriodicInterval(15*time.Minute), func() (river.JobArgs, *river.InsertOpts) { return recruitArgs{}, nil }, &river.PeriodicJobOpts{RunOnStart: true})}})
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
