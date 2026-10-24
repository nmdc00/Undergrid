package network

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// The trigger is a controlled timing seam after approval-policy counting, not
// an application failure or a reachable API feature. Both racing writes enter
// through owner HTTP and the real restricted application LOGIN.
func TestOwnerPlanFinalApprovalSerializesPeerPolicyRevocation(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	enableTestTerms(t, s)
	a, b := planningClient(t, s), planningClient(t, s)
	_, aid := a.call("GET", "/v1/me", "", nil)
	_, bid := b.call("GET", "/v1/me", "", nil)
	code, room := a.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Serialized exact plan approval", "visibility": "restricted", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	swarm := room["id"].(string)
	code, invitation := a.call("POST", "/v1/swarms/"+swarm+"/invitations", NewID(), map[string]any{"agent_id": bid["agent_id"], "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, invitation)
	}
	code, out := b.versioned("POST", "/v1/swarms/"+swarm+"/membership-response", NewID(), invitation["etag"].(string), map[string]any{"invitation_id": invitation["id"], "decision": "accept"})
	if code != 200 {
		t.Fatal(code, out)
	}
	_, snapshot := a.call("GET", "/v1/swarms/"+swarm+"/context", "", nil)
	principals := []any{aid["principal_id"], bid["principal_id"]}
	spec := map[string]any{"objective": "Serialized exact plan approval", "required_principals": principals, "acceptance_principals": principals, "publication_principals": principals, "rights": map[string]any{"template_id": "test-contribution-v1", "license_id": "CC0-1.0", "publication": "exact_manifest_approval"}, "tasks": []any{map[string]any{"key": "catalog", "assignee_agent_id": bid["agent_id"], "depends_on": []any{}, "output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 86400}}, "respond_by": time.Now().Add(time.Hour)}
	code, plan := a.versioned("POST", "/v1/swarms/"+swarm+"/plans", NewID(), snapshot["swarm"].(map[string]any)["etag"].(string), spec)
	if code != 201 {
		t.Fatal(code, plan)
	}
	task := plan["tasks"].([]any)[0].(map[string]any)
	code, out = b.versioned("POST", "/v1/tasks/"+task["id"].(string)+"/assignment-response", NewID(), task["etag"].(string), map[string]any{"decision": "accept", "plan_hash": plan["content_hash"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = b.call("POST", "/v1/plans/"+plan["id"].(string)+"/approvals", NewID(), map[string]any{"decision": "approve", "content_hash": plan["content_hash"]})
	if code != 200 || out["state"] != "proposed" {
		t.Fatal(code, out)
	}
	_, policy := ownerCallMatch(b, "GET", "/v1/owner/policy", "", "", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	guardian, err := pgx.Connect(ctx, db.Config().ConnString())
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	key := time.Now().UnixNano() & 0x7fffffff
	if _, err = guardian.Exec(ctx, "SELECT pg_advisory_lock($1)", key); err != nil {
		guardian.Close(ctx)
		cancel()
		t.Fatal(err)
	}
	name := "owner_plan_gate_" + strings.ReplaceAll(NewID(), "-", "")
	fn := pgx.Identifier{"undergrid", name}.Sanitize()
	trigger := pgx.Identifier{name}.Sanitize()
	var requests sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		cleanup, stop := context.WithTimeout(context.Background(), 4*time.Second)
		defer stop()
		guardian.Exec(cleanup, "SELECT pg_advisory_unlock($1)", key)
		guardian.Close(cleanup)
		drained := make(chan struct{})
		go func() { requests.Wait(); close(drained) }()
		select {
		case <-drained:
		case <-cleanup.Done():
			t.Error("owner race requests did not cancel")
		}
		if _, err := db.Exec(cleanup, "DROP TRIGGER IF EXISTS "+trigger+" ON undergrid.plan_versions; DROP FUNCTION IF EXISTS "+fn+"()"); err != nil {
			t.Error("remove controlled gate", err)
		}
	})
	sql := "CREATE FUNCTION " + fn + "() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id='" + plan["id"].(string) + "'::uuid AND NEW.state='accepted' AND OLD.state<>'accepted' THEN PERFORM pg_advisory_xact_lock(" + strconv.FormatInt(key, 10) + "); END IF; RETURN NEW; END $$; CREATE TRIGGER " + trigger + " BEFORE UPDATE ON undergrid.plan_versions FOR EACH ROW EXECUTE FUNCTION " + fn + "()"
	if _, err = db.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	type result struct {
		code int
		body map[string]any
		err  error
	}
	call := func(c testClient, path, key, match string, body any, ch chan<- result) {
		defer requests.Done()
		r := httptest.NewRequest("POST", path, bytes.NewReader(canonical(body))).WithContext(ctx)
		r.Header.Set("X-Test-Owner", c.owner)
		r.Header.Set("Idempotency-Key", key)
		r.Header.Set("If-Match", match)
		w := httptest.NewRecorder()
		c.s.ServeHTTP(w, r)
		var out map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &out)
		ch <- result{w.Code, out, err}
	}
	approval := make(chan result, 1)
	requests.Add(1)
	go call(a, "/v1/owner/agents/"+aid["agent_id"].(string)+"/plans/"+plan["id"].(string)+"/approvals", NewID(), "", map[string]any{"decision": "approve", "content_hash": plan["content_hash"], "version": plan["version"], "acknowledge_contribution_terms": true}, approval)
	wait := func(predicate func() (bool, error)) {
		t.Helper()
		deadline := time.NewTimer(3 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			ok, err := predicate()
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				return
			}
			select {
			case <-deadline.C:
				t.Fatal("controlled race condition not reached")
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-ticker.C:
			}
		}
	}
	wait(func() (bool, error) {
		var found bool
		err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND objid=$1::oid AND NOT granted)", key).Scan(&found)
		return found, err
	})
	revocation := make(chan result, 1)
	requests.Add(1)
	go call(b, "/v1/owner/policy", NewID(), policy["etag"].(string), map[string]any{"template_ids": []string{}, "license_ids": []string{}, "task_categories": []string{}, "max_active_tasks": 0, "expires_at": time.Now().Add(time.Hour)}, revocation)
	var early *result
	wait(func() (bool, error) {
		select {
		case response := <-revocation:
			early = &response
			return true, nil
		default:
		}
		var blocked bool
		err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%SELECT id FROM principals WHERE issuer=%')`).Scan(&blocked)
		return blocked, err
	})
	if _, err = guardian.Exec(ctx, "SELECT pg_advisory_unlock($1)", key); err != nil {
		t.Fatal(err)
	}
	receive := func(ch <-chan result) result {
		t.Helper()
		select {
		case r := <-ch:
			if r.err != nil {
				t.Fatal(r.err)
			}
			return r
		case <-ctx.Done():
			t.Fatal(ctx.Err())
			return result{}
		}
	}
	accepted := receive(approval)
	if accepted.code != 200 || accepted.body["state"] != "accepted" {
		t.Fatal("final exact approval", accepted)
	}
	if early == nil {
		r := receive(revocation)
		early = &r
	} else {
		if early.code != 200 || early.err != nil {
			t.Fatal("peer policy revocation did not commit", early)
		}
		var voteVersion, currentVersion int64
		var state, head string
		if err := db.QueryRow(ctx, `SELECT x.policy_version,p.version,v.state,s.active_plan_id::text FROM undergrid.approvals x JOIN undergrid.principals p ON p.id=x.principal_id JOIN undergrid.plan_versions v ON v.id=x.plan_id JOIN undergrid.swarms s ON s.id=v.swarm_id WHERE x.plan_id=$1 AND x.principal_id=$2 AND x.purpose='plan'`, plan["id"], bid["principal_id"]).Scan(&voteVersion, &currentVersion, &state, &head); err != nil {
			t.Fatal(err)
		}
		if state != "accepted" || head != plan["id"] || voteVersion == currentVersion {
			t.Fatal("race did not produce the expected persisted stale approval", state, head, voteVersion, currentVersion)
		}
		t.Errorf("peer revocation committed before final acceptance: HTTP=%d persisted plan=%s active head=%s vote policy=%d current policy=%d", early.code, state, head, voteVersion, currentVersion)
	}
	if early.code != 200 || early.err != nil {
		t.Fatal("peer policy revocation", early)
	}
}

// Preliminary identity resolution must not authorize a vote if the owner or
// attributed agent loses authority while waiting for the canonical lock set.
func TestOwnerPlanAuthorityRecheckedAfterCanonicalWait(t *testing.T) {
	for _, change := range []string{"agent_revoked", "owner_suspended"} {
		t.Run(change, func(t *testing.T) {
			s, db := ownerSwarmCreationFixture(t)
			enableTestTerms(t, s)
			owner := planningClient(t, s)
			_, identity := owner.call("GET", "/v1/me", "", nil)
			code, room := owner.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Current owner authority", "visibility": "private", "recruitment_roles": []any{}})
			if code != 201 {
				t.Fatal(code, room)
			}
			principal, agent := identity["principal_id"].(string), identity["agent_id"].(string)
			spec := map[string]any{"objective": "Current owner authority", "required_principals": []string{principal}, "acceptance_principals": []string{principal}, "publication_principals": []string{principal}, "rights": map[string]any{"template_id": "test-contribution-v1", "license_id": "CC0-1.0", "publication": "exact_manifest_approval"}, "tasks": []any{map[string]any{"key": "catalog", "assignee_agent_id": agent, "depends_on": []string{}, "output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 86400}}, "respond_by": time.Now().Add(time.Hour)}
			code, plan := owner.versioned("POST", "/v1/swarms/"+room["id"].(string)+"/plans", NewID(), room["etag"].(string), spec)
			if code != 201 {
				t.Fatal(code, plan)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			guardian, err := pgx.Connect(ctx, db.Config().ConnString())
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
			var guardianPID int
			if err = hold.QueryRow(ctx, "SELECT pg_backend_pid() FROM undergrid.principals WHERE id=$1 FOR UPDATE", principal).Scan(&guardianPID); err != nil {
				hold.Rollback(ctx)
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
						t.Error("authority-wait request did not cancel")
					}
				}
			})
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
				path := "/v1/owner/agents/" + agent + "/plans/" + plan["id"].(string) + "/approvals"
				r := httptest.NewRequest("POST", path, bytes.NewReader(canonical(map[string]any{"decision": "approve", "content_hash": plan["content_hash"], "version": plan["version"], "acknowledge_contribution_terms": true}))).WithContext(ctx)
				r.Header.Set("X-Test-Owner", owner.owner)
				r.Header.Set("Idempotency-Key", key)
				w := httptest.NewRecorder()
				s.ServeHTTP(w, r)
				var out map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &out)
				response <- result{w.Code, out, err}
			}()
			deadline := time.Now().Add(3 * time.Second)
			for {
				var blocked bool
				err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND $1::integer=ANY(pg_blocking_pids(pid)) AND query='SELECT lock_plan_authorities($1)')`, guardianPID).Scan(&blocked)
				if err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("request never waited on canonical principal locks")
				}
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(20 * time.Millisecond):
				}
			}
			if change == "agent_revoked" {
				_, err = db.Exec(ctx, "UPDATE undergrid.agents SET status='revoked' WHERE id=$1", agent)
			} else {
				_, err = hold.Exec(ctx, "UPDATE undergrid.principals SET status='suspended' WHERE id=$1", principal)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = hold.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case out := <-response:
				if out.err != nil || out.code != 403 {
					t.Fatal("authority lost during wait", out)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			var votes, commands int
			if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM undergrid.approvals WHERE plan_id=$1),(SELECT count(*) FROM undergrid.idempotency WHERE actor_id=$2 AND key=$3)`, plan["id"], agent, key).Scan(&votes, &commands); err != nil || votes != 0 || commands != 0 {
				t.Fatal("lost authority persisted effects", votes, commands, err)
			}
		})
	}
}
