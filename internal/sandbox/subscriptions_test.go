package sandbox

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestSubscriptionHTTPConsentIsOwnerOnlyAndBounded(t *testing.T) {
	s, db, owner, token, payment, _ := refundFixture(t)
	var credential string
	if err := db.QueryRow(context.Background(), "SELECT id::text FROM undergrid_sandbox.sandbox_credentials WHERE digest=$1", hash([]byte(token))).Scan(&credential); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(s, func(r *http.Request) (Owner, error) {
		if r.Header.Get("X-Fixture-Owner") != owner.Subject {
			return Owner{}, ErrUnauthorized
		}
		return owner, nil
	}, map[string]Quote{"platform-test": payment.Quote})
	body := map[string]any{"credential_id": credential, "product": "platform-test", "maximum_atomic": "600", "period_seconds": 86400, "renewal_count": 2, "expires_at": time.Now().Add(72 * time.Hour)}
	code, out := refundCall(t, h, "", owner.Subject, "POST", "/v1/owner/subscriptions", "bounded-consent", body)
	if code != 201 || out["state"] != "active" || out["consent_hash"] == "" {
		t.Fatalf("bounded owner consent: %d %v", code, out)
	}
	code, out = refundCall(t, h, token, "", "POST", "/v1/owner/subscriptions", "", body)
	if code != 401 {
		t.Fatalf("agent granted recurring authority: %d %v", code, out)
	}
	body["renewal_count"] = 3
	code, out = refundCall(t, h, "", owner.Subject, "POST", "/v1/owner/subscriptions", "", body)
	if code != 400 {
		t.Fatalf("unbounded third renewal consent: %d %v", code, out)
	}
	body["renewal_count"] = 2
	body["maximum_atomic"] = "599"
	code, out = refundCall(t, h, "", owner.Subject, "POST", "/v1/owner/subscriptions", "", body)
	if code != 400 {
		t.Fatalf("consent below fixed offer: %d %v", code, out)
	}
}

func TestSubscriptionRenewalsReserveOnceAndCannotExceedTwo(t *testing.T) {
	s, db, owner, token, p, _ := refundFixture(t)
	ctx := context.Background()
	var credential string
	if err := db.QueryRow(ctx, "SELECT id::text FROM undergrid_sandbox.sandbox_credentials WHERE digest=$1", hash([]byte(token))).Scan(&credential); err != nil {
		t.Fatal(err)
	}
	policy := Policy{Asset: "USD", Cap: "2000", Products: []string{"platform-test"}, Payees: []string{"stripe-test-account"}, ExpiresAt: time.Now().Add(90 * time.Hour)}
	if err := s.SetPolicy(ctx, owner, 2, policy); err != nil {
		t.Fatal(err)
	}
	consent := SubscriptionConsent{CredentialID: credential, Maximum: "600", PeriodSeconds: 86400, RenewalCount: 2, ExpiresAt: time.Now().Add(72 * time.Hour)}
	sub, err := s.CreateSubscription(ctx, owner, "bounded-consent", consent, p.Quote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.PrepareRenewal(ctx, sub.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("renewal before period: %v", err)
	}
	for ordinal := 1; ordinal <= 2; ordinal++ {
		if _, err = db.Exec(ctx, "UPDATE undergrid_sandbox.sandbox_subscriptions SET next_due_at=now()-interval '1 second' WHERE id=$1", sub.ID); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := s.PrepareRenewal(ctx, sub.ID); results <- err }()
		}
		wg.Wait()
		close(results)
		ok, conflict := 0, 0
		for err := range results {
			if err == nil {
				ok++
			} else if errors.Is(err, ErrConflict) {
				conflict++
			} else {
				t.Fatal(err)
			}
		}
		if ok != 1 || conflict != 1 {
			t.Fatalf("concurrent renewal duplicate: %d/%d", ok, conflict)
		}
	}
	var count int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_payments WHERE subscription_id=$1 AND state='authorized' AND reservation_period IS NOT NULL", sub.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("bounded durable reservations: %d %v", count, err)
	}
	if _, err = s.PrepareRenewal(ctx, sub.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("third renewal: %v", err)
	}
	if err = s.CancelSubscription(ctx, owner, sub.ID, sub.Hash); err != nil {
		t.Fatal(err)
	}
	var payment string
	if err = db.QueryRow(ctx, "SELECT id::text FROM undergrid_sandbox.sandbox_payments WHERE subscription_id=$1 LIMIT 1", sub.ID).Scan(&payment); err != nil {
		t.Fatal(err)
	}
	blocked, err := s.BeginDispatch(ctx, payment)
	if !errors.Is(err, ErrUnauthorized) || blocked.State != "canceled" {
		t.Fatalf("canceled consent dispatched: %+v %v", blocked, err)
	}
}

