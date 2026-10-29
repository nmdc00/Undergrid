package sandbox

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/jackc/pgx/v5"
	stripe "github.com/stripe/stripe-go/v87"
)

func (s *StripeService) workerPayment(ctx context.Context, id string) (Payment, error) {
	var p Payment
	err := s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error { var err error; p, err = loadPayment(ctx, tx, id); return err })
	return p, err
}
func (s *StripeService) DispatchPayment(ctx context.Context, id string) error {
	if !s.verified.Load() {
		return ErrUnauthorized
	}
	p, err := s.workerPayment(ctx, id)
	if err != nil {
		return err
	}
	if p.State != "authorized" {
		return ErrConflict
	}
	if p.Quote.Product != s.config.Product || p.Quote.Payee != s.config.AccountID || p.Quote.Rail != "stripe_test" || p.Quote.Asset != "USD" || p.Quote.Fee != "0" {
		return ErrConflict
	}
	// Bounded subscription invoices require their own manual collection contract.
	var recurring bool
	if err = s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT subscription_id IS NOT NULL FROM sandbox_payments WHERE id=$1", id).Scan(&recurring)
	}); err != nil {
		return err
	}
	if recurring {
		return ErrConflict
	}
	price, err := s.client.V1Prices.Retrieve(ctx, s.config.PriceID, nil)
	if err != nil {
		return err
	}
	if price.ID != s.config.PriceID || price.Livemode || !price.Active || price.Currency != "usd" || price.Type != "one_time" || price.Product == nil || price.Product.ID != s.config.StripeProductID || strconv.FormatInt(price.UnitAmount, 10) != p.Quote.Amount {
		return ErrConflict
	}
	p, err = s.store.BeginDispatch(ctx, id)
	if err != nil {
		return err
	}
	metadata := map[string]string{"undergrid_payment_id": p.ID, "undergrid_quote_hash": p.Hash, "designation": "TEST"}
	params := &stripe.CheckoutSessionCreateParams{Mode: stripe.String("payment"), SuccessURL: stripe.String(s.config.SuccessURL), CancelURL: stripe.String(s.config.CancelURL), ClientReferenceID: stripe.String(p.ID), Metadata: metadata, LineItems: []*stripe.CheckoutSessionCreateLineItemParams{{Price: stripe.String(s.config.PriceID), Quantity: stripe.Int64(1)}}, PaymentIntentData: &stripe.CheckoutSessionCreatePaymentIntentDataParams{Metadata: metadata}, AllowPromotionCodes: stripe.Bool(false), AutomaticTax: &stripe.CheckoutSessionCreateAutomaticTaxParams{Enabled: stripe.Bool(false)}, AdaptivePricing: &stripe.CheckoutSessionCreateAdaptivePricingParams{Enabled: stripe.Bool(false)}, AllowedPaymentMethodTypes: []*string{stripe.String("card")}}
	params.SetIdempotencyKey(p.ProviderKey)
	session, err := s.client.V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		if markErr := s.store.MarkUnknown(ctx, id); markErr != nil {
			return errors.Join(err, markErr)
		}
		return err
	}
	if err = s.attachSession(ctx, p, session); err != nil {
		_ = s.store.MarkUnknown(ctx, id)
		return err
	}
	return nil
}
func (s *StripeService) attachSession(ctx context.Context, p Payment, session *stripe.CheckoutSession) error {
	if session == nil || session.ID == "" || session.Livemode || session.Mode != "payment" || session.Currency != "usd" || strconv.FormatInt(session.AmountTotal, 10) != p.Quote.Amount || session.ClientReferenceID != p.ID || session.Metadata["undergrid_payment_id"] != p.ID || session.Metadata["undergrid_quote_hash"] != p.Hash {
		return ErrConflict
	}
	u, err := url.Parse(session.URL)
	if (session.URL != "" && (err != nil || u.Scheme != "https" || u.Host != "checkout.stripe.com" || u.User != nil)) || (session.Status == "open" && session.URL == "") {
		return ErrConflict
	}
	return s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		var previous *string
		if err := tx.QueryRow(ctx, "SELECT checkout_session_id FROM sandbox_payments WHERE id=$1 FOR UPDATE", p.ID).Scan(&previous); err != nil {
			return err
		}
		if previous != nil && *previous != session.ID {
			return ErrConflict
		}
		_, err := tx.Exec(ctx, "UPDATE sandbox_payments SET checkout_session_id=$2,checkout_url=$3,state=CASE WHEN state IN('submitting','unknown') THEN 'pending' ELSE state END,updated_at=now() WHERE id=$1", p.ID, session.ID, session.URL)
		return err
	})
}

