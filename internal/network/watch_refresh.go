package network

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

const watchRefreshQueue = "intention_watch_refresh"
const watchFanoutQueue = "intention_watch_fanout"
const watcherRefreshPageSize = 256

type watchRefreshArgs struct {
	CandidateID string `json:"candidate_id"`
	WatcherID   string `json:"watcher_id,omitempty"`
	AfterID     string `json:"after_id,omitempty"`
}

func (watchRefreshArgs) Kind() string { return "intention_watch_refresh_v1" }

type watchRefreshWorker struct {
	river.WorkerDefaults[watchRefreshArgs]
	service *Service
}

func (w *watchRefreshWorker) Work(ctx context.Context, job *river.Job[watchRefreshArgs]) error {
	args := job.Args
	if !validID(args.CandidateID) || args.WatcherID != "" && !validID(args.WatcherID) || args.AfterID != "" && !validID(args.AfterID) || args.WatcherID != "" && args.AfterID != "" {
		return errors.New("invalid watcher refresh identity")
	}
	if args.WatcherID == "" {
		return w.service.enqueueCompatibleWatchers(ctx, args)
	}
	db, err := pgx.Connect(ctx, w.service.dsn)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if _, err = db.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		return err
	}
	// A denial-only read avoids taking owner locks for obsolete durable pairs.
	// It grants no authority: every eligible result still enters the original
	// transaction and rechecks current ownership, state and projection.
	var eligible bool
	err = db.QueryRow(ctx, `SELECT EXISTS(
	 SELECT 1 FROM intents i JOIN principals p ON p.id=i.principal_id JOIN agents a ON a.id=i.agent_id
	 WHERE i.id=$1 AND i.state='active' AND i.watch_enabled AND i.expires_at>clock_timestamp()
	 AND p.status='active' AND a.status='active'
	 AND EXISTS(SELECT 1 FROM public_intents() candidate WHERE candidate.id=$2)
	)`, args.WatcherID, args.CandidateID).Scan(&eligible)
	if err != nil || !eligible {
		return err
	}
	err = w.service.scanIntentCandidate(ctx, db, args.WatcherID, args.CandidateID, true)
	var locked *pgconn.PgError
	if errors.As(err, &locked) && locked.Code == "55P03" {
		return river.JobSnooze(250 * time.Millisecond)
	}
	return err
}

// Fanout reads a bounded page without taking unrelated owner locks. Only
// compatible pairs enter the urgent queue; each pair rechecks current authority
// and projection in the shared locked scan transaction before publishing.
func (s *Service) enqueueCompatibleWatchers(ctx context.Context, args watchRefreshArgs) error {
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
	var raw []byte
	var principal string
	err = tx.QueryRow(ctx, "SELECT projection,principal_id::text FROM public_intents() WHERE id=$1", args.CandidateID).Scan(&raw, &principal)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var candidate IntentInput
	if err = json.Unmarshal(raw, &candidate); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT i.id::text,r.document FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision JOIN principals p ON p.id=i.principal_id JOIN agents a ON a.id=i.agent_id WHERE i.state='active' AND i.watch_enabled AND i.expires_at>now() AND i.principal_id<>$1 AND p.status='active' AND a.status='active' AND ($2='' OR i.id::text>$2) ORDER BY i.id LIMIT $3`, principal, args.AfterID, watcherRefreshPageSize)
	if err != nil {
		return err
	}
	compatible := []string{}
	last := ""
	count := 0
	for rows.Next() {
		var id string
		var document []byte
		if err = rows.Scan(&id, &document); err != nil {
			rows.Close()
			return err
		}
		last = id
		count++
		var watcher IntentInput
		if err = json.Unmarshal(document, &watcher); err != nil {
			rows.Close()
			return err
		}
		semanticEligible := false
		if s.semantic != nil {
			_, _, semanticEligible = s.semantic.selectedProjection(watcher, "")
		}
		if CompareDemand(watcher, candidate).Compatible || (semanticEligible && compareDemandConstraints(watcher, candidate).Compatible) {
			compatible = append(compatible, id)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	queue, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		return err
	}
	for _, watcher := range compatible {
		if _, err = queue.InsertTx(ctx, tx, watchRefreshArgs{CandidateID: args.CandidateID, WatcherID: watcher}, &river.InsertOpts{Queue: watchRefreshQueue, Priority: 1, MaxAttempts: 10}); err != nil {
			return err
		}
	}
	if count == watcherRefreshPageSize {
		if _, err = queue.InsertTx(ctx, tx, watchRefreshArgs{CandidateID: args.CandidateID, AfterID: last}, &river.InsertOpts{Queue: watchFanoutQueue, Priority: 2, MaxAttempts: 10}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
