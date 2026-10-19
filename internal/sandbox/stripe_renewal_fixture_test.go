package sandbox

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	stripe "github.com/stripe/stripe-go/v87"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type renewalInvoiceFixture struct {
	id, item, intent, charge, state string
	metadata, itemMetadata          map[string]string
}

type renewalProviderFixture struct {
	refundStatus                          atomic.Value
	providerMu                            sync.Mutex
	loseOperation                         atomic.Value
	hideInvoices                          atomic.Bool
	afterFinalize                         atomic.Value
	invoices                              map[string]*renewalInvoiceFixture
	observer                              *pgxpool.Pool
	base                                  *stripeRecoveryFixture
	app                                   *Store
	service                               *StripeService
	sub                                   Subscription
	billing                               StripeBillingSetup
	customer, session, intent, method     string
	complete                              atomic.Bool
	loseCustomer, loseSession             atomic.Bool
	fault                                 atomic.Value
	mu                                    sync.Mutex
	counts                                map[string]int
	customerMeta, sessionMeta, intentMeta map[string]string
}

func (f *renewalProviderFixture) posts(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counts[path]
}
func newRenewalProviderFixture(t *testing.T) *renewalProviderFixture {
	t.Helper()
	base := newStripeRecoveryFixture(t)
	app := New(usdcFixtureRoleDSN(t, base.db, "undergrid_sandbox_app"))
	worker := New(usdcFixtureRoleDSN(t, base.db, "undergrid_sandbox_worker"))
	var credential string
	if err := base.db.QueryRow(context.Background(), "SELECT id::text FROM undergrid_sandbox.sandbox_credentials WHERE digest=$1", hash([]byte(base.token))).Scan(&credential); err != nil {
		t.Fatal(err)
	}
	sub, err := app.CreateSubscription(context.Background(), base.owner, "manual-test-renewals", SubscriptionConsent{CredentialID: credential, Maximum: "100", PeriodSeconds: 86400, RenewalCount: 2, ExpiresAt: time.Now().Add(72 * time.Hour)}, base.p.Quote)
	if err != nil {
		t.Fatal(err)
	}
	billing, err := app.RequestStripeBillingSetup(context.Background(), base.owner, sub.ID, sub.Hash, "owner-payer-setup")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = base.db.Exec(context.Background(), "UPDATE undergrid_sandbox.sandbox_payments SET state='canceled' WHERE id=$1", base.p.ID); err != nil {
		t.Fatal(err)
	}
	f := &renewalProviderFixture{base: base, app: app, sub: sub, billing: billing, customer: "cus_" + randomID(), session: "cs_test_" + randomID(), intent: "seti_" + randomID(), method: "pm_" + randomID(), counts: map[string]int{}}
	f.refundStatus.Store("pending")
	f.fault.Store("")
	f.loseOperation.Store("")
	f.afterFinalize.Store("")
	f.invoices = map[string]*renewalInvoiceFixture{}
	f.observer, err = pgxpool.New(context.Background(), base.db.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.observer.Close)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { f.serve(t, w, r) }))
	t.Cleanup(provider.Close)
	backend := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(provider.URL), HTTPClient: provider.Client(), MaxNetworkRetries: stripe.Int64(0), LeveledLogger: &stripe.LeveledLogger{Level: stripe.LevelNull}})
	cfg := base.service.config
	cfg.RenewalsEnabled = true
	f.service, err = newStripeService(worker, cfg, stripe.NewClient(cfg.SecretKey, stripe.WithBackends(&stripe.Backends{API: backend})))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.VerifyAccount(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *renewalProviderFixture) serve(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	f.providerMu.Lock()
	defer f.providerMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	write := func(v any) { json.NewEncoder(w).Encode(v) }
	if r.Method == "POST" {
		r.ParseForm()
		f.mu.Lock()
		f.counts[r.URL.Path]++
		f.mu.Unlock()
	}
	if r.Method == "POST" {
		state := ""
		id := r.Form.Get("metadata[undergrid_payment_id]")
		switch {
		case r.URL.Path == "/v1/customers":
			state = "customer_unknown"
		case r.URL.Path == "/v1/checkout/sessions":
			state = "session_unknown"
		case r.URL.Path == "/v1/invoices":
			state = "invoice_unknown"
		case r.URL.Path == "/v1/invoiceitems":
			state = "item_unknown"
			if invoice := f.invoices[r.Form.Get("invoice")]; invoice != nil {
				id = invoice.metadata["undergrid_payment_id"]
			}
		case strings.HasSuffix(r.URL.Path, "/finalize"):
			state = "finalize_unknown"
		case strings.HasSuffix(r.URL.Path, "/pay"):
			state = "pay_unknown"
		}
		if strings.HasPrefix(r.URL.Path, "/v1/invoices/") {
			parts := strings.Split(r.URL.Path, "/")
			if invoice := f.invoices[parts[3]]; invoice != nil {
				id = invoice.metadata["undergrid_payment_id"]
			}
		}
		var actual string
		var err error
		if r.URL.Path == "/v1/refunds" {
			state = "pending"
			refund := f.base.refund.Load().(Refund)
			err = f.observer.QueryRow(context.Background(), "SELECT state FROM undergrid_sandbox.sandbox_refunds WHERE id=$1", refund.ID).Scan(&actual)
		} else if id != "" {
			err = f.observer.QueryRow(context.Background(), "SELECT state FROM undergrid_sandbox.sandbox_stripe_renewal_claims WHERE payment_id=$1", id).Scan(&actual)
		} else {
			err = f.observer.QueryRow(context.Background(), "SELECT state FROM undergrid_sandbox.sandbox_stripe_billing_setups WHERE id=$1", f.billing.ID).Scan(&actual)
		}
		if err != nil || actual != state {
			t.Errorf("provider mutation before committed claim %s state=%s want=%s err=%v", r.URL.Path, actual, state, err)
			w.WriteHeader(400)
			return
		}
	}
	metadata := func(prefix string) map[string]string {
		m := map[string]string{}
		for k, v := range r.Form {
			if strings.HasPrefix(k, prefix+"[") && strings.HasSuffix(k, "]") && len(v) == 1 {
				m[strings.TrimSuffix(strings.TrimPrefix(k, prefix+"["), "]")] = v[0]
			}
		}
		return m
	}
	customer := func() any {
		return map[string]any{"id": f.customer, "object": "customer", "livemode": false, "metadata": f.customerMeta}
	}
	session := func() any {
		status := "open"
		if f.complete.Load() {
			status = "complete"
		}
		return map[string]any{"id": f.session, "object": "checkout.session", "livemode": false, "mode": "setup", "status": status, "customer": f.customer, "setup_intent": f.intent, "client_reference_id": f.billing.ID, "metadata": f.sessionMeta, "url": "https://checkout.stripe.com/c/pay/test_setup"}
	}
	switch r.URL.Path {
	case "/v1/account":
		write(map[string]any{"id": f.base.account, "object": "account"})
	case "/v1/prices/price_fixture":
		write(map[string]any{"id": "price_fixture", "object": "price", "livemode": false, "active": true, "currency": "usd", "unit_amount": 100, "type": "one_time", "product": "prod_fixture"})
	case "/v1/customers":
		if r.Method == "POST" {
			f.customerMeta = metadata("metadata")
			if f.loseCustomer.Swap(false) {
				w.WriteHeader(502)
				return
			}
			write(customer())
		} else {
			write(map[string]any{"object": "list", "has_more": false, "data": []any{customer()}})
		}
	case "/v1/customers/" + f.customer:
		write(customer())
	case "/v1/checkout/sessions":
		if r.Method == "POST" {
			if r.Form.Get("mode") != "setup" || r.Form.Get("customer") != f.customer || r.Form.Get("client_reference_id") != f.billing.ID {
				t.Errorf("unbound setup creation: %v", r.Form)
				w.WriteHeader(400)
				return
			}
			f.sessionMeta = metadata("metadata")
			f.intentMeta = metadata("setup_intent_data[metadata]")
			if f.loseSession.Swap(false) {
				w.WriteHeader(502)
				return
			}
			write(session())
		} else {
			write(map[string]any{"object": "list", "has_more": false, "data": []any{session()}})
		}
	case "/v1/checkout/sessions/" + f.session:
		write(session())
	case "/v1/setup_intents/" + f.intent:
		customer, metadata := f.customer, f.intentMeta
		if f.fault.Load().(string) == "setup_customer" {
			customer = "cus_unrelated"
		}
		if f.fault.Load().(string) == "setup_consent" {
			metadata = map[string]string{"undergrid_consent_hash": hash([]byte("unrelated"))}
		}
		write(map[string]any{"id": f.intent, "object": "setup_intent", "livemode": false, "status": "succeeded", "usage": "off_session", "customer": customer, "payment_method": f.method, "metadata": metadata})
	case "/v1/payment_methods/" + f.method:
		customer := f.customer
		if f.fault.Load().(string) == "method_customer" {
			customer = "cus_unrelated"
		}
		write(map[string]any{"id": f.method, "object": "payment_method", "livemode": f.fault.Load().(string) == "live_method", "type": "card", "customer": customer})
	case "/v1/invoices":
		if r.Method == "POST" {
			if r.Form.Get("auto_advance") != "false" || r.Form.Get("collection_method") != "charge_automatically" || r.Form.Get("pending_invoice_items_behavior") != "exclude" || r.Form.Get("customer") != f.customer || r.Form.Get("currency") != "usd" || r.Form.Get("automatic_tax[enabled]") != "false" || r.Form.Get("subscription") != "" {
				t.Errorf("invoice not manually bounded: %v", r.Form)
				w.WriteHeader(400)
				return
			}
			m := metadata("metadata")
			suffix := hash([]byte(m["undergrid_payment_id"]))[:24]
			invoice := &renewalInvoiceFixture{id: "in_" + suffix, item: "ii_" + suffix, intent: "pi_" + suffix, charge: "ch_" + suffix, state: "draft", metadata: m}
			f.invoices[invoice.id] = invoice
			if f.loseOperation.Load().(string) == "invoice" {
				f.loseOperation.Store("")
				w.WriteHeader(502)
				return
			}
			write(f.invoiceObject(invoice))
		} else {
			items := []any{}
			if !f.hideInvoices.Load() {
				for _, invoice := range f.invoices {
					items = append(items, f.invoiceObject(invoice))
				}
			}
			write(map[string]any{"object": "list", "has_more": false, "data": items})
		}
	case "/v1/refunds":
		refund := f.base.refund.Load().(Refund)
		charge := ""
		for _, invoice := range f.invoices {
			if invoice.metadata["undergrid_payment_id"] == refund.PaymentID {
				charge = invoice.charge
			}
		}
		write(map[string]any{"id": "re_" + hash([]byte(refund.ID))[:24], "object": "refund", "livemode": false, "charge": charge, "amount": 40, "currency": "usd", "status": f.refundStatus.Load().(string), "metadata": map[string]string{"undergrid_refund_id": refund.ID, "undergrid_refund_hash": refund.Hash}})
	case "/v1/invoiceitems":
		invoice := f.invoices[r.Form.Get("invoice")]
		if invoice == nil || r.Form.Get("customer") != f.customer || r.Form.Get("pricing[price]") != "price_fixture" || r.Form.Get("quantity") != "1" || r.Form.Get("discountable") != "false" {
			t.Errorf("unbounded invoice item: %v", r.Form)
			w.WriteHeader(400)
			return
		}
		invoice.itemMetadata = metadata("metadata")
		if f.loseOperation.Load().(string) == "item" {
			f.loseOperation.Store("")
			w.WriteHeader(502)
			return
		}
		write(map[string]any{"id": invoice.item, "object": "invoiceitem", "livemode": false, "amount": 100, "currency": "usd", "invoice": invoice.id})
	default:
		if strings.HasPrefix(r.URL.Path, "/v1/refunds/") {
			refund := f.base.refund.Load().(Refund)
			charge := ""
			for _, invoice := range f.invoices {
				if invoice.metadata["undergrid_payment_id"] == refund.PaymentID {
					charge = invoice.charge
				}
			}
			write(map[string]any{"id": "re_" + hash([]byte(refund.ID))[:24], "object": "refund", "livemode": false, "charge": charge, "amount": 40, "currency": "usd", "status": f.refundStatus.Load().(string), "metadata": map[string]string{"undergrid_refund_id": refund.ID, "undergrid_refund_hash": refund.Hash}})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/invoices/") {
			parts := strings.Split(r.URL.Path, "/")
			invoice := f.invoices[parts[3]]
			if invoice == nil {
				w.WriteHeader(404)
				return
			}
			if r.Method == "POST" && len(parts) == 5 {
				switch parts[4] {
				case "finalize":
					if r.Form.Get("auto_advance") != "false" || invoice.itemMetadata == nil {
						t.Error("invoice auto collection or empty finalization")
						w.WriteHeader(400)
						return
					}
					invoice.state = "open"
					action := f.afterFinalize.Load().(string)
					if action != "" {
						f.afterFinalize.Store("")
						switch action {
						case "revoked":
							_, err := f.observer.Exec(context.Background(), "UPDATE undergrid_sandbox.sandbox_credentials SET revoked_at=clock_timestamp() WHERE digest=$1", hash([]byte(f.base.token)))
							if err != nil {
								t.Error(err)
							}
						case "expired":
							_, err := f.observer.Exec(context.Background(), "UPDATE undergrid_sandbox.sandbox_credentials SET expires_at=clock_timestamp()-interval '1 second' WHERE digest=$1", hash([]byte(f.base.token)))
							if err != nil {
								t.Error(err)
							}
						case "canceled":
							if err := f.app.CancelSubscription(context.Background(), f.base.owner, f.sub.ID, f.sub.Hash); err != nil {
								t.Error(err)
							}
						}
					}
				case "pay":
					if invoice.state != "open" || r.Form.Get("payment_method") != f.method || r.Form.Get("paid_out_of_band") != "false" || r.Form.Get("forgive") != "false" {
						t.Error("invoice paid without bound payment method")
						w.WriteHeader(400)
						return
					}
					invoice.state = "paid"
				default:
					w.WriteHeader(404)
					return
				}
			}
			if r.Method == "POST" && len(parts) == 5 && f.loseOperation.Load().(string) == parts[4] {
				f.loseOperation.Store("")
				w.WriteHeader(502)
				return
			}
			write(f.invoiceObject(invoice))
			return
		}
		for _, invoice := range f.invoices {
			if r.URL.Path == "/v1/payment_intents/"+invoice.intent {
				write(map[string]any{"id": invoice.intent, "object": "payment_intent", "livemode": false, "status": "succeeded", "amount": 100, "amount_received": 100, "currency": "usd", "customer": f.customer, "payment_method": f.method, "latest_charge": invoice.charge})
				return
			}
			if r.URL.Path == "/v1/charges/"+invoice.charge {
				write(map[string]any{"id": invoice.charge, "object": "charge", "livemode": false, "status": "succeeded", "amount": 100, "amount_captured": 100, "currency": "usd", "paid": true, "captured": true, "customer": f.customer, "payment_intent": invoice.intent, "payment_method": f.method})
				return
			}
		}
		t.Errorf("unexpected renewal provider call %s %s", r.Method, r.URL.Path)
		w.WriteHeader(404)
	}
}

