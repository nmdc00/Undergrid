package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	stripe "github.com/stripe/stripe-go/v87"
	"github.com/stripe/stripe-go/v87/webhook"
)

func TestStripeSignedInboxReconcilesRetrievedObjectInsteadOfPayload(t *testing.T) {
	s, db, owner, token, original, _ := refundFixture(t)
	ctx := context.Background()
	accountID := "acct_" + randomID()
	if err := s.SetPolicy(ctx, owner, 2, Policy{Asset: "USD", Cap: "1000", Products: []string{"platform-test"}, Payees: []string{accountID}, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	q := original.Quote
	q.Payee = accountID
	q.Amount = "100"
	q.ExpiresAt = time.Now().Add(time.Hour)
	p, err := s.CreateQuote(ctx, token, "stripe-quote", q)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorize(ctx, token, p.ID, p.Hash, "stripe-approve"); err != nil {
		t.Fatal(err)
	}
	chargeID := "ch_" + randomID()
	eventID := "evt_" + randomID()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/account" {
			json.NewEncoder(w).Encode(map[string]any{"id": accountID, "object": "account"})
			return
		}
		if r.URL.Path == "/v1/charges/"+chargeID {
			json.NewEncoder(w).Encode(map[string]any{"id": chargeID, "object": "charge", "livemode": false, "amount": 100, "amount_captured": 100, "currency": "usd", "paid": true, "captured": true, "status": "succeeded", "metadata": map[string]string{"undergrid_payment_id": p.ID, "undergrid_quote_hash": p.Hash}})
			return
		}
		t.Errorf("unexpected fixture provider path %s", r.URL.Path)
		w.WriteHeader(404)
	}))
	defer provider.Close()
	backend := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(provider.URL), HTTPClient: provider.Client(), MaxNetworkRetries: stripe.Int64(0)})
	client := stripe.NewClient("sk_test_fixture", stripe.WithBackends(&stripe.Backends{API: backend}))
	cfg := StripeConfig{SecretKey: "sk_test_fixture", WebhookSecret: "whsec_fixture", AccountID: accountID, Product: "platform-test", PriceID: "price_fixture", StripeProductID: "prod_fixture", SuccessURL: "https://sandbox-fixture.invalid/success", CancelURL: "https://sandbox-fixture.invalid/cancel"}
	service, err := newStripeService(s, cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.VerifyAccount(ctx); err != nil {
		t.Fatal(err)
	}
	body := encode(map[string]any{"id": eventID, "object": "event", "api_version": stripe.APIVersion, "type": "charge.succeeded", "livemode": false, "data": map[string]any{"object": map[string]any{"id": chargeID, "amount": 999999, "metadata": map[string]string{"undergrid_payment_id": randomID()}}}})
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: body, Secret: cfg.WebhookSecret})
	call := func(raw []byte, sig string) int {
		r := httptest.NewRequest("POST", "/v1/provider/stripe/events", bytes.NewReader(raw))
		r.Header.Set("Stripe-Signature", sig)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		service.ServeHTTP(w, r)
		return w.Code
	}
	if code := call(body, signed.Header); code != 200 {
		t.Fatalf("signed durable ingest: %d", code)
	}
	if code := call(body, signed.Header); code != 200 {
		t.Fatalf("signed exact retry: %d", code)
	}
	if code := call(append(body, ' '), signed.Header); code != 400 {
		t.Fatalf("mutated raw bytes accepted: %d", code)
	}
	stale := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: body, Secret: cfg.WebhookSecret, Timestamp: time.Now().Add(-10 * time.Minute)})
	if code := call(body, stale.Header); code != 400 {
		t.Fatalf("stale signature accepted: %d", code)
	}
	var count int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_stripe_inbox WHERE event_id=$1", eventID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("durable dedupe: %d %v", count, err)
	}
	if err = service.ProcessEvents(ctx); err != nil {
		t.Fatal(err)
	}
	if err = service.ProcessEvents(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetPayment(ctx, token, p.ID)
	if err != nil || got.State != "settled" {
		t.Fatalf("verified fixture object did not settle: %+v %v", got, err)
	}
	var amount string
	if err = db.QueryRow(ctx, "SELECT amount::text FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1", p.ID).Scan(&amount); err != nil || amount != "100" {
		t.Fatalf("payload supplied authority/amount: %s %v", amount, err)
	}
	var evidenceClass string
	if err = db.QueryRow(ctx, "SELECT evidence->>'verified_by' FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1", p.ID).Scan(&evidenceClass); err != nil || evidenceClass != "stripe_api_fixture" {
		t.Fatalf("fixture mislabeled provider evidence: %s %v", evidenceClass, err)
	}
	live := bytes.Replace(body, []byte(`"livemode":false`), []byte(`"livemode":true`), 1)
	liveSigned := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: live, Secret: cfg.WebhookSecret})
	if code := call(live, liveSigned.Header); code != 400 {
		t.Fatalf("live event accepted: %d", code)
	}
	cfg.SecretKey = "sk_live_rejected"
	if _, err = newStripeService(s, cfg, client); err == nil {
		t.Fatal("live credential accepted")
	}
}

