package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Observe a real actor lock wait while the persisted credential is valid, then
// release it only after database wall time proves expiry. No clock is mocked.
func executeAfterCredentialExpires(t *testing.T, db *pgx.Conn, actor, token string, operation func(context.Context) error) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var expires time.Time
	if err := db.QueryRow(ctx, "UPDATE undergrid_sandbox.sandbox_credentials SET expires_at=clock_timestamp()+interval '3 seconds' WHERE actor_id=$1 AND digest=$2 RETURNING expires_at", actor, hash([]byte(token))).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	return executeAfterActorDeadline(t, db, actor, expires, operation)
}

func executeAfterActorDeadline(t *testing.T, db *pgx.Conn, actor string, expires time.Time, operation func(context.Context) error) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
	if _, err = held.Exec(ctx, "SELECT id FROM undergrid_sandbox.sandbox_actors WHERE id=$1 FOR UPDATE", actor); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- operation(ctx) }()
	for {
		var blocked, valid bool
		if err = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::integer=ANY(pg_blocking_pids(pid))),clock_timestamp()<$2::timestamptz", int32(locker.PgConn().PID()), expires).Scan(&blocked, &valid); err != nil {
			t.Fatal(err)
		}
		if blocked {
			if !valid {
				t.Fatal("credential expired before observed authority wait")
			}
			break
		}
		select {
		case err := <-done:
			t.Fatalf("operation never reached actor lock: %v", err)
		default:
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		time.Sleep(10 * time.Millisecond)
	}
	for {
		var expired bool
		if err = db.QueryRow(ctx, "SELECT clock_timestamp()>=$1::timestamptz", expires).Scan(&expired); err != nil {
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
	case err := <-done:
		return err
	case <-ctx.Done():
		t.Fatal(ctx.Err())
		return ctx.Err()
	}
}

