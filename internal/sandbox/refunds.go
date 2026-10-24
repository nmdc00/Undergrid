package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
)

type Refund struct {
	ID          string `json:"id"`
	PaymentID   string `json:"payment_id"`
	Hash        string `json:"refund_hash"`
	Asset       string `json:"asset"`
	Amount      string `json:"amount_atomic"`
	State       string `json:"state"`
	Designation string `json:"designation"`
	ProviderKey string `json:"-"`
}

func loadRefund(ctx context.Context, tx pgx.Tx, id string, lock bool) (Refund, error) {
	r := Refund{Designation: "TEST — NOT A LIVE COMMITMENT"}
	query := "SELECT id::text,payment_id::text,refund_hash,asset,amount::text,state,provider_key FROM sandbox_refunds WHERE id=$1"
	if lock {
		query += " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, query, id).Scan(&r.ID, &r.PaymentID, &r.Hash, &r.Asset, &r.Amount, &r.State, &r.ProviderKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrUnauthorized
	}
	return r, err
}

// Authorized, pending and unknown refunds reserve the same verified refundable
// charge. Succeeded refunds are counted by immutable effects, never twice.
func refundCapacity(ctx context.Context, tx pgx.Tx, payment, amount, exclude string) (bool, error) {
	var excluded any
	if exclude != "" {
		excluded = exclude
	}
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT
 COALESCE((SELECT sum(CASE kind WHEN 'charge' THEN amount WHEN 'refund' THEN -amount ELSE 0 END) FROM sandbox_refundable_effects WHERE payment_id=$1),0)
 - COALESCE((SELECT sum(amount) FROM sandbox_refunds WHERE payment_id=$1 AND ($3::uuid IS NULL OR id<>$3) AND state IN('authorized','pending','unknown')),0) >= $2::numeric`, payment, amount, excluded).Scan(&allowed)
	return allowed, err
}
func refundPermission(a identity, p Payment) error {
	if !a.policy.AllowRefunds || !permits(a.policy, p.Quote) {
		return ErrUnauthorized
	}
	return nil
}
func rememberRefund(ctx context.Context, tx pgx.Tx, a identity, op, key, digest, id string) error {
	_, err := tx.Exec(ctx, "INSERT INTO sandbox_idempotency(actor_id,credential_id,operation,key,request_hash,refund_id) VALUES($1,$2,$3,$4,$5,$6)", a.actor, a.credential, op, key, digest, id)
	return err
}
func (s *Store) RequestRefund(ctx context.Context, token, key, payment, amount string) (Refund, error) {
	var r Refund
	if !validUUID(payment) || !positive(amount, false) {
		return r, ErrInvalid
	}
	err := s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		a, err := agentContext(ctx, tx, token, "refunds:request")
		if err != nil {
			return err
		}
		p, err := loadPayment(ctx, tx, payment)
		if err != nil {
			return err
		}
		if err = refundPermission(a, p); err != nil {
			return err
		}
		digest := hash(encode([]string{payment, amount}))
		old, err := replay(ctx, tx, a, "refund-request", key, digest)
		if err != nil {
			return err
		}
		if old != "" {
			r, err = loadRefund(ctx, tx, old, false)
			return err
		}
		if p.State != "settled" {
			return ErrConflict
		}
		allowed, err := refundCapacity(ctx, tx, payment, amount, "")
		if err != nil {
			return err
		}
		if !allowed {
			return ErrLimit
		}
		id := randomID()
		bound := hash(encode([]string{a.actor, a.credential, payment, p.Hash, amount, id}))
		if _, err = tx.Exec(ctx, "INSERT INTO sandbox_refunds(id,actor_id,payment_id,credential_id,asset,amount,refund_hash,provider_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", id, a.actor, payment, a.credential, p.Quote.Asset, amount, bound, "ugs-refund-"+id); err != nil {
			return err
		}
		if err = rememberRefund(ctx, tx, a, "refund-request", key, digest, id); err != nil {
			return err
		}
		r, err = loadRefund(ctx, tx, id, false)
		return err
	})
	return r, err
}
func (s *Store) GetRefund(ctx context.Context, token, id string) (Refund, error) {
	var r Refund
	if !validUUID(id) {
		return r, ErrInvalid
	}
	err := s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		if _, err := agentContext(ctx, tx, token, "refunds:read"); err != nil {
			return err
		}
		var err error
		r, err = loadRefund(ctx, tx, id, false)
		return err
	})
	return r, err
}
func (s *Store) AuthorizeRefund(ctx context.Context, token, id, refundHash, key string) (Refund, error) {
	var r Refund
	if !validUUID(id) {
		return r, ErrInvalid
	}
	err := s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		a, err := agentContext(ctx, tx, token, "refunds:authorize")
		if err != nil {
			return err
		}
		r, err = loadRefund(ctx, tx, id, true)
		if err != nil {
			return err
		}
		p, err := loadPayment(ctx, tx, r.PaymentID)
		if err != nil {
			return err
		}
		if err = refundPermission(a, p); err != nil {
			return err
		}
		digest := hash(encode([]string{id, refundHash}))
		old, err := replay(ctx, tx, a, "refund-authorize", key, digest)
		if err != nil {
			return err
		}
		if old != "" {
			r, err = loadRefund(ctx, tx, old, false)
			return err
		}
		var credential string
		if err = tx.QueryRow(ctx, "SELECT credential_id::text FROM sandbox_refunds WHERE id=$1", id).Scan(&credential); err != nil {
			return err
		}
		if credential != a.credential || r.Hash != refundHash || r.State != "requested" {
			return ErrConflict
		}
		allowed, err := refundCapacity(ctx, tx, r.PaymentID, r.Amount, id)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrLimit
		}
		if _, err = tx.Exec(ctx, "UPDATE sandbox_refunds SET state='authorized',policy_version=$2,authority_proof=$3,updated_at=now() WHERE id=$1", id, a.version, encode(map[string]any{"actor": a.actor, "credential": a.credential, "refund_hash": r.Hash, "policy_version": a.version, "designation": "TEST"})); err != nil {
			return err
		}
		if err = rememberRefund(ctx, tx, a, "refund-authorize", key, digest, id); err != nil {
			return err
		}
		r.State = "authorized"
		return nil
	})
	return r, err
}

// Worker claims commit before provider IO. Unknown outcomes cannot redispatch.
func (s *Store) BeginRefundDispatch(ctx context.Context, id string) (Refund, error) {
	var r Refund
	blocked := false
	if !validUUID(id) {
		return r, ErrInvalid
	}
	err := s.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		var actor string
		if err := tx.QueryRow(ctx, "SELECT actor_id::text FROM sandbox_refunds WHERE id=$1", id).Scan(&actor); err != nil {
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
		var err error
		r, err = loadRefund(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if r.State != "authorized" {
			return ErrConflict
		}
		p, err := loadPayment(ctx, tx, r.PaymentID)
		if err != nil {
			return err
		}
		var valid bool
		if err = tx.QueryRow(ctx, "SELECT r.policy_version=$2 AND c.revoked_at IS NULL AND c.expires_at>clock_timestamp() FROM sandbox_refunds r JOIN sandbox_credentials c ON c.id=r.credential_id WHERE r.id=$1", id, version).Scan(&valid); err != nil {
			return err
		}
		allowed, err := refundCapacity(ctx, tx, r.PaymentID, r.Amount, id)
		if err != nil {
			return err
		}
		blocked = !valid || !allowed || !policy.AllowRefunds || !permits(policy, p.Quote)
		r.State = "pending"
		if blocked {
			r.State = "failed"
		}
		_, err = tx.Exec(ctx, "UPDATE sandbox_refunds SET state=$2,updated_at=now() WHERE id=$1", id, r.State)
		return err
	})
	if err == nil && blocked {
		err = ErrUnauthorized
	}
	return r, err
}
func (s *Store) MarkRefundUnknown(ctx context.Context, id string) error {
	if !validUUID(id) {
		return ErrInvalid
	}
	return s.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE sandbox_refunds SET state='unknown',updated_at=now() WHERE id=$1 AND state IN('pending','unknown')", id)
		if err == nil && tag.RowsAffected() != 1 {
			return ErrConflict
		}
		return err
	})
}