func TestStripeCheckoutClaimCommitsBeforeIOAndUnknownNeverResubmits(t *testing.T) {
	s, db, owner, token, old, _ := refundFixture(t)
	ctx := context.Background()
	accountID := "acct_" + randomID()
	if err := s.SetPolicy(ctx, owner, 2, Policy{Asset: "USD", Cap: "1000", Products: []string{"platform-test"}, Payees: []string{accountID}, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	q := old.Quote
	q.Amount = "100"
	q.Payee = accountID
	q.ExpiresAt = time.Now().Add(time.Hour)
	p, err := s.CreateQuote(ctx, token, "checkout", q)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorize(ctx, token, p.ID, p.Hash, "checkout"); err != nil {
		t.Fatal(err)
	}
	var posts atomic.Int32
	sessionID := "cs_test_" + randomID()
	checkoutURL := "https://checkout.stripe.com/c/pay/" + sessionID
	session := map[string]any{"id": sessionID, "object": "checkout.session", "livemode": false, "mode": "payment", "currency": "usd", "amount_total": 100, "client_reference_id": p.ID, "metadata": map[string]string{"undergrid_payment_id": p.ID, "undergrid_quote_hash": p.Hash}, "url": checkoutURL, "payment_status": "unpaid", "status": "open"}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/account":
			json.NewEncoder(w).Encode(map[string]any{"id": accountID, "object": "account"})
		case "/v1/prices/price_fixture":
			fmt.Fprint(w, `{"id":"price_fixture","object":"price","livemode":false,"active":true,"currency":"usd","unit_amount":100,"type":"one_time","product":"prod_fixture"}`)
		case "/v1/checkout/sessions":
			if r.Method == "POST" {
				posts.Add(1)
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Header.Get("Idempotency-Key") != "ugs-"+p.ID || r.Form.Get("line_items[0][price]") != "price_fixture" || r.Form.Get("mode") != "payment" || r.Form.Get("payment_intent_data[metadata][undergrid_quote_hash]") != p.Hash {
					t.Errorf("unbound provider request: %s %v", r.Header.Get("Idempotency-Key"), r.Form)
				}
				var state string
				if err := db.QueryRow(ctx, "SELECT state FROM undergrid_sandbox.sandbox_payments WHERE id=$1", p.ID).Scan(&state); err != nil || state != "submitting" {
					t.Errorf("provider called before durable claim: %s %v", state, err)
				}
				w.WriteHeader(502)
				fmt.Fprint(w, `{"error":{"type":"api_error","message":"fixture response lost"}}`)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"object": "list", "has_more": false, "data": []any{session}})
		default:
			t.Errorf("unexpected provider path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer provider.Close()
	backend := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(provider.URL), HTTPClient: provider.Client(), MaxNetworkRetries: stripe.Int64(0)})
	cfg := StripeConfig{SecretKey: "sk_test_fixture", WebhookSecret: "whsec_fixture", AccountID: accountID, Product: "platform-test", PriceID: "price_fixture", StripeProductID: "prod_fixture", SuccessURL: "https://sandbox-fixture.invalid/success", CancelURL: "https://sandbox-fixture.invalid/cancel"}
	service, err := newStripeService(s, cfg, stripe.NewClient(cfg.SecretKey, stripe.WithBackends(&stripe.Backends{API: backend})))
	if err != nil {
		t.Fatal(err)
	}
	if err = service.VerifyAccount(ctx); err != nil {
		t.Fatal(err)
	}
	if err = service.DispatchPayment(ctx, p.ID); err == nil {
		t.Fatal("unknown provider outcome reported success")
	}
	got, err := s.GetPayment(ctx, token, p.ID)
	if err != nil || got.State != "unknown" {
		t.Fatalf("lost response released reservation: %+v %v", got, err)
	}
	if err = service.DispatchPayment(ctx, p.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("unknown outcome redispatched: %v", err)
	}
	if posts.Load() != 1 {
		t.Fatalf("provider retried create after unknown: %d", posts.Load())
	}
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- service.RunWorker(workerCtx) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, err = s.GetPayment(ctx, token, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.State == "pending" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not recover unknown creation")
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
		t.Fatal("Stripe worker did not stop")
	}

	got, err = s.GetPayment(ctx, token, p.ID)
	if err != nil || got.State != "pending" || got.CheckoutURL != checkoutURL {
		t.Fatalf("unknown recovery did not bind original session: %+v %v", got, err)
	}
	if posts.Load() != 1 {
		t.Fatal("reconciliation created a new charge")
	}
}

