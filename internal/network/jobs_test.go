package network

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func testWorkerDSN(t *testing.T, dsn, applicationName string) string {
	t.Helper()
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		query := u.Query()
		query.Set("application_name", applicationName)
		u.RawQuery = query.Encode()
		return u.String()
	}
	return dsn + " application_name=" + applicationName
}

func TestCommittedIntentionRunsThroughDurableWorker(t *testing.T) {
	s, db := fixture(t)
	a, b := client(t, s), client(t, s)
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Worker catalog " + NewID()
	code, first := a.call("POST", "/v1/intents", "queued-first", input)
	if code != 201 {
		t.Fatal(code, first)
	}
	code, second := b.call("POST", "/v1/intents", "queued-second", input)
	if code != 201 {
		t.Fatal(code, second)
	}
	var queued int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE args->>'cause_id'=$1", first["id"]).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("transactional job missing %d %v", queued, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() { stopped <- RunWorker(ctx, s.dsn, "development") }()
	defer func() {
		cancel()
		select {
		case err := <-stopped:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(12 * time.Second):
			t.Error("worker did not stop")
		}
	}()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		code, page := a.call("GET", "/v1/intents/"+first["id"].(string)+"/opportunities", "", nil)
		if code != 200 {
			t.Fatal(code, page)
		}
		if len(page["items"].([]any)) == 1 {
			return
		}
		select {
		case err := <-stopped:
			t.Fatalf("worker exited: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatal("committed job did not produce opportunity")
}

// Hold a real domain transaction blocked throughout shutdown. A completed
// opportunity alone does not prove that the worker's other jobs have drained.
func TestWorkerShutdownCancelsBlockedTransactionAndPreservesQueuedWork(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	code, created := a.call("POST", "/v1/intents", "shutdown-intent", intentFixture())
	if code != 201 {
		t.Fatal(code, created)
	}
	id := created["id"].(string)
	observer, err := pgx.Connect(context.Background(), s.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close(context.Background())
	lock, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(context.Background())
	if _, err = lock.Exec(context.Background(), "LOCK TABLE undergrid.principals IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan error, 1)
	started := time.Now().UTC()
	workerName := "shutdown_test_" + NewID()
	workerDSN := testWorkerDSN(t, s.dsn, workerName)
	go func() { stopped <- RunWorker(ctx, workerDSN, "development") }()
	deadline := time.Now().Add(10 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		if err = observer.QueryRow(context.Background(), "SELECT EXISTS(SELECT FROM pg_stat_activity WHERE datname=current_database() AND application_name=$1 AND wait_event_type='Lock' AND $2::integer=ANY(pg_blocking_pids(pid)))", workerName, db.PgConn().PID()).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case err := <-stopped:
			t.Fatalf("worker exited before blocked work: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
	}
	// RLS may read the locked principal table before the explicit domain query.
	// Prove the tagged worker waits on this exact lock owner, not SQL spelling.
	if !blocked {
		t.Fatal("no worker transaction reached the held principal lock")
	}
	rows, err := observer.Query(context.Background(), "SELECT id FROM undergrid_queue.river_job WHERE state='running' AND attempted_at >= $1", started)
	if err != nil {
		t.Fatal(err)
	}
	var running []int64
	for rows.Next() {
		var job int64
		if err = rows.Scan(&job); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		running = append(running, job)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(running) == 0 {
		t.Fatalf("blocked work was not durably running: %v %v", running, err)
	}
	cancel()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("blocked worker shutdown: %v", err)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("worker failed to cancel blocked database work before shutdown bound")
	}
	var unfinished int
	if err = observer.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE id=ANY($1) AND state='running'", running).Scan(&unfinished); err != nil || unfinished != 0 {
		t.Fatalf("shutdown left running jobs without retry state: %d %v", unfinished, err)
	}
	var state string
	var version int64
	if err = observer.QueryRow(context.Background(), "SELECT state,version FROM undergrid.intents WHERE id=$1", id).Scan(&state, &version); err != nil {
		t.Fatal(err)
	}
	if state != "active" || version != 1 {
		t.Fatalf("acknowledged intention changed during abort: %s v%d", state, version)
	}
	var retained int
	if err = observer.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE args->>'cause_id'=$1 AND state NOT IN ('cancelled','discarded')", id).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("committed work lost after shutdown: %d %v", retained, err)
	}
}
