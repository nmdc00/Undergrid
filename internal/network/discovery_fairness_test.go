package network

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func TestCommittedDiscoverySurvivesBlockedRecruitment(t *testing.T) {
	s, db := fixture(t)
	organizer := collaborationClient(t, s)
	collaborationClient(t, s)
	code, room := organizer.call("POST", "/v1/swarms", "blocked-recruit-room", map[string]any{"objective": "Recruit a catalog builder", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "builder", "schema_family": "source_catalog_v1", "description": "Build catalog"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	code, organizerIdentity := organizer.call("GET", "/v1/me", "", nil)
	if code != 200 {
		t.Fatalf("organizer identity: %d %v", code, organizerIdentity)
	}
	a, b := client(t, s), client(t, s)
	code, firstOwner := a.call("GET", "/v1/me", "", nil)
	if code != 200 {
		t.Fatalf("first owner identity: %d %v", code, firstOwner)
	}
	code, secondOwner := b.call("GET", "/v1/me", "", nil)
	if code != 200 {
		t.Fatalf("second owner identity: %d %v", code, secondOwner)
	}
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Fair discovery " + NewID()
	ctx := context.Background()
	var targetAgent, targetPrincipal string
	if err := db.QueryRow(ctx, `SELECT a.id::text,a.principal_id::text
		FROM undergrid.agents a JOIN undergrid.principals p ON p.id=a.principal_id
		WHERE a.id<>$1 AND a.principal_id<>$2 AND a.principal_id<>$3
		AND a.status='active' AND p.status='active'
		AND a.profile->>'public'='true' AND a.profile->>'recruitment'='true'
		AND a.profile->'capabilities' @> '["source_catalog_v1"]'::jsonb
		ORDER BY a.id LIMIT 1`, organizerIdentity["agent_id"], firstOwner["principal_id"], secondOwner["principal_id"]).Scan(&targetAgent, &targetPrincipal); err != nil {
		t.Fatalf("select first eligible non-coordinator recruitment principal: %v", err)
	}
	if targetAgent == organizerIdentity["agent_id"] {
		t.Fatal("selected coordinator as the recruitment target")
	}
	// Explicitly queue two jobs for only this room. River fetches jobs by
	// priority, scheduled_at, then ID; its startup scheduler can promote due
	// retryable/scheduled jobs after this query. Place these test-owned jobs
	// just ahead of every due priority-1 default-queue row, across all states,
	// without changing existing job state or priority. This keeps the lock
	// precondition causal under either scheduler ordering.
	queueDB, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer queueDB.Close(ctx)
	var earliestDefault time.Time
	if err = queueDB.QueryRow(ctx, `SELECT COALESCE(min(scheduled_at),clock_timestamp())
		FROM undergrid_queue.river_job WHERE queue=$1 AND priority=1
		AND scheduled_at<=clock_timestamp()`, river.QueueDefault).Scan(&earliestDefault); err != nil {
		t.Fatal(err)
	}
	queue, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		t.Fatal(err)
	}
	jobTx, err := queueDB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	jobIDs := make([]int64, 0, 2)
	for range 2 {
		inserted, insertErr := queue.InsertTx(ctx, jobTx, recruitArgs{SwarmID: room["id"].(string)}, &river.InsertOpts{
			Queue: river.QueueDefault, Priority: 1, MaxAttempts: 10,
		})
		if insertErr != nil {
			_ = jobTx.Rollback(ctx)
			t.Fatal(insertErr)
		}
		if inserted.Job.State != "available" {
			_ = jobTx.Rollback(ctx)
			t.Fatalf("controlled recruitment insert was not immediately available: state=%s job_id=%d", inserted.Job.State, inserted.Job.ID)
		}
		jobIDs = append(jobIDs, inserted.Job.ID)
	}
	// River marks any insert with a nonzero ScheduledAt as scheduled, even if
	// the requested time is in the past. Keep these two controlled jobs in the
	// available state, then adjust only their schedule values in this same
	// fixture transaction so a startup scheduler cannot delay them behind the
	// existing ready backlog. The all-state minimum also fences rows River may
	// promote from retryable/scheduled while the worker starts.
	updated, err := jobTx.Exec(ctx, `UPDATE undergrid_queue.river_job SET scheduled_at=$1
		WHERE id=ANY($2::bigint[]) AND queue=$3 AND priority=1 AND state='available'`, earliestDefault.Add(-time.Second), jobIDs, river.QueueDefault)
	if err != nil {
		_ = jobTx.Rollback(ctx)
		t.Fatal(err)
	}
	if updated.RowsAffected() != 2 {
		_ = jobTx.Rollback(ctx)
		t.Fatalf("only controlled fixture jobs should be rescheduled while available: updated=%d", updated.RowsAffected())
	}
	if err = jobTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// This lock must use the fixture connection, not queueDB: the queued rows
	// have already committed independently, and the principal lock remains held
	// until the worker test's defer rolls it back.
	lock, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	var lockerPID int
	if err = lock.QueryRow(ctx, "SELECT pg_backend_pid() FROM undergrid.principals WHERE id=$1 FOR UPDATE", targetPrincipal).Scan(&lockerPID); err != nil {
		t.Fatal(err)
	}
	observer, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close(ctx)
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	tag := "fairness_" + NewID()
	workerDSN := testWorkerDSN(t, s.dsn, tag)
	go func() { done <- RunWorker(workerCtx, workerDSN, "development") }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(12 * time.Second):
			t.Error("worker did not stop")
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	lastBlocked, lastRunning := 0, 0
	for {
		var blocked int
		if err = observer.QueryRow(ctx, `WITH RECURSIVE blocked AS (
 SELECT pid FROM pg_stat_activity WHERE $2=ANY(pg_blocking_pids(pid))
 UNION SELECT a.pid FROM pg_stat_activity a JOIN blocked b ON b.pid=ANY(pg_blocking_pids(a.pid))
) SELECT count(*) FROM pg_stat_activity a JOIN blocked b ON b.pid=a.pid WHERE a.application_name=$1`, tag, lockerPID).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		var running int
		if err = observer.QueryRow(ctx, `SELECT count(*) FROM undergrid_queue.river_job
			WHERE id=ANY($1::bigint[]) AND kind='builder_recruitment_v1' AND state='running'`, jobIDs).Scan(&running); err != nil {
			t.Fatal(err)
		}
		lastBlocked, lastRunning = blocked, running
		if running == len(jobIDs) && blocked >= len(jobIDs) {
			break
		}
		if time.Now().After(deadline) {
			var tagged int
			if err = observer.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE application_name=$1", tag).Scan(&tagged); err != nil {
				t.Fatal(err)
			}
			t.Fatalf("two controlled recruitment jobs did not block: jobs=%d/%d job_ids=%v controlled_swarm=%s tagged_connections=%d blocked_connections=%d locker_pid=%d target_agent=%s", lastRunning, len(jobIDs), jobIDs, room["id"], tagged, lastBlocked, lockerPID, targetAgent)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Commit only after both default slots are demonstrably blocked, so an
	// earlier repair cannot accidentally produce the test's opportunities. The
	// candidate is committed first and the source second: its cause-specific
	// scan can run immediately, so the source must see an already committed peer.
	code, candidate := a.call("POST", "/v1/intents", "fair-candidate", input)
	if code != 201 {
		t.Fatal(code, candidate)
	}
	code, source := b.call("POST", "/v1/intents", "fair-source", input)
	if code != 201 {
		t.Fatal(code, source)
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		code, page := b.call("GET", "/v1/intents/"+source["id"].(string)+"/opportunities", "", nil)
		if code != 200 {
			t.Fatal(code, page)
		}
		if len(page["items"].([]any)) == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("unrelated blocked recruitment consumed discovery capacity")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestLegacyDefaultDiscoveryJobPreservesWatcherAndRecruitment(t *testing.T) {
	s, db := fixture(t)
	a, b := client(t, s), client(t, s)
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Legacy wakeup " + NewID()
	code, first := a.call("POST", "/v1/intents", "legacy-before", input)
	if code != 201 {
		t.Fatal(code, first)
	}
	ctx := context.Background()
	if err := s.ScanDueIntentions(ctx); err != nil {
		t.Fatal(err)
	}
	code, second := b.call("POST", "/v1/intents", "legacy-candidate", input)
	if code != 201 {
		t.Fatal(code, second)
	}
	organizer := collaborationClient(t, s)
	// Seed the full daily cap explicitly; retained accounts may expire.
	for range recruitmentDailyLimit {
		collaborationClient(t, s)
	}
	code, room := organizer.call("POST", "/v1/swarms", "legacy-role", map[string]any{"objective": "Legacy recruitment", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "builder", "schema_family": "source_catalog_v1", "description": "Build catalog"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	queue, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	args := scanArgs{second["id"].(string)}
	inserted, err := queue.InsertTx(ctx, tx, args, &river.InsertOpts{Queue: river.QueueDefault, MaxAttempts: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// Persist a release-shaped legacy job, then traverse the registered worker's
	// real domain entrypoint without another repair masking its missing effects.
	worker := scanWorker{service: s}
	if err = worker.Work(ctx, &river.Job[scanArgs]{JobRow: inserted.Job, Args: args}); err != nil {
		t.Fatal(err)
	}
	code, page := a.call("GET", "/v1/intents/"+first["id"].(string)+"/opportunities", "", nil)
	if code != 200 || len(page["items"].([]any)) != 1 {
		t.Fatalf("legacy job lost existing watch: %d %v", code, page)
	}
	// A bounded repair must emit five offers, not promise a particular random
	// recipient is among them. The chosen-recipient read/admission lifecycle
	// is proved separately through its explicitly seeded checked offer.
	var offers, events, inbox int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM undergrid.opportunities WHERE source_swarm_id=$1 AND kind='builder_recruitment'`, room["id"]).Scan(&offers); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM undergrid.events e JOIN undergrid.opportunities o ON o.id=e.aggregate_id WHERE o.source_swarm_id=$1 AND e.type='opportunity.available'`, room["id"]).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM undergrid.inbox i JOIN undergrid.events e ON e.id=i.event_id JOIN undergrid.opportunities o ON o.id=e.aggregate_id WHERE o.source_swarm_id=$1 AND e.type='opportunity.available'`, room["id"]).Scan(&inbox); err != nil {
		t.Fatal(err)
	}
	if offers != 5 || events != 5 || inbox != 5 {
		t.Fatalf("legacy job lost bounded recruitment repair: offers=%d events=%d inbox=%d", offers, events, inbox)
	}
}

func TestFailedDiscoveryRefreshDoesNotAbsorbNewCommit(t *testing.T) {
	assertFailedRefreshLeavesImmediateWakeup(t)
}

func TestDiscoveryRefreshCoalescesWithAvailableRetries(t *testing.T) {
	_, db := fixture(t)
	queue, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	// Model previously promoted retries without changing or deleting backlog.
	for range 3 {
		if _, err = queue.InsertTx(context.Background(), tx, scanArgs{"refresh"}, &river.InsertOpts{Queue: discoveryRepairQueue}); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFailedRefreshLeavesImmediateWakeup(t)
}

func assertFailedRefreshLeavesImmediateWakeup(t *testing.T) {
	t.Helper()
	s, db := fixture(t)
	a, b := client(t, s), client(t, s)
	input := intentFixture()
	code, first := a.call("POST", "/v1/intents", "before-failed-refresh", input)
	if code != 201 {
		t.Fatal(code, first)
	}
	var oldRefresh int64
	if err := db.QueryRow(context.Background(), "SELECT id FROM undergrid_queue.river_job WHERE queue=$1 AND args->>'cause_id'='refresh' AND state='available' ORDER BY id LIMIT 1", discoveryRepairQueue).Scan(&oldRefresh); err != nil {
		t.Fatal(err)
	}
	// Model a real retriable failure with a later provider-independent retry.
	// A fresh committed candidate must have an immediate wakeup of its own.
	if _, err := db.Exec(context.Background(), "UPDATE undergrid_queue.river_job SET state='retryable',attempt=1,scheduled_at=now()+interval '1 hour' WHERE id=$1", oldRefresh); err != nil {
		t.Fatal(err)
	}
	var queuedBefore int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE kind='intention_scan_v1' AND queue=$1 AND args->>'cause_id'='refresh' AND state='available'", discoveryRepairQueue).Scan(&queuedBefore); err != nil {
		t.Fatal(err)
	}
	code, second := b.call("POST", "/v1/intents", "after-failed-refresh", input)
	if code != 201 {
		t.Fatal(code, second)
	}
	var available int
	// Previously retried jobs can already be available in a retained database.
	// A queued global refresh covers the new commit; otherwise exactly one must
	// be inserted. Neither path may coalesce into the delayed failed row.
	expected := max(1, queuedBefore)
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE kind='intention_scan_v1' AND queue=$1 AND args->>'cause_id'='refresh' AND state='available'", discoveryRepairQueue).Scan(&available); err != nil || available != expected {
		t.Fatalf("failed delayed sweep absorbed or duplicated wakeup: before=%d available=%d expected=%d err=%v", queuedBefore, available, expected, err)
	}
	var failedUnchanged bool
	if err := db.QueryRow(context.Background(), "SELECT state='retryable' AND attempt=1 AND scheduled_at>clock_timestamp()+interval '30 minutes' FROM undergrid_queue.river_job WHERE id=$1", oldRefresh).Scan(&failedUnchanged); err != nil || !failedUnchanged {
		t.Fatalf("new commit changed delayed failure: unchanged=%v err=%v", failedUnchanged, err)
	}
	var foreground int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE kind='intention_scan_v1' AND queue=$1 AND args->>'cause_id'=$2 AND state='available'", discoveryQueue, second["id"]).Scan(&foreground); err != nil || foreground != 1 {
		t.Fatalf("committed intention lost its foreground job: count=%d err=%v", foreground, err)
	}
	t.Logf("queued refreshes before=%d after=%d; delayed failure unchanged; one committed foreground job", queuedBefore, available)
}

func TestCommittedDiscoveryRefreshesExistingWatcher(t *testing.T) {
	s, db := fixture(t)
	a, b := client(t, s), client(t, s)
	blockedWatcher := client(t, s)
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Later candidate " + NewID()
	code, first := a.call("POST", "/v1/intents", "watch-before-candidate", input)
	if code != 201 {
		t.Fatal(code, first)
	}
	privateInput := input
	privateInput.Visibility = "private"
	code, privateWatch := blockedWatcher.call("POST", "/v1/intents", "private-watch-before-candidate", privateInput)
	if code != 201 {
		t.Fatal(code, privateWatch)
	}
	// The retained watcher must not depend on the repair lane being available.
	var oldPause *time.Time
	err := db.QueryRow(context.Background(), "SELECT paused_at FROM undergrid_queue.river_queue WHERE name=$1", discoveryRepairQueue).Scan(&oldPause)
	if err != nil && err != pgx.ErrNoRows {
		t.Fatal(err)
	}
	if _, err = db.Exec(context.Background(), "INSERT INTO undergrid_queue.river_queue(name,paused_at) VALUES($1,now()) ON CONFLICT(name) DO UPDATE SET paused_at=excluded.paused_at", discoveryRepairQueue); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec(context.Background(), "UPDATE undergrid_queue.river_queue SET paused_at=$2 WHERE name=$1", discoveryRepairQueue, oldPause); err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- RunWorker(ctx, s.dsn, "development") }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(12 * time.Second):
			t.Error("worker did not stop")
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var scanned bool
		if err := db.QueryRow(context.Background(), "SELECT count(*)=2 FROM undergrid.intents WHERE id=ANY($1::uuid[]) AND last_scan IS NOT NULL", []string{first["id"].(string), privateWatch["id"].(string)}).Scan(&scanned); err != nil {
			t.Fatal(err)
		}
		if scanned {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first watch did not scan before candidate arrival")
		}
		time.Sleep(20 * time.Millisecond)
	}
	code, before := a.call("GET", "/v1/intents/"+first["id"].(string)+"/opportunities", "", nil)
	if code != 200 || len(before["items"].([]any)) != 0 {
		t.Fatal("private watch became a public candidate", code, before)
	}
	locker, err := pgx.Connect(context.Background(), s.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer locker.Close(context.Background())
	held, err := locker.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Rollback(context.Background())
	if _, err = held.Exec(context.Background(), "SELECT id FROM undergrid.principals WHERE id=(SELECT principal_id FROM undergrid.intents WHERE id=$1) FOR UPDATE", privateWatch["id"]); err != nil {
		t.Fatal(err)
	}
	code, second := b.call("POST", "/v1/intents", "candidate-after-scan", input)
	if code != 201 {
		t.Fatal(code, second)
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		code, page := a.call("GET", "/v1/intents/"+first["id"].(string)+"/opportunities", "", nil)
		if code != 200 {
			t.Fatal(code, page)
		}
		for _, item := range page["items"].([]any) {
			opportunity := item.(map[string]any)
			if opportunity["candidate_intent_id"] == second["id"] {
				var opportunities, events, inbox int
				if err := db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.opportunities WHERE source_intent_id=$1 AND candidate_intent_id=$2`, first["id"], second["id"]).Scan(&opportunities); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='opportunity.available'`, opportunity["id"]).Scan(&events); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.inbox i JOIN undergrid.events e ON e.id=i.event_id WHERE e.aggregate_id=$1 AND e.type='opportunity.available'`, opportunity["id"]).Scan(&inbox); err != nil {
					t.Fatal(err)
				}
				if opportunities != 1 || events != 1 || inbox != 1 {
					t.Fatalf("duplicate/missing committed effects: opportunities=%d events=%d inbox=%d", opportunities, events, inbox)
				}
				goto WatcherMatched
			}
		}

		if time.Now().After(deadline) {
			t.Fatal("committed refresh lost the previously scanned watcher")
		}
		time.Sleep(20 * time.Millisecond)
	}
WatcherMatched:
	deadline = time.Now().Add(5 * time.Second)
	for {
		var queued int
		if err := db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid_queue.river_job WHERE kind='intention_watch_refresh_v1' AND args->>'candidate_id'=$1 AND args->>'watcher_id'=$2 AND state IN('available','running','scheduled','retryable')`, second["id"], privateWatch["id"]).Scan(&queued); err != nil {
			t.Fatal(err)
		}
		if queued > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("locked watcher did not retain durable refresh work")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err = held.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		code, page := blockedWatcher.call("GET", "/v1/intents/"+privateWatch["id"].(string)+"/opportunities", "", nil)
		if code != 200 {
			t.Fatal(code, page)
		}
		matched := false
		for _, item := range page["items"].([]any) {
			if item.(map[string]any)["candidate_intent_id"] == second["id"] {
				matched = true
			}
		}
		var completed bool
		if err := db.QueryRow(context.Background(), `SELECT EXISTS(SELECT FROM undergrid_queue.river_job WHERE kind='intention_watch_refresh_v1' AND args->>'candidate_id'=$1 AND args->>'watcher_id'=$2 AND state='completed')`, second["id"], privateWatch["id"]).Scan(&completed); err != nil {
			t.Fatal(err)
		}
		if matched && completed {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("locked watcher refresh did not recover after lock release")
		}
		time.Sleep(20 * time.Millisecond)
	}

}

// Exercise a full durable continuation independently of whichever UUIDs a
// retained fixture happened to allocate. Synthetic rows are test-only watches,
// not independent owners or provider observations.
func TestWatcherFanoutContinuesBeyondBoundedPage(t *testing.T) {
	s, db := fixture(t)
	a, b := client(t, s), client(t, s)
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Fanout boundary " + NewID()
	code, source := a.call("POST", "/v1/intents", "fanout-source", input)
	if code != 201 {
		t.Fatal(code, source)
	}
	code, candidate := b.call("POST", "/v1/intents", "fanout-candidate", input)
	if code != 201 {
		t.Fatal(code, candidate)
	}
	ids := make([]string, 0, 258)
	for range 257 {
		ids = append(ids, "ffffffff-ffff-4fff-8fff-"+NewID()[24:])
	}
	target := "ffffffff-ffff-4fff-bfff-" + NewID()[24:]
	ids = append(ids, target)
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO undergrid.intents(id,principal_id,agent_id,state,visibility,expires_at,watch_enabled)
	 SELECT u.id,i.principal_id,i.agent_id,i.state,i.visibility,i.expires_at,i.watch_enabled FROM undergrid.intents i CROSS JOIN unnest($1::uuid[]) u(id) WHERE i.id=$2`, ids, source["id"])
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO undergrid.intent_revisions(intent_id,revision,document,public_projection,content_hash)
	 SELECT u.id,1,r.document,r.public_projection,r.content_hash FROM undergrid.intent_revisions r CROSS JOIN unnest($1::uuid[]) u(id) WHERE r.intent_id=$2 AND r.revision=1`, ids, source["id"])
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		// Retain fixture history while making these synthetic watches inactive.
		if _, err := db.Exec(ctx, "UPDATE undergrid.intents SET state='withdrawn' WHERE id=ANY($1::uuid[])", ids); err != nil {
			t.Error(err)
		}
	}()
	args := watchRefreshArgs{CandidateID: candidate["id"].(string), AfterID: "ffffffff-ffff-4fff-8000-000000000000"}
	if err = s.enqueueCompatibleWatchers(ctx, args); err != nil {
		t.Fatal(err)
	}
	var targetJobs int
	countTarget := func() int {
		t.Helper()
		if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid_queue.river_job WHERE kind='intention_watch_refresh_v1' AND args->>'candidate_id'=$1 AND args->>'watcher_id'=$2", args.CandidateID, target).Scan(&targetJobs); err != nil {
			t.Fatal(err)
		}
		return targetJobs
	}
	if countTarget() != 0 {
		t.Fatal("last watcher incorrectly appeared in the bounded first page")
	}
	var after string
	if err = db.QueryRow(ctx, "SELECT args->>'after_id' FROM undergrid_queue.river_job WHERE kind='intention_watch_refresh_v1' AND queue=$1 AND args->>'candidate_id'=$2 AND args->>'after_id'>$3 ORDER BY id DESC LIMIT 1", watchFanoutQueue, args.CandidateID, args.AfterID).Scan(&after); err != nil {
		t.Fatal("missing durable continuation", err)
	}
	args.AfterID = after
	if err = s.enqueueCompatibleWatchers(ctx, args); err != nil {
		t.Fatal(err)
	}
	if countTarget() != 1 {
		t.Fatalf("continuation lost/duplicated final compatible watcher: %d", targetJobs)
	}
}