func TestStripeRefundDispatchKeepsIndependentUnknownAuthority(t *testing.T) {
	s, db, owner, token, old, _ := refundFixture(t)
	ctx := context.Background()
	accountID := "acct_" + randomID()
	policy := Policy{Asset: "USD", Cap: "1000", Products: []string{"platform-test"}, Payees: []string{accountID}, ExpiresAt: time.Now().Add(time.Hour), AllowRefunds: true}
	if err := s.SetPolicy(ctx, owner, 2, policy); err != nil {
		t.Fatal(err)
	}
	q := old.Quote
	q.Payee = accountID
	q.Amount = "100"
	q.ExpiresAt = time.Now().Add(time.Hour)
	p, err := s.CreateQuote(ctx, token, "refund-provider", q)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorize(ctx, token, p.ID, p.Hash, "refund-provider"); err != nil {
		t.Fatal(err)
	}
	chargeID := "ch_" + randomID()
	refundID := "re_" + randomID()
	var refund Refund
	var posts atomic.Int32
	var badRetrievedHash atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/account":
			json.NewEncoder(w).Encode(map[string]any{"id": accountID, "object": "account"})
		case "/v1/charges/" + chargeID:
			json.NewEncoder(w).Encode(map[string]any{"id": chargeID, "object": "charge", "livemode": false, "amount": 100, "amount_captured": 100, "currency": "usd", "paid": true, "captured": true, "status": "succeeded", "metadata": map[string]string{"undergrid_payment_id": p.ID, "undergrid_quote_hash": p.Hash}})
		case "/v1/refunds":
			if r.Method == "POST" {
				posts.Add(1)
				r.ParseForm()
				if r.Form.Get("charge") != chargeID || r.Form.Get("amount") != "40" || r.Header.Get("Idempotency-Key") != refund.ProviderKey {
					t.Errorf("unbound refund request: %v", r.Form)
				}
				var state string
				if err := db.QueryRow(ctx, "SELECT state FROM undergrid_sandbox.sandbox_refunds WHERE id=$1", refund.ID).Scan(&state); err != nil || state != "pending" {
					t.Errorf("refund IO before claim commit: %s %v", state, err)
				}
				w.WriteHeader(502)
				fmt.Fprint(w, `{"error":{"type":"api_error","message":"fixture response lost"}}`)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"object": "list", "has_more": false, "data": []any{map[string]any{"id": refundID, "object": "refund", "charge": chargeID, "amount": 40, "currency": "usd", "status": "succeeded", "metadata": map[string]string{"undergrid_refund_id": refund.ID, "undergrid_refund_hash": refund.Hash}}}})
		case "/v1/refunds/" + refundID:
			objectHash := refund.Hash
			if badRetrievedHash.Load() {
				objectHash = "mismatched-exact-consent"
			}
			json.NewEncoder(w).Encode(map[string]any{"id": refundID, "object": "refund", "charge": chargeID, "amount": 40, "currency": "usd", "status": "succeeded", "metadata": map[string]string{"undergrid_refund_id": refund.ID, "undergrid_refund_hash": objectHash}})
		default:
			t.Errorf("unexpected refund fixture URL %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer provider.Close()
	backend := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(provider.URL), HTTPClient: provider.Client(), MaxNetworkRetries: stripe.Int64(0)})
	cfg := StripeConfig{SecretKey: "sk_test_fixture", WebhookSecret: "whsec_fixture", AccountID: accountID, Product: "platform-test", PriceID: "price_fixture", StripeProductID: "prod_fixture", SuccessURL: "https://sandbox-fixture.invalid/success", CancelURL: "https://sandbox-fixture.invalid/cancel"}
	service, err := newStripeService(s, cfg, stripe.NewClient(cfg.SecretKey, stripe.WithBackends(&stripe.Backends{API: backend})))
	if err != nil {
		t.Fatal(err)
	}
	if err = service.VerifyAccount(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.verifyCharge(ctx, chargeID, "fixture-object/"+chargeID); err != nil {
		t.Fatal(err)
	}
	refundToken, err := s.IssueCredential(ctx, owner, "refund-agent", []string{"refunds:request", "refunds:authorize", "refunds:read"})
	if err != nil {
		t.Fatal(err)
	}
	refund, err = s.RequestRefund(ctx, refundToken, "partial", p.ID, "40")
	if err != nil {
		t.Fatal(err)
	}
	refund, err = s.AuthorizeRefund(ctx, refundToken, refund.ID, refund.Hash, "partial")
	if err != nil {
		t.Fatal(err)
	}
	if err = service.DispatchRefund(ctx, refund.ID); err == nil {
		t.Fatal("lost refund response reported success")
	}
	unknown, err := s.GetRefund(ctx, refundToken, refund.ID)
	if err != nil || unknown.State != "unknown" {
		t.Fatalf("refund unknown lost: %+v %v", unknown, err)
	}
	if err = service.DispatchRefund(ctx, refund.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("unknown refund redispatched: %v", err)
	}
	if err = s.RevokeCredentials(ctx, owner); err != nil {
		t.Fatal(err)
	}
	badRetrievedHash.Store(true)
	if err = service.ReconcileUnknownRefund(ctx, refund.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("provider object with mismatched consent hash accepted: %v", err)
	}
	badRetrievedHash.Store(false)
	if err = service.ReconcileUnknownRefund(ctx, refund.ID); err != nil {
		t.Fatal(err)
	}
	var state string
	var total string
	if err = db.QueryRow(ctx, "SELECT state FROM undergrid_sandbox.sandbox_refunds WHERE id=$1", refund.ID).Scan(&state); err != nil || state != "succeeded" {
		t.Fatalf("late refund lost after revoke: %s %v", state, err)
	}
	if err = db.QueryRow(ctx, "SELECT sum(amount)::text FROM undergrid_sandbox.sandbox_effects WHERE refund_id=$1", refund.ID).Scan(&total); err != nil || total != "40" {
		t.Fatalf("partial refund object not recorded: %s %v", total, err)
	}
	if posts.Load() != 1 {
		t.Fatalf("duplicate refund POST: %d", posts.Load())
	}
}