// Reconciliation searches a bounded provider page stream by the original
// immutable identity. Absence or truncation stays unknown; no creation retry.
func (s *StripeService) ReconcileUnknown(ctx context.Context, id string) error {
	if !s.verified.Load() {
		return ErrUnauthorized
	}
	p, err := s.workerPayment(ctx, id)
	if err != nil {
		return err
	}
	if p.State != "unknown" && p.State != "submitting" {
		return ErrConflict
	}
	var created int64
	if err = s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT floor(extract(epoch from created_at))::bigint FROM sandbox_payments WHERE id=$1", id).Scan(&created)
	}); err != nil {
		return err
	}
	list := s.client.V1CheckoutSessions.List(ctx, &stripe.CheckoutSessionListParams{ListParams: stripe.ListParams{Limit: stripe.Int64(100)}, CreatedRange: &stripe.RangeQueryParams{GreaterThanOrEqual: created - 60}})
	var match *stripe.CheckoutSession
	count := 0
	for session, err := range list.All(ctx) {
		if err != nil {
			return err
		}
		count++
		if count > 1000 {
			return ErrLimit
		}
		if session.ClientReferenceID == p.ID && session.Metadata["undergrid_payment_id"] == p.ID && session.Metadata["undergrid_quote_hash"] == p.Hash {
			if match != nil && match.ID != session.ID {
				return ErrConflict
			}
			match = session
		}
	}
	if match == nil {
		return ErrConflict
	}
	// List relationships are normally ID-only. Retrieve the unique candidate
	// with the required expansion before interpreting canonical settlement.
	session, err := s.client.V1CheckoutSessions.Retrieve(ctx, match.ID, &stripe.CheckoutSessionRetrieveParams{Expand: []*string{stripe.String("payment_intent.latest_charge")}})
	if err != nil {
		return err
	}
	if session.ID != match.ID {
		return ErrConflict
	}
	return s.reconcileSession(ctx, p, session, "object-session/"+session.ID)
}

func (s *StripeService) DispatchRefund(ctx context.Context, id string) error {
	if !s.verified.Load() {
		return ErrUnauthorized
	}
	var refund Refund
	err := s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error { var err error; refund, err = loadRefund(ctx, tx, id, false); return err })
	if err != nil {
		return err
	}
	if refund.State != "authorized" {
		return ErrConflict
	}
	p, err := s.workerPayment(ctx, refund.PaymentID)
	if err != nil {
		return err
	}
	if p.Quote.Rail != "stripe_test" || p.Quote.Payee != s.config.AccountID || p.Quote.Product != s.config.Product {
		return ErrConflict
	}
	var chargeID string
	if err = s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT provider_reference FROM sandbox_payments WHERE id=$1", p.ID).Scan(&chargeID)
	}); err != nil {
		return err
	}
	amount, err := strconv.ParseInt(refund.Amount, 10, 64)
	if err != nil {
		return ErrInvalid
	}
	refund, err = s.store.BeginRefundDispatch(ctx, id)
	if err != nil {
		return err
	}
	params := &stripe.RefundCreateParams{Charge: stripe.String(chargeID), Amount: stripe.Int64(amount), Metadata: map[string]string{"undergrid_refund_id": refund.ID, "undergrid_refund_hash": refund.Hash, "designation": "TEST"}}
	params.SetIdempotencyKey(refund.ProviderKey)
	result, err := s.client.V1Refunds.Create(ctx, params)
	if err != nil {
		if markErr := s.store.MarkRefundUnknown(ctx, id); markErr != nil {
			return errors.Join(err, markErr)
		}
		return err
	}
	if result.ID == "" || result.Charge == nil || result.Charge.ID != chargeID || result.Amount != amount || result.Currency != "usd" || result.Metadata["undergrid_refund_id"] != refund.ID || result.Metadata["undergrid_refund_hash"] != refund.Hash {
		_ = s.store.MarkRefundUnknown(ctx, id)
		return ErrConflict
	}
	return s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE sandbox_refunds SET provider_reference=$2,updated_at=now() WHERE id=$1 AND state='pending'", id, result.ID)
		return err
	})
}
func (s *StripeService) ReconcileUnknownRefund(ctx context.Context, id string) error {
	if !s.verified.Load() {
		return ErrUnauthorized
	}
	var refund Refund
	var reference *string
	err := s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		var err error
		refund, err = loadRefund(ctx, tx, id, false)
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, "SELECT provider_reference FROM sandbox_refunds WHERE id=$1", id).Scan(&reference)
	})
	if err != nil {
		return err
	}
	if refund.State != "unknown" && refund.State != "pending" {
		return ErrConflict
	}
	if reference != nil {
		return s.reconcileEvent(ctx, stripeInboxEvent{ID: "object-refund/" + *reference, Type: "refund.updated", ObjectID: *reference})
	}
	var chargeID string
	if err = s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT provider_reference FROM sandbox_payments WHERE id=$1", refund.PaymentID).Scan(&chargeID)
	}); err != nil {
		return err
	}
	list := s.client.V1Refunds.List(ctx, &stripe.RefundListParams{ListParams: stripe.ListParams{Limit: stripe.Int64(100)}, Charge: stripe.String(chargeID)})
	var match *stripe.Refund
	count := 0
	for result, err := range list.All(ctx) {
		if err != nil {
			return err
		}
		count++
		if count > 1000 {
			return ErrLimit
		}
		if result.Metadata["undergrid_refund_id"] == refund.ID && result.Metadata["undergrid_refund_hash"] == refund.Hash {
			if match != nil && match.ID != result.ID {
				return ErrConflict
			}
			match = result
		}
	}
	if match == nil {
		return ErrConflict
	}
	return s.reconcileEvent(ctx, stripeInboxEvent{ID: "object-refund/" + match.ID, Type: "refund.updated", ObjectID: match.ID})
}
