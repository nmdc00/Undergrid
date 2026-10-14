package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

// BeginDispatch is worker-only. Claiming the payment commits before an external
// provider call. A crash leaves submitting, which requires reconciliation rather
// than a new charge. The payment's provider key never changes.
func (s *Store) BeginDispatch(ctx context.Context, id string) (Payment, error) {
	var p Payment
	blocked := false
	err := s.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		var actor string
		if err := tx.QueryRow(ctx, "SELECT actor_id::text FROM sandbox_payments WHERE id=$1", id).Scan(&actor); err != nil {
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
		p, err = loadPayment(ctx, tx, id)
		if err != nil {
			return err
		}
		if p.State != "authorized" {
			return ErrConflict
		}
		var valid bool
		err = tx.QueryRow(ctx, `SELECT p.policy_version=$2 AND p.reservation_period=(now() AT TIME ZONE 'UTC')::date AND c.revoked_at IS NULL AND c.expires_at>now() FROM sandbox_payments p JOIN sandbox_credentials c ON c.id=p.credential_id WHERE p.id=$1`, id, version).Scan(&valid)
		if err != nil {
			return err
		}
		var consentValid bool
		if err = tx.QueryRow(ctx, "SELECT subscription_id IS NULL OR EXISTS(SELECT FROM sandbox_subscriptions s WHERE s.id=p.subscription_id AND s.state<>'canceled' AND s.expires_at>now()) FROM sandbox_payments p WHERE p.id=$1", id).Scan(&consentValid); err != nil {
			return err
		}
		walletErr := s.checkUSDCQuote(ctx, tx, p, true)
		if walletErr != nil && !errors.Is(walletErr, ErrConflict) && !errors.Is(walletErr, ErrUnauthorized) {
			return walletErr
		}
		walletValid := walletErr == nil
		if walletValid && p.Quote.Asset == "USDC" && p.WalletAuthorization == nil {
			return ErrUnauthorized
		}
		if walletValid {
			payerErr := s.checkPayerAuthorization(ctx, tx, p)
			if payerErr != nil && !errors.Is(payerErr, ErrConflict) && !errors.Is(payerErr, ErrUnauthorized) && !errors.Is(payerErr, ErrInvalid) {
				return payerErr
			}
			walletValid = payerErr == nil
		}
		if !walletValid || !consentValid || !valid || !permits(policy, p.Quote) || !p.Quote.ExpiresAt.After(time.Now()) {
			blocked = true
			p.State = "canceled"
		} else {
			p.State = "submitting"
		}
		_, err = tx.Exec(ctx, "UPDATE sandbox_payments SET state=$2,updated_at=now() WHERE id=$1", id, p.State)
		return err
	})
	if err == nil && blocked {
		err = ErrUnauthorized
	}
	return p, err
}
func (s *Store) MarkUnknown(ctx context.Context, id string) error {
	return s.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE sandbox_payments SET state='unknown',updated_at=now() WHERE id=$1 AND state IN('submitting','pending','unknown')", id)
		if err == nil && tag.RowsAffected() != 1 {
			return ErrConflict
		}
		return err
	})
}

// VerifiedEffect is internal to provider reconciliation. It must never be bound
// directly to an agent HTTP payload: adapters verify the provider object first.
// This slice intentionally exposes no webhook/effect ingestion HTTP endpoint.
type VerifiedEffect struct {
	Provider, Account, EventID, FactID, PaymentID, Kind, Asset, Amount string
	RefundID                                                           string
	Evidence                                                           json.RawMessage
}

func (s *Store) RecordEffect(ctx context.Context, e VerifiedEffect) error {
	if err := validateEffect(e); err != nil {
		return err
	}
	return s.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error { return recordEffect(ctx, tx, e) })
}

func validateEffect(e VerifiedEffect) error {
	if e.RefundID != "" && (e.Kind != "refund" || !validUUID(e.RefundID)) {
		return ErrInvalid
	}
	if e.Provider != "stripe_test" || e.Account == "" || e.EventID == "" || e.FactID == "" || !positive(e.Amount, false) || !json.Valid(e.Evidence) || (e.Kind != "charge" && e.Kind != "refund") {
		return ErrInvalid
	}
	return nil
}

