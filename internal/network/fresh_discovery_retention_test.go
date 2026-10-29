package network

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func capacityWorkerPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE undergrid_worker")
		return err
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func capacityJobSnapshot(t *testing.T, db *pgx.Conn, ids []int64) map[int64]string {
	t.Helper()
	rows, err := db.Query(context.Background(), "SELECT id,(to_jsonb(j)-'queue')::text FROM undergrid_queue.river_job j WHERE id=ANY($1::bigint[])", ids)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var value string
		if err = rows.Scan(&id, &value); err != nil {
			t.Fatal(err)
		}
		out[id] = value
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(out) != len(ids) {
		t.Fatalf("accepted rows disappeared: got %d want %d", len(out), len(ids))
	}
	return out
}

// No consumers run in this oracle. Every persisted job field except queue must
// remain byte-for-byte equivalent, including future retry schedules and errors.
func TestDiscoveryAgeoutIsBoundedSkipsLocksAndPreservesEveryOtherJobField(t *testing.T) {
	_, db, worker, owner := externalNotificationFixture(t)
	ctx := context.Background()
	input := intentFixture()
	code, intention := owner.call("POST", "/v1/intents", "ageout-intention", input)
	if code != 201 {
		t.Fatal(code, intention)
	}
	id := intention["id"].(string)
	code, result := owner.versioned("PATCH", "/v1/intents/"+id, "retire-ageout-intention", intention["etag"].(string), map[string]any{"state": "withdrawn"})
	if code != 200 {
		t.Fatal(code, result)
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
	eligible, excluded := []int64{}, []int64{}
	originalQueues := map[int64]string{}
	excludedQueues := map[int64]string{}
	targetQueues := map[int64]string{}
	lanes := []struct {
		fresh, retained string
		args            river.JobArgs
	}{
		{freshDiscoveryQueue, discoveryQueue, scanArgs{CauseID: id}},
		{freshWatchFanoutQueue, watchFanoutQueue, watchRefreshArgs{CandidateID: id}},
		{freshWatchRefreshQueue, watchRefreshQueue, watchRefreshArgs{CandidateID: id, WatcherID: id}},
	}
	for i := 0; i < 300; i++ {
		lane := lanes[i%len(lanes)]
		inserted, err := queue.InsertTx(ctx, tx, lane.args, &river.InsertOpts{Queue: lane.fresh, Priority: 3, MaxAttempts: 9, Metadata: []byte(`{"fixture":"queue-preservation"}`)})
		if err != nil {
			t.Fatal(err)
		}
		jobID := inserted.Job.ID
		state := []string{"available", "retryable", "scheduled"}[i%3]
		if _, err = tx.Exec(ctx, `UPDATE undergrid_queue.river_job SET created_at=clock_timestamp()-interval '2 minutes',state=$2::undergrid_queue.river_job_state,scheduled_at=CASE WHEN $2='available' THEN clock_timestamp()-interval '1 minute' ELSE clock_timestamp()+interval '2 hours' END,attempt=CASE WHEN $2='retryable' THEN 2 ELSE 0 END,errors=ARRAY[jsonb_build_object('at',clock_timestamp(),'attempt',1,'error','retained fixture evidence','trace','')],tags=ARRAY['retained'] WHERE id=$1`, jobID, state); err != nil {
			t.Fatal(err)
		}
		eligible = append(eligible, jobID)
		originalQueues[jobID] = lane.fresh
		targetQueues[jobID] = lane.retained
	}
	for _, exclusion := range []string{"running", "pending", "completed", "cancelled", "discarded", "fresh", "unique", "wrong_kind", "other_queue"} {
		inserted, err := queue.InsertTx(ctx, tx, scanArgs{CauseID: id}, &river.InsertOpts{Queue: freshDiscoveryQueue, Priority: 2, MaxAttempts: 10})
		if err != nil {
			t.Fatal(err)
		}
		jobID := inserted.Job.ID
		if _, err = tx.Exec(ctx, "UPDATE undergrid_queue.river_job SET created_at=clock_timestamp()-interval '2 minutes' WHERE id=$1", jobID); err != nil {
			t.Fatal(err)
		}
		switch exclusion {
		case "running", "pending", "completed", "cancelled", "discarded":
			_, err = tx.Exec(ctx, `UPDATE undergrid_queue.river_job SET state=$2::undergrid_queue.river_job_state,finalized_at=CASE WHEN $2 IN('completed','cancelled','discarded') THEN clock_timestamp() ELSE NULL END WHERE id=$1`, jobID, exclusion)
		case "fresh":
			_, err = tx.Exec(ctx, "UPDATE undergrid_queue.river_job SET created_at=clock_timestamp() WHERE id=$1", jobID)
		case "unique":
			_, err = tx.Exec(ctx, "UPDATE undergrid_queue.river_job SET unique_key=decode(replace($2,'-',''),'hex') WHERE id=$1", jobID, NewID())
		case "wrong_kind":
			_, err = tx.Exec(ctx, "UPDATE undergrid_queue.river_job SET kind='builder_recruitment_v1' WHERE id=$1", jobID)
		case "other_queue":
			_, err = tx.Exec(ctx, "UPDATE undergrid_queue.river_job SET queue=$2 WHERE id=$1", jobID, discoveryQueue)
		}
		if err != nil {
			t.Fatal(err)
		}
		excluded = append(excluded, jobID)
		excludedQueues[jobID] = freshDiscoveryQueue
		if exclusion == "other_queue" {
			excludedQueues[jobID] = discoveryQueue
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	all := append(append([]int64{}, eligible...), excluded...)
	before := capacityJobSnapshot(t, db, all)
	held, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Rollback(ctx)
	if _, err = held.Exec(ctx, "SELECT id FROM undergrid_queue.river_job WHERE id=ANY($1::bigint[]) FOR UPDATE", eligible[:3]); err != nil {
		t.Fatal(err)
	}
	pool := capacityWorkerPool(t, worker.dsn)
	for _, want := range []int64{256, 41, 0} {
		got, err := retainAgedDiscovery(ctx, pool)
		if err != nil || got != want {
			t.Fatalf("bounded unlocked ageout: got %d want %d err=%v", got, want, err)
		}
	}
	if err = held.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := retainAgedDiscovery(ctx, pool); err != nil || got != 3 {
		t.Fatalf("released eligible rows did not progress: %d %v", got, err)
	}
	after := capacityJobSnapshot(t, db, all)
	for _, jobID := range all {
		if before[jobID] != after[jobID] {
			t.Fatalf("maintenance changed accepted job %d beyond queue", jobID)
		}
		var actual string
		if err = db.QueryRow(ctx, "SELECT queue FROM undergrid_queue.river_job WHERE id=$1", jobID).Scan(&actual); err != nil {
			t.Fatal(err)
		}
		if want, ok := targetQueues[jobID]; ok {
			if actual != want {
				t.Fatalf("job %d lane=%s want=%s from=%s", jobID, actual, want, originalQueues[jobID])
			}
		} else {
			if actual != excludedQueues[jobID] {
				t.Fatalf("excluded job %d moved to %s", jobID, actual)
			}
		}
	}
}

func TestDiscoveryRuntimeMaintenanceRetainsJobsAcceptedAfterStartup(t *testing.T) {
	_, db, worker, owner := externalNotificationFixture(t)
	t.Setenv("UNDERGRID_SOURCES_FILE", "")
	ctx := context.Background()
	code, intention := owner.call("POST", "/v1/intents", "runtime-maintenance-watch", intentFixture())
	if code != 201 {
		t.Fatal(code, intention)
	}
	id := intention["id"].(string)
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- RunWorker(workerCtx, worker.dsn, "development") }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(12 * time.Second):
			t.Error("maintenance worker did not stop within twelve seconds")
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		var started bool
		if err := db.QueryRow(ctx, "SELECT last_scan IS NOT NULL FROM undergrid.intents WHERE id=$1", id).Scan(&started); err != nil {
			t.Fatal(err)
		}
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not complete initial scan")
		}
		time.Sleep(20 * time.Millisecond)
	}
	held, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Rollback(ctx)
	if _, err = held.Exec(ctx, "SELECT id FROM undergrid.principals WHERE id=(SELECT principal_id FROM undergrid.intents WHERE id=$1) FOR UPDATE", id); err != nil {
		t.Fatal(err)
	}
	insertDB, err := pgx.Connect(ctx, db.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer insertDB.Close(ctx)
	tx, err := insertDB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	queue, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		t.Fatal(err)
	}
	ids := []int64{}
	for range 40 {
		row, err := queue.InsertTx(ctx, tx, scanArgs{CauseID: id}, &river.InsertOpts{Queue: freshDiscoveryQueue, Priority: 1, MaxAttempts: 10})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, row.Job.ID)
	}
	if _, err = tx.Exec(ctx, "UPDATE undergrid_queue.river_job SET created_at=clock_timestamp()-interval '2 minutes' WHERE id=ANY($1::bigint[])", ids); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for {
		var retained, unstartedFresh int
		if err = insertDB.QueryRow(ctx, `SELECT count(*) FILTER(WHERE queue=$2),count(*) FILTER(WHERE queue=$3 AND state<>'running') FROM undergrid_queue.river_job WHERE id=ANY($1::bigint[])`, ids, discoveryQueue, freshDiscoveryQueue).Scan(&retained, &unstartedFresh); err != nil {
			t.Fatal(err)
		}
		if retained >= 38 && unstartedFresh == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("periodic maintenance did not age post-start work: retained=%d unstartedFresh=%d", retained, unstartedFresh)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err = held.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		var completed int
		if err = insertDB.QueryRow(ctx, "SELECT count(*) FROM undergrid_queue.river_job WHERE id=ANY($1::bigint[]) AND state='completed'", ids).Scan(&completed); err != nil {
			t.Fatal(err)
		}
		if completed == len(ids) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("accepted maintenance work did not recover after authority release: %d/%d", completed, len(ids))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Actual worker dispatch reads the persisted River row's queue after ageout.
// Pagination and pair jobs must inherit that scheduling ownership.
func TestDiscoveryFanoutDescendantsFollowActualFreshOrRetainedJobQueue(t *testing.T) {
	s, db, worker, _ := externalNotificationFixture(t)
	ctx := context.Background()
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Capacity page " + NewID()
	candidateOwner := client(t, s)
	code, candidate := candidateOwner.call("POST", "/v1/intents", "fanout-candidate", input)
	if code != 201 {
		t.Fatal(code, candidate)
	}
	candidateID := candidate["id"].(string)
	for range watcherRefreshPageSize + 1 {
		watcher := client(t, s)
		code, out := watcher.call("POST", "/v1/intents", "fanout-watcher", input)
		if code != 201 {
			t.Fatal(code, out)
		}
	}
	pool := capacityWorkerPool(t, worker.dsn)
	queue, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		t.Fatal(err)
	}
	execute := func(id int64) {
		t.Helper()
		row, err := queue.JobGet(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		var args watchRefreshArgs
		if err = json.Unmarshal(row.EncodedArgs, &args); err != nil {
			t.Fatal(err)
		}
		if err = (&watchRefreshWorker{service: worker}).Work(ctx, &river.Job[watchRefreshArgs]{JobRow: row, Args: args}); err != nil {
			t.Fatal(err)
		}
	}
	for _, retained := range []bool{false, true} {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		inserted, err := queue.InsertTx(ctx, tx, watchRefreshArgs{CandidateID: candidateID}, &river.InsertOpts{Queue: freshWatchFanoutQueue, Priority: 2, MaxAttempts: 10})
		if err != nil {
			t.Fatal(err)
		}
		if retained {
			if _, err = tx.Exec(ctx, "UPDATE undergrid_queue.river_job SET created_at=clock_timestamp()-interval '2 minutes' WHERE id=$1", inserted.Job.ID); err != nil {
				t.Fatal(err)
			}
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if retained {
			// The normal HTTP setup has also accepted fresh queue work. Some of
			// it may now be aged and precede this target in a bounded batch, so
			// use the actual startup drain rather than assume one empty batch.
			if err = retainDiscoveryStartup(ctx, pool); err != nil {
				t.Fatal(err)
			}
		}
		// Observe only this dispatch's new descendants. Older fresh-phase pairs
		// may legitimately age into retained ownership during a slow fixture.
		var actualLane string
		if err = db.QueryRow(ctx, "SELECT queue FROM undergrid_queue.river_job WHERE id=$1", inserted.Job.ID).Scan(&actualLane); err != nil {
			t.Fatal(err)
		}
		wantLane := freshWatchFanoutQueue
		if retained {
			wantLane = watchFanoutQueue
		}
		if actualLane != wantLane {
			t.Fatalf("persisted dispatch queue=%s want=%s", actualLane, wantLane)
		}
		execute(inserted.Job.ID)
		pairLane, fanoutLane := freshWatchRefreshQueue, freshWatchFanoutQueue
		if retained {
			pairLane, fanoutLane = watchRefreshQueue, watchFanoutQueue
		}
		var continuation int64
		if err = db.QueryRow(ctx, "SELECT id FROM undergrid_queue.river_job WHERE queue=$1 AND kind='intention_watch_refresh_v1' AND args->>'candidate_id'=$2 AND id>$3 AND COALESCE(args->>'after_id','')<>'' ORDER BY id DESC LIMIT 1", fanoutLane, candidateID, inserted.Job.ID).Scan(&continuation); err != nil {
			t.Fatal("missing inherited continuation", err)
		}
		execute(continuation)
		var pairs, distinctWatchers int
		if err = db.QueryRow(ctx, "SELECT count(*),count(DISTINCT args->>'watcher_id') FROM undergrid_queue.river_job WHERE queue=$1 AND kind='intention_watch_refresh_v1' AND args->>'candidate_id'=$2 AND id>$3 AND COALESCE(args->>'watcher_id','')<>''", pairLane, candidateID, inserted.Job.ID).Scan(&pairs, &distinctWatchers); err != nil {
			t.Fatal(err)
		}
		if pairs != watcherRefreshPageSize+1 || distinctWatchers != pairs {
			t.Fatalf("descendant routing retained=%v: %d pairs/%d distinct", retained, pairs, distinctWatchers)
		}
	}
}
