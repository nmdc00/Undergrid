package sandbox

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/jackc/pgx/v5"
	stripe "github.com/stripe/stripe-go/v87"
)

func billingMetadata(b StripeBillingSetup) map[string]string {
	return map[string]string{"undergrid_setup_id": b.ID, "undergrid_actor_id": b.actor, "undergrid_subscription_id": b.subscription, "undergrid_consent_hash": b.consent, "designation": "TEST"}
}
func exactBillingMetadata(b StripeBillingSetup, metadata map[string]string) bool {
	for k, v := range billingMetadata(b) {
		if metadata[k] != v {
			return false
		}
	}
	return true
}
func (s *StripeService) billingMutation(ctx context.Context, b StripeBillingSetup, current bool, fn func(pgx.Tx, StripeBillingSetup) error) error {
	return s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT id FROM sandbox_actors WHERE id=$1 FOR UPDATE", b.actor); err != nil {
			return err
		}
		sub, err := loadSubscription(ctx, tx, b.subscription, true)
		if err != nil {
			return err
		}
		actual, err := loadStripeBilling(ctx, tx, b.subscription)
		if err != nil {
			return err
		}
		var subscriptionActor string
		if err = tx.QueryRow(ctx, "SELECT actor_id::text FROM sandbox_subscriptions WHERE id=$1", sub.ID).Scan(&subscriptionActor); err != nil {
			return err
		}
		if actual.ID != b.ID || actual.actor != b.actor || actual.actor != subscriptionActor || actual.consent != b.consent || actual.consent != sub.Hash || actual.State != b.State {
			return ErrConflict
		}
		if current {
			if err = s.store.currentBillingConsent(ctx, tx, sub); err != nil {
				return err
			}
		}
		return fn(tx, actual)
	})
}
func (s *StripeService) billingStage(ctx context.Context, b StripeBillingSetup, next string, current bool) error {
	return s.billingMutation(ctx, b, current, func(tx pgx.Tx, actual StripeBillingSetup) error {
		if actual.account != "" && (actual.account != s.config.AccountID || actual.price != s.config.PriceID) {
			return ErrConflict
		}
		_, err := tx.Exec(ctx, "UPDATE sandbox_stripe_billing_setups SET state=$2,account=COALESCE(account,$3),price_id=COALESCE(price_id,$4),updated_at=clock_timestamp() WHERE id=$1", b.ID, next, s.config.AccountID, s.config.PriceID)
		return err
	})
}
func (s *StripeService) billing(ctx context.Context, subscription string) (StripeBillingSetup, error) {
	var b StripeBillingSetup
	err := s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error { var err error; b, err = loadStripeBilling(ctx, tx, subscription); return err })
	return b, err
}
func (s *StripeService) processBillingSetups(ctx context.Context) error {
	var ids []string
	err := s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT b.subscription_id::text FROM sandbox_stripe_billing_setups b JOIN sandbox_subscriptions s ON s.id=b.subscription_id WHERE b.state<>'ready' AND s.offer->>'payee'=$1 AND s.offer->>'product'=$2 ORDER BY b.updated_at,b.id LIMIT 16`, s.config.AccountID, s.config.Product)
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
		// Each aggregate retains its irreversible attempt claim. A failed provider
		// observation never causes another creation, nor starves later setup rows.
		_ = s.processBillingSetup(ctx, id)
		if err = s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "UPDATE sandbox_stripe_billing_setups SET updated_at=clock_timestamp() WHERE subscription_id=$1", id)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}
func (s *StripeService) processBillingSetup(ctx context.Context, id string) error {
	for range 6 {
		b, err := s.billing(ctx, id)
		if err != nil {
			return err
		}
		if b.account != "" && (b.account != s.config.AccountID || b.price != s.config.PriceID) {
			return ErrConflict
		}
		switch b.State {
		case "requested":
			var sub Subscription
			err = s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error { var err error; sub, err = loadSubscription(ctx, tx, id, false); return err })
			if err != nil {
				return err
			}
			price, err := s.client.V1Prices.Retrieve(ctx, s.config.PriceID, nil)
			if err != nil {
				return err
			}
			if price.Livemode || !price.Active || price.ID != s.config.PriceID || price.Type != "one_time" || price.Currency != "usd" || price.Product == nil || price.Product.ID != s.config.StripeProductID || strconv.FormatInt(price.UnitAmount, 10) != sub.Offer.Amount || sub.Offer.Fee != "0" {
				return ErrConflict
			}
			if err = s.billingStage(ctx, b, "customer_unknown", true); err != nil {
				return err
			}
			params := &stripe.CustomerCreateParams{Metadata: billingMetadata(b)}
			params.SetIdempotencyKey("ugs-billing-" + b.ID + "-customer")
			customer, err := s.client.V1Customers.Create(ctx, params)
			if err != nil {
				return err
			}
			b.State = "customer_unknown"
			if err = s.attachBillingCustomer(ctx, b, customer); err != nil {
				return err
			}
		case "customer_unknown":
			var found *stripe.Customer
			n := 0
			list := s.client.V1Customers.List(ctx, &stripe.CustomerListParams{ListParams: stripe.ListParams{Limit: stripe.Int64(100)}, CreatedRange: &stripe.RangeQueryParams{GreaterThanOrEqual: b.created.Unix() - 60}})
			for customer, err := range list.All(ctx) {
				if err != nil {
					return err
				}
				n++
				if n > 1000 {
					return ErrLimit
				}
				if exactBillingMetadata(b, customer.Metadata) {
					if found != nil {
						return ErrConflict
					}
					found = customer
				}
			}
			if found == nil {
				return nil
			}
			if err = s.attachBillingCustomer(ctx, b, found); err != nil {
				return err
			}
		case "customer_ready":
			if err = s.verifyBillingCustomer(ctx, b); err != nil {
				return err
			}
			if err = s.billingStage(ctx, b, "session_unknown", true); err != nil {
				return err
			}
			params := &stripe.CheckoutSessionCreateParams{Mode: stripe.String("setup"), Customer: stripe.String(b.customer), SuccessURL: stripe.String(s.config.SuccessURL), CancelURL: stripe.String(s.config.CancelURL), ClientReferenceID: stripe.String(b.ID), Metadata: billingMetadata(b), SetupIntentData: &stripe.CheckoutSessionCreateSetupIntentDataParams{Metadata: billingMetadata(b)}, AllowedPaymentMethodTypes: []*string{stripe.String("card")}}
			params.SetIdempotencyKey("ugs-billing-" + b.ID + "-setup")
			session, err := s.client.V1CheckoutSessions.Create(ctx, params)
			if err != nil {
				return err
			}
			b.State = "session_unknown"
			if err = s.attachBillingSession(ctx, b, session); err != nil {
				return err
			}
		case "session_unknown":
			var found *stripe.CheckoutSession
			n := 0
			list := s.client.V1CheckoutSessions.List(ctx, &stripe.CheckoutSessionListParams{ListParams: stripe.ListParams{Limit: stripe.Int64(100)}, Customer: stripe.String(b.customer), CreatedRange: &stripe.RangeQueryParams{GreaterThanOrEqual: b.created.Unix() - 60}})
			for session, err := range list.All(ctx) {
				if err != nil {
					return err
				}
				n++
				if n > 1000 {
					return ErrLimit
				}
				if exactBillingMetadata(b, session.Metadata) {
					if found != nil {
						return ErrConflict
					}
					found = session
				}
			}
			if found == nil {
				return nil
			}
			if err = s.attachBillingSession(ctx, b, found); err != nil {
				return err
			}
		case "pending":
			return s.reconcileBillingSetup(ctx, b)
		case "ready":
			return nil
		default:
			return ErrConflict
		}
	}
	return nil
}
func (s *StripeService) attachBillingCustomer(ctx context.Context, b StripeBillingSetup, c *stripe.Customer) error {
	if c == nil || c.ID == "" || c.Deleted || c.Livemode || !exactBillingMetadata(b, c.Metadata) {
		return ErrConflict
	}
	return s.billingMutation(ctx, b, false, func(tx pgx.Tx, _ StripeBillingSetup) error {
		_, err := tx.Exec(ctx, "UPDATE sandbox_stripe_billing_setups SET customer_id=$2,state='customer_ready',updated_at=clock_timestamp() WHERE id=$1 AND customer_id IS NULL", b.ID, c.ID)
		return err
	})
}
func (s *StripeService) verifyBillingCustomer(ctx context.Context, b StripeBillingSetup) error {
	customer, err := s.client.V1Customers.Retrieve(ctx, b.customer, nil)
	if err != nil {
		return err
	}
	if customer.ID != b.customer || customer.Deleted || customer.Livemode || !exactBillingMetadata(b, customer.Metadata) {
		return ErrConflict
	}
	return nil
}
func validSetupSession(b StripeBillingSetup, session *stripe.CheckoutSession) bool {
	return session != nil && session.ID != "" && !session.Livemode && session.Mode == "setup" && session.Customer != nil && session.Customer.ID == b.customer && session.ClientReferenceID == b.ID && exactBillingMetadata(b, session.Metadata)
}
func (s *StripeService) attachBillingSession(ctx context.Context, b StripeBillingSetup, session *stripe.CheckoutSession) error {
	if !validSetupSession(b, session) {
		return ErrConflict
	}
	// Completed sessions may no longer have an active hosted URL. The exact
	// customer/session/SetupIntent graph, not a redirect link, proves setup.
	if session.URL == "" {
		if session.Status != "complete" {
			return ErrConflict
		}
	} else {
		u, err := url.Parse(session.URL)
		if err != nil || u.Scheme != "https" || u.Host != "checkout.stripe.com" || u.User != nil {
			return ErrConflict
		}
	}
	return s.billingMutation(ctx, b, false, func(tx pgx.Tx, _ StripeBillingSetup) error {
		_, err := tx.Exec(ctx, "UPDATE sandbox_stripe_billing_setups SET session_id=$2,setup_url=$3,state='pending',updated_at=clock_timestamp() WHERE id=$1 AND session_id IS NULL", b.ID, session.ID, session.URL)
		return err
	})
}
func (s *StripeService) reconcileBillingSetup(ctx context.Context, b StripeBillingSetup) error {
	if err := s.verifyBillingCustomer(ctx, b); err != nil {
		return err
	}
	session, err := s.client.V1CheckoutSessions.Retrieve(ctx, b.session, nil)
	if err != nil {
		return err
	}
	if !validSetupSession(b, session) || session.ID != b.session {
		return ErrConflict
	}
	if session.Status != "complete" {
		return nil
	}
	if session.SetupIntent == nil || session.SetupIntent.ID == "" {
		return ErrConflict
	}
	intent, err := s.client.V1SetupIntents.Retrieve(ctx, session.SetupIntent.ID, nil)
	if err != nil {
		return err
	}
	if intent.ID != session.SetupIntent.ID || intent.Livemode || intent.Status != "succeeded" || intent.Usage != "off_session" || intent.Customer == nil || intent.Customer.ID != b.customer || intent.PaymentMethod == nil || intent.PaymentMethod.ID == "" || !exactBillingMetadata(b, intent.Metadata) {
		return ErrConflict
	}
	method, err := s.client.V1PaymentMethods.Retrieve(ctx, intent.PaymentMethod.ID, nil)
	if err != nil {
		return err
	}
	if method.ID != intent.PaymentMethod.ID || method.Livemode || method.Type != "card" || method.Customer == nil || method.Customer.ID != b.customer {
		return ErrConflict
	}
	if b.State == "ready" {
		if b.intent != intent.ID || b.method != method.ID {
			return ErrConflict
		}
		return nil
	}
	evidence, _ := json.Marshal(map[string]any{"verified_by": s.evidenceClass, "account": s.config.AccountID, "customer_id": b.customer, "session_id": b.session, "setup_intent_id": intent.ID, "payment_method_id": method.ID, "consent_hash": b.consent, "designation": "TEST"})
	return s.billingMutation(ctx, b, false, func(tx pgx.Tx, _ StripeBillingSetup) error {
		_, err := tx.Exec(ctx, "UPDATE sandbox_stripe_billing_setups SET setup_intent_id=$2,payment_method_id=$3,evidence=$4,state='ready',updated_at=clock_timestamp() WHERE id=$1", b.ID, intent.ID, method.ID, evidence)
		return err
	})
}
