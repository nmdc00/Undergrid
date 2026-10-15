package network

import (
	"bytes"
	"context"
	"crypto/sha256"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestNetworkCredentialExpiryAfterAuthorityWait(t *testing.T) {
	for _, table := range []string{"principals", "agents", "credentials"} {
		t.Run(table, func(t *testing.T) {
			s, db := fixture(t)
			role := "expiry_api_" + strings.ReplaceAll(NewID(), "-", "")
			if _, err := db.Exec(context.Background(), "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION; GRANT undergrid_app TO "+pgx.Identifier{role}.Sanitize()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{role}.Sanitize()) })
			s.dsn = networkFixtureRoleDSN(t, db, role)
			c := client(t, s)
			validClient := c
			_, identity := c.call("GET", "/v1/me", "", nil)
			// Credential issuance metadata is immutable. Create a separate short-lived
			// token instead of modifying an already-issued credential or its trigger.
			c.token = "ug_expiry_" + NewID()
			hash := sha256.Sum256([]byte(c.token))
			credential := NewID()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var expiry time.Time
			if err := db.QueryRow(ctx, `INSERT INTO undergrid.credentials(id,agent_id,digest,scopes,audience,expires_at) VALUES($1,$2,$3,$4,'undergrid-network',clock_timestamp()+interval '2 seconds') RETURNING expires_at`, credential, identity["agent_id"], hash[:], []string{"intents:read", "intents:write"}).Scan(&expiry); err != nil {
				t.Fatal(err)
			}
			id := credential
			if table == "principals" {
				id = identity["principal_id"].(string)
			}
			if table == "agents" {
				id = identity["agent_id"].(string)
			}
			locker, err := pgx.Connect(ctx, db.Config().ConnString())
			if err != nil {
				t.Fatal(err)
			}
			defer locker.Close(context.Background())
			held, err := locker.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer held.Rollback(context.Background())
			// table is a fixed test-case identifier, never user input.
			if _, err = held.Exec(ctx, "SELECT id FROM undergrid."+table+" WHERE id=$1 FOR UPDATE", id); err != nil {
				t.Fatal(err)
			}
			var jobsBefore int
			if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid_queue.river_job").Scan(&jobsBefore); err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest("POST", "/v1/intents", bytes.NewReader(canonical(intentFixture()))).WithContext(ctx)
			request.Header.Set("Authorization", "Bearer "+c.token)
			request.Header.Set("Idempotency-Key", "expired-after-lock-"+table)
			done := make(chan int, 1)
			go func() { w := httptest.NewRecorder(); s.ServeHTTP(w, request); done <- w.Code }()
			for {
				var blocked bool
				if err = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::integer=ANY(pg_blocking_pids(pid)))", int32(locker.PgConn().PID())).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				select {
				case status := <-done:
					t.Fatalf("request did not reach authority wait: %d", status)
				default:
				}
				if ctx.Err() != nil {
					t.Fatal("request never waited on held authority")
				}
				time.Sleep(10 * time.Millisecond)
			}
			// The real request is blocked while the immutable credential is
			// still valid. Advance no clocks and mutate no issuance metadata.
			var valid bool
			if err = db.QueryRow(ctx, "SELECT clock_timestamp()<$1::timestamptz", expiry).Scan(&valid); err != nil || !valid {
				t.Fatalf("fixture did not establish a valid-token authority wait: valid=%v err=%v", valid, err)
			}
			for {
				var expired bool
				if err = db.QueryRow(ctx, "SELECT clock_timestamp()>=$1::timestamptz", expiry).Scan(&expired); err != nil {
					t.Fatal(err)
				}
				if expired {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err = held.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case status := <-done:
				if status != 401 {
					t.Fatalf("credential expired during %s wait but command returned %d", table, status)
				}
			case <-ctx.Done():
				t.Fatal("request failed to finish after authority release")
			}
			var effects int
			if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid.intents WHERE principal_id=$1", identity["principal_id"]).Scan(&effects); err != nil || effects != 0 {
				t.Fatal("expired command persisted intentions", effects, err)
			}
			var events, idem, jobsAfter int
			if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid.events WHERE principal_id=$1 AND type='intent.created'", identity["principal_id"]).Scan(&events); err != nil || events != 0 {
				t.Fatalf("expired command persisted events=%d err=%v", events, err)
			}
			if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid.idempotency WHERE key=$1 AND actor_id=$2", "expired-after-lock-"+table, identity["agent_id"]).Scan(&idem); err != nil || idem != 0 {
				t.Fatalf("expired command persisted idempotency=%d err=%v", idem, err)
			}
			if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid_queue.river_job").Scan(&jobsAfter); err != nil || jobsAfter != jobsBefore {
				t.Fatalf("expired command changed jobs before=%d after=%d err=%v", jobsBefore, jobsAfter, err)
			}
			if status, result := validClient.call("POST", "/v1/intents", "still-valid-after-expired-wait", intentFixture()); status != 201 {
				t.Fatalf("valid credential was also rejected after %s wait: %d %v", table, status, result)
			}

		})
	}
}
