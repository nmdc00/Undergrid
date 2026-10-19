package sandbox

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
	stripe "github.com/stripe/stripe-go/v87"
)

func renewalMetadata(p Payment, b StripeBillingSetup, c stripeRenewalClaim) map[string]string {
	m := billingMetadata(b)
	m["undergrid_payment_id"] = p.ID
	m["undergrid_quote_hash"] = p.Hash
	m["undergrid_renewal_ordinal"] = strconv.Itoa(c.ordinal)
	return m
}
func exactRenewalMetadata(p Payment, b StripeBillingSetup, c stripeRenewalClaim, m map[string]string) bool {
	for k, v := range renewalMetadata(p, b, c) {
		if m[k] != v {
			return false
		}
	}
	return true
}
func (s *StripeService) tickRenewalCollections(ctx context.Context) error {
	var ids []string
	err := s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT p.id::text FROM sandbox_payments p JOIN sandbox_subscriptions sub ON sub.id=p.subscription_id JOIN sandbox_stripe_billing_setups b ON b.subscription_id=sub.id WHERE b.state='ready' AND p.state IN('authorized','submitting','unknown','pending') AND p.quote->>'payee'=$1 AND p.quote->>'product'=$2 AND p.quote->>'rail'='stripe_test' ORDER BY p.updated_at,p.id LIMIT 16`, s.config.AccountID, s.config.Product)
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
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.collectRenewal(ctx, id); err != nil {
			_ = s.store.MarkUnknown(ctx, id)
		}
		if err = s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "UPDATE sandbox_payments SET updated_at=clock_timestamp() WHERE id=$1", id)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}
func (s *StripeService) collectRenewal(ctx context.Context, id string) error {
	if !s.verified.Load() || !s.config.RenewalsEnabled {
		return ErrUnauthorized
	}
	p, b, c, err := s.renewal(ctx, id)
	if err != nil {
		return err
	}
	if p.State == "authorized" {
		if err = s.reconcileBillingSetup(ctx, b); err != nil {
			return err
		}
		p, err = s.store.BeginDispatch(ctx, id)
		if err != nil {
			return err
		}
		p, b, c, err = s.renewal(ctx, id)
		if err != nil {
			return err
		}
		params := &stripe.InvoiceCreateParams{Customer: stripe.String(b.customer), Currency: stripe.String("usd"), AutoAdvance: stripe.Bool(false), CollectionMethod: stripe.String("charge_automatically"), PendingInvoiceItemsBehavior: stripe.String("exclude"), AutomaticTax: &stripe.InvoiceCreateAutomaticTaxParams{Enabled: stripe.Bool(false)}, Metadata: renewalMetadata(p, b, c)}
		params.AddExtra("discounts", "")
		params.SetIdempotencyKey(p.ProviderKey + "-invoice")
		invoice, err := s.client.V1Invoices.Create(ctx, params)
		if err != nil {
			return err
		}
		if err = s.attachRenewalInvoice(ctx, p, b, c, invoice); err != nil {
			return err
		}
	}
	for range 8 {
		p, b, c, err = s.renewal(ctx, id)
		if err != nil {
			return err
		}
		if c.state == "settled" {
			return nil
		}
		if c.invoice == "" {
			var found *stripe.Invoice
			n := 0
			list := s.client.V1Invoices.List(ctx, &stripe.InvoiceListParams{ListParams: stripe.ListParams{Limit: stripe.Int64(100)}, Customer: stripe.String(b.customer)})
			for invoice, err := range list.All(ctx) {
				if err != nil {
					return err
				}
				n++
				if n > 1000 {
					return ErrLimit
				}
				if exactRenewalMetadata(p, b, c, invoice.Metadata) {
					if found != nil {
						return ErrConflict
					}
					found = invoice
				}
			}
			if found == nil {
				return nil
			}
			if err = s.attachRenewalInvoice(ctx, p, b, c, found); err != nil {
				return err
			}
			continue
		}
		params := &stripe.InvoiceRetrieveParams{}
		params.AddExpand("payments.data.payment.payment_intent")
		invoice, err := s.client.V1Invoices.Retrieve(ctx, c.invoice, params)
		if err != nil {
			return err
		}
		item, err := s.verifyRenewalInvoice(p, b, c, invoice)
		if err != nil {
			return err
		}
		if invoice.Status == "paid" {
			return s.recordRenewalInvoice(ctx, p, b, c, invoice)
		}
		switch c.state {
		case "draft":
			if invoice.Status != "draft" || item != "" {
				return ErrConflict
			}
			if err = s.renewalStage(ctx, p, c, "item_unknown"); err != nil {
				return err
			}
			params := &stripe.InvoiceItemCreateParams{Customer: stripe.String(b.customer), Invoice: stripe.String(c.invoice), Currency: stripe.String("usd"), Pricing: &stripe.InvoiceItemCreatePricingParams{Price: stripe.String(b.price)}, Quantity: stripe.Int64(1), Discountable: stripe.Bool(false), Metadata: renewalMetadata(p, b, c)}
			params.SetIdempotencyKey(p.ProviderKey + "-item")
			if _, err = s.client.V1InvoiceItems.Create(ctx, params); err != nil {
				return err
			}
		case "item_unknown":
			if item == "" {
				return nil
			}
			if err = s.renewalMutation(ctx, p, c, false, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, "UPDATE sandbox_stripe_renewal_claims SET item_id=$2,state='ready',updated_at=clock_timestamp() WHERE payment_id=$1", p.ID, item)
				return err
			}); err != nil {
				return err
			}
		case "ready":
			if invoice.Status != "draft" || item == "" {
				return ErrConflict
			}
			if err = s.reconcileBillingSetup(ctx, b); err != nil {
				return err
			}
			if err = s.renewalStage(ctx, p, c, "finalize_unknown"); err != nil {
				return err
			}
			params := &stripe.InvoiceFinalizeInvoiceParams{AutoAdvance: stripe.Bool(false)}
			params.SetIdempotencyKey(p.ProviderKey + "-finalize")
			if _, err = s.client.V1Invoices.FinalizeInvoice(ctx, c.invoice, params); err != nil {
				return err
			}
		case "finalize_unknown":
			if invoice.Status != "open" {
				return nil
			}
			if err = s.renewalMutation(ctx, p, c, false, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, "UPDATE sandbox_stripe_renewal_claims SET state='open',updated_at=clock_timestamp() WHERE payment_id=$1", p.ID)
				return err
			}); err != nil {
				return err
			}
		case "open":
			if invoice.Status != "open" {
				return ErrConflict
			}
			if err = s.reconcileBillingSetup(ctx, b); err != nil {
				return err
			}
			if err = s.renewalStage(ctx, p, c, "pay_unknown"); err != nil {
				return err
			}
			params := &stripe.InvoicePayParams{PaymentMethod: stripe.String(b.method), OffSession: stripe.Bool(true), PaidOutOfBand: stripe.Bool(false), Forgive: stripe.Bool(false)}
			params.SetIdempotencyKey(p.ProviderKey + "-pay")
			if _, err = s.client.V1Invoices.Pay(ctx, c.invoice, params); err != nil {
				return err
			}
		case "pay_unknown":
			return nil
		default:
			return ErrConflict
		}
	}
	return nil
}
func (s *StripeService) verifyRenewalInvoice(p Payment, b StripeBillingSetup, c stripeRenewalClaim, i *stripe.Invoice) (string, error) {
	if i == nil || i.ID == "" || (c.invoice != "" && i.ID != c.invoice) || i.Livemode || i.Customer == nil || i.Customer.ID != b.customer || i.Currency != "usd" || i.AutoAdvance || i.AutomaticallyFinalizesAt != 0 || i.CollectionMethod != "charge_automatically" || !exactRenewalMetadata(p, b, c, i.Metadata) || i.Parent != nil || len(i.Discounts) != 0 || len(i.TotalDiscountAmounts) != 0 || len(i.TotalTaxes) != 0 || len(i.DefaultTaxRates) != 0 || len(i.TotalPretaxCreditAmounts) != 0 || i.AmountShipping != 0 || i.StartingBalance != 0 || i.PrePaymentCreditNotesAmount != 0 || i.PostPaymentCreditNotesAmount != 0 || i.AmountPaidOffStripe != 0 || i.AmountOverpaid != 0 || i.AutomaticTax != nil && i.AutomaticTax.Enabled {
		return "", ErrConflict
	}
	if i.Lines == nil || i.Lines.HasMore || len(i.Lines.Data) > 1 {
		return "", ErrConflict
	}
	amount, err := strconv.ParseInt(p.Quote.Amount, 10, 64)
	if err != nil {
		return "", ErrInvalid
	}
	if len(i.Lines.Data) == 0 {
		if i.Status != "draft" || i.Total != 0 || i.AmountDue != 0 || c.state != "invoice_unknown" && c.state != "draft" && c.state != "item_unknown" {
			return "", ErrConflict
		}
		return "", nil
	}
	line := i.Lines.Data[0]
	if line.ID == "" || line.Livemode || line.Invoice != i.ID || line.Currency != "usd" || line.Amount != amount || line.Quantity != 1 || line.Subscription != nil || line.Discountable || len(line.Discounts) != 0 || len(line.DiscountAmounts) != 0 || len(line.PretaxCreditAmounts) != 0 || len(line.Taxes) != 0 || line.Pricing == nil || line.Pricing.PriceDetails == nil || (line.Pricing.PriceDetails.Price == nil || line.Pricing.PriceDetails.Price.ID != b.price) || line.Pricing.PriceDetails.Product != s.config.StripeProductID || !exactRenewalMetadata(p, b, c, line.Metadata) || line.Parent == nil || line.Parent.Type != "invoice_item_details" || line.Parent.InvoiceItemDetails == nil || line.Parent.InvoiceItemDetails.Proration || line.Parent.InvoiceItemDetails.InvoiceItem == "" {
		return "", ErrConflict
	}
	item := line.Parent.InvoiceItemDetails.InvoiceItem
	if c.item != "" && item != c.item || i.Total != amount || i.AmountDue != amount {
		return "", ErrConflict
	}
	return item, nil
}
func (s *StripeService) attachRenewalInvoice(ctx context.Context, p Payment, b StripeBillingSetup, c stripeRenewalClaim, i *stripe.Invoice) error {
	if _, err := s.verifyRenewalInvoice(p, b, c, i); err != nil {
		return err
	}
	return s.renewalMutation(ctx, p, c, false, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE sandbox_stripe_renewal_claims SET invoice_id=$2,state='draft',updated_at=clock_timestamp() WHERE payment_id=$1 AND invoice_id IS NULL", p.ID, i.ID)
		return err
	})
}
func (s *StripeService) renewalInvoiceEffect(ctx context.Context, p Payment, b StripeBillingSetup, c stripeRenewalClaim, i *stripe.Invoice) (VerifiedEffect, string, error) {
	if _, err := s.verifyRenewalInvoice(p, b, c, i); err != nil {
		return VerifiedEffect{}, "", err
	}
	amount, _ := strconv.ParseInt(p.Quote.Amount, 10, 64)
	if i.Status != "paid" || i.AmountPaid != amount || i.AmountRemaining != 0 || i.Payments == nil || i.Payments.HasMore || len(i.Payments.Data) != 1 {
		return VerifiedEffect{}, "", ErrConflict
	}
	pay := i.Payments.Data[0]
	if pay.ID == "" || pay.Livemode || pay.Invoice == nil || pay.Invoice.ID != i.ID || pay.Status != "paid" || pay.Currency != "usd" || pay.AmountPaid != amount || pay.AmountRequested != amount || pay.Payment == nil || pay.Payment.Type != "payment_intent" || pay.Payment.PaymentIntent == nil {
		return VerifiedEffect{}, "", ErrConflict
	}
	intent, err := s.client.V1PaymentIntents.Retrieve(ctx, pay.Payment.PaymentIntent.ID, nil)
	if err != nil {
		return VerifiedEffect{}, "", err
	}
	if intent.ID != pay.Payment.PaymentIntent.ID || intent.Livemode || intent.Status != "succeeded" || intent.Currency != "usd" || intent.Amount != amount || intent.AmountReceived != amount || intent.Customer == nil || intent.Customer.ID != b.customer || intent.PaymentMethod == nil || intent.PaymentMethod.ID != b.method || intent.LatestCharge == nil {
		return VerifiedEffect{}, "", ErrConflict
	}
	charge, err := s.client.V1Charges.Retrieve(ctx, intent.LatestCharge.ID, nil)
	if err != nil {
		return VerifiedEffect{}, "", err
	}
	if charge.ID != intent.LatestCharge.ID || charge.Livemode || !charge.Paid || !charge.Captured || charge.Status != "succeeded" || charge.Currency != "usd" || charge.Amount != amount || charge.AmountCaptured != amount || charge.Customer == nil || charge.Customer.ID != b.customer || charge.PaymentIntent == nil || charge.PaymentIntent.ID != intent.ID || charge.PaymentMethod != b.method {
		return VerifiedEffect{}, "", ErrConflict
	}
	evidence := encode(map[string]any{"verified_by": s.evidenceClass, "account": s.config.AccountID, "api_version": stripe.APIVersion, "livemode": false, "object_id": charge.ID, "invoice_id": i.ID, "payment_intent_id": intent.ID, "quote_hash": p.Hash, "consent_hash": c.consent, "renewal_ordinal": c.ordinal, "designation": "TEST"})
	return VerifiedEffect{Provider: "stripe_test", Account: s.config.AccountID, EventID: "invoice:" + i.ID, FactID: charge.ID, PaymentID: p.ID, Kind: "charge", Asset: "USD", Amount: p.Quote.Amount, Evidence: evidence}, intent.ID, nil
}
func (s *StripeService) recordRenewalInvoice(ctx context.Context, p Payment, b StripeBillingSetup, c stripeRenewalClaim, i *stripe.Invoice) error {
	effect, _, err := s.renewalInvoiceEffect(ctx, p, b, c, i)
	if err != nil {
		return err
	}
	return s.recordStripeCharge(ctx, p, effect)
}