func recordEffect(ctx context.Context, tx pgx.Tx, e VerifiedEffect) error {
	var actor string
	if err := tx.QueryRow(ctx, "SELECT actor_id::text FROM sandbox_payments WHERE id=$1", e.PaymentID).Scan(&actor); err != nil {
		return err
	}
	// Use the same actor-first lock order as authorization and dispatch.
	if _, err := tx.Exec(ctx, "SELECT id FROM sandbox_actors WHERE id=$1 FOR UPDATE", actor); err != nil {
		return err
	}
	p, err := loadPayment(ctx, tx, e.PaymentID)
	if err != nil {
		return err
	}
	if e.Asset != p.Quote.Asset || e.Account != p.Quote.Payee || e.Provider != p.Quote.Rail {
		return ErrConflict
	}
	if e.Kind == "charge" && e.Amount != p.Quote.Amount {
		return ErrConflict
	}
	if e.RefundID != "" {
		r, err := loadRefund(ctx, tx, e.RefundID, true)
		if err != nil {
			return err
		}
		if r.PaymentID != e.PaymentID || r.Amount != e.Amount || r.Asset != e.Asset {
			return ErrConflict
		}
	}
	// Event replay must retain its original payment and body. Economic facts have
	// separate identities because several legitimate provider events name one charge.
	var oldPayment string
	var oldEvidence []byte
	var oldFact, oldKind, oldAsset, oldAmount, oldRefund string
	err = tx.QueryRow(ctx, "SELECT payment_id::text,evidence,fact_id,kind,asset,amount::text,COALESCE(refund_id::text,'') FROM sandbox_provider_events WHERE provider=$1 AND account=$2 AND environment='test' AND event_id=$3", e.Provider, e.Account, e.EventID).Scan(&oldPayment, &oldEvidence, &oldFact, &oldKind, &oldAsset, &oldAmount, &oldRefund)
	if err == nil {
		var equal bool
		if err = tx.QueryRow(ctx, "SELECT $1::jsonb=$2::jsonb", oldEvidence, []byte(e.Evidence)).Scan(&equal); err != nil {
			return err
		}
		if oldPayment != e.PaymentID || !equal || oldFact != e.FactID || oldKind != e.Kind || oldAsset != e.Asset || oldAmount != e.Amount || oldRefund != e.RefundID {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var kind, asset, amount string
	err = tx.QueryRow(ctx, "SELECT payment_id::text,kind,asset,amount::text,COALESCE(refund_id::text,'') FROM sandbox_effects WHERE provider=$1 AND account=$2 AND environment='test' AND fact_id=$3", e.Provider, e.Account, e.FactID).Scan(&oldPayment, &kind, &asset, &amount, &oldRefund)
	exists := err == nil
	if exists && (oldPayment != e.PaymentID || kind != e.Kind || asset != e.Asset || amount != e.Amount || oldRefund != e.RefundID) {
		return ErrConflict
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if !exists {
		if e.Kind == "refund" {
			var valid bool
			err = tx.QueryRow(ctx, `SELECT COALESCE(sum(CASE kind WHEN 'charge' THEN amount WHEN 'refund' THEN -amount ELSE 0 END),0)>=$2::numeric FROM sandbox_effects WHERE payment_id=$1`, e.PaymentID, e.Amount).Scan(&valid)
			if err != nil {
				return err
			}
			if !valid {
				return ErrConflict
			}
		}
		if e.Kind == "charge" {
			var charged bool
			if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM sandbox_effects WHERE payment_id=$1 AND kind='charge')", e.PaymentID).Scan(&charged); err != nil {
				return err
			}
			if charged {
				return ErrConflict
			}
		}
		if _, err = tx.Exec(ctx, "INSERT INTO sandbox_effects(provider,account,environment,fact_id,payment_id,kind,asset,amount,evidence,refund_id) VALUES($1,$2,'test',$3,$4,$5,$6,$7,$8,nullif($9,'')::uuid)", e.Provider, e.Account, e.FactID, e.PaymentID, e.Kind, e.Asset, e.Amount, []byte(e.Evidence), e.RefundID); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, "INSERT INTO sandbox_provider_events(provider,account,environment,event_id,payment_id,evidence,fact_id,kind,asset,amount,refund_id) VALUES($1,$2,'test',$3,$4,$5,$6,$7,$8,$9,nullif($10,'')::uuid)", e.Provider, e.Account, e.EventID, e.PaymentID, []byte(e.Evidence), e.FactID, e.Kind, e.Asset, e.Amount, e.RefundID); err != nil {
		return err
	}
	// Valid late provider facts survive policy revocation or local cancellation;
	// financial truth must not be discarded because authorization changed.
	if e.Kind == "charge" {
		_, err = tx.Exec(ctx, "UPDATE sandbox_payments SET state='settled',provider_reference=$2,updated_at=now() WHERE id=$1", e.PaymentID, e.FactID)
	}
	if err == nil && e.RefundID != "" {
		_, err = tx.Exec(ctx, "UPDATE sandbox_refunds SET state='succeeded',provider_reference=$2,updated_at=now() WHERE id=$1", e.RefundID, e.FactID)
	}
	return err
}

// Monetary truth and its mismatch classification commit atomically. This is
// worker-only evidence, never permission to dispatch or accept agent payloads.
func (s *Store) recordUnexpectedCharge(ctx context.Context, e VerifiedEffect, kind, object, evidenceClass string) error {
	if err := validateEffect(e); err != nil {
		return err
	}
	if e.Kind != "charge" || (kind != "checkout_binding_mismatch" && kind != "refund_binding_mismatch") {
		return ErrInvalid
	}
	return s.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		if err := recordEffect(ctx, tx, e); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO sandbox_anomalies(payment_id,kind,object_id,evidence_class) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING", e.PaymentID, kind, object, evidenceClass)
		return err
	})
}
