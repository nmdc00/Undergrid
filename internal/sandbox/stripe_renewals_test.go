package sandbox

import (
	"bytes"
	"context"
	"github.com/jackc/pgx/v5"
	stripe "github.com/stripe/stripe-go/v87"
	"github.com/stripe/stripe-go/v87/webhook"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStripeRenewalBillingSetupRequiresExactOwnerConsent(t *testing.T) {
	f := newStripeRecoveryFixture(t)
	ctx := context.Background()
	var credential string
	if err := f.db.QueryRow(ctx, "SELECT id::text FROM undergrid_sandbox.sandbox_credentials WHERE digest=$1", hash([]byte(f.token))).Scan(&credential); err != nil {
		t.Fatal(err)
	}
	app := New(usdcFixtureRoleDSN(t, f.db, "undergrid_sandbox_app"))
	sub, err := app.CreateSubscription(ctx, f.owner, "billing-consent", SubscriptionConsent{CredentialID: credential, Maximum: "100", PeriodSeconds: 86400, RenewalCount: 2, ExpiresAt: time.Now().Add(72 * time.Hour)}, f.p.Quote)
	if err != nil {
		t.Fatal(err)
	}
	h := mustWalletHandler(t, app, func(r *http.Request) (Owner, error) {
		if r.Header.Get("X-Fixture-Owner") != f.owner.Subject {
			return Owner{}, ErrUnauthorized
		}
		return f.owner, nil
	}, map[string]Quote{"platform-test": f.p.Quote})
	path := "/v1/owner/subscriptions/" + sub.ID + "/billing-setup"
	body := map[string]any{"consent_hash": sub.Hash}
	if status, _ := refundCall(t, h, f.token, "", "POST", path, "agent-cannot-save-card", body); status != 401 {
		t.Fatalf("agent requested billing setup: %d", status)
	}
	status, out := refundCall(t, h, "", f.owner.Subject, "POST", path, "owner-card-setup", body)
	if status != 201 || out["state"] != "requested" {
		t.Fatalf("owner exact consent setup: %d %v", status, out)
	}
	status, replay := refundCall(t, h, "", f.owner.Subject, "POST", path, "owner-card-setup", body)
	if status != 201 || replay["id"] != out["id"] {
		t.Fatalf("setup replay: %d %v", status, replay)
	}
	if status, _ = refundCall(t, h, "", f.owner.Subject, "POST", path, "changed-consent", map[string]any{"consent_hash": hash([]byte("other"))}); status != 409 {
		t.Fatalf("setup accepted changed consent: %d", status)
	}
	if status, _ = refundCall(t, h, "", f.owner.Subject, "GET", path, "", nil); status != 200 {
		t.Fatalf("owner setup read: %d", status)
	}
	if status, _ = refundCall(t, h, "", f.owner.Subject, "POST", path, "client-provider-ids", map[string]any{"consent_hash": sub.Hash, "customer_id": "cus_unrelated", "payment_method_id": "pm_unrelated"}); status != 400 {
		t.Fatalf("client billing authority accepted: %d", status)
	}
}

func TestStripeRenewalHostedSetupBindsRetrievedPayer(t *testing.T) {
	f := newRenewalProviderFixture(t)
	if err := f.service.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, err := f.app.StripeBillingSetup(context.Background(), f.base.owner, f.sub.ID)
	if err != nil || b.State != "pending" || b.URL == "" {
		t.Fatalf("hosted setup not prepared: %+v %v", b, err)
	}
	f.complete.Store(true)
	if err = f.service.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, err = f.app.StripeBillingSetup(context.Background(), f.base.owner, f.sub.ID)
	if err != nil || b.State != "ready" {
		t.Fatalf("canonical hosted setup not ready: %+v %v", b, err)
	}
	if f.posts("/v1/customers") != 1 || f.posts("/v1/checkout/sessions") != 1 {
		t.Fatal("setup created more than once")
	}
	for range 2 {
		if err = f.service.tick(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if f.posts("/v1/customers") != 1 || f.posts("/v1/checkout/sessions") != 1 {
		t.Fatal("ready setup replayed provider creation")
	}
}

func TestStripeRenewalSetupLostResponsesNeverRepeatCreation(t *testing.T) {
	for _, lost := range []string{"customer", "session"} {
		t.Run(lost, func(t *testing.T) {
			f := newRenewalProviderFixture(t)
			if lost == "customer" {
				f.loseCustomer.Store(true)
			} else {
				f.loseSession.Store(true)
			}
			if err := f.service.tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			b, err := f.app.StripeBillingSetup(context.Background(), f.base.owner, f.sub.ID)
			if err != nil {
				t.Fatal(err)
			}
			if b.State != lost+"_unknown" {
				t.Fatalf("lost response state=%s", b.State)
			}
			f.complete.Store(true)
			for range 2 {
				if err = f.service.tick(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			b, err = f.app.StripeBillingSetup(context.Background(), f.base.owner, f.sub.ID)
			if err != nil || b.State != "ready" || f.posts("/v1/customers") != 1 || f.posts("/v1/checkout/sessions") != 1 {
				t.Fatalf("unknown setup repeated creation: state=%s customer=%d session=%d err=%v", b.State, f.posts("/v1/customers"), f.posts("/v1/checkout/sessions"), err)
			}
		})
	}
}
func TestStripeRenewalSetupRejectsUnrelatedPayerEvidence(t *testing.T) {
	for _, fault := range []string{"setup_customer", "setup_consent", "method_customer", "live_method"} {
		t.Run(fault, func(t *testing.T) {
			f := newRenewalProviderFixture(t)
			if err := f.service.tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			f.complete.Store(true)
			f.fault.Store(fault)
			if err := f.service.tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			b, err := f.app.StripeBillingSetup(context.Background(), f.base.owner, f.sub.ID)
			if err != nil || b.State != "pending" {
				t.Fatalf("unrelated payer proof granted ready: %+v %v", b, err)
			}
			var effects int
			if err = f.base.db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1", f.base.p.ID).Scan(&effects); err != nil || effects != 0 {
				t.Fatalf("setup invented money effects=%d err=%v", effects, err)
			}
		})
	}
}

func TestStripeRenewalCollectionIsBoundedManualAndCanonical(t *testing.T) {
	f := newRenewalProviderFixture(t)
	ctx := context.Background()
	if err := f.service.tick(ctx); err != nil {
		t.Fatal(err)
	}
	f.complete.Store(true)
	if err := f.service.tick(ctx); err != nil {
		t.Fatal(err)
	}
	for ordinal := 1; ordinal <= 2; ordinal++ {
		if _, err := f.base.db.Exec(ctx, "UPDATE undergrid_sandbox.sandbox_subscriptions SET next_due_at=clock_timestamp()-interval '1 second' WHERE id=$1", f.sub.ID); err != nil {
			t.Fatal(err)
		}
		p, err := f.service.store.PrepareRenewal(ctx, f.sub.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.service.tick(ctx); err != nil {
			t.Fatal(err)
		}
		var state, charge string
		var facts int
		if err = f.base.db.QueryRow(ctx, "SELECT state,COALESCE(provider_reference,''),(SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=p.id AND kind='charge') FROM undergrid_sandbox.sandbox_payments p WHERE id=$1", p.ID).Scan(&state, &charge, &facts); err != nil {
			t.Fatal(err)
		}
		if state != "settled" || facts != 1 || charge == "" {
			t.Fatalf("renewal %d not canonically collected: state=%s charge=%s facts=%d", ordinal, state, charge, facts)
		}
		for range 2 {
			if err = f.service.tick(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := f.service.store.PrepareRenewal(ctx, f.sub.ID); err != ErrConflict {
		t.Fatalf("third renewal: %v", err)
	}
	if f.posts("/v1/invoices") != 2 || f.posts("/v1/invoiceitems") != 2 {
		t.Fatalf("unbounded or duplicate invoices: invoice=%d items=%d", f.posts("/v1/invoices"), f.posts("/v1/invoiceitems"))
	}
}

func (f *renewalProviderFixture) prepare(t *testing.T) Payment {
	t.Helper()
	ctx := context.Background()
	if err := f.service.tick(ctx); err != nil {
		t.Fatal(err)
	}
	f.complete.Store(true)
	if err := f.service.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.base.db.Exec(ctx, "UPDATE undergrid_sandbox.sandbox_subscriptions SET next_due_at=clock_timestamp()-interval '1 second' WHERE id=$1", f.sub.ID); err != nil {
		t.Fatal(err)
	}
	p, err := f.service.store.PrepareRenewal(ctx, f.sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestStripeRenewalUnknownResponseRecoversWithoutAnotherAttempt(t *testing.T) {
	for _, operation := range []string{"invoice", "item", "finalize", "pay"} {
		t.Run(operation, func(t *testing.T) {
			f := newRenewalProviderFixture(t)
			p := f.prepare(t)
			ctx := context.Background()
			var key, period string
			if err := f.base.db.QueryRow(ctx, "SELECT provider_key,reservation_period::text FROM undergrid_sandbox.sandbox_payments WHERE id=$1", p.ID).Scan(&key, &period); err != nil {
				t.Fatal(err)
			}
			f.loseOperation.Store(operation)
			if err := f.service.tick(ctx); err != nil {
				t.Fatal(err)
			}
			var state string
			if err := f.base.db.QueryRow(ctx, "SELECT state FROM undergrid_sandbox.sandbox_payments WHERE id=$1", p.ID).Scan(&state); err != nil || state != "unknown" {
				t.Fatalf("lost %s state=%s err=%v", operation, state, err)
			}
			if operation == "pay" {
				if _, err := f.base.db.Exec(ctx, "UPDATE undergrid_sandbox.sandbox_credentials SET revoked_at=clock_timestamp() WHERE digest=$1", hash([]byte(f.base.token))); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if err := f.service.tick(ctx); err != nil {
					t.Fatal(err)
				}
			}
			var retained bool
			var facts int
			if err := f.base.db.QueryRow(ctx, `SELECT state='settled' AND provider_key=$2 AND reservation_period::text=$3,(SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=p.id AND kind='charge') FROM undergrid_sandbox.sandbox_payments p WHERE id=$1`, p.ID, key, period).Scan(&retained, &facts); err != nil {
				t.Fatal(err)
			}
			var finalize, pay int
			f.mu.Lock()
			for path, n := range f.counts {
				if strings.HasSuffix(path, "/finalize") {
					finalize += n
				}
				if strings.HasSuffix(path, "/pay") {
					pay += n
				}
			}
			f.mu.Unlock()
			if !retained || facts != 1 || f.posts("/v1/invoices") != 1 || f.posts("/v1/invoiceitems") != 1 || finalize != 1 || pay != 1 {
				t.Fatalf("lost %s repeat/missing fact retained=%v facts=%d invoice=%d item=%d finalize=%d pay=%d", operation, retained, facts, f.posts("/v1/invoices"), f.posts("/v1/invoiceitems"), finalize, pay)
			}
		})
	}
}
func TestStripeRenewalUnknownInvisibleInvoiceKeepsReservation(t *testing.T) {
	f := newRenewalProviderFixture(t)
	p := f.prepare(t)
	ctx := context.Background()
	f.loseOperation.Store("invoice")
	f.hideInvoices.Store(true)
	for range 3 {
		if err := f.service.tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var state string
	var facts int
	if err := f.base.db.QueryRow(ctx, "SELECT state,(SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=p.id) FROM undergrid_sandbox.sandbox_payments p WHERE id=$1", p.ID).Scan(&state, &facts); err != nil {
		t.Fatal(err)
	}
	if state != "unknown" || facts != 0 || f.posts("/v1/invoices") != 1 || f.posts("/v1/invoiceitems") != 0 {
		t.Fatalf("invisible unknown released or reattempted: state=%s facts=%d creates=%d items=%d", state, facts, f.posts("/v1/invoices"), f.posts("/v1/invoiceitems"))
	}
	f.hideInvoices.Store(false)
	if err := f.service.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.base.db.QueryRow(ctx, "SELECT state FROM undergrid_sandbox.sandbox_payments WHERE id=$1", p.ID).Scan(&state); err != nil || state != "settled" {
		t.Fatalf("restored invoice recovery=%s err=%v", state, err)
	}
}
func TestStripeRenewalRechecksConsentAndCredentialBeforePay(t *testing.T) {
	for _, action := range []string{"revoked", "expired", "canceled"} {
		t.Run(action, func(t *testing.T) {
			f := newRenewalProviderFixture(t)
			p := f.prepare(t)
			f.afterFinalize.Store(action)
			ctx := context.Background()
			for range 2 {
				if err := f.service.tick(ctx); err != nil {
					t.Fatal(err)
				}
			}
			var state string
			var facts int
			if err := f.base.db.QueryRow(ctx, "SELECT state,(SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=p.id) FROM undergrid_sandbox.sandbox_payments p WHERE id=$1", p.ID).Scan(&state, &facts); err != nil {
				t.Fatal(err)
			}
			pay := 0
			f.mu.Lock()
			for path, n := range f.counts {
				if strings.HasSuffix(path, "/pay") {
					pay += n
				}
			}
			f.mu.Unlock()
			if state != "unknown" || facts != 0 || pay != 0 {
				t.Fatalf("%s after finalize still paid: state=%s facts=%d pays=%d", action, state, facts, pay)
			}
		})
	}
}

func TestStripeRenewalCanonicalInvoiceChargeSupportsRefund(t *testing.T) {
	f := newRenewalProviderFixture(t)
	p := f.prepare(t)
	ctx := context.Background()
	if err := f.service.tick(ctx); err != nil {
		t.Fatal(err)
	}
	token, err := f.app.IssueCredential(ctx, f.base.owner, "invoice-refunder", []string{"refunds:request", "refunds:authorize", "refunds:read"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.app.RequestRefund(ctx, token, "invoice-refund", p.ID, "40")
	if err != nil {
		t.Fatal(err)
	}
	r, err = f.app.AuthorizeRefund(ctx, token, r.ID, r.Hash, "invoice-refund")
	if err != nil {
		t.Fatal(err)
	}
	f.base.refund.Store(r)
	if err = f.service.DispatchRefund(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	f.refundStatus.Store("succeeded")
	id := "re_" + hash([]byte(r.ID))[:24]
	if err = f.service.reconcileEvent(ctx, stripeInboxEvent{ID: "evt_invoice_refund", Type: "refund.updated", ObjectID: id}); err != nil {
		t.Fatalf("canonical renewal refund rejected: %v", err)
	}
	if err = f.service.reconcileEvent(ctx, stripeInboxEvent{ID: "evt_invoice_refund_replay", Type: "refund.updated", ObjectID: id}); err != nil {
		t.Fatal(err)
	}
	var state string
	var charges, refunds int
	if err = f.base.db.QueryRow(ctx, `SELECT state,(SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$2 AND kind='charge'),(SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$2 AND kind='refund') FROM undergrid_sandbox.sandbox_refunds WHERE id=$1`, r.ID, p.ID).Scan(&state, &charges, &refunds); err != nil {
		t.Fatal(err)
	}
	if state != "succeeded" || charges != 1 || refunds != 1 || f.posts("/v1/refunds") != 1 {
		t.Fatalf("invoice refund effects state=%s charges=%d refunds=%d posts=%d", state, charges, refunds, f.posts("/v1/refunds"))
	}
}

func TestStripeRenewalSignedSetupAndInvoiceEventsReconcileCanonicalObjects(t *testing.T) {
	f := newRenewalProviderFixture(t)
	ctx := context.Background()
	if err := f.service.tick(ctx); err != nil {
		t.Fatal(err)
	}
	f.complete.Store(true)
	sendRenewalStripeEvent(t, f, "checkout.session.completed", f.session, "evt_setup_"+randomID())
	if err := f.service.ProcessEvents(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := f.app.StripeBillingSetup(ctx, f.base.owner, f.sub.ID)
	if err != nil || b.State != "ready" {
		t.Fatalf("setup event did not bind canonical payer: state=%s err=%v", b.State, err)
	}
	p := f.prepare(t)
	f.loseOperation.Store("pay")
	if err = f.service.tick(ctx); err != nil {
		t.Fatal(err)
	}
	invoice := ""
	for id := range f.invoices {
		invoice = id
	}
	eventID := "evt_invoice_" + randomID()
	sendRenewalStripeEvent(t, f, "invoice.paid", invoice, eventID)
	sendRenewalStripeEvent(t, f, "invoice.paid", invoice, eventID)
	if err = f.service.ProcessEvents(ctx); err != nil {
		t.Fatal(err)
	}
	var state string
	var facts, events int
	if err = f.base.db.QueryRow(ctx, `SELECT state,(SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=p.id AND kind='charge'),(SELECT count(*) FROM undergrid_sandbox.sandbox_stripe_inbox WHERE event_id=$2 AND state='processed') FROM undergrid_sandbox.sandbox_payments p WHERE id=$1`, p.ID, eventID).Scan(&state, &facts, &events); err != nil {
		t.Fatal(err)
	}
	if state != "settled" || facts != 1 || events != 1 {
		t.Fatalf("canonical invoice event state=%s facts=%d events=%d", state, facts, events)
	}
}
func sendRenewalStripeEvent(t *testing.T, f *renewalProviderFixture, kind, object, id string) {
	t.Helper()
	raw := encode(map[string]any{"id": id, "object": "event", "api_version": stripe.APIVersion, "type": kind, "livemode": false, "data": map[string]any{"object": map[string]any{"id": object, "amount": 99999999, "customer": "cus_untrusted_payload"}}})
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: raw, Secret: f.service.config.WebhookSecret})
	req := httptest.NewRequest("POST", "/v1/provider/stripe/events", bytes.NewReader(raw))
	req.Header.Set("Stripe-Signature", signed.Header)
	w := httptest.NewRecorder()
	f.service.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("signed invoice event intake %d %s", w.Code, w.Body.String())
	}
}

func TestStripeBillingAppCannotInsertProviderReadiness(t *testing.T) {
	f := newRenewalProviderFixture(t)
	ctx := context.Background()
	var protected bool
	err := f.app.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		var state, customer, proof bool
		if err := tx.QueryRow(ctx, `SELECT has_column_privilege(current_user,'undergrid_sandbox.sandbox_stripe_billing_setups','state','INSERT'),has_column_privilege(current_user,'undergrid_sandbox.sandbox_stripe_billing_setups','customer_id','INSERT'),has_column_privilege(current_user,'undergrid_sandbox.sandbox_stripe_billing_setups','evidence','INSERT')`).Scan(&state, &customer, &proof); err != nil {
			return err
		}
		protected = !state && !customer && !proof
		return nil
	})
	if err != nil || !protected {
		t.Fatalf("app can insert worker-only payer evidence protected=%v err=%v", protected, err)
	}
}

func TestStripeRenewalRejectsMismatchedCanonicalInvoiceEvidence(t *testing.T) {
	for _, fault := range []string{"invoice_customer", "invoice_automatic", "invoice_consent", "line_price", "line_proration", "extra_line", "invoice_tax", "invoice_credit", "paid_invoice_binding", "intent_customer", "charge_live", "charge_amount"} {
		t.Run(fault, func(t *testing.T) {
			f := newRenewalProviderFixture(t)
			p := f.prepare(t)
			f.fault.Store(fault)
			if err := f.service.tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			var state string
			var facts int
			if err := f.base.db.QueryRow(context.Background(), "SELECT state,(SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=p.id) FROM undergrid_sandbox.sandbox_payments p WHERE id=$1", p.ID).Scan(&state, &facts); err != nil {
				t.Fatal(err)
			}
			if state != "unknown" || facts != 0 || f.posts("/v1/invoices") != 1 {
				t.Fatalf("unrelated %s accepted state=%s facts=%d invoices=%d", fault, state, facts, f.posts("/v1/invoices"))
			}
		})
	}
}
func TestStripeRenewalConcurrentTicksCollectOnce(t *testing.T) {
	f := newRenewalProviderFixture(t)
	p := f.prepare(t)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.service.tick(context.Background()) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := f.service.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	var state string
	var facts int
	if err := f.base.db.QueryRow(context.Background(), "SELECT state,(SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=p.id AND kind='charge') FROM undergrid_sandbox.sandbox_payments p WHERE id=$1", p.ID).Scan(&state, &facts); err != nil {
		t.Fatal(err)
	}
	pay := 0
	f.mu.Lock()
	for path, n := range f.counts {
		if strings.HasSuffix(path, "/pay") {
			pay += n
		}
	}
	f.mu.Unlock()
	if state != "settled" || facts != 1 || f.posts("/v1/invoices") != 1 || f.posts("/v1/invoiceitems") != 1 || pay != 1 {
		t.Fatalf("concurrent invoice duplicated: state=%s facts=%d invoices=%d items=%d pays=%d", state, facts, f.posts("/v1/invoices"), f.posts("/v1/invoiceitems"), pay)
	}
}

func TestStripeBillingSetupRejectsOtherOwnerAndCanceledReplay(t *testing.T) {
	f := newRenewalProviderFixture(t)
	ctx := context.Background()
	other := Owner{Issuer: f.base.owner.Issuer, Subject: "other-billing-owner-" + randomID()}
	if _, err := f.app.Enroll(ctx, other); err != nil {
		t.Fatal(err)
	}
	h := mustWalletHandler(t, f.app, func(r *http.Request) (Owner, error) {
		subject := r.Header.Get("X-Fixture-Owner")
		if subject == "" {
			return Owner{}, ErrUnauthorized
		}
		return Owner{Issuer: f.base.owner.Issuer, Subject: subject}, nil
	}, map[string]Quote{"platform-test": f.base.p.Quote})
	path := "/v1/owner/subscriptions/" + f.sub.ID + "/billing-setup"
	body := map[string]any{"consent_hash": f.sub.Hash}
	for _, method := range []string{"GET", "POST"} {
		if status, _ := refundCall(t, h, "", other.Subject, method, path, "other-owner", body); status != 401 {
			t.Fatalf("other owner %s billing setup: %d", method, status)
		}
	}
	if err := f.app.CancelSubscription(ctx, f.base.owner, f.sub.ID, f.sub.Hash); err != nil {
		t.Fatal(err)
	}
	if status, _ := refundCall(t, h, "", f.base.owner.Subject, "POST", path, "owner-payer-setup", body); status != 401 {
		t.Fatalf("canceled consent setup replay: %d", status)
	}
	if f.posts("/v1/customers") != 0 || f.posts("/v1/checkout/sessions") != 0 {
		t.Fatal("denied setup caused provider work")
	}
}

func TestStripeRenewalRecoveredCompletedSetupCanHaveNoURL(t *testing.T) {
	f := newRenewalProviderFixture(t)
	ctx := context.Background()
	f.loseSession.Store(true)
	if err := f.service.tick(ctx); err != nil {
		t.Fatal(err)
	}
	f.complete.Store(true)
	f.fault.Store("completed_session_no_url")
	for range 2 {
		if err := f.service.tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	b, err := f.app.StripeBillingSetup(ctx, f.base.owner, f.sub.ID)
	if err != nil || b.State != "ready" || b.URL != "" {
		t.Fatalf("completed setup without URL not recovered: %+v %v", b, err)
	}
	if f.posts("/v1/customers") != 1 || f.posts("/v1/checkout/sessions") != 1 {
		t.Fatal("recovery repeated payer creation")
	}
}
func TestStripeRenewalRecoveryStillRejectsMissingOpenOrUnsafeCompletedURL(t *testing.T) {
	for _, fault := range []string{"open_session_no_url", "completed_session_unsafe_url"} {
		t.Run(fault, func(t *testing.T) {
			f := newRenewalProviderFixture(t)
			ctx := context.Background()
			f.loseSession.Store(true)
			if err := f.service.tick(ctx); err != nil {
				t.Fatal(err)
			}
			f.complete.Store(fault == "completed_session_unsafe_url")
			f.fault.Store(fault)
			for range 2 {
				if err := f.service.tick(ctx); err != nil {
					t.Fatal(err)
				}
			}
			b, err := f.app.StripeBillingSetup(ctx, f.base.owner, f.sub.ID)
			if err != nil || b.State != "session_unknown" {
				t.Fatal("invalid setup URL accepted", b, err)
			}
			if f.posts("/v1/checkout/sessions") != 1 {
				t.Fatal("unknown setup URL caused another creation")
			}
		})
	}
}
func TestStripeRenewalDoesNotPayUnexpectedOpenInvoiceBalance(t *testing.T) {
	for _, fault := range []string{"open_invoice_partial_paid", "open_invoice_short_remaining", "open_invoice_zero_remaining"} {
		t.Run(fault, func(t *testing.T) {
			f := newRenewalProviderFixture(t)
			ctx := context.Background()
			p := f.prepare(t)
			var period string
			if err := f.base.db.QueryRow(ctx, "SELECT reservation_period::text FROM undergrid_sandbox.sandbox_payments WHERE id=$1", p.ID).Scan(&period); err != nil {
				t.Fatal(err)
			}
			f.fault.Store(fault)
			for range 2 {
				if err := f.service.tick(ctx); err != nil {
					t.Fatal(err)
				}
			}
			var state, key, afterPeriod string
			var facts int
			if err := f.base.db.QueryRow(ctx, `SELECT state,provider_key,reservation_period::text,(SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=p.id) FROM undergrid_sandbox.sandbox_payments p WHERE id=$1`, p.ID).Scan(&state, &key, &afterPeriod, &facts); err != nil {
				t.Fatal(err)
			}
			pay := 0
			f.mu.Lock()
			for path, n := range f.counts {
				if strings.HasSuffix(path, "/pay") {
					pay += n
				}
			}
			f.mu.Unlock()
			if pay != 0 || state != "unknown" || facts != 0 || key != p.ProviderKey || period != afterPeriod {
				t.Fatalf("unexpected balance reached pay: posts=%d state=%s facts=%d sameKey=%v samePeriod=%v", pay, state, facts, key == p.ProviderKey, period == afterPeriod)
			}
		})
	}
}
