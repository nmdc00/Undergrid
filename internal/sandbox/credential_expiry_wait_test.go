package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestSandboxCredentialExpiryAfterAuthorityWait(t *testing.T) {
	for _, table := range []string{"sandbox_actors", "sandbox_credentials"} {
		t.Run(table, func(t *testing.T) {
			f := newWalletHTTPFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			role := "sandbox_expiry_" + strings.ReplaceAll(randomID(), "-", "")
			if _, err := f.db.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION; GRANT undergrid_sandbox_app TO "+pgx.Identifier{role}.Sanitize()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { f.db.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{role}.Sanitize()) })
			cfg := f.db.Config()
			runtimeURL := url.URL{Scheme: "postgresql", User: url.User(role), Host: net.JoinHostPort("localhost", strconv.Itoa(int(cfg.Port))), Path: "/" + cfg.Database}
			query := url.Values{"host": {cfg.Host}, "sslmode": {"disable"}}
			runtimeURL.RawQuery = query.Encode()
			login, err := pgx.Connect(ctx, runtimeURL.String())
			if err != nil {
				t.Fatal(err)
			}
			var actual string
			var forbidden bool
			err = login.QueryRow(ctx, `SELECT session_user, r.rolsuper OR r.rolbypassrls OR pg_has_role(session_user,'undergrid_sandbox_worker','MEMBER') OR EXISTS(SELECT 1 FROM pg_namespace n WHERE n.nspname='undergrid_sandbox' AND n.nspowner=r.oid) FROM pg_roles r WHERE r.rolname=session_user`).Scan(&actual, &forbidden)
			login.Close(ctx)
			if err != nil || actual != role || forbidden {
				t.Fatalf("restricted app LOGIN not established: role=%q forbidden=%v err=%v", actual, forbidden, err)
			}
			f.store.dsn = runtimeURL.String()
			q := Quote{Product: "platform-test", Asset: "USD", Amount: "100", Fee: "0", Payee: "stripe-test-account", Rail: "stripe_test", TermsID: "test-terms", TermsHash: strings.Repeat("a", 64), ExpiresAt: time.Now().Add(time.Hour)}
			handler := mustWalletHandler(t, f.store, nil, map[string]Quote{"platform-test": q})
			token := "ugs_expiry_" + randomID()
			credential := randomID()
			var expiry time.Time
			if err = f.db.QueryRow(ctx, `INSERT INTO undergrid_sandbox.sandbox_credentials(id,actor_id,agent_name,digest,audience,scopes,expires_at) VALUES($1,$2,'expiry-fixture',$3,'undergrid-sandbox',ARRAY['payments:quote','payments:authorize','payments:read'],clock_timestamp()+interval '3 seconds') RETURNING expires_at`, credential, f.actorID, hash([]byte(token))).Scan(&expiry); err != nil {
				t.Fatal(err)
			}
			payment, err := f.store.CreateQuote(ctx, token, "before-expiry-wait", q)
			if err != nil {
				t.Fatal(err)
			}
			locker, err := pgx.Connect(ctx, f.db.Config().ConnString())
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
			if table == "sandbox_actors" {
				id = f.actorID
			}
			if _, err = held.Exec(ctx, "SELECT id FROM undergrid_sandbox."+table+" WHERE id=$1 FOR UPDATE", id); err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(map[string]string{"quote_hash": payment.Hash})
			req := httptest.NewRequest(http.MethodPost, "/v1/payments/"+payment.ID+"/authorize", bytes.NewReader(body)).WithContext(ctx)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Idempotency-Key", "expired-during-financial-authority-wait")
			done := make(chan int, 1)
			go func() { w := httptest.NewRecorder(); handler.ServeHTTP(w, req); done <- w.Code }()
			for {
				var blocked bool
				if err = f.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::integer=ANY(pg_blocking_pids(pid)))", int32(locker.PgConn().PID())).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				select {
				case status := <-done:
					t.Fatalf("authorization did not reach held authority: %d", status)
				default:
				}
				if ctx.Err() != nil {
					t.Fatal("authorization never waited on authority")
				}
				time.Sleep(10 * time.Millisecond)
			}
			var valid bool
			if err = f.db.QueryRow(ctx, "SELECT clock_timestamp()<$1::timestamptz", expiry).Scan(&valid); err != nil || !valid {
				t.Fatalf("fixture failed to establish valid-token wait: %v %v", valid, err)
			}
			for {
				var expired bool
				if err = f.db.QueryRow(ctx, "SELECT clock_timestamp()>=$1::timestamptz", expiry).Scan(&expired); err != nil {
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
					t.Fatalf("expired sandbox credential authorized after %s wait: HTTP%d", table, status)
				}
			case <-ctx.Done():
				t.Fatal("authorization failed to finish after unlock")
			}
			var state string
			var authorized bool
			if err = f.db.QueryRow(ctx, "SELECT state,policy_version IS NOT NULL OR authority_proof IS NOT NULL OR reservation_period IS NOT NULL FROM undergrid_sandbox.sandbox_payments WHERE id=$1", payment.ID).Scan(&state, &authorized); err != nil || state != "quoted" || authorized {
				t.Fatalf("expired credential changed payment/reservation: state=%s authority=%v err=%v", state, authorized, err)
			}
			var receipts, effects int
			if err = f.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_idempotency WHERE credential_id=$1 AND operation='authorize'", credential).Scan(&receipts); err != nil || receipts != 0 {
				t.Fatalf("expired credential committed authorization receipts=%d err=%v", receipts, err)
			}
			if err = f.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1", payment.ID).Scan(&effects); err != nil || effects != 0 {
				t.Fatalf("expired credential committed financial effects=%d err=%v", effects, err)
			}
			control, err := f.store.CreateQuote(ctx, f.token, "still-valid-control", q)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.Authorize(ctx, f.token, control.ID, control.Hash, "still-valid-authority"); err != nil {
				t.Fatalf("valid financial credential rejected: %v", err)
			}
		})
	}
}
