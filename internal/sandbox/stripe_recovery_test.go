package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	stripe "github.com/stripe/stripe-go/v87"
)

// This fixture models one trusted test account's canonical object graph. It is
// explicitly a provider fixture, never actual Stripe account qualification.
type stripeRecoveryFixture struct {
	store                                                           *Store
	db                                                              *pgx.Conn
	owner                                                           Owner
	token                                                           string
	p                                                               Payment
	service                                                         *StripeService
	account, sessionID, intentID, otherIntentID, chargeID, refundID string
	mode                                                            atomic.Int32 // 0 open, 1 paid complete, 2 expired/canceled, 3 wrapper mismatch
	refundMode                                                      atomic.Int32 // 0 pending, 1 succeeded, 2 canceled, 3 wrong consent
	lost                                                            atomic.Bool
	posts                                                           atomic.Int32
	sessionGets                                                     atomic.Int32
	chargeBinding                                                   atomic.Int32 // 0 exact PI, 1 absent, 2 unrelated
	wrongRetrievedSession                                           atomic.Bool
	refund                                                          atomic.Value
}

func newStripeRecoveryFixture(t *testing.T) *stripeRecoveryFixture {
	t.Helper()
	s, db, owner, token, old, _ := refundFixture(t)
	ctx := context.Background()
	f := &stripeRecoveryFixture{store: s, db: db, owner: owner, token: token, account: "acct_" + randomID(), sessionID: "cs_test_" + randomID(), intentID: "pi_" + randomID(), otherIntentID: "pi_" + randomID(), chargeID: "ch_" + randomID(), refundID: "re_" + randomID()}
	f.refund.Store(Refund{})
	if err := s.SetPolicy(ctx, owner, 2, Policy{Asset: "USD", Cap: "1000", Products: []string{"platform-test"}, Payees: []string{f.account}, ExpiresAt: time.Now().Add(time.Hour), AllowRefunds: true}); err != nil {
		t.Fatal(err)
	}
	q := old.Quote
	q.Payee = f.account
	q.Amount = "100"
	q.ExpiresAt = time.Now().Add(time.Hour)
	p, err := s.CreateQuote(ctx, token, "recovery", q)
	if err != nil {
		t.Fatal(err)
	}
	f.p = p
	if _, err = s.Authorize(ctx, token, p.ID, p.Hash, "recovery"); err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		write := func(v any) { json.NewEncoder(w).Encode(v) }
		charge := map[string]any{"id": f.chargeID, "object": "charge", "livemode": false, "amount": 100, "amount_captured": 100, "currency": "usd", "paid": true, "captured": true, "status": "succeeded", "payment_intent": f.intentID, "metadata": map[string]string{"undergrid_payment_id": p.ID, "undergrid_quote_hash": p.Hash}}
		if f.chargeBinding.Load() == 1 {
			delete(charge, "payment_intent")
		}
		if f.chargeBinding.Load() == 2 {
			charge["payment_intent"] = f.otherIntentID
		}
		intent := func(id, status string) any {
			return map[string]any{"id": id, "object": "payment_intent", "livemode": false, "amount": 100, "currency": "usd", "status": status, "metadata": map[string]string{"undergrid_payment_id": p.ID, "undergrid_quote_hash": p.Hash}, "latest_charge": func() any {
				if status == "succeeded" {
					return charge
				}
				return nil
			}()}
		}
		session := func() any {
			hash := p.Hash
			status, pay, url := "open", "unpaid", any("https://checkout.stripe.com/c/pay/"+f.sessionID)
			piStatus := "requires_payment_method"
			if f.mode.Load() == 1 || f.mode.Load() == 3 {
				status, pay, url, piStatus = "complete", "paid", nil, "succeeded"
			}
			if f.mode.Load() == 2 {
				status, pay, url, piStatus = "expired", "unpaid", nil, "canceled"
			}
			if f.mode.Load() == 3 {
				hash = "wrong-session-binding"
			}
			return map[string]any{"id": f.sessionID, "object": "checkout.session", "livemode": false, "mode": "payment", "currency": "usd", "amount_total": 100, "client_reference_id": p.ID, "metadata": map[string]string{"undergrid_payment_id": p.ID, "undergrid_quote_hash": hash}, "status": status, "payment_status": pay, "url": url, "payment_intent": intent(f.intentID, piStatus)}
		}
		refund := func() any {
			r := f.refund.Load().(Refund)
			status, hash := "pending", r.Hash
			switch f.refundMode.Load() {
			case 1:
				status = "succeeded"
			case 2:
				status = "canceled"
			case 3:
				status = "succeeded"
				hash = "wrong-refund-binding"
			}
			return map[string]any{"id": f.refundID, "object": "refund", "charge": f.chargeID, "amount": 40, "currency": "usd", "status": status, "metadata": map[string]string{"undergrid_refund_id": r.ID, "undergrid_refund_hash": hash}}
		}
		switch r.URL.Path {
		case "/v1/account":
			write(map[string]any{"id": f.account, "object": "account"})
		case "/v1/prices/price_fixture":
			write(map[string]any{"id": "price_fixture", "object": "price", "livemode": false, "active": true, "currency": "usd", "unit_amount": 100, "type": "one_time", "product": "prod_fixture"})
		case "/v1/checkout/sessions":
			if r.Method == "POST" {
				f.posts.Add(1)
				if f.lost.Load() {
					w.WriteHeader(502)
					fmt.Fprint(w, `{"error":{"type":"api_error","message":"fixture lost response"}}`)
					return
				}
				write(session())
			} else {
				listed := session().(map[string]any)
				listed["payment_intent"] = f.intentID // unexpanded list relationship
				write(map[string]any{"object": "list", "has_more": false, "data": []any{listed}})
			}
		case "/v1/checkout/sessions/" + f.sessionID:
			f.sessionGets.Add(1)
			retrieved := session().(map[string]any)
			if f.wrongRetrievedSession.Load() {
				retrieved["id"] = "cs_test_unrelated"
			}
			write(retrieved)
		case "/v1/payment_intents/" + f.intentID:
			write(intent(f.intentID, "canceled"))
		case "/v1/payment_intents/" + f.otherIntentID:
			write(intent(f.otherIntentID, "canceled"))
		case "/v1/charges/" + f.chargeID:
			write(charge)
		case "/v1/refunds":
			write(refund())
		case "/v1/refunds/" + f.refundID:
			write(refund())
		default:
			t.Errorf("unexpected recovery fixture path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(provider.Close)
	backend := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(provider.URL), HTTPClient: provider.Client(), MaxNetworkRetries: stripe.Int64(0), LeveledLogger: &stripe.LeveledLogger{Level: stripe.LevelNull}})
	cfg := StripeConfig{SecretKey: "sk_test_fixture", WebhookSecret: "whsec_fixture", AccountID: f.account, Product: "platform-test", PriceID: "price_fixture", StripeProductID: "prod_fixture", SuccessURL: "https://sandbox-fixture.invalid/success", CancelURL: "https://sandbox-fixture.invalid/cancel"}
	f.service, err = newStripeService(s, cfg, stripe.NewClient(cfg.SecretKey, stripe.WithBackends(&stripe.Backends{API: backend})))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.VerifyAccount(ctx); err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *stripeRecoveryFixture) payment(t *testing.T) Payment {
	t.Helper()
	p, err := f.store.GetPayment(context.Background(), f.token, f.p.ID)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStripeWorkerRecoversKnownClosedSessionAfterMissingWebhook(t *testing.T) {
	f := newStripeRecoveryFixture(t)
	ctx := context.Background()
	if err := f.service.DispatchPayment(ctx, f.p.ID); err != nil {
		t.Fatal(err)
	}
	f.mode.Store(1)
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- f.service.RunWorker(workerCtx) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("recovery worker did not stop")
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		p := f.payment(t)
		if p.State == "settled" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("known completed session unreconciled: %s", p.State)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if f.posts.Load() != 1 {
		t.Fatal("pending reconciliation redispatched creation")
	}
}
func TestStripeUnknownClosedSessionRecoveryDoesNotNeedActiveURL(t *testing.T) {
	f := newStripeRecoveryFixture(t)
	ctx := context.Background()
	f.lost.Store(true)
	if err := f.service.DispatchPayment(ctx, f.p.ID); err == nil {
		t.Fatal("lost response reported successful")
	}
	f.mode.Store(1)
	if err := f.service.ReconcileUnknown(ctx, f.p.ID); err != nil {
		t.Fatalf("completed session without active URL: %v", err)
	}
	if p := f.payment(t); p.State != "settled" || p.CheckoutURL != "" {
		t.Fatalf("closed session recovery: %+v", p)
	}
	if f.posts.Load() != 1 {
		t.Fatal("unknown creation reposted")
	}
	if f.sessionGets.Load() == 0 {
		t.Fatal("unknown recovery used an unexpanded list object as canonical session")
	}
}

func TestStripeCompletedSessionRequiresExactChargeIntentBinding(t *testing.T) {
	for _, binding := range []int32{1, 2} {
		t.Run(fmt.Sprint(binding), func(t *testing.T) {
			f := newStripeRecoveryFixture(t)
			ctx := context.Background()
			if err := f.service.DispatchPayment(ctx, f.p.ID); err != nil {
				t.Fatal(err)
			}
			f.mode.Store(1)
			f.chargeBinding.Store(binding)
			if err := f.service.ReconcilePayment(ctx, f.p.ID); !errors.Is(err, ErrConflict) {
				t.Fatalf("mismatched charge intent accepted without classification: %v", err)
			}
			p := f.payment(t)
			if len(p.Anomalies) != 1 || p.Anomalies[0] != "checkout_binding_mismatch" {
				t.Fatalf("canonical charge mismatch hidden: %+v", p)
			}
		})
	}
}

func TestStripeUnknownSessionRetrievalMustMatchListIdentity(t *testing.T) {
	f := newStripeRecoveryFixture(t)
	ctx := context.Background()
	f.lost.Store(true)
	if err := f.service.DispatchPayment(ctx, f.p.ID); err == nil {
		t.Fatal("expected unknown creation")
	}
	f.mode.Store(1)
	f.wrongRetrievedSession.Store(true)
	if err := f.service.ReconcileUnknown(ctx, f.p.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("canonical retrieval changed object identity: %v", err)
	}
	if p := f.payment(t); p.State != "unknown" {
		t.Fatalf("wrong canonical session mutated payment: %+v", p)
	}
	var facts int
	if err := f.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1", f.p.ID).Scan(&facts); err != nil || facts != 0 {
		t.Fatalf("wrong session wrote monetary facts: %d %v", facts, err)
	}
}
func TestStripeCanceledIntentMustBelongToStoredSession(t *testing.T) {
	f := newStripeRecoveryFixture(t)
	ctx := context.Background()
	if err := f.service.DispatchPayment(ctx, f.p.ID); err != nil {
		t.Fatal(err)
	}
	err := f.service.reconcileEvent(ctx, stripeInboxEvent{ID: "evt_" + randomID(), Type: "payment_intent.canceled", ObjectID: f.otherIntentID})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("unrelated canceled intent with copied metadata: %v", err)
	}
	if p := f.payment(t); p.State != "pending" {
		t.Fatalf("unrelated cancellation released reservation: %+v", p)
	}
	f.mode.Store(2)
	if err = f.service.reconcileEvent(ctx, stripeInboxEvent{ID: "evt_" + randomID(), Type: "payment_intent.canceled", ObjectID: f.intentID}); err != nil {
		t.Fatal(err)
	}
	if p := f.payment(t); p.State != "failed" {
		t.Fatalf("bound terminal cancellation: %+v", p)
	}
}
func TestStripeCanceledRefundReleasesOnlyBoundReservation(t *testing.T) {
	f := newStripeRecoveryFixture(t)
	ctx := context.Background()
	if _, _, err := f.service.verifyCharge(ctx, f.chargeID, "fixture-charge/"+f.chargeID); err != nil {
		t.Fatal(err)
	}
	token, err := f.store.IssueCredential(ctx, f.owner, "refund", []string{"refunds:request", "refunds:authorize", "refunds:read"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.store.RequestRefund(ctx, token, "refund", f.p.ID, "40")
	if err != nil {
		t.Fatal(err)
	}
	r, err = f.store.AuthorizeRefund(ctx, token, r.ID, r.Hash, "refund")
	if err != nil {
		t.Fatal(err)
	}
	f.refund.Store(r)
	if err = f.service.DispatchRefund(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	f.refundMode.Store(2)
	if err = f.service.reconcileEvent(ctx, stripeInboxEvent{ID: "evt_" + randomID(), Type: "refund.updated", ObjectID: f.refundID}); err != nil {
		t.Fatal(err)
	}
	got, err := f.store.GetRefund(ctx, token, r.ID)
	if err != nil || got.State != "failed" {
		t.Fatalf("canceled refund retained reservation: %+v %v", got, err)
	}
	var effects int
	if err = f.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE refund_id=$1", r.ID).Scan(&effects); err != nil || effects != 0 {
		t.Fatalf("cancellation invented refund effect: %d %v", effects, err)
	}
	next, err := f.store.RequestRefund(ctx, token, "new", f.p.ID, "100")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.AuthorizeRefund(ctx, token, next.ID, next.Hash, "new"); err != nil {
		t.Fatalf("verified cancellation retained reserved capacity: %v", err)
	}
}
func TestStripeWrapperMismatchRecordsTruthWithExplicitAnomaly(t *testing.T) {
	f := newStripeRecoveryFixture(t)
	ctx := context.Background()
	if err := f.service.DispatchPayment(ctx, f.p.ID); err != nil {
		t.Fatal(err)
	}
	f.mode.Store(3)
	err := f.service.reconcileEvent(ctx, stripeInboxEvent{ID: "evt_" + randomID(), Type: "checkout.session.completed", ObjectID: f.sessionID})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("mismatched enclosing session accepted: %v", err)
	}
	var effects, anomalies int
	if err = f.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1 AND kind='charge'", f.p.ID).Scan(&effects); err != nil || effects != 1 {
		t.Fatalf("canonical charge truth discarded: %d %v", effects, err)
	}
	if err = f.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_anomalies WHERE payment_id=$1 AND kind='checkout_binding_mismatch'", f.p.ID).Scan(&anomalies); err != nil || anomalies != 1 {
		t.Fatalf("verified mismatch unflagged: %d %v", anomalies, err)
	}
	if p := f.payment(t); len(p.Anomalies) != 1 || p.Anomalies[0] != "checkout_binding_mismatch" {
		t.Fatalf("owner cannot inspect classification: %+v", p)
	}
}
