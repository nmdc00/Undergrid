package network

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOwnerTaskExecutionFanoutFailureRollsBackAndRecoversExactly(t *testing.T) {
	f, task := acceptedOwnerExecutionTask(t)
	ctx := context.Background()
	name := "owner_execution_fault_" + strings.ReplaceAll(NewID(), "-", "")
	quoted := pgx.Identifier{"undergrid", name}.Sanitize()
	trigger := pgx.Identifier{name}.Sanitize()
	sql := "CREATE FUNCTION " + quoted + "() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='event_fanout_v1' AND EXISTS(SELECT 1 FROM undergrid.events e WHERE e.id::text=NEW.args->>'event_id' AND e.aggregate_id::text='" + task["id"].(string) + "' AND e.type='task.progress_recorded') THEN RAISE EXCEPTION 'fixture execution fanout failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER " + trigger + " BEFORE INSERT ON undergrid_queue.river_job FOR EACH ROW EXECUTE FUNCTION " + quoted + "()"
	if _, err := f.db.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	defer f.db.Exec(ctx, "DROP TRIGGER IF EXISTS "+trigger+" ON undergrid_queue.river_job; DROP FUNCTION IF EXISTS "+quoted+"()")
	path := "/v1/owner/agents/" + f.agent + "/tasks/" + task["id"].(string) + "/progress"
	key := NewID()
	body := map[string]any{"state": "in_progress", "claim_epoch": task["claim_epoch"]}
	if code, out := ownerCallMatch(f.owner, "POST", path, key, task["etag"].(string), body); code != 500 || out["error"].(map[string]any)["retryable"] != true {
		t.Fatal("fanout failure not retryable", code, out)
	}
	var state string
	var version int
	var events, receipts int
	if err := f.db.QueryRow(ctx, `SELECT state,version,(SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='task.progress_recorded'),(SELECT count(*) FROM undergrid.idempotency WHERE actor_id=$2 AND key=$3) FROM undergrid.tasks WHERE id=$1`, task["id"], f.agent, key).Scan(&state, &version, &events, &receipts); err != nil || state != "accepted" || version != 2 || events != 0 || receipts != 0 {
		t.Fatal("partial execution persisted", state, version, events, receipts, err)
	}
	if _, err := f.db.Exec(ctx, "DROP TRIGGER "+trigger+" ON undergrid_queue.river_job; DROP FUNCTION "+quoted+"()"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if code, out := ownerCallMatch(f.owner, "POST", path, key, task["etag"].(string), body); code != 200 || out["version"] != float64(3) || out["progress_deadline"] != task["progress_deadline"] || out["cure_deadline"] != task["cure_deadline"] {
			t.Fatal("exact recovery failed", code, out)
		}
	}
	if err := f.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='task.progress_recorded'),(SELECT count(*) FROM undergrid.idempotency WHERE actor_id=$2 AND key=$3)`, task["id"], f.agent, key).Scan(&events, &receipts); err != nil || events != 1 || receipts != 1 {
		t.Fatal("recovery duplicated effects", events, receipts, err)
	}
}

func TestOwnerTaskExecutionRechecksPolicyAfterCanonicalWait(t *testing.T) {
	f, task := acceptedOwnerExecutionTask(t)
	_, identity := f.owner.call("GET", "/v1/me", "", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	guardian, err := pgx.Connect(ctx, f.db.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	hold, err := guardian.Begin(ctx)
	if err != nil {
		guardian.Close(ctx)
		t.Fatal(err)
	}
	done := make(chan struct{})
	started := false
	t.Cleanup(func() {
		cancel()
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		hold.Rollback(cleanup)
		guardian.Close(cleanup)
		if started {
			select {
			case <-done:
			case <-cleanup.Done():
				t.Error("execution request did not cancel")
			}
		}
	})
	var pid int
	if err = hold.QueryRow(ctx, `SELECT pg_backend_pid() FROM undergrid.principals WHERE id=$1 FOR UPDATE`, identity["principal_id"]).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	type result struct {
		code int
		body map[string]any
		err  error
	}
	response := make(chan result, 1)
	key := NewID()
	started = true
	go func() {
		defer close(done)
		path := "/v1/owner/agents/" + f.agent + "/tasks/" + task["id"].(string) + "/progress"
		r := httptest.NewRequest("POST", path, bytes.NewReader(canonical(map[string]any{"state": "in_progress", "claim_epoch": task["claim_epoch"]}))).WithContext(ctx)
		r.Header.Set("X-Test-Owner", f.owner.owner)
		r.Header.Set("Idempotency-Key", key)
		r.Header.Set("If-Match", task["etag"].(string))
		w := httptest.NewRecorder()
		f.owner.s.ServeHTTP(w, r)
		var body map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &body)
		response <- result{w.Code, body, err}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var blocked bool
		if err = f.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND $1::integer=ANY(pg_blocking_pids(pid)) AND query='SELECT lock_plan_authorities($1)')`, pid).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("execution never waited on canonical policy locks")
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	// Controlled concurrent withdrawal, committed before the blocked request resumes.
	if _, err = hold.Exec(ctx, `UPDATE undergrid.principals SET policy=jsonb_set(policy,'{template_ids}','[]'::jsonb) WHERE id=$1`, identity["principal_id"]); err != nil {
		t.Fatal(err)
	}
	if err = hold.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case out := <-response:
		if out.err != nil || out.code != 403 || out.body["error"].(map[string]any)["code"] != "owner_policy_denied" {
			t.Fatal("policy withdrawal lost after wait", out)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var state string
	var events, receipts int
	if err = f.db.QueryRow(ctx, `SELECT state,(SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='task.progress_recorded'),(SELECT count(*) FROM undergrid.idempotency WHERE actor_id=$2 AND key=$3) FROM undergrid.tasks WHERE id=$1`, task["id"], f.agent, key).Scan(&state, &events, &receipts); err != nil || state != "accepted" || events != 0 || receipts != 0 {
		t.Fatal("withdrawn execution persisted effects", state, events, receipts, err)
	}
}
