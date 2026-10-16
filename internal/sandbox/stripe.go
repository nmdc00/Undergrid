package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	stripe "github.com/stripe/stripe-go/v87"
	"github.com/stripe/stripe-go/v87/webhook"
)

type StripeConfig struct {
	SecretKey, WebhookSecret, AccountID, Product, PriceID, StripeProductID, SuccessURL, CancelURL string
}
type StripeService struct {
	store         *Store
	config        StripeConfig
	client        *stripe.Client
	verified      atomic.Bool
	evidenceClass string
}

func NewStripeService(s *Store, c StripeConfig) (*StripeService, error) {
	backend := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{HTTPClient: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, MaxNetworkRetries: stripe.Int64(0), LeveledLogger: &stripe.LeveledLogger{Level: stripe.LevelNull}})
	service, err := newStripeService(s, c, stripe.NewClient(c.SecretKey, stripe.WithBackends(&stripe.Backends{API: backend})))
	if err == nil {
		service.evidenceClass = "stripe_api_object"
	}
	return service, err
}

// Backend injection is private and used only by deterministic provider fixtures.
func newStripeService(s *Store, c StripeConfig, client *stripe.Client) (*StripeService, error) {
	if !strings.HasPrefix(c.SecretKey, "sk_test_") || !strings.HasPrefix(c.WebhookSecret, "whsec_") || c.AccountID == "" || c.Product == "" || !strings.HasPrefix(c.PriceID, "price_") || !strings.HasPrefix(c.StripeProductID, "prod_") {
		return nil, ErrInvalid
	}
	for _, raw := range []string{c.SuccessURL, c.CancelURL} {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return nil, ErrInvalid
		}
	}
	return &StripeService{store: s, config: c, client: client, evidenceClass: "stripe_api_fixture"}, nil
}
func (s *StripeService) VerifyAccount(ctx context.Context) error {
	account, err := s.client.V1Accounts.Retrieve(ctx, nil)
	if err != nil {
		return errors.New("Stripe test account unavailable")
	}
	if account.ID != s.config.AccountID {
		return ErrConflict
	}
	s.verified.Store(true)
	return nil
}
func (s *StripeService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "POST" || r.URL.Path != "/v1/provider/stripe/events" {
		w.WriteHeader(404)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 65537))
	if err != nil || len(raw) > 65536 {
		w.WriteHeader(413)
		return
	}
	event, err := webhook.ConstructEvent(raw, r.Header.Get("Stripe-Signature"), s.config.WebhookSecret)
	if err != nil || event.Livemode || event.Account != "" && event.Account != s.config.AccountID || event.ID == "" || event.APIVersion != stripe.APIVersion || event.Data == nil {
		w.WriteHeader(400)
		return
	}
	var object struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(event.Data.Raw, &object); err != nil || object.ID == "" {
		w.WriteHeader(400)
		return
	}
	err = s.store.transaction(r.Context(), "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), "INSERT INTO sandbox_stripe_inbox(account,event_id,api_version,event_type,object_id,payload,payload_hash) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(account,event_id) DO NOTHING", s.config.AccountID, event.ID, event.APIVersion, string(event.Type), object.ID, raw, hash(raw)); err != nil {
			return err
		}
		var old string
		if err := tx.QueryRow(r.Context(), "SELECT payload_hash FROM sandbox_stripe_inbox WHERE account=$1 AND event_id=$2", s.config.AccountID, event.ID).Scan(&old); err != nil {
			return err
		}
		if old != hash(raw) {
			return ErrConflict
		}
		return nil
	})
	if errors.Is(err, ErrConflict) {
		w.WriteHeader(409)
		return
	}
	if err != nil {
		w.WriteHeader(503)
		return
	}
	w.WriteHeader(200)
	io.WriteString(w, `{"received":true,"designation":"TEST"}`)
}

type stripeInboxEvent struct{ ID, Type, ObjectID string }

