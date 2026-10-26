package network

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

const semanticEmbeddingQueue = "semantic_embedding"
const semanticReadyQueue = "semantic_ready"
const semanticFanoutQueue = "semantic_fanout"
const semanticPairQueue = "semantic_pairs"

type semanticEmbeddingArgs struct {
	IntentID string `json:"intent_id"`
}

func (semanticEmbeddingArgs) Kind() string { return "semantic_embedding_v1" }

type semanticEmbeddingWorker struct {
	river.WorkerDefaults[semanticEmbeddingArgs]
	service *Service
}

func (w *semanticEmbeddingWorker) Work(ctx context.Context, job *river.Job[semanticEmbeddingArgs]) error {
	if !validID(job.Args.IntentID) {
		return errors.New("invalid embedding intention")
	}
	if w.service.semantic == nil {
		return nil
	}
	db, err := pgx.Connect(ctx, w.service.dsn)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if _, err = db.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		return err
	}
	return w.service.ensureSemanticEmbedding(ctx, db, job.Args.IntentID)
}

type semanticReadyArgs struct {
	IntentID    string `json:"intent_id"`
	Revision    int    `json:"revision"`
	ProfileHash string `json:"profile_hash"`
}

func (semanticReadyArgs) Kind() string { return "semantic_ready_v1" }

type semanticReadyWorker struct {
	river.WorkerDefaults[semanticReadyArgs]
	service *Service
}

func (w *semanticReadyWorker) Work(ctx context.Context, job *river.Job[semanticReadyArgs]) error {
	a := job.Args
	if !validID(a.IntentID) || a.Revision < 1 {
		return errors.New("invalid ready intention")
	}
	if w.service.semantic == nil || a.ProfileHash != w.service.semantic.hash {
		return nil
	}
	db, err := pgx.Connect(ctx, w.service.dsn)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if _, err = db.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		return err
	}
	var ready bool
	if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1 AND i.current_revision=$2 AND r.embedding_profile_hash=$3 AND r.embedding IS NOT NULL)`, a.IntentID, a.Revision, a.ProfileHash).Scan(&ready); err != nil || !ready {
		return err
	}
	// This lane never performs provider I/O. Domain locks and all authority are
	// rechecked in the same production transaction used by ordinary pair refresh.
	return retryDiscoveryLock(w.service.scanIntentCandidate(ctx, db, a.IntentID, "", true))
}
func retryDiscoveryLock(err error) error {
	var locked *pgconn.PgError
	if errors.As(err, &locked) && locked.Code == "55P03" {
		return river.JobSnooze(250 * time.Millisecond)
	}
	return err
}

// Observation and its ready wakeups commit together. Revision/profile pins stop
// obsolete work; no running or delayed job absorbs a later revision's wakeup.
func enqueueSemanticReady(ctx context.Context, tx pgx.Tx, id string, revision int, profile string) error {
	// A committed observation promotes its already-durable repair wakeup. This
	// preserves freshness behind failed/obsolete projections without another job
	// identity or a nontransactional priority race.
	if _, err := tx.Exec(ctx, `UPDATE undergrid_queue.river_job SET priority=1 WHERE kind='semantic_repair_v1' AND args->>'intent_id'=$1 AND args->>'revision'=$2 AND args->>'profile_hash'=$3 AND state IN ('available','pending','scheduled','retryable')`, id, fmt.Sprint(revision), profile); err != nil {
		return err
	}
	client, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		return err
	}
	if _, err = client.InsertTx(ctx, tx, semanticReadyArgs{id, revision, profile}, &river.InsertOpts{Queue: semanticReadyQueue, Priority: 1, MaxAttempts: 10}); err != nil {
		return err
	}
	_, err = client.InsertTx(ctx, tx, watchRefreshArgs{CandidateID: id, ReadyProfileHash: profile, ReadyRevision: revision}, &river.InsertOpts{Queue: semanticFanoutQueue, Priority: 1, MaxAttempts: 10})
	return err
}

const semanticRepairQueue = "semantic_repair"

type semanticRepairArgs struct {
	IntentID    string `json:"intent_id"`
	Revision    int    `json:"revision"`
	ProfileHash string `json:"profile_hash"`
}

func (semanticRepairArgs) Kind() string { return "semantic_repair_v1" }

type semanticRepairWorker struct {
	river.WorkerDefaults[semanticRepairArgs]
	service *Service
}

func (w *semanticRepairWorker) Work(ctx context.Context, job *river.Job[semanticRepairArgs]) error {
	a := job.Args
	if !validID(a.IntentID) || a.Revision < 1 {
		return errors.New("invalid repair intention")
	}
	if w.service.semantic == nil || a.ProfileHash != w.service.semantic.hash {
		return nil
	}
	db, err := pgx.Connect(ctx, w.service.dsn)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if _, err = db.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		return err
	}
	var current bool
	if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM intents WHERE id=$1 AND current_revision=$2 AND state='active' AND watch_enabled)`, a.IntentID, a.Revision).Scan(&current); err != nil || !current {
		return err
	}
	// Repair external projections through the same bounded provider lane, without
	// keeping this local discovery slot waiting for the external request.
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	queue, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		return err
	}
	if _, err = queue.InsertTx(ctx, tx, semanticEmbeddingArgs{IntentID: a.IntentID}, &river.InsertOpts{Queue: semanticEmbeddingQueue, Priority: 2, MaxAttempts: 10}); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return retryDiscoveryLock(w.service.scanIntent(ctx, db, a.IntentID))
}