func TestStripeVerifiedCanceledIntentReleasesOnlyUnsettledReservation(t *testing.T) {
	s, db, owner, token, old, _ := refundFixture(t)
	ctx := context.Background()
	accountID := "acct_" + randomID()
	if err := s.SetPolicy(ctx, owner, 2, Policy{Asset: "USD", Cap: "1000", Products: []string{"platform-test"}, Payees: []string{accountID}, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	q := old.Quote
	q.Payee = accountID
	q.Amount = "100"
	q.ExpiresAt = time.Now().Add(time.Hour)
	p, err := s.CreateQuote(ctx, token, "failure", q)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorize(ctx, token, p.ID, p.Hash, "failure"); err != nil {
		t.Fatal(err)
	}
	intentID := "pi_" + randomID()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/account":
			json.NewEncoder(w).Encode(map[string]any{"id": accountID, "object": "account"})
		case "/v1/payment_intents/" + intentID:
			json.NewEncoder(w).Encode(map[string]any{"id": intentID, "object": "payment_intent", "livemode": false, "amount": 100, "currency": "usd", "status": "canceled", "metadata": map[string]string{"undergrid_payment_id": p.ID, "undergrid_quote_hash": p.Hash}})
		default:
			t.Errorf("unexpected failure fixture URL %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer provider.Close()
	backend := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(provider.URL), HTTPClient: provider.Client(), MaxNetworkRetries: stripe.Int64(0)})
	cfg := StripeConfig{SecretKey: "sk_test_fixture", WebhookSecret: "whsec_fixture", AccountID: accountID, Product: "platform-test", PriceID: "price_fixture", StripeProductID: "prod_fixture", SuccessURL: "https://sandbox-fixture.invalid/success", CancelURL: "https://sandbox-fixture.invalid/cancel"}
	service, err := newStripeService(s, cfg, stripe.NewClient(cfg.SecretKey, stripe.WithBackends(&stripe.Backends{API: backend})))
	if err != nil {
		t.Fatal(err)
	}
	if err = service.VerifyAccount(ctx); err != nil {
		t.Fatal(err)
	}
	if err = service.reconcileEvent(ctx, stripeInboxEvent{ID: "evt_" + randomID(), Type: "payment_intent.canceled", ObjectID: intentID}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetPayment(ctx, token, p.ID)
	if err != nil || got.State != "failed" {
		t.Fatalf("verified cancellation did not terminate reservation: %+v %v", got, err)
	}
	var effects int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1", p.ID).Scan(&effects); err != nil || effects != 0 {
		t.Fatalf("failure fabricated charge: %d %v", effects, err)
	}
}
