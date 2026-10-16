package sandbox

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *StripeService) RunWorker(ctx context.Context) error {
	if err := s.VerifyAccount(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := s.tick(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func (s *StripeService) tick(ctx context.Context) error {
	if err := s.ProcessEvents(ctx); err != nil {
		return err
	}
	type candidate struct {
		ID, State string
		Refund    bool
	}
	var candidates []candidate
	err := s.store.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT p.id::text,p.state,false FROM sandbox_payments p WHERE subscription_id IS NULL AND state IN('authorized','submitting','unknown','pending') AND quote->>'payee'=$1 AND quote->>'product'=$2 AND quote->>'rail'='stripe_test'
 UNION ALL SELECT r.id::text,r.state,true FROM sandbox_refunds r JOIN sandbox_payments p ON p.id=r.payment_id WHERE r.state IN('authorized','pending','unknown') AND p.quote->>'payee'=$1 AND p.quote->>'product'=$2 AND p.quote->>'rail'='stripe_test' LIMIT 100`, s.config.AccountID, s.config.Product)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c candidate
			if err = rows.Scan(&c.ID, &c.State, &c.Refund); err != nil {
				return err
			}
			candidates = append(candidates, c)
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	for _, c := range candidates {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Outcomes are durable per aggregate. A failed provider request does not
		// stop processing signed facts or permit a new creation attempt.
		if c.Refund {
			if c.State == "authorized" {
				_ = s.DispatchRefund(ctx, c.ID)
			} else {
				_ = s.ReconcileUnknownRefund(ctx, c.ID)
			}
		} else {
			if c.State == "authorized" {
				_ = s.DispatchPayment(ctx, c.ID)
			} else if c.State == "pending" {
				_ = s.ReconcilePayment(ctx, c.ID)
			} else {
				_ = s.ReconcileUnknown(ctx, c.ID)
			}
		}
	}
	return nil
}
