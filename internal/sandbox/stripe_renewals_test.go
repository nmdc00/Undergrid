package sandbox

import (
	"context"
	"net/http"
	"strings"
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