// Commit bounded independent work without provider I/O. Pending/retrying work is
// excluded before LIMIT so poison projections cannot monopolize each sweep.
// Revision share locks bridge readiness selection and insertion: observation
// either commits first (ready priority here), or promotes the inserted job later.
func (s *Service) enqueueSemanticRepairs(ctx context.Context, refresh bool) error {
	db, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if _, err = db.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT i.id,i.current_revision,r.document,COALESCE(r.embedding_profile_hash=$1 AND r.embedding IS NOT NULL,false) FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.state='active' AND i.watch_enabled AND ($2 OR i.next_scan<=now()) AND NOT EXISTS (SELECT 1 FROM undergrid_queue.river_job j WHERE j.kind='semantic_repair_v1' AND j.args->>'intent_id'=i.id::text AND j.args->>'revision'=i.current_revision::text AND j.args->>'profile_hash'=$1 AND j.state IN ('available','pending','running','scheduled','retryable')) ORDER BY i.next_scan,i.id LIMIT 1000 FOR SHARE OF r`, s.semantic.hash, refresh)
	if err != nil {
		return err
	}
	type repairJob struct {
		args     semanticRepairArgs
		priority int
	}
	jobs := []repairJob{}
	for rows.Next() {
		a := semanticRepairArgs{ProfileHash: s.semantic.hash}
		var raw []byte
		var ready bool
		if err = rows.Scan(&a.IntentID, &a.Revision, &raw, &ready); err != nil {
			rows.Close()
			return err
		}
		priority := 4
		input, err := decodeStoredIntent(raw, a.Revision)
		if err != nil {
			rows.Close()
			return err
		}
		// Selected semantic queries retain foreground priority in both due
		// repair and explicit refresh; exact-only watches remain background.
		if _, _, eligible := s.semantic.selectedProjection(input, ""); eligible {
			priority = 2
			if ready {
				priority = 1
			}
		}

		jobs = append(jobs, repairJob{a, priority})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	client, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		return err
	}
	for _, job := range jobs {
		_, err = client.InsertTx(ctx, tx, job.args, &river.InsertOpts{Queue: semanticRepairQueue, Priority: job.priority, MaxAttempts: 10, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled, rivertype.JobStateRetryable}}})
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
