package sandbox

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
)

type SubscriptionConsent struct {
	CredentialID  string    `json:"credential_id"`
	Maximum       string    `json:"maximum_atomic"`
	PeriodSeconds int       `json:"period_seconds"`
	RenewalCount  int       `json:"renewal_count"`
	ExpiresAt     time.Time `json:"expires_at"`
}
type Subscription struct {
	ID          string              `json:"id"`
	Hash        string              `json:"consent_hash"`
	State       string              `json:"state"`
	Consent     SubscriptionConsent `json:"consent"`
	Offer       Quote               `json:"offer"`
	Reserved    int                 `json:"renewals_reserved"`
	NextDueAt   time.Time           `json:"next_due_at"`
	Designation string              `json:"designation"`
}

func loadSubscription(ctx context.Context, tx pgx.Tx, id string, lock bool) (Subscription, error) {
	sub := Subscription{Designation: "TEST — NOT A LIVE COMMITMENT"}
	query := "SELECT id::text,consent_hash,state,credential_id::text,maximum::text,period_seconds,renewal_count,expires_at,offer,renewals_reserved,next_due_at FROM sandbox_subscriptions WHERE id=$1"
	if lock {
		query += " FOR UPDATE"
	}
	var raw []byte
	err := tx.QueryRow(ctx, query, id).Scan(&sub.ID, &sub.Hash, &sub.State, &sub.Consent.CredentialID, &sub.Consent.Maximum, &sub.Consent.PeriodSeconds, &sub.Consent.RenewalCount, &sub.Consent.ExpiresAt, &raw, &sub.Reserved, &sub.NextDueAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return sub, ErrUnauthorized
	}
	if err == nil {
		err = json.Unmarshal(raw, &sub.Offer)
	}
	return sub, err
}

// The offer comes only from the operator's fixed catalog, never the owner body.
// Consent is explicit, bounded and immutable. It does not itself debit anything.
func (s *Store) CreateSubscription(ctx context.Context, o Owner, c SubscriptionConsent, q Quote) (Subscription, error) {
	var sub Subscription
	now := time.Now().UTC()
	if !validUUID(c.CredentialID) || !positive(c.Maximum, false) || c.RenewalCount < 1 || c.RenewalCount > 2 || c.PeriodSeconds < 86400 || c.PeriodSeconds > 7776000 || !c.ExpiresAt.After(now) || c.ExpiresAt.After(now.Add(90*24*time.Hour)) || q.Rail != "stripe_test" || q.Asset != "USD" || !positive(q.Amount, false) || !positive(q.Fee, true) || q.Product == "" || q.Payee == "" || q.TermsID == "" || len(q.TermsHash) != 64 {
		return sub, ErrInvalid
	}
	if _, err := hex.DecodeString(q.TermsHash); err != nil {
		return sub, ErrInvalid
	}
	amount, _ := new(big.Int).SetString(q.Amount, 10)
	fee, _ := new(big.Int).SetString(q.Fee, 10)
	maximum, _ := new(big.Int).SetString(c.Maximum, 10)
	if amount.Add(amount, fee).Cmp(maximum) > 0 {
		return sub, ErrInvalid
	}
	c.ExpiresAt = c.ExpiresAt.UTC()
	q.ExpiresAt = time.Time{} // each debit gets its own bounded dispatch quote
	err := s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		a, err := ownerContext(ctx, tx, o)
		if err != nil {
			return err
		}
		if !permits(a.policy, q) {
			return ErrUnauthorized
		}
		var valid bool
		if err = tx.QueryRow(ctx, "SELECT revoked_at IS NULL AND expires_at>now() AND 'payments:authorize'=ANY(scopes) FROM sandbox_credentials WHERE id=$1 AND actor_id=$2", c.CredentialID, a.actor).Scan(&valid); errors.Is(err, pgx.ErrNoRows) {
			return ErrUnauthorized
		} else if err != nil {
			return err
		}
		if !valid {
			return ErrUnauthorized
		}
		id := randomID()
		bound := hash(encode(struct {
			Actor, Nonce string
			Consent      SubscriptionConsent
			Offer        Quote
		}{a.actor, id, c, q}))
		_, err = tx.Exec(ctx, "INSERT INTO sandbox_subscriptions(id,actor_id,credential_id,offer,maximum,period_seconds,renewal_count,expires_at,next_due_at,consent_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,now()+make_interval(secs=>$6::integer),$9)", id, a.actor, c.CredentialID, encode(q), c.Maximum, c.PeriodSeconds, c.RenewalCount, c.ExpiresAt, bound)
		if err != nil {
			return err
		}
		sub, err = loadSubscription(ctx, tx, id, false)
		return err
	})
	return sub, err
}
func (s *Store) GetSubscription(ctx context.Context, o Owner, id string) (Subscription, error) {
	var sub Subscription
	if !validUUID(id) {
		return sub, ErrInvalid
	}
	err := s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		if _, err := ownerContext(ctx, tx, o); err != nil {
			return err
		}
		var err error
		sub, err = loadSubscription(ctx, tx, id, false)
		return err
	})
	return sub, err
}
func (s *Store) CancelSubscription(ctx context.Context, o Owner, id, consentHash string) error {
	if !validUUID(id) {
		return ErrInvalid
	}
	return s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		if _, err := ownerContext(ctx, tx, o); err != nil {
			return err
		}
		sub, err := loadSubscription(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if sub.Hash != consentHash {
			return ErrConflict
		}
		_, err = tx.Exec(ctx, "UPDATE sandbox_subscriptions SET state='canceled',updated_at=now() WHERE id=$1", id)
		return err
	})
}

