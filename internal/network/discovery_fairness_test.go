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
	organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
	code, room := organizer.call("POST", "/v1/swarms", "blocked-recruit-room", map[string]any{"objective": "Recruit a catalog builder", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "builder", "schema_family": "source_catalog_v1", "description": "Build catalog"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	_, target := builder.call("GET", "/v1/me", "", nil)
	a, b := client(t, s), client(t, s)
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Fair discovery " + NewID()
	ctx := context.Background()
	lock, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	var lockerPID int
	if err = lock.QueryRow(ctx, "SELECT pg_backend_pid() FROM undergrid.principals WHERE id=$1 FOR UPDATE", target["principal_id"]).Scan(&lockerPID); err != nil {
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
	go func() { done <- RunWorker(workerCtx, s.dsn+" application_name="+tag, "development") }()
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
	for {
		var blocked int
		if err = observer.QueryRow(ctx, `WITH RECURSIVE blocked AS (
 SELECT pid FROM pg_stat_activity WHERE $2=ANY(pg_blocking_pids(pid))
 UNION SELECT a.pid FROM pg_stat_activity a JOIN blocked b ON b.pid=ANY(pg_blocking_pids(a.pid))
) SELECT count(*) FROM pg_stat_activity a JOIN blocked b ON b.pid=a.pid WHERE a.application_name=$1`, tag, lockerPID).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked >= 2 {
			var recruiting int
			if err = observer.QueryRow(ctx, "SELECT count(*) FROM undergrid_queue.river_job WHERE kind='builder_recruitment_v1' AND state='running'").Scan(&recruiting); err != nil {
				t.Fatal(err)
			}
			if recruiting >= 2 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("two recruitment jobs did not reach the controlled principal lock")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Commit only after both default slots are demonstrably blocked, so an
	// earlier repair cannot accidentally produce the test's opportunities.
	code, first := a.call("POST", "/v1/intents", "fair-first", input)
	if code != 201 {
		t.Fatal(code, first)
	}
	code, second := b.call("POST", "/v1/intents", "fair-second", input)
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
