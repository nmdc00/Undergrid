package sandbox

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// RunRenewalWorker prepares bounded reservations only. A trusted provider
// adapter must finalize/collect manually after BeginDispatch rechecks authority.
func (s *Store) RunRenewalWorker(ctx context.Context) error {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := s.sweepRenewals(ctx); err != nil {
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

func (s *Store) sweepRenewals(ctx context.Context) error {
	var ids []string
	err := s.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT id::text FROM sandbox_subscriptions WHERE state='active' AND next_due_at<=now() AND expires_at>now() ORDER BY updated_at,id LIMIT 100")
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
		if _, err = s.PrepareRenewal(ctx, id); err != nil {
			if !errors.Is(err, ErrUnauthorized) && !errors.Is(err, ErrLimit) && !errors.Is(err, ErrConflict) {
				return err
			}
			// Rotate blocked candidates rather than allowing one actor to starve others.
			if err = s.transaction(ctx, "undergrid_sandbox_worker", func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, "UPDATE sandbox_subscriptions SET updated_at=now() WHERE id=$1", id)
				return err
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