func TestSubscriptionRevocationAndCapacityAreRechecked(t *testing.T) {
	s, db, owner, token, p, _ := refundFixture(t)
	ctx := context.Background()
	var credential string
	if err := db.QueryRow(ctx, "SELECT id::text FROM undergrid_sandbox.sandbox_credentials WHERE digest=$1", hash([]byte(token))).Scan(&credential); err != nil {
		t.Fatal(err)
	}
	sub, err := s.CreateSubscription(ctx, owner, "bounded-consent", SubscriptionConsent{CredentialID: credential, Maximum: "600", PeriodSeconds: 86400, RenewalCount: 2, ExpiresAt: time.Now().Add(72 * time.Hour)}, p.Quote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "UPDATE undergrid_sandbox.sandbox_subscriptions SET next_due_at=now()-interval '1 second' WHERE id=$1", sub.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PrepareRenewal(ctx, sub.ID); !errors.Is(err, ErrLimit) {
		t.Fatalf("renewal bypassed gross cap: %v", err)
	}
	if err = s.RevokeCredentials(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PrepareRenewal(ctx, sub.ID); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked recurring credential: %v", err)
	}
	var count int
	if err = db.QueryRow(ctx, "SELECT renewals_reserved FROM undergrid_sandbox.sandbox_subscriptions WHERE id=$1", sub.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed renewal consumed consent: %d %v", count, err)
	}
	other := Owner{Issuer: owner.Issuer, Subject: randomID()}
	if _, err = s.Enroll(ctx, other); err != nil {
		t.Fatal(err)
	}
	if err = s.CancelSubscription(ctx, other, sub.ID, sub.Hash); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("cross-owner canceled consent: %v", err)
	}
}

func TestRenewalWorkerRunsPersistedConsentAndStops(t *testing.T) {
	s, db, owner, token, p, _ := refundFixture(t)
	ctx := context.Background()
	if err := s.SetPolicy(ctx, owner, 2, Policy{Asset: "USD", Cap: "2000", Products: []string{"platform-test"}, Payees: []string{"stripe-test-account"}, ExpiresAt: time.Now().Add(72 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var credential string
	if err := db.QueryRow(ctx, "SELECT id::text FROM undergrid_sandbox.sandbox_credentials WHERE digest=$1", hash([]byte(token))).Scan(&credential); err != nil {
		t.Fatal(err)
	}
	sub, err := s.CreateSubscription(ctx, owner, "bounded-consent", SubscriptionConsent{CredentialID: credential, Maximum: "600", PeriodSeconds: 86400, RenewalCount: 2, ExpiresAt: time.Now().Add(72 * time.Hour)}, p.Quote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "UPDATE undergrid_sandbox.sandbox_subscriptions SET next_due_at=now()-interval '1 second' WHERE id=$1", sub.ID); err != nil {
		t.Fatal(err)
	}
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.RunRenewalWorker(workerCtx) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var count int
		if err = db.QueryRow(ctx, "SELECT renewals_reserved FROM undergrid_sandbox.sandbox_subscriptions WHERE id=$1", sub.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker failed to prepare persisted due consent")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
}
