package sandbox

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestSubscriptionHTTPRetryCannotMultiplyRenewalConsent(t *testing.T) {
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
	code, first := refundCall(t, h, "", owner.Subject, "POST", "/v1/owner/subscriptions", "one-consent", body)
	if code != 201 {
		t.Fatalf("first consent: %d %v", code, first)
	}
	code, replay := refundCall(t, h, "", owner.Subject, "POST", "/v1/owner/subscriptions", "one-consent", body)
	if code != 201 || first["id"] != replay["id"] || first["consent_hash"] != replay["consent_hash"] {
		t.Fatalf("response-loss retry multiplied consent: first=%v replay=%d %v", first, code, replay)
	}
	// A fresh service instance has no in-memory response cache to lean on.
	h.store = New(s.dsn)
	code, replay = refundCall(t, h, "", owner.Subject, "POST", "/v1/owner/subscriptions", "one-consent", body)
	if code != 201 || replay["id"] != first["id"] {
		t.Fatalf("restart lost original consent: %d %v", code, replay)
	}
	body["renewal_count"] = 1
	code, out := refundCall(t, h, "", owner.Subject, "POST", "/v1/owner/subscriptions", "one-consent", body)
	if code != 409 {
		t.Fatalf("changed consent reused key: %d %v", code, out)
	}
	body["renewal_count"] = 2
	var wg sync.WaitGroup
	results := make(chan map[string]any, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, out := refundCall(t, h, "", owner.Subject, "POST", "/v1/owner/subscriptions", "concurrent-consent", body)
			if code != 201 {
				t.Errorf("concurrent consent: %d %v", code, out)
			}
			results <- out
		}()
	}
	wg.Wait()
	close(results)
	var concurrentID any
	for out := range results {
		if concurrentID == nil {
			concurrentID = out["id"]
		} else if concurrentID != out["id"] {
			t.Fatalf("parallel retry produced different authority: %v", out)
		}
	}
	var count int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_sandbox.sandbox_subscriptions WHERE credential_id=$1", credential).Scan(&count); err != nil || count != 2 {
		t.Fatalf("expected exactly two distinct commands: %d %v", count, err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_sandbox.sandbox_idempotency i JOIN undergrid_sandbox.sandbox_subscriptions s ON s.id=i.subscription_id AND s.actor_id=i.actor_id WHERE s.credential_id=$1 AND i.credential_id IS NULL AND i.payment_id IS NULL AND i.refund_id IS NULL AND i.operation='subscription.create'", credential).Scan(&count); err != nil || count != 2 {
		t.Fatalf("normalized owner result refs: %d %v", count, err)
	}
	if err := s.CancelSubscription(context.Background(), owner, first["id"].(string), first["consent_hash"].(string)); err != nil {
		t.Fatal(err)
	}
	code, replay = refundCall(t, h, "", owner.Subject, "POST", "/v1/owner/subscriptions", "one-consent", body)
	if code != 201 || replay["id"] != first["id"] || replay["state"] != "canceled" {
		t.Fatalf("retry resurrected canceled authority: %d %v", code, replay)
	}
	if err := s.RevokeCredentials(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	code, out = refundCall(t, h, "", owner.Subject, "POST", "/v1/owner/subscriptions", "one-consent", body)
	if code != 401 {
		t.Fatalf("replay bypassed revoked authority: %d %v", code, out)
	}
}

func TestSubscriptionHTTPRequiresStableOwnerCommandKey(t *testing.T) {
	s, db, owner, token, payment, _ := refundFixture(t)
	var credential string
	if err := db.QueryRow(context.Background(), "SELECT id::text FROM undergrid_sandbox.sandbox_credentials WHERE digest=$1", hash([]byte(token))).Scan(&credential); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(s, func(*http.Request) (Owner, error) { return owner, nil }, map[string]Quote{"platform-test": payment.Quote})
	body := map[string]any{"credential_id": credential, "product": "platform-test", "maximum_atomic": "600", "period_seconds": 86400, "renewal_count": 2, "expires_at": time.Now().Add(72 * time.Hour)}
	code, out := refundCall(t, h, "", owner.Subject, "POST", "/v1/owner/subscriptions", "", body)
	if code != 400 {
		t.Fatalf("missing command key created consent: %d %v", code, out)
	}
}
