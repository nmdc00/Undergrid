package network

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

const externalNotificationQueue = "external_notifications"
const externalNotificationMatchQueue = "external_notification_matches"
const externalFreshNotificationQueue = "external_fresh_notifications"
const externalFreshMatchQueue = "external_fresh_matches"
const externalReadyMatchQueue = "external_ready_matches"
const externalTextMatchSQL = "to_tsvector('simple',c.search_text) @@ plainto_tsquery('simple',q.query)"

const externalSnapshotSQL = `SELECT md5(config::text || COALESCE((SELECT string_agg(id::text || source_version || content_hash || status,'|' ORDER BY id) FROM undergrid.external_observations WHERE source_id=$1),'')) FROM undergrid.external_sources WHERE id=$1`

type externalNotificationArgs struct {
	IntentionID        string `json:"intention_id,omitempty"`
	AfterID            string `json:"after_id,omitempty"`
	Revision           int    `json:"revision,omitempty"`
	SourceID           string `json:"source_id,omitempty"`
	SnapshotHash       string `json:"snapshot_hash,omitempty"`
	AfterObservationID string `json:"after_observation_id,omitempty"`
}

func (externalNotificationArgs) Kind() string { return "external_notification_scan_v1" }

func enqueueExternalNotifications(ctx context.Context, tx pgx.Tx, args externalNotificationArgs) error {
	client, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		return err
	}
	queue, priority := externalNotificationQueue, 2
	if args.IntentionID != "" {
		queue, priority = externalNotificationMatchQueue, 1
	}
	if args.SourceID != "" {
		queue = externalFreshNotificationQueue
		if args.IntentionID != "" {
			queue = externalFreshMatchQueue
		}
	}
	// A confirmed matching page cannot queue behind the initial fanout that
	// produced it. Ready continuation capacity preserves bounded progress.
	if args.IntentionID != "" && args.AfterObservationID != "" {
		queue, priority = externalReadyMatchQueue, 1
	}
	_, err = client.InsertTx(ctx, tx, args, &river.InsertOpts{Queue: queue, MaxAttempts: 10, Priority: priority, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateRetryable, rivertype.JobStateScheduled}}})
	return err
}

type externalNotificationWorker struct {
	river.WorkerDefaults[externalNotificationArgs]
	service *Service
}