func (s *StripeService) ProcessEvents(ctx context.Context) error {
	if !s.verified.Load() {
		return ErrUnauthorized
	}
	var events []stripeInboxEvent
	err := s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT event_id,event_type,object_id FROM sandbox_stripe_inbox WHERE account=$1 AND state='pending' AND attempts<10 ORDER BY received_at,event_id LIMIT 100", s.config.AccountID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e stripeInboxEvent
			if err = rows.Scan(&e.ID, &e.Type, &e.ObjectID); err != nil {
				return err
			}
			events = append(events, e)
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	for _, e := range events {
		outcome := s.reconcileEvent(ctx, e)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		state, category := "processed", ""
		if outcome != nil {
			state = "pending"
			category = "provider_unavailable"
			if errors.Is(outcome, ErrConflict) || errors.Is(outcome, ErrInvalid) || errors.Is(outcome, ErrUnauthorized) {
				state = "blocked"
				category = "object_contract_mismatch"
			}
		}
		if err = s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "UPDATE sandbox_stripe_inbox SET attempts=attempts+1,state=CASE WHEN $3='pending' AND attempts>=9 THEN 'blocked' ELSE $3 END,last_error=nullif($4,''),processed_at=CASE WHEN $3='processed' THEN now() ELSE NULL END WHERE account=$1 AND event_id=$2 AND state='pending'", s.config.AccountID, e.ID, state, category)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *StripeService) fetchCharge(ctx context.Context, id, event string) (*stripe.Charge, Payment, VerifiedEffect, error) {
	var p Payment
	ch, err := s.client.V1Charges.Retrieve(ctx, id, nil)
	if err != nil {
		return nil, p, VerifiedEffect{}, err
	}
	if ch.ID != id || ch.Livemode || ch.Currency != "usd" || !ch.Paid || !ch.Captured || ch.Status != "succeeded" || ch.Amount <= 0 || ch.AmountCaptured != ch.Amount || !validUUID(ch.Metadata["undergrid_payment_id"]) {
		return ch, p, VerifiedEffect{}, ErrConflict
	}
	err = s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		var err error
		p, err = loadPayment(ctx, tx, ch.Metadata["undergrid_payment_id"])
		return err
	})
	if err != nil {
		return ch, p, VerifiedEffect{}, err
	}
	if p.Quote.Payee != s.config.AccountID || p.Quote.Product != s.config.Product || p.Quote.Rail != "stripe_test" || p.Quote.Asset != "USD" || p.Quote.Amount != strconv.FormatInt(ch.Amount, 10) || p.Quote.Fee != "0" || p.Hash != ch.Metadata["undergrid_quote_hash"] {
		return ch, p, VerifiedEffect{}, ErrConflict
	}
	evidence := encode(map[string]any{"verified_by": s.evidenceClass, "account": s.config.AccountID, "api_version": stripe.APIVersion, "livemode": false, "object_id": ch.ID, "amount_atomic": p.Quote.Amount, "quote_hash": p.Hash, "designation": "TEST"})
	effect := VerifiedEffect{Provider: "stripe_test", Account: s.config.AccountID, EventID: event, FactID: ch.ID, PaymentID: p.ID, Kind: "charge", Asset: "USD", Amount: p.Quote.Amount, Evidence: evidence}
	return ch, p, effect, nil
}
func (s *StripeService) verifyCharge(ctx context.Context, id, event string) (*stripe.Charge, Payment, error) {
	ch, p, effect, err := s.fetchCharge(ctx, id, event)
	if err != nil {
		return ch, p, err
	}
	return ch, p, s.store.RecordEffect(ctx, effect)
}
func (s *StripeService) unexpectedCharge(ctx context.Context, id, event, kind, object string) error {
	_, _, effect, err := s.fetchCharge(ctx, id, event)
	if err != nil {
		return err
	}
	if err = s.store.recordUnexpectedCharge(ctx, effect, kind, object, s.evidenceClass); err != nil {
		return err
	}
	return ErrConflict
}
func (s *StripeService) reconcileEvent(ctx context.Context, e stripeInboxEvent) error {
	switch e.Type {
	case "charge.succeeded":
		_, _, err := s.verifyCharge(ctx, e.ObjectID, e.ID)
		return err
	case "refund.created", "refund.updated", "refund.failed":
		refund, err := s.client.V1Refunds.Retrieve(ctx, e.ObjectID, nil)
		if err != nil {
			return err
		}
		if refund.ID != e.ObjectID || refund.Currency != "usd" || refund.Charge == nil || refund.Amount <= 0 {
			return ErrConflict
		}
		_, p, chargeEffect, err := s.fetchCharge(ctx, refund.Charge.ID, e.ID+"/charge")
		if err != nil {
			return err
		}
		id := refund.Metadata["undergrid_refund_id"]
		if id != "" && !validUUID(id) {
			return s.unexpectedCharge(ctx, refund.Charge.ID, e.ID+"/charge", "refund_binding_mismatch", refund.ID)
		}
		if id != "" {
			if err = s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
				r, err := loadRefund(ctx, tx, id, false)
				if err != nil {
					return err
				}
				if r.PaymentID != p.ID || r.Hash != refund.Metadata["undergrid_refund_hash"] || r.Amount != strconv.FormatInt(refund.Amount, 10) || r.Asset != "USD" {
					return ErrConflict
				}
				return nil
			}); err != nil {
				if errors.Is(err, ErrConflict) || errors.Is(err, ErrUnauthorized) {
					return s.unexpectedCharge(ctx, refund.Charge.ID, e.ID+"/charge", "refund_binding_mismatch", refund.ID)
				}
				return err
			}
		}
		if err = s.store.RecordEffect(ctx, chargeEffect); err != nil {
			return err
		}
		if refund.Status == "succeeded" {
			return s.store.RecordEffect(ctx, VerifiedEffect{Provider: "stripe_test", Account: s.config.AccountID, EventID: e.ID, FactID: refund.ID, PaymentID: p.ID, RefundID: id, Kind: "refund", Asset: "USD", Amount: strconv.FormatInt(refund.Amount, 10), Evidence: encode(map[string]any{"verified_by": s.evidenceClass, "object_id": refund.ID, "charge_id": refund.Charge.ID, "livemode": false, "api_version": stripe.APIVersion, "designation": "TEST"})})
		}
		if id != "" && (refund.Status == "failed" || refund.Status == "canceled") {
			return s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
				r, err := loadRefund(ctx, tx, id, true)
				if err != nil {
					return err
				}
				if r.PaymentID != p.ID || r.Amount != strconv.FormatInt(refund.Amount, 10) {
					return ErrConflict
				}
				_, err = tx.Exec(ctx, "UPDATE sandbox_refunds SET state='failed',provider_reference=$2,updated_at=now() WHERE id=$1 AND state IN('pending','unknown')", id, refund.ID)
				return err
			})
		}
		return nil
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		session, err := s.client.V1CheckoutSessions.Retrieve(ctx, e.ObjectID, &stripe.CheckoutSessionRetrieveParams{Expand: []*string{stripe.String("payment_intent.latest_charge")}})
		if err != nil {
			return err
		}
		if !validUUID(session.ClientReferenceID) {
			return ErrConflict
		}
		p, err := s.workerPayment(ctx, session.ClientReferenceID)
		if err != nil {
			return err
		}
		return s.reconcileSession(ctx, p, session, e.ID)
	case "payment_intent.canceled":
		intent, err := s.client.V1PaymentIntents.Retrieve(ctx, e.ObjectID, nil)
		if err != nil {
			return err
		}
		if intent.ID != e.ObjectID || intent.Livemode || intent.Currency != "usd" || intent.Status != "canceled" || !validUUID(intent.Metadata["undergrid_payment_id"]) {
			return ErrConflict
		}
		p, err := s.workerPayment(ctx, intent.Metadata["undergrid_payment_id"])
		if err != nil {
			return err
		}
		if p.Hash != intent.Metadata["undergrid_quote_hash"] || p.Quote.Payee != s.config.AccountID || p.Quote.Product != s.config.Product || p.Quote.Rail != "stripe_test" || p.Quote.Amount != strconv.FormatInt(intent.Amount, 10) {
			return ErrConflict
		}
		session, err := s.knownSession(ctx, p)
		if err != nil {
			return err
		}
		if session.PaymentIntent == nil || session.PaymentIntent.ID != intent.ID || session.Status != "expired" || session.PaymentStatus != "unpaid" {
			return ErrConflict
		}
		return s.reconcileSession(ctx, p, session, e.ID)
	default:
		return ErrInvalid
	}
}
