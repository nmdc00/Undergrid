package sandbox

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type StripeBillingSetup struct {
	ID                                                                              string `json:"id"`
	State                                                                           string `json:"state"`
	URL                                                                             string `json:"setup_url,omitempty"`
	Designation                                                                     string `json:"designation"`
	subscription, actor, consent, account, price, customer, session, intent, method string
	created                                                                         time.Time
}

func loadStripeBilling(ctx context.Context, tx pgx.Tx, subscription string) (StripeBillingSetup, error) {
	var b StripeBillingSetup
	err := tx.QueryRow(ctx, `SELECT id::text,subscription_id::text,actor_id::text,consent_hash,state,COALESCE(account,''),COALESCE(price_id,''),COALESCE(customer_id,''),COALESCE(session_id,''),COALESCE(setup_url,''),COALESCE(setup_intent_id,''),COALESCE(payment_method_id,''),created_at FROM sandbox_stripe_billing_setups WHERE subscription_id=$1`, subscription).Scan(&b.ID, &b.subscription, &b.actor, &b.consent, &b.State, &b.account, &b.price, &b.customer, &b.session, &b.URL, &b.intent, &b.method, &b.created)
	b.Designation = "TEST"
	return b, err
}
func (s *Store) RequestStripeBillingSetup(ctx context.Context, o Owner, subscription, consent, key string) (StripeBillingSetup, error) {
	var b StripeBillingSetup
	if !validUUID(subscription) || len(consent) != 64 || key == "" || len(key) > 128 {
		return b, ErrInvalid
	}
	err := s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		a, err := ownerContext(ctx, tx, o)
		if err != nil {
			return err
		}
		sub, err := loadSubscription(ctx, tx, subscription, true)
		if err != nil {
			return err
		}
		var actor string
		if err = tx.QueryRow(ctx, "SELECT actor_id::text FROM sandbox_subscriptions WHERE id=$1", subscription).Scan(&actor); err != nil {
			return err
		}
		if actor != a.actor {
			return ErrUnauthorized
		}
		if sub.Hash != consent {
			return ErrConflict
		}
		if err = s.currentBillingConsent(ctx, tx, sub); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO sandbox_stripe_billing_setups(id,subscription_id,actor_id,consent_hash) VALUES($1,$2,$3,$4) ON CONFLICT(subscription_id) DO NOTHING`, randomID(), subscription, a.actor, consent)
		if err != nil {
			return err
		}
		b, err = loadStripeBilling(ctx, tx, subscription)
		return err
	})
	return b, err
}
func (s *Store) currentBillingConsent(ctx context.Context, tx pgx.Tx, sub Subscription) error {
	if sub.State == "canceled" || !sub.Consent.ExpiresAt.After(time.Now()) {
		return ErrUnauthorized
	}
	var valid bool
	if err := tx.QueryRow(ctx, "SELECT revoked_at IS NULL AND expires_at>clock_timestamp() AND 'payments:authorize'=ANY(scopes) FROM sandbox_credentials WHERE id=$1", sub.Consent.CredentialID).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return ErrUnauthorized
	}
	return nil
}
func (s *Store) StripeBillingSetup(ctx context.Context, o Owner, subscription string) (StripeBillingSetup, error) {
	var b StripeBillingSetup
	if !validUUID(subscription) {
		return b, ErrInvalid
	}
	err := s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		if _, err := ownerContext(ctx, tx, o); err != nil {
			return err
		}
		var err error
		b, err = loadStripeBilling(ctx, tx, subscription)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnauthorized
		}
		return err
	})
	return b, err
}