// PrepareRenewal reserves before any provider invoice finalization/collection.
// Repeated ticks cannot debit again; a pending/unknown debit keeps its reservation.
func (s *Store) PrepareRenewal(ctx context.Context, id string) (Payment, error) {
	var payment Payment
	if !validUUID(id) {
		return payment, ErrInvalid
	}
	err := s.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		var actor string
		if err := tx.QueryRow(ctx, "SELECT actor_id::text FROM sandbox_subscriptions WHERE id=$1", id).Scan(&actor); err != nil {
			return err
		}
		var raw []byte
		var version int64
		if err := tx.QueryRow(ctx, "SELECT policy,version FROM sandbox_actors WHERE id=$1 FOR UPDATE", actor).Scan(&raw, &version); err != nil {
			return err
		}
		var policy Policy
		if err := json.Unmarshal(raw, &policy); err != nil {
			return err
		}
		sub, err := loadSubscription(ctx, tx, id, true)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if sub.State != "active" || sub.Reserved >= sub.Consent.RenewalCount || sub.NextDueAt.After(now) {
			return ErrConflict
		}
		if !sub.Consent.ExpiresAt.After(now) || !permits(policy, sub.Offer) {
			return ErrUnauthorized
		}
		var valid bool
		if err = tx.QueryRow(ctx, "SELECT revoked_at IS NULL AND expires_at>now() AND 'payments:authorize'=ANY(scopes) FROM sandbox_credentials WHERE id=$1", sub.Consent.CredentialID).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return ErrUnauthorized
		}
		q := sub.Offer
		q.ExpiresAt = now.Add(15 * time.Minute)
		if q.ExpiresAt.After(sub.Consent.ExpiresAt) {
			q.ExpiresAt = sub.Consent.ExpiresAt
		}
		if err = checkPurchaseCapacity(ctx, tx, actor, policy, q); err != nil {
			return err
		}
		paymentID := randomID()
		ordinal := sub.Reserved + 1
		bound := hash(encode(struct {
			Consent, Actor, Credential, Nonce string
			Ordinal                           int
			Quote                             Quote
		}{sub.Hash, actor, sub.Consent.CredentialID, paymentID, ordinal, q}))
		proof := encode(map[string]any{"actor": actor, "credential": sub.Consent.CredentialID, "quote_hash": bound, "consent_hash": sub.Hash, "renewal_ordinal": ordinal, "policy_version": version, "designation": "TEST"})
		_, err = tx.Exec(ctx, "INSERT INTO sandbox_payments(id,actor_id,credential_id,quote,quote_hash,asset,amount,fee,provider_key,state,policy_version,authority_proof,reservation_period,subscription_id,renewal_ordinal) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'authorized',$10,$11,(now() AT TIME ZONE 'UTC')::date,$12,$13)", paymentID, actor, sub.Consent.CredentialID, encode(q), bound, q.Asset, q.Amount, q.Fee, "ugs-"+paymentID, version, proof, id, ordinal)
		if err != nil {
			return err
		}
		state := "active"
		if ordinal == sub.Consent.RenewalCount {
			state = "exhausted"
		}
		if _, err = tx.Exec(ctx, "UPDATE sandbox_subscriptions SET renewals_reserved=$2,state=$3,next_due_at=now()+make_interval(secs=>period_seconds),updated_at=now() WHERE id=$1", id, ordinal, state); err != nil {
			return err
		}
		payment, err = loadPayment(ctx, tx, paymentID)
		return err
	})
	return payment, err
}

func checkPurchaseCapacity(ctx context.Context, tx pgx.Tx, actor string, p Policy, q Quote) error {
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT COALESCE(sum(amount+fee),0)+$2::numeric+$3::numeric <= $4::numeric FROM sandbox_payments WHERE actor_id=$1 AND asset=$5 AND reservation_period=(now() AT TIME ZONE 'UTC')::date AND state IN('authorized','submitting','pending','unknown','settled')`, actor, q.Amount, q.Fee, p.Cap, q.Asset).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrLimit
	}
	return nil
}