func (w *externalNotificationWorker) Work(ctx context.Context, job *river.Job[externalNotificationArgs]) error {
	db, err := pgx.Connect(ctx, w.service.dsn)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if _, err = db.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		return err
	}
	if job.Args.SourceID != "" {
		if !validID(job.Args.SourceID) || job.Args.SnapshotHash == "" {
			return errors.New("invalid external source wakeup")
		}
		// Valid UUID configuration permits uppercase hexadecimal, while stored
		// UUID text and keyset source filters use the canonical lowercase form.
		job.Args.SourceID = strings.ToLower(job.Args.SourceID)
		var current string
		err = db.QueryRow(ctx, externalSnapshotSQL, job.Args.SourceID).Scan(&current)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if current != job.Args.SnapshotHash {
			return nil
		}
	}
	if job.Args.AfterObservationID != "" && !validID(job.Args.AfterObservationID) {
		return errors.New("invalid external observation continuation")
	}
	if job.Args.IntentionID != "" {
		if !validID(job.Args.IntentionID) {
			return errors.New("invalid external watch ID")
		}
		return w.service.scanExternalIntention(ctx, db, job.Args)
	}
	if job.Args.AfterID != "" && !validID(job.Args.AfterID) {
		return errors.New("invalid external watch continuation")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT i.id,i.current_revision,r.document FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.state='active' AND i.watch_enabled AND i.expires_at>now() AND ($1='' OR i.id::text>$1) ORDER BY i.id LIMIT 100`, job.Args.AfterID)
	if err != nil {
		return err
	}
	type watch struct {
		id       string
		revision int
		query    string
	}
	ids := []watch{}
	for rows.Next() {
		var id watch
		var raw []byte
		if err = rows.Scan(&id.id, &id.revision, &raw); err != nil {
			rows.Close()
			return err
		}
		var input IntentInput
		if err = json.Unmarshal(raw, &input); err != nil {
			rows.Close()
			return err
		}
		id.query = externalMatchingQuery(input)
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// Filter this bounded page with the same query selection and SQL match
	// predicate used by the authority-bearing matching transaction. Unrelated
	// watches must not occupy fresh match slots ahead of an actual lead.
	watchIDs, watchRevisions, queries := []string{}, []int{}, []string{}
	for _, id := range ids {
		watchIDs = append(watchIDs, id.id)
		watchRevisions = append(watchRevisions, id.revision)
		queries = append(queries, id.query)
	}
	rows, err = tx.Query(ctx, `SELECT q.id,q.revision FROM unnest($1::text[],$2::integer[],$3::text[]) AS q(id,revision,query)
 WHERE EXISTS(SELECT 1 FROM external_notification_candidates c WHERE `+externalTextMatchSQL+` AND ($4='' OR c.source_id::text=$4)) ORDER BY q.id`, watchIDs, watchRevisions, queries, job.Args.SourceID)
	if err != nil {
		return err
	}
	matches := []watch{}
	for rows.Next() {
		var id watch
		if err = rows.Scan(&id.id, &id.revision); err != nil {
			rows.Close()
			return err
		}
		matches = append(matches, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range matches {
		if err = enqueueExternalNotifications(ctx, tx, externalNotificationArgs{IntentionID: id.id, Revision: id.revision, SourceID: job.Args.SourceID, SnapshotHash: job.Args.SnapshotHash}); err != nil {
			return err
		}
	}
	if len(ids) == 100 {
		if err = enqueueExternalNotifications(ctx, tx, externalNotificationArgs{AfterID: ids[len(ids)-1].id, SourceID: job.Args.SourceID, SnapshotHash: job.Args.SnapshotHash}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func externalMatchingQuery(input IntentInput) string {
	if query := strings.TrimSpace(input.PublicSummary); query != "" {
		return query
	}
	return strings.TrimSpace(input.Objective)
}

// The source refresh never receives a private query. This bounded local full-text
// comparison provides a discovery lead, with hard fit explicitly unassessed.
func (s *Service) scanExternalMatches(ctx context.Context, tx pgx.Tx, principal, agent, intent string, input IntentInput) error {
	return s.scanExternalMatchPage(ctx, tx, principal, agent, intent, input, externalNotificationArgs{IntentionID: intent})
}

func (s *Service) scanExternalMatchPage(ctx context.Context, tx pgx.Tx, principal, agent, intent string, input IntentInput, args externalNotificationArgs) error {
	query := externalMatchingQuery(input)
	if query == "" {
		return nil
	}
	var revision int
	if err := tx.QueryRow(ctx, "SELECT current_revision FROM intents WHERE id=$1", intent).Scan(&revision); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT c.id,c.source_version,c.kind,c.content_hash,c.source_fingerprint FROM external_notification_candidates c CROSS JOIN (SELECT $1::text AS query) q
 WHERE `+externalTextMatchSQL+`
 AND ($2='' OR c.source_id::text=$2) AND ($3='' OR c.id::text>$3)
 ORDER BY c.id LIMIT 50`, query, args.SourceID, args.AfterObservationID)
	if err != nil {
		return err
	}
	type candidate struct{ id, version, kind, hash, fingerprint string }
	candidates := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.id, &c.version, &c.kind, &c.hash, &c.fingerprint); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, c := range candidates {
		hash := digest(canonical([]any{"permitted-text-v1", intent, revision, c.id, c.version, c.hash, c.fingerprint}))
		evidence := map[string]any{"algorithm": "permitted-text-v1", "fit_status": "not_assessed", "hard_constraints": "unknown", "identity_assurance": "unknown", "source_version": c.version}
		id := NewID()
		tag, err := tx.Exec(ctx, `INSERT INTO opportunities(id,recipient_agent_id,source_intent_id,kind,input_hash,evidence,external_observation_id,external_intent_revision,external_observation_hash,external_source_fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`, id, agent, intent, c.kind, hash, canonical(evidence), c.id, revision, c.hash, c.fingerprint)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 1 {
			if err = s.notify(ctx, tx, principal, agent, id, 1, "opportunity.available", "/v1/opportunities/"+id); err != nil {
				return err
			}
		}
	}
	if len(candidates) == 50 {
		args.IntentionID = intent
		args.Revision = revision
		args.AfterObservationID = candidates[len(candidates)-1].id
		args.AfterID = ""
		if args.SourceID != "" {
			return enqueueExternalNotifications(ctx, tx, args)
		}
		// The ordinary watch's first page spans approved sources. Continue each
		// current source independently, pinned to that source's snapshot.
		rows, err := tx.Query(ctx, `SELECT DISTINCT source_id FROM external_notification_candidates ORDER BY source_id`)
		if err != nil {
			return err
		}
		sources := []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			sources = append(sources, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range sources {
			continuation := args
			continuation.SourceID = id
			if err = tx.QueryRow(ctx, externalSnapshotSQL, id).Scan(&continuation.SnapshotHash); err != nil {
				return err
			}
			if err = enqueueExternalNotifications(ctx, tx, continuation); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) scanExternalIntention(ctx context.Context, db *pgx.Conn, args externalNotificationArgs) error {
	id, expectedRevision := args.IntentionID, args.Revision
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var principal, agent string
	err = tx.QueryRow(ctx, "SELECT principal_id,agent_id FROM intents WHERE id=$1", id).Scan(&principal, &agent)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var active bool
	if err = tx.QueryRow(ctx, "SELECT status='active' FROM principals WHERE id=$1 FOR UPDATE", principal).Scan(&active); err != nil {
		return err
	}
	if !active {
		return nil
	}
	if err = tx.QueryRow(ctx, "SELECT status='active' FROM agents WHERE id=$1 FOR UPDATE", agent).Scan(&active); err != nil {
		return err
	}
	if !active {
		return nil
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT r.document FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1 AND i.state='active' AND i.watch_enabled AND i.expires_at>now() AND ($2=0 OR i.current_revision=$2) FOR UPDATE OF i`, id, expectedRevision).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var input IntentInput
	if err = json.Unmarshal(raw, &input); err != nil {
		return err
	}
	if err = s.scanExternalMatchPage(ctx, tx, principal, agent, id, input, args); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func externalOpportunityItems(ctx context.Context, tx pgx.Tx, a actor, intentOrID string, single bool, limit int) ([]any, error) {
	if !slices.Contains(a.Scopes, "intents:read") {
		return []any{}, nil
	}
	rows, err := tx.Query(ctx, `SELECT o.id,o.source_intent_id,o.kind,o.evidence,l.id,l.source_id,l.source_version,l.origin,l.canonical_url,l.publisher,l.permission_notice,l.metadata,l.observed_at,l.checked_at
 FROM opportunities o CROSS JOIN LATERAL external_fit_source(o.source_intent_id,o.external_observation_id) l
 WHERE o.recipient_agent_id=$1 AND external_opportunity_current(o.id)
 AND ($2='' OR CASE WHEN $3 THEN o.id::text=$2 ELSE o.source_intent_id::text=$2 END)
 ORDER BY o.created_at,o.id LIMIT $4`, a.AgentID, intentOrID, single, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, intent, kind string
		var evidence []byte
		var lead externalLead
		if err = rows.Scan(&id, &intent, &kind, &evidence, &lead.ID, &lead.SourceID, &lead.SourceVersion, &lead.Origin, &lead.CanonicalURL, &lead.Publisher, &lead.PermissionNotice, &lead.Metadata, &lead.ObservedAt, &lead.CheckedAt); err != nil {
			return nil, err
		}
		lead = externalLeadProjection(lead)
		items = append(items, map[string]any{"id": id, "source_intent_id": intent, "observation_id": lead.ID, "kind": kind, "origin": kind, "evidence": json.RawMessage(evidence), "lead": lead})
	}
	return items, rows.Err()
}
