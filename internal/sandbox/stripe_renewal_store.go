package sandbox

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

type stripeRenewalClaim struct {
	state, invoice, item, intent, charge string
	ordinal                              int
	subscription, consent                string
}

func (s *Store) claimStripeRenewal(ctx context.Context, tx pgx.Tx, p Payment) error {
	var subscription *string
	if err := tx.QueryRow(ctx, "SELECT subscription_id::text FROM sandbox_payments WHERE id=$1", p.ID).Scan(&subscription); err != nil {
		return err
	}
	if subscription == nil {
		return nil
	}
	b, err := loadStripeBilling(ctx, tx, *subscription)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUnauthorized
	}
	if err != nil {
		return err
	}
	sub, err := loadSubscription(ctx, tx, *subscription, false)
	if err != nil {
		return err
	}
	if b.State != "ready" || b.consent != sub.Hash || b.account != p.Quote.Payee || b.price == "" {
		return ErrUnauthorized
	}
	_, err = tx.Exec(ctx, "INSERT INTO sandbox_stripe_renewal_claims(payment_id,setup_id,quote_hash,provider_key,state) VALUES($1,$2,$3,$4,'invoice_unknown')", p.ID, b.ID, p.Hash, p.ProviderKey)
	return err
}
func (s *StripeService) renewal(ctx context.Context, id string) (Payment, StripeBillingSetup, stripeRenewalClaim, error) {
	var p Payment
	var b StripeBillingSetup
	var c stripeRenewalClaim
	err := s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		var err error
		p, err = loadPayment(ctx, tx, id)
		if err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, "SELECT p.subscription_id::text,p.renewal_ordinal,s.consent_hash FROM sandbox_payments p JOIN sandbox_subscriptions s ON s.id=p.subscription_id WHERE p.id=$1", id).Scan(&c.subscription, &c.ordinal, &c.consent); err != nil {
			return err
		}
		b, err = loadStripeBilling(ctx, tx, c.subscription)
		if err != nil {
			return err
		}
		if b.consent != c.consent || b.account != s.config.AccountID || b.price != s.config.PriceID || b.State != "ready" || p.Quote.Payee != b.account || p.Quote.Product != s.config.Product || p.Quote.Asset != "USD" || p.Quote.Fee != "0" || p.Quote.Rail != "stripe_test" {
			return ErrConflict
		}
		var quote, key string
		err = tx.QueryRow(ctx, "SELECT state,COALESCE(invoice_id,''),COALESCE(item_id,''),COALESCE(payment_intent_id,''),COALESCE(charge_id,''),quote_hash,provider_key FROM sandbox_stripe_renewal_claims WHERE payment_id=$1", id).Scan(&c.state, &c.invoice, &c.item, &c.intent, &c.charge, &quote, &key)
		if errors.Is(err, pgx.ErrNoRows) && p.State == "authorized" {
			return nil
		}
		if err != nil {
			return err
		}
		if quote != p.Hash || key != p.ProviderKey {
			return ErrConflict
		}
		return nil
	})
	return p, b, c, err
}
func (s *StripeService) renewalMutation(ctx context.Context, p Payment, c stripeRenewalClaim, current bool, fn func(pgx.Tx) error) error {
	return s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		var actor string
		if err := tx.QueryRow(ctx, "SELECT actor_id::text FROM sandbox_payments WHERE id=$1", p.ID).Scan(&actor); err != nil {
			return err
		}
		var raw []byte
		var version int64
		if err := tx.QueryRow(ctx, "SELECT policy,version FROM sandbox_actors WHERE id=$1 FOR UPDATE", actor).Scan(&raw, &version); err != nil {
			return err
		}
		actual, err := loadPayment(ctx, tx, p.ID)
		if err != nil {
			return err
		}
		var state, quote, key string
		if err = tx.QueryRow(ctx, "SELECT state,quote_hash,provider_key FROM sandbox_stripe_renewal_claims WHERE payment_id=$1", p.ID).Scan(&state, &quote, &key); err != nil {
			return err
		}
		if state != c.state || quote != actual.Hash || key != actual.ProviderKey || actual.Hash != p.Hash {
			return ErrConflict
		}
		if current {
			if actual.State != "submitting" && actual.State != "unknown" {
				return ErrConflict
			}
			var policy Policy
			if err = json.Unmarshal(raw, &policy); err != nil {
				return err
			}
			allowed, err := s.store.currentPaymentAuthority(ctx, tx, actual, policy, version)
			if err != nil {
				return err
			}
			if !allowed {
				return ErrUnauthorized
			}
		}
		return fn(tx)
	})
}
func (s *StripeService) renewalStage(ctx context.Context, p Payment, c stripeRenewalClaim, next string) error {
	return s.renewalMutation(ctx, p, c, true, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE sandbox_stripe_renewal_claims SET state=$2,updated_at=clock_timestamp() WHERE payment_id=$1", p.ID, next)
		return err
	})
}