func TestFinancialExecutionCredentialExpiresDuringActorWait(t *testing.T) {
	t.Run("stripe_dispatch_no_provider_spend", func(t *testing.T) {
		f := newStripeRecoveryFixture(t)
		worker := New(usdcFixtureRoleDSN(t, f.db, "undergrid_sandbox_worker"))
		f.service.store = worker
		var actor string
		if err := f.db.QueryRow(context.Background(), "SELECT actor_id::text FROM undergrid_sandbox.sandbox_payments WHERE id=$1", f.p.ID).Scan(&actor); err != nil {
			t.Fatal(err)
		}
		err := executeAfterCredentialExpires(t, f.db, actor, f.token, func(ctx context.Context) error { return f.service.DispatchPayment(ctx, f.p.ID) })
		var state string
		var effects int
		if e := f.db.QueryRow(context.Background(), "SELECT state,(SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=p.id) FROM undergrid_sandbox.sandbox_payments p WHERE id=$1", f.p.ID).Scan(&state, &effects); e != nil {
			t.Fatal(e)
		}
		if !errors.Is(err, ErrUnauthorized) || state != "canceled" || effects != 0 || f.posts.Load() != 0 {
			t.Fatalf("expired dispatch err=%v state=%s effects=%d provider_posts=%d", err, state, effects, f.posts.Load())
		}
	})
	t.Run("renewal_no_reservation_or_consent_consumption", func(t *testing.T) {
		s, db, owner, token, p, _ := refundFixture(t)
		ctx := context.Background()
		var credential, actor string
		if err := db.QueryRow(ctx, "SELECT id::text,actor_id::text FROM undergrid_sandbox.sandbox_credentials WHERE digest=$1", hash([]byte(token))).Scan(&credential, &actor); err != nil {
			t.Fatal(err)
		}
		if err := s.SetPolicy(ctx, owner, 2, Policy{Asset: "USD", Cap: "2000", Products: []string{"platform-test"}, Payees: []string{"stripe-test-account"}, ExpiresAt: time.Now().Add(90 * time.Hour)}); err != nil {
			t.Fatal(err)
		}
		sub, err := s.CreateSubscription(ctx, owner, "execution-expiry", SubscriptionConsent{CredentialID: credential, Maximum: "600", PeriodSeconds: 86400, RenewalCount: 2, ExpiresAt: time.Now().Add(72 * time.Hour)}, p.Quote)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, "UPDATE undergrid_sandbox.sandbox_subscriptions SET next_due_at=clock_timestamp()-interval '1 second' WHERE id=$1", sub.ID); err != nil {
			t.Fatal(err)
		}
		var due time.Time
		if err = db.QueryRow(ctx, "SELECT next_due_at FROM undergrid_sandbox.sandbox_subscriptions WHERE id=$1", sub.ID).Scan(&due); err != nil {
			t.Fatal(err)
		}
		worker := New(usdcFixtureRoleDSN(t, db, "undergrid_sandbox_worker"))
		err = executeAfterCredentialExpires(t, db, actor, token, func(ctx context.Context) error { _, err := worker.PrepareRenewal(ctx, sub.ID); return err })
		var reserved, payments, effects int
		var state string
		var actualDue time.Time
		if e := db.QueryRow(ctx, `SELECT state,renewals_reserved,next_due_at,(SELECT count(*) FROM undergrid_sandbox.sandbox_payments WHERE subscription_id=s.id),(SELECT count(*) FROM undergrid_sandbox.sandbox_effects e JOIN undergrid_sandbox.sandbox_payments p ON p.id=e.payment_id WHERE p.subscription_id=s.id) FROM undergrid_sandbox.sandbox_subscriptions s WHERE id=$1`, sub.ID).Scan(&state, &reserved, &actualDue, &payments, &effects); e != nil {
			t.Fatal(e)
		}
		if !errors.Is(err, ErrUnauthorized) || state != "active" || reserved != 0 || payments != 0 || effects != 0 || !actualDue.Equal(due) {
			t.Fatalf("expired renewal err=%v state=%s reserved=%d payments=%d effects=%d due_preserved=%v", err, state, reserved, payments, effects, actualDue.Equal(due))
		}
	})
	t.Run("usdc_retains_historical_risk_without_disclosure", func(t *testing.T) {
		f := newUSDCChainFixture(t)
		f.phase.Store(2)
		f.prebroadcastHead.Store(100)
		if err := f.service.VerifyConfiguration(context.Background()); err != nil {
			t.Fatal(err)
		}
		var key, period, proof string
		if err := f.owner.db.QueryRow(context.Background(), "SELECT provider_key,reservation_period::text,payer_authorization_hash FROM undergrid_sandbox.sandbox_payments WHERE id=$1", f.payment.ID).Scan(&key, &period, &proof); err != nil {
			t.Fatal(err)
		}
		err := executeAfterCredentialExpires(t, f.owner.db, f.owner.actorID, f.owner.token, func(ctx context.Context) error { return f.service.DispatchPayment(ctx, f.payment.ID) })
		var retained bool
		var effects int
		if e := f.owner.db.QueryRow(context.Background(), `SELECT p.state='unknown' AND p.provider_key=$2 AND p.reservation_period::text=$3 AND p.payer_authorization_hash=$4 AND c.start_block IS NULL AND c.authorization_hash=$4,(SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=p.id) FROM undergrid_sandbox.sandbox_payments p JOIN undergrid_sandbox.sandbox_usdc_claims c ON c.payment_id=p.id WHERE p.id=$1`, f.payment.ID, key, period, proof).Scan(&retained, &effects); e != nil {
			t.Fatal(e)
		}
		if !errors.Is(err, ErrUnauthorized) || !retained || effects != 0 || f.verifies.Load() != 0 || f.submits.Load() != 0 {
			t.Fatalf("expired USDC err=%v retained=%v effects=%d verify=%d submit=%d", err, retained, effects, f.verifies.Load(), f.submits.Load())
		}
	})
}