func (f *renewalProviderFixture) invoiceObject(i *renewalInvoiceFixture) any {
	lines := []any{}
	amount := 0
	if i.itemMetadata != nil {
		amount = 100
		lines = append(lines, map[string]any{"id": "il_" + i.item, "object": "line_item", "livemode": false, "invoice": i.id, "currency": "usd", "amount": 100, "quantity": 1, "discountable": false, "metadata": i.itemMetadata, "pricing": map[string]any{"type": "price_details", "price_details": map[string]any{"price": "price_fixture", "product": "prod_fixture"}}, "parent": map[string]any{"type": "invoice_item_details", "invoice_item_details": map[string]any{"invoice_item": i.item, "proration": false}}})
	}
	paid, remaining := 0, amount
	payments := []any{}
	if i.state == "paid" {
		paid = 100
		remaining = 0
		payments = append(payments, map[string]any{"id": "inpay_" + i.id, "object": "invoice_payment", "livemode": false, "status": "paid", "invoice": i.id, "amount_requested": 100, "amount_paid": 100, "currency": "usd", "payment": map[string]any{"type": "payment_intent", "payment_intent": i.intent}})
	}
	return map[string]any{"id": i.id, "object": "invoice", "livemode": false, "customer": f.customer, "currency": "usd", "auto_advance": false, "collection_method": "charge_automatically", "metadata": i.metadata, "status": i.state, "amount_due": amount, "total": amount, "amount_paid": paid, "amount_remaining": remaining, "lines": map[string]any{"object": "list", "has_more": false, "data": lines}, "payments": map[string]any{"object": "list", "has_more": false, "data": payments}}
}
