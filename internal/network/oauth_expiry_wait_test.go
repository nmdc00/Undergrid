package network

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// The bound grant can outlive a subsequently issued access token. Both must be
// valid when the transaction obtains authority, including after a lock wait.
func TestOAuthAccessTokenExpiryAfterAuthorityWait(t *testing.T) {
	for _, table := range []string{"principals", "agents", "credentials"} {
		t.Run(table, func(t *testing.T) {
			s, db := fixture(t)
			role := "oauth_expiry_" + strings.ReplaceAll(NewID(), "-", "")
			if _, err := db.Exec(context.Background(), "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION; GRANT undergrid_app TO "+pgx.Identifier{role}.Sanitize()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{role}.Sanitize()) })
			s.dsn = networkFixtureRoleDSN(t, db, role)
			c := client(t, s)
			_, identity := c.call("GET", "/v1/me", "", nil)
			as := newTestOAuthAS(t)
			as.configure(t, s)
			subject, clientID, credential := NewID(), NewID(), NewID()
			if _, err := db.Exec(context.Background(), `INSERT INTO undergrid.credentials(id,agent_id,kind,oauth_issuer,oauth_subject,oauth_client_id,scopes,audience,expires_at) VALUES($1,$2,'oauth_grant',$3,$4,$5,ARRAY['intents:read','intents:write'],$6,clock_timestamp()+interval '1 hour')`, credential, identity["agent_id"], as.issuer, subject, clientID, as.resource); err != nil {
				t.Fatal(err)
			}
			// Warm discovery/JWKS before the short token is issued so the test measures
			// the actual authority wait rather than network discovery latency.
			longToken := as.token(t, subject, clientID, "intents:read intents:write", time.Now().Add(time.Hour), "at+jwt", as.resource)
			if code, out, _ := oauthCall(t, s, c.owner, longToken, http.MethodGet, "/v1/me", "", "", nil); code != 200 {
				t.Fatalf("long grant control: %d %v", code, out)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
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
			id := credential
			if table == "principals" {
				id = identity["principal_id"].(string)
			}
			if table == "agents" {
				id = identity["agent_id"].(string)
			}
			if _, err = held.Exec(ctx, "SELECT id FROM undergrid."+table+" WHERE id=$1 FOR UPDATE", id); err != nil {
				t.Fatal(err)
			}
			expiry := time.Now().Add(3 * time.Second).Truncate(time.Second)
			token := as.token(t, subject, clientID, "intents:read intents:write", expiry, "at+jwt", as.resource)
			key := "oauth-expired-after-lock-" + table
			req := httptest.NewRequest(http.MethodPost, "/v1/intents", bytes.NewReader(canonical(intentFixture()))).WithContext(ctx)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Idempotency-Key", key)
			done := make(chan int, 1)
			go func() { w := httptest.NewRecorder(); s.ServeHTTP(w, req); done <- w.Code }()
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
					t.Fatalf("request failed before authority wait: %d", status)
				default:
				}
				if ctx.Err() != nil {
					t.Fatal("request never waited on authority")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !time.Now().Before(expiry) {
				t.Fatal("fixture did not establish valid-token wait")
			}
			for time.Now().Before(expiry) {
				time.Sleep(10 * time.Millisecond)
			}
			if err = held.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case status := <-done:
				if status != 401 {
					t.Fatalf("JWT expired during %s wait but command returned %d", table, status)
				}
			case <-ctx.Done():
				t.Fatal("request did not finish")
			}
			var intents, events, receipts int
			if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid.intents WHERE principal_id=$1", identity["principal_id"]).Scan(&intents); err != nil || intents != 0 {
				t.Fatalf("expiredJWT intents=%d err=%v", intents, err)
			}
			if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid.events WHERE principal_id=$1 AND type='intent.created'", identity["principal_id"]).Scan(&events); err != nil || events != 0 {
				t.Fatalf("expiredJWT events=%d err=%v", events, err)
			}
			if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid.idempotency WHERE actor_id=$1 AND key=$2", identity["agent_id"], key).Scan(&receipts); err != nil || receipts != 0 {
				t.Fatalf("expiredJWT receipts=%d err=%v", receipts, err)
			}
			if code, out, _ := oauthCall(t, s, c.owner, longToken, http.MethodPost, "/v1/intents", "oauth-still-valid-"+table, "", intentFixture()); code != 201 {
				t.Fatalf("unexpired JWT control: %d %v", code, out)
			}
		})
	}
}
