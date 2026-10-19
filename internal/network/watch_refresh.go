package network

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

const watchRefreshQueue = "intention_watch_refresh"
const watchFanoutQueue = "intention_watch_fanout"
const watcherRefreshPageSize = 256

type watchRefreshArgs struct {
	CandidateID      string `json:"candidate_id"`
	WatcherID        string `json:"watcher_id,omitempty"`
	AfterID          string `json:"after_id,omitempty"`
	ReadyProfileHash string `json:"ready_profile_hash,omitempty"`
	ReadyRevision    int    `json:"ready_revision,omitempty"`
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
	if args.ReadyProfileHash != "" {
		if args.ReadyRevision < 1 {
			return errors.New("invalid ready candidate revision")
		}
		if w.service.semantic == nil || args.ReadyProfileHash != w.service.semantic.hash {
			return nil
		}
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
	return retryDiscoveryLock(w.service.scanIntentCandidate(ctx, db, args.WatcherID, args.CandidateID, true))
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
	profile := ""
	if s.semantic != nil {
		profile = s.semantic.hash
	}
	if args.ReadyProfileHash != "" {
		var current bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1 AND i.current_revision=$2 AND r.embedding_profile_hash=$3 AND r.embedding IS NOT NULL)`, args.CandidateID, args.ReadyRevision, args.ReadyProfileHash).Scan(&current); err != nil || !current {
			return err
		}
	}
	rows, err := tx.Query(ctx, `SELECT i.id::text,r.document,
 CASE WHEN r.embedding IS NOT NULL AND target.embedding IS NOT NULL AND r.embedding_profile_hash=$4 AND target.embedding_profile_hash=$4 THEN 1-(r.embedding OPERATOR(public.<=>) target.embedding) END
 FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision
 JOIN principals p ON p.id=i.principal_id JOIN agents a ON a.id=i.agent_id
 CROSS JOIN (SELECT cr.embedding,cr.embedding_profile_hash FROM intents ci JOIN intent_revisions cr ON cr.intent_id=ci.id AND cr.revision=ci.current_revision WHERE ci.id=$5) target
 WHERE i.state='active' AND i.watch_enabled AND i.expires_at>clock_timestamp() AND i.principal_id<>$1 AND p.status='active' AND a.status='active' AND ($2='' OR i.id::text>$2) ORDER BY i.id LIMIT $3`, principal, args.AfterID, watcherRefreshPageSize, profile, args.CandidateID)
	if err != nil {
		return err
	}
	compatible := []string{}
	last := ""
	count := 0
	for rows.Next() {
		var id string
		var document []byte
		var cosine *float64
		if err = rows.Scan(&id, &document, &cosine); err != nil {
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
		if CompareDemand(watcher, candidate).Compatible || (semanticEligible && cosine != nil && *cosine >= s.semantic.minimumSimilarity(watcher) && compareDemandConstraints(watcher, candidate).Compatible) {
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
	pairQueue, fanoutQueue := watchRefreshQueue, watchFanoutQueue
	if args.ReadyProfileHash != "" {
		pairQueue, fanoutQueue = semanticPairQueue, semanticFanoutQueue
	}
	for _, watcher := range compatible {
		if _, err = queue.InsertTx(ctx, tx, watchRefreshArgs{CandidateID: args.CandidateID, WatcherID: watcher, ReadyProfileHash: args.ReadyProfileHash, ReadyRevision: args.ReadyRevision}, &river.InsertOpts{Queue: pairQueue, Priority: 1, MaxAttempts: 10}); err != nil {
			return err
		}
	}
	if count == watcherRefreshPageSize {
		if _, err = queue.InsertTx(ctx, tx, watchRefreshArgs{CandidateID: args.CandidateID, AfterID: last, ReadyProfileHash: args.ReadyProfileHash, ReadyRevision: args.ReadyRevision}, &river.InsertOpts{Queue: fanoutQueue, Priority: 2, MaxAttempts: 10}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
