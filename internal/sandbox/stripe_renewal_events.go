package sandbox

import (
	"context"

	"github.com/jackc/pgx/v5"
	stripe "github.com/stripe/stripe-go/v87"
)

func (s *StripeService) reconcileBillingSessionEvent(ctx context.Context, session *stripe.CheckoutSession) error {
	if session == nil || !validUUID(session.ClientReferenceID) {
		return ErrConflict
	}
	var b StripeBillingSetup
	err := s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		var subscription string
		if err := tx.QueryRow(ctx, "SELECT subscription_id::text FROM sandbox_stripe_billing_setups WHERE id=$1", session.ClientReferenceID).Scan(&subscription); err != nil {
			return err
		}
		var err error
		b, err = loadStripeBilling(ctx, tx, subscription)
		return err
	})
	if err != nil {
		return err
	}
	if b.account != s.config.AccountID || b.price != s.config.PriceID {
		return ErrConflict
	}
	if b.session == "" && b.State == "session_unknown" {
		if err = s.attachBillingSession(ctx, b, session); err != nil {
			return err
		}
		b, err = s.billing(ctx, b.subscription)
		if err != nil {
			return err
		}
	}
	if b.session != session.ID || !validSetupSession(b, session) {
		return ErrConflict
	}
	return s.reconcileBillingSetup(ctx, b)
}
func (s *StripeService) reconcileRenewalInvoiceEvent(ctx context.Context, id string) error {
	params := &stripe.InvoiceRetrieveParams{}
	params.AddExpand("payments.data.payment.payment_intent")
	invoice, err := s.client.V1Invoices.Retrieve(ctx, id, params)
	if err != nil {
		return err
	}
	if invoice.ID != id || !validUUID(invoice.Metadata["undergrid_payment_id"]) {
		return ErrConflict
	}
	p, b, c, err := s.renewal(ctx, invoice.Metadata["undergrid_payment_id"])
	if err != nil {
		return err
	}
	if c.invoice == "" && c.state == "invoice_unknown" {
		if err = s.attachRenewalInvoice(ctx, p, b, c, invoice); err != nil {
			return err
		}
		p, b, c, err = s.renewal(ctx, p.ID)
		if err != nil {
			return err
		}
	}
	if c.invoice != invoice.ID {
		return ErrConflict
	}
	if _, err = s.verifyRenewalInvoice(p, b, c, invoice); err != nil {
		return err
	}
	if invoice.Status == "paid" {
		return s.recordRenewalInvoice(ctx, p, b, c, invoice)
	}
	// A payment failure or an unpaid/void invoice is not proof that an in-flight
	// debit cannot arrive. Keep its original unknown reservation; never resubmit.
	if p.State == "submitting" || p.State == "pending" || p.State == "unknown" {
		return s.store.MarkUnknown(ctx, p.ID)
	}
	return nil
}
