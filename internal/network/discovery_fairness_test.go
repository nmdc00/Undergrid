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
	organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
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
	code, page = builder.call("GET", "/v1/opportunities?kind=builder_recruitment", "", nil)
	if code != 200 {
		t.Fatal(code, page)
	}
	found := false
	for _, raw := range page["items"].([]any) {
		if raw.(map[string]any)["swarm_id"] == room["id"] {
			found = true
		}
	}
	if !found {
		t.Fatal("legacy job lost recruitment repair", page)
	}
}

func TestFailedDiscoveryRefreshDoesNotAbsorbNewCommit(t *testing.T) {
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
	code, second := b.call("POST", "/v1/intents", "after-failed-refresh", input)
	if code != 201 {
		t.Fatal(code, second)
	}
	var available int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE queue=$1 AND args->>'cause_id'='refresh' AND state='available'", discoveryRepairQueue).Scan(&available); err != nil || available != 1 {
		t.Fatalf("failed delayed sweep absorbed the new wakeup: available=%d err=%v", available, err)
	}
}

func TestCommittedDiscoveryRefreshesExistingWatcher(t *testing.T) {
	s, db := fixture(t)
	a, b := client(t, s), client(t, s)
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Later candidate " + NewID()
	code, first := a.call("POST", "/v1/intents", "watch-before-candidate", input)
	if code != 201 {
		t.Fatal(code, first)
	}
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
		if err := db.QueryRow(context.Background(), "SELECT last_scan IS NOT NULL FROM undergrid.intents WHERE id=$1", first["id"]).Scan(&scanned); err != nil {
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
		if len(page["items"].([]any)) == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("committed refresh lost the previously scanned watcher")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
