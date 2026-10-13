package sandbox

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
	stripe "github.com/stripe/stripe-go/v87"
)

func (s *StripeService) knownSession(ctx context.Context, p Payment) (*stripe.CheckoutSession, error) {
	var id *string
	if err := s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT checkout_session_id FROM sandbox_payments WHERE id=$1", p.ID).Scan(&id)
	}); err != nil {
		return nil, err
	}
	if id == nil {
		return nil, ErrConflict
	}
	session, err := s.client.V1CheckoutSessions.Retrieve(ctx, *id, &stripe.CheckoutSessionRetrieveParams{Expand: []*string{stripe.String("payment_intent.latest_charge")}})
	if err == nil && session.ID != *id {
		return nil, ErrConflict
	}
	return session, err
}
func (s *StripeService) ReconcilePayment(ctx context.Context, id string) error {
	if !s.verified.Load() {
		return ErrUnauthorized
	}
	p, err := s.workerPayment(ctx, id)
	if err != nil {
		return err
	}
	if p.State != "pending" && p.State != "unknown" && p.State != "submitting" {
		return ErrConflict
	}
	session, err := s.knownSession(ctx, p)
	if err != nil {
		return err
	}
	return s.reconcileSession(ctx, p, session, "object-session/"+session.ID)
}
func (s *StripeService) reconcileSession(ctx context.Context, p Payment, session *stripe.CheckoutSession, event string) error {
	var current *string
	if err := s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT checkout_session_id FROM sandbox_payments WHERE id=$1", p.ID).Scan(&current)
	}); err != nil {
		return err
	}
	valid := session != nil && session.ID != "" && !session.Livemode && session.Mode == "payment" && session.Currency == "usd" && strconv.FormatInt(session.AmountTotal, 10) == p.Quote.Amount && session.ClientReferenceID == p.ID && session.Metadata["undergrid_payment_id"] == p.ID && session.Metadata["undergrid_quote_hash"] == p.Hash && (current == nil || *current == session.ID) && p.Quote.Payee == s.config.AccountID && p.Quote.Product == s.config.Product && p.Quote.Rail == "stripe_test" && p.Quote.Asset == "USD" && p.Quote.Fee == "0"
	if !valid {
		if session != nil && session.PaymentIntent != nil && session.PaymentIntent.LatestCharge != nil {
			return s.unexpectedCharge(ctx, session.PaymentIntent.LatestCharge.ID, event, "checkout_binding_mismatch", session.ID)
		}
		return ErrConflict
	}
	switch {
	case session.Status == "open" && session.PaymentStatus == "unpaid":
		return s.attachSession(ctx, p, session)
	case session.Status == "complete" && session.PaymentStatus == "paid":
		if session.PaymentIntent == nil || session.PaymentIntent.LatestCharge == nil {
			return ErrConflict
		}
		ch, charged, effect, err := s.fetchCharge(ctx, session.PaymentIntent.LatestCharge.ID, event)
		if err != nil {
			return err
		}
		if charged.ID != p.ID || ch.PaymentIntent != nil && ch.PaymentIntent.ID != session.PaymentIntent.ID {
			return s.unexpectedCharge(ctx, ch.ID, event, "checkout_binding_mismatch", session.ID)
		}
		// Bind before monetary mutation; conflicting session identity cannot release
		// another payment. Paid sessions legitimately have no active redirect URL.
		if err = s.attachSession(ctx, p, session); err != nil {
			return err
		}
		return s.store.RecordEffect(ctx, effect)
	case session.Status == "expired" && session.PaymentStatus == "unpaid":
		if session.PaymentIntent != nil && (session.PaymentIntent.Status != "canceled" || session.PaymentIntent.LatestCharge != nil) {
			return ErrConflict
		}
		if err := s.attachSession(ctx, p, session); err != nil {
			return err
		}
		return s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
			var actor string
			if err := tx.QueryRow(ctx, "SELECT actor_id::text FROM sandbox_payments WHERE id=$1", p.ID).Scan(&actor); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "SELECT id FROM sandbox_actors WHERE id=$1 FOR UPDATE", actor); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "UPDATE sandbox_payments SET state='failed',updated_at=now() WHERE id=$1 AND checkout_session_id=$2 AND state IN('authorized','submitting','pending','unknown')", p.ID, session.ID)
			return err
		})
	default:
		return ErrConflict
	}
}
