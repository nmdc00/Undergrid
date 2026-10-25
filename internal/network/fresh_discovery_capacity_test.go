package network

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// Both a pre-isolation backlog and an accepted fresh-lane backlog surviving a
// restart must leave capacity for newly committed exact discovery. These are
// actual runtime effects; five-second bounds are unchanged from the regression.
func TestFreshDiscoveryCapacitySurvivesRetainedAndRestartBacklogs(t *testing.T) {
	s, db, worker, _ := externalNotificationFixture(t)
	workerDSN := worker.dsn
	t.Setenv("UNDERGRID_SOURCES_FILE", "")
	oldOwner := client(t, s)
	oldInput := intentFixture()
	oldInput.Visibility = "redacted_public"
	oldInput.PublicSummary = "Retained backlog " + NewID()
	code, old := oldOwner.call("POST", "/v1/intents", "retained-source", oldInput)
	if code != 201 {
		t.Fatal(code, old)
	}
	oldID := old["id"].(string)
	t.Cleanup(func() {
		code, result := oldOwner.versioned("PATCH", "/v1/intents/"+oldID, "retire-backlog", old["etag"].(string), map[string]any{"state": "withdrawn"})
		if code != 200 {
			t.Errorf("retire %d %v", code, result)
		}
	})
	locker, err := pgx.Connect(context.Background(), workerDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer locker.Close(context.Background())
	for _, dsn := range []string{s.dsn, workerDSN} {
		connection, err := pgx.Connect(context.Background(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		var database, session string
		var superuser, bypass bool
		err = connection.QueryRow(context.Background(), "SELECT current_database(),session_user,rolsuper,rolbypassrls FROM pg_roles WHERE rolname=session_user").Scan(&database, &session, &superuser, &bypass)
		connection.Close(context.Background())
		if err != nil || session != connection.Config().User || superuser || bypass {
			t.Fatalf("expected actual restricted runtime LOGIN: session=%s super=%v bypass=%v err=%v", session, superuser, bypass, err)
		}
		t.Logf("Actual capacity fixture database=%s restricted session_user=%s", database, session)
	}
	held, err := locker.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Rollback(context.Background())
	if _, err = held.Exec(context.Background(), "SET LOCAL ROLE undergrid_worker"); err != nil {
		t.Fatal(err)
	}
	if _, err = held.Exec(context.Background(), "SELECT id FROM undergrid.principals WHERE id=(SELECT principal_id FROM undergrid.intents WHERE id=$1) FOR UPDATE", oldID); err != nil {
		t.Fatal(err)
	}
	queue, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		t.Fatal(err)
	}
	retainedGroups := [][]int64{}
	for _, phase := range []struct{ name, scan, fanout string }{{"legacy", "intention_discovery", "intention_watch_fanout"}, {"restart", "intention_fresh_discovery", "intention_fresh_watch_fanout"}} {
		t.Run(phase.name, func(t *testing.T) {
			tx, err := db.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			ids := []int64{}
			for range 8 {
				inserted, e := queue.InsertTx(context.Background(), tx, scanArgs{CauseID: oldID}, &river.InsertOpts{Queue: phase.scan, Priority: 1, MaxAttempts: 10})
				if e != nil {
					t.Fatal(e)
				}
				ids = append(ids, inserted.Job.ID)
			}
			// This same-priority, accepted fanout work is deliberately ahead of the
			// next candidate. It must survive, rather than be discarded for the test.
			for range 320 {
				inserted, e := queue.InsertTx(context.Background(), tx, watchRefreshArgs{CandidateID: oldID}, &river.InsertOpts{Queue: phase.fanout, Priority: 2, MaxAttempts: 10})
				if e != nil {
					t.Fatal(e)
				}
				ids = append(ids, inserted.Job.ID)
			}
			if _, err = tx.Exec(context.Background(), "UPDATE undergrid_queue.river_job SET created_at=clock_timestamp()-interval '2 minutes',scheduled_at=clock_timestamp()-interval '2 minutes' WHERE id=ANY($1::bigint[])", ids); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(context.Background()); err != nil {
				t.Fatal(err)
			}
			retainedGroups = append(retainedGroups, ids)
			owner, candidateOwner := client(t, s), client(t, s)
			input := intentFixture()
			input.Visibility = "redacted_public"
			input.PublicSummary = "Fresh exact recovery " + NewID()
			code, source := owner.call("POST", "/v1/intents", "fresh-watch", input)
			if code != 201 {
				t.Fatal(code, source)
			}
			sourceID := source["id"].(string)
			t.Cleanup(func() {
				code, out := owner.versioned("PATCH", "/v1/intents/"+sourceID, "retire-fresh-watch", source["etag"].(string), map[string]any{"state": "withdrawn"})
				if code != 200 {
					t.Errorf("retire %d %v", code, out)
				}
			})
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- RunWorker(ctx, workerDSN, "development") }()
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
				if err = db.QueryRow(context.Background(), "SELECT last_scan IS NOT NULL FROM undergrid.intents WHERE id=$1", sourceID).Scan(&scanned); err != nil {
					t.Fatal(err)
				}
				if scanned {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("fresh initial scan starved behind retained work")
				}
				time.Sleep(20 * time.Millisecond)
			}
			code, candidate := candidateOwner.call("POST", "/v1/intents", "fresh-candidate", input)
			if code != 201 {
				t.Fatal(code, candidate)
			}
			candidateID := candidate["id"].(string)
			t.Cleanup(func() {
				code, out := candidateOwner.versioned("PATCH", "/v1/intents/"+candidateID, "retire-fresh-candidate", candidate["etag"].(string), map[string]any{"state": "withdrawn"})
				if code != 200 {
					t.Errorf("retire %d %v", code, out)
				}
			})
			deadline = time.Now().Add(5 * time.Second)
			for {
				var opportunities, events, inbox int
				err = db.QueryRow(context.Background(), `SELECT count(*),count(e.id),count(b.event_id) FROM undergrid.opportunities o LEFT JOIN undergrid.events e ON e.aggregate_id=o.id AND e.type='opportunity.available' LEFT JOIN undergrid.inbox b ON b.event_id=e.id WHERE o.source_intent_id=$1 AND o.candidate_intent_id=$2`, sourceID, candidateID).Scan(&opportunities, &events, &inbox)
				if err != nil {
					t.Fatal(err)
				}
				if opportunities == 1 && events == 1 && inbox == 1 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("fresh fanout starved or duplicated: %d/%d/%d", opportunities, events, inbox)
				}
				time.Sleep(20 * time.Millisecond)
			}
			var preserved int
			if err = db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid_queue.river_job WHERE id=ANY($1::bigint[]) AND max_attempts=10 AND ((kind='intention_scan_v1' AND args=jsonb_build_object('cause_id',$2::text) AND priority=1) OR (kind='intention_watch_refresh_v1' AND args=jsonb_build_object('candidate_id',$2::text) AND priority=2))`, ids, oldID).Scan(&preserved); err != nil || preserved != len(ids) {
				t.Fatal("retained job identity lost", preserved, err)
			}
		})
	}
	// Normal worker cancellation can leave an attempted job with future retry
	// backoff. Observe still-unattempted work eligible now instead of resetting
	// any accepted schedule, attempt or error to manufacture bounded recovery.
	eligibleRetained := []int64{}
	for _, ids := range retainedGroups {
		var id int64
		if err = db.QueryRow(context.Background(), "SELECT id FROM undergrid_queue.river_job WHERE id=ANY($1::bigint[]) AND attempt=0 AND state='available' AND scheduled_at<=clock_timestamp() ORDER BY id LIMIT 1", ids[:8]).Scan(&id); err != nil {
			t.Fatalf("expected known eligible unattempted retained scan: %v", err)
		}
		eligibleRetained = append(eligibleRetained, id)
	}
	if err = held.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- RunWorker(ctx, workerDSN, "development") }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(12 * time.Second):
			t.Error("recovery worker did not stop")
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var completed int
		if err = db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE id=ANY($1::bigint[]) AND state='completed'", eligibleRetained).Scan(&completed); err != nil {
			t.Fatal(err)
		}
		if completed == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("retained direct work failed to recover after lock release", completed)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
