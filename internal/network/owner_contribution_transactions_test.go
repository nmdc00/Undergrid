package network

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestOwnerContributionFinalizeDenialsAndFanoutRollbackRecoverExactly(t *testing.T) {
	x := newArtifactScenario(t)
	owner := x.actors[2]
	_, who := owner.call("GET", "/v1/me", "", nil)
	base := "/v1/owner/agents/" + who["agent_id"].(string)
	data := []byte(`{"sources":[{"source_id":"atomic-owner","source_url":"https://example.org/"}]}`)
	code, upload := ownerCallMatch(owner, "POST", base+"/tasks/"+x.task["id"].(string)+"/artifact-uploads", NewID(), x.task["etag"].(string), map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data)})
	if code != 201 {
		t.Fatal(code)
	}
	target, err := url.Parse(upload["upload_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if w := ownerContributionPut(owner, target.RequestURI(), data); w.Code != 204 {
		t.Fatal(w.Code)
	}
	path := base + "/artifacts/" + upload["id"].(string) + "/finalize"
	for _, name := range []string{"plan_hash", "claim_epoch", "byte_hash", "manifest_license"} {
		t.Run(name, func(t *testing.T) {
			body := x.finalBody(data)
			want := 409
			switch name {
			case "plan_hash":
				body["plan_hash"] = strings.Repeat("0", 64)
			case "claim_epoch":
				body["claim_epoch"] = 2
			case "byte_hash":
				body["sha256"] = strings.Repeat("0", 64)
				want = 422
			case "manifest_license":
				body["manifest"].(map[string]any)["license_id"] = "proprietary"
				want = 422
			}
			if code, _ := ownerCallMatch(owner, "POST", path, NewID(), "", body); code != want {
				t.Fatal("invalid finalization accepted", code, want)
			}
		})
	}
	name := "owner_contribution_fault_" + strings.ReplaceAll(NewID(), "-", "")
	function := pgx.Identifier{"undergrid", name}.Sanitize()
	trigger := pgx.Identifier{name}.Sanitize()
	sql := "CREATE FUNCTION " + function + "() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='event_fanout_v1' AND EXISTS(SELECT 1 FROM undergrid.events e WHERE e.id::text=NEW.args->>'event_id' AND e.aggregate_id::text='" + upload["id"].(string) + "' AND e.type='artifact.sealed') THEN RAISE EXCEPTION 'fixture contribution fanout failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER " + trigger + " BEFORE INSERT ON undergrid_queue.river_job FOR EACH ROW EXECUTE FUNCTION " + function + "()"
	if _, err = x.db.Exec(context.Background(), sql); err != nil {
		t.Fatal(err)
	}
	defer x.db.Exec(context.Background(), "DROP TRIGGER IF EXISTS "+trigger+" ON undergrid_queue.river_job; DROP FUNCTION IF EXISTS "+function+"()")
	key := NewID()
	body := x.finalBody(data)
	if code, out := ownerCallMatch(owner, "POST", path, key, "", body); code != 500 || out["error"].(map[string]any)["retryable"] != true {
		t.Fatal("fanout failure not retryable", code)
	}
	var state, taskState string
	var sha *string
	var jobs, events, receipts int
	if err = x.db.QueryRow(context.Background(), `SELECT a.state,a.sha256,t.state,(SELECT count(*) FROM undergrid_queue.river_job WHERE args->>'artifact_id'=a.id::text),(SELECT count(*) FROM undergrid.events WHERE aggregate_id=a.id AND type='artifact.sealed'),(SELECT count(*) FROM undergrid.idempotency WHERE actor_id=$2 AND key=$3) FROM undergrid.artifacts a JOIN undergrid.tasks t ON t.id=a.task_id WHERE a.id=$1`, upload["id"], who["agent_id"], key).Scan(&state, &sha, &taskState, &jobs, &events, &receipts); err != nil || state != "quarantine" || sha != nil || taskState != "accepted" || jobs != 0 || events != 0 || receipts != 0 {
		t.Fatal("failed seal leaked effects", state, sha, taskState, jobs, events, receipts, err)
	}
	if _, err = x.db.Exec(context.Background(), "DROP TRIGGER "+trigger+" ON undergrid_queue.river_job; DROP FUNCTION "+function+"()"); err != nil {
		t.Fatal(err)
	}
	code, sealed := ownerCallMatch(owner, "POST", path, key, "", body)
	if code != 200 {
		t.Fatal(code)
	}
	if code, recovered := ownerCallMatch(owner, "POST", path, key, "", body); code != 200 || !reflect.DeepEqual(sealed, recovered) {
		t.Fatal("retry did not recover exact seal", code)
	}
	if err = x.db.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM undergrid_queue.river_job WHERE args->>'artifact_id'=$1),(SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1::uuid AND type='artifact.sealed'),(SELECT count(*) FROM undergrid.idempotency WHERE actor_id=$2 AND key=$3)`, upload["id"], who["agent_id"], key).Scan(&jobs, &events, &receipts); err != nil || jobs != 1 || events != 1 || receipts != 1 {
		t.Fatal("recovery duplicated effects", jobs, events, receipts, err)
	}
}

func TestOwnerContributionFinalizeRechecksAuthorityAfterCanonicalWait(t *testing.T) {
	for _, change := range []string{"policy_withdrawn", "agent_revoked", "owner_suspended", "membership_withdrawn"} {
		t.Run(change, func(t *testing.T) {
			x := newArtifactScenario(t)
			owner := x.actors[2]
			_, who := owner.call("GET", "/v1/me", "", nil)
			base := "/v1/owner/agents/" + who["agent_id"].(string)
			data := []byte(`{"sources":[{"source_id":"owner-wait","source_url":"https://example.org/"}]}`)
			code, upload := ownerCallMatch(owner, "POST", base+"/tasks/"+x.task["id"].(string)+"/artifact-uploads", NewID(), x.task["etag"].(string), map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data)})
			if code != 201 {
				t.Fatal(code)
			}
			target, err := url.Parse(upload["upload_url"].(string))
			if err != nil {
				t.Fatal(err)
			}
			if w := ownerContributionPut(owner, target.RequestURI(), data); w.Code != 204 {
				t.Fatal(w.Code)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			guardian, err := pgx.Connect(ctx, x.db.Config().ConnString())
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			hold, err := guardian.Begin(ctx)
			if err != nil {
				guardian.Close(ctx)
				cancel()
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
						t.Error("contribution request did not cancel")
					}
				}
			})
			var pid int
			if err = hold.QueryRow(ctx, "SELECT pg_backend_pid() FROM undergrid.principals WHERE id=$1 FOR UPDATE", who["principal_id"]).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			key := NewID()
			response := make(chan *httptest.ResponseRecorder, 1)
			started = true
			go func() {
				defer close(done)
				r := httptest.NewRequest("POST", base+"/artifacts/"+upload["id"].(string)+"/finalize", bytes.NewReader(canonical(x.finalBody(data)))).WithContext(ctx)
				r.Header.Set("X-Test-Owner", owner.owner)
				r.Header.Set("Idempotency-Key", key)
				w := httptest.NewRecorder()
				owner.s.ServeHTTP(w, r)
				response <- w
			}()
			deadline := time.Now().Add(3 * time.Second)
			for {
				var blocked bool
				if err = x.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND $1::integer=ANY(pg_blocking_pids(pid)) AND query='SELECT lock_artifact_authorities($1)')`, pid).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("owner seal never waited on canonical artifact authorities")
				}
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(20 * time.Millisecond):
				}
			}
			want := 403
			switch change {
			case "policy_withdrawn":
				_, err = hold.Exec(ctx, `UPDATE undergrid.principals SET policy=jsonb_set(policy,'{template_ids}','[]'::jsonb) WHERE id=$1`, who["principal_id"])
			case "agent_revoked":
				_, err = hold.Exec(ctx, `UPDATE undergrid.agents SET status='revoked' WHERE id=$1`, who["agent_id"])
				// The existing owned-agent guard explicitly returns403 inactive_agent.
			case "owner_suspended":
				_, err = hold.Exec(ctx, `UPDATE undergrid.principals SET status='suspended' WHERE id=$1`, who["principal_id"])
			case "membership_withdrawn":
				_, err = hold.Exec(ctx, `UPDATE undergrid.memberships SET state='withdrawn' WHERE swarm_id=$1 AND agent_id=$2`, x.room["id"], who["agent_id"])
				want = 404
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = hold.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case w := <-response:
				var problem map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
					t.Fatal(err)
				}
				expectedCode := map[string]string{"policy_withdrawn": "owner_policy_denied", "agent_revoked": "inactive_agent", "owner_suspended": "registration_required", "membership_withdrawn": "not_found"}[change]
				if w.Code != want || problem["error"].(map[string]any)["code"] != expectedCode {
					var out map[string]any
					json.Unmarshal(w.Body.Bytes(), &out)
					t.Fatal("authority lost after canonical wait", w.Code, want, out)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			var state string
			var jobs, receipts int
			if err = x.db.QueryRow(ctx, `SELECT state,(SELECT count(*) FROM undergrid_queue.river_job WHERE args->>'artifact_id'=a.id::text),(SELECT count(*) FROM undergrid.idempotency WHERE actor_id=$2 AND key=$3) FROM undergrid.artifacts a WHERE id=$1`, upload["id"], who["agent_id"], key).Scan(&state, &jobs, &receipts); err != nil || state != "quarantine" || jobs != 0 || receipts != 0 {
				t.Fatal("revoked seal persisted effects", state, jobs, receipts, err)
			}
		})
	}
}
