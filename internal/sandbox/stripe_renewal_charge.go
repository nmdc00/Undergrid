package sandbox

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	stripe "github.com/stripe/stripe-go/v87"
)

// Invoice-generated charges do not inherit Checkout payment metadata. For a
// customer created by our owner-bound setup, only its durable invoice graph can
// identify the purchase; arbitrary charge metadata cannot redirect the fact.
func (s *StripeService) fetchRenewalCharge(ctx context.Context, ch *stripe.Charge, event string) (Payment, VerifiedEffect, bool, error) {
	var p Payment
	var effect VerifiedEffect
	if ch.Customer == nil {
		return p, effect, false, nil
	}
	var managed bool
	var ids []string
	err := s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM sandbox_stripe_billing_setups WHERE customer_id=$1 AND account=$2)", ch.Customer.ID, s.config.AccountID).Scan(&managed); err != nil {
			return err
		}
		if !managed {
			return nil
		}
		rows, err := tx.Query(ctx, `SELECT c.payment_id::text FROM sandbox_stripe_renewal_claims c JOIN sandbox_stripe_billing_setups b ON b.id=c.setup_id WHERE b.customer_id=$1 AND b.account=$2 AND c.invoice_id IS NOT NULL AND (c.charge_id IS NULL OR c.charge_id=$3) LIMIT 17`, ch.Customer.ID, s.config.AccountID, ch.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	if err != nil || !managed {
		return p, effect, managed, err
	}
	if len(ids) > 16 {
		return p, effect, true, ErrLimit
	}
	if ch.PaymentIntent == nil {
		return p, effect, true, ErrConflict
	}
	for _, id := range ids {
		p, b, c, err := s.renewal(ctx, id)
		if err != nil {
			return p, effect, true, err
		}
		params := &stripe.InvoiceRetrieveParams{}
		params.AddExpand("payments.data.payment.payment_intent")
		invoice, err := s.client.V1Invoices.Retrieve(ctx, c.invoice, params)
		if err != nil {
			return p, effect, true, err
		}
		linked := false
		if invoice.Payments != nil && !invoice.Payments.HasMore {
			for _, payment := range invoice.Payments.Data {
				if payment.Payment != nil && payment.Payment.PaymentIntent != nil && payment.Payment.PaymentIntent.ID == ch.PaymentIntent.ID {
					linked = true
				}
			}
		}
		if !linked {
			continue
		}
		effect, _, err = s.renewalInvoiceEffect(ctx, p, b, c, invoice)
		if err != nil {
			return p, effect, true, err
		}
		if effect.FactID != ch.ID {
			return p, effect, true, ErrConflict
		}
		effect.EventID = event
		return p, effect, true, nil
	}
	return p, effect, true, ErrConflict
}
func (s *StripeService) recordStripeCharge(ctx context.Context, p Payment, effect VerifiedEffect) error {
	_, _, c, err := s.renewal(ctx, p.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return s.store.RecordEffect(ctx, effect)
	}
	if err != nil {
		return err
	}
	var evidence struct {
		Invoice string `json:"invoice_id"`
		Intent  string `json:"payment_intent_id"`
	}
	if json.Unmarshal(effect.Evidence, &evidence) != nil || evidence.Invoice != c.invoice || evidence.Intent == "" || c.charge != "" && c.charge != effect.FactID || c.intent != "" && c.intent != evidence.Intent {
		return ErrConflict
	}
	return s.renewalMutation(ctx, p, c, false, func(tx pgx.Tx) error {
		if err := recordEffect(ctx, tx, effect); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE sandbox_stripe_renewal_claims SET state='settled',payment_intent_id=$2,charge_id=$3,updated_at=clock_timestamp() WHERE payment_id=$1", p.ID, evidence.Intent, effect.FactID)
		return err
	})
}