func TestFinancialRefundCredentialExpiresDuringActorWait(t *testing.T) {
	f := newStripeRecoveryFixture(t)
	ctx := context.Background()
	if _, _, err := f.service.verifyCharge(ctx, f.chargeID, "expiry-fixture-charge/"+f.chargeID); err != nil {
		t.Fatal(err)
	}
	token, err := f.store.IssueCredential(ctx, f.owner, "expiry-refund", []string{"refunds:request", "refunds:authorize", "refunds:read"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.store.RequestRefund(ctx, token, "expiry-refund", f.p.ID, "40")
	if err != nil {
		t.Fatal(err)
	}
	r, err = f.store.AuthorizeRefund(ctx, token, r.ID, r.Hash, "expiry-refund")
	if err != nil {
		t.Fatal(err)
	}
	f.refund.Store(r)
	var actor string
	if err = f.db.QueryRow(ctx, "SELECT actor_id::text FROM undergrid_sandbox.sandbox_refunds WHERE id=$1", r.ID).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	f.service.store = New(usdcFixtureRoleDSN(t, f.db, "undergrid_sandbox_worker"))
	err = executeAfterCredentialExpires(t, f.db, actor, token, func(ctx context.Context) error { return f.service.DispatchRefund(ctx, r.ID) })
	var state, key string
	var refundEffects int
	if e := f.db.QueryRow(ctx, `SELECT state,provider_key,(SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$2 AND kind='refund') FROM undergrid_sandbox.sandbox_refunds WHERE id=$1`, r.ID, f.p.ID).Scan(&state, &key, &refundEffects); e != nil {
		t.Fatal(e)
	}
	if !errors.Is(err, ErrUnauthorized) || state != "failed" || key != r.ProviderKey || refundEffects != 0 || f.refundPosts.Load() != 0 {
		t.Fatalf("expired refund err=%v state=%s key_retained=%v effects=%d provider_posts=%d", err, state, key == r.ProviderKey, refundEffects, f.refundPosts.Load())
	}
}

// Renewal quotes inherit consent expiry. The existing post-wait quote check
// already denies this case even when a transaction-start consent predicate is stale.
func TestRenewalConsentExpiryDuringActorWaitIsBoundedByDispatchQuote(t *testing.T) {
	s, db, owner, token, p, _ := refundFixture(t)
	ctx := context.Background()
	var credential, actor string
	if err := db.QueryRow(ctx, "SELECT id::text,actor_id::text FROM undergrid_sandbox.sandbox_credentials WHERE digest=$1", hash([]byte(token))).Scan(&credential, &actor); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPolicy(ctx, owner, 2, Policy{Asset: "USD", Cap: "2000", Products: []string{"platform-test"}, Payees: []string{"stripe-test-account"}, ExpiresAt: time.Now().Add(90 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	worker := New(usdcFixtureRoleDSN(t, db, "undergrid_sandbox_worker"))
	sub, err := s.CreateSubscription(ctx, owner, "consent-deadline", SubscriptionConsent{CredentialID: credential, Maximum: "600", PeriodSeconds: 86400, RenewalCount: 2, ExpiresAt: time.Now().Add(4 * time.Second)}, p.Quote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "UPDATE undergrid_sandbox.sandbox_subscriptions SET next_due_at=clock_timestamp()-interval '1 second' WHERE id=$1", sub.ID); err != nil {
		t.Fatal(err)
	}
	debit, err := worker.PrepareRenewal(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if debit.Quote.ExpiresAt.After(sub.Consent.ExpiresAt) {
		t.Fatal("renewal quote outlives original consent")
	}
	err = executeAfterActorDeadline(t, db, actor, sub.Consent.ExpiresAt, func(ctx context.Context) error { _, err := worker.BeginDispatch(ctx, debit.ID); return err })
	var state string
	var effects int
	if e := db.QueryRow(ctx, "SELECT state,(SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=p.id) FROM undergrid_sandbox.sandbox_payments p WHERE id=$1", debit.ID).Scan(&state, &effects); e != nil {
		t.Fatal(e)
	}
	if !errors.Is(err, ErrUnauthorized) || state != "canceled" || effects != 0 {
		t.Fatalf("expired consent err=%v state=%s effects=%d", err, state, effects)
	}
}
