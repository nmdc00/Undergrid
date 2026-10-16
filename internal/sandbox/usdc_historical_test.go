package sandbox

import (
	"context"
	"testing"
)

func TestUSDCHistoricalPrebroadcastRecoversWithoutNewAuthorityOrSubmission(t *testing.T) {
	for _, tc := range []struct {
		name            string
		head            int64
		revoke, missing bool
	}{
		{name: "same_head_created_second", head: 100},
		{name: "earlier_block_after_credential_revocation", head: 105, revoke: true},
		{name: "consumed_nonce_without_canonical_transfer", head: 105, revoke: true, missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newUSDCChainFixture(t)
			ctx := context.Background()
			f.phase.Store(1)
			if tc.missing {
				f.phase.Store(2)
			}
			f.prebroadcastHead.Store(tc.head)
			f.nonceConsumed.Store(true)
			if err := f.service.VerifyConfiguration(ctx); err != nil {
				t.Fatal(err)
			}
			if tc.revoke {
				if _, err := f.owner.db.Exec(ctx, "UPDATE undergrid_sandbox.sandbox_credentials SET revoked_at=now() WHERE actor_id=$1", f.owner.actorID); err != nil {
					t.Fatal(err)
				}
			}
			var key, period string
			if err := f.owner.db.QueryRow(ctx, "SELECT provider_key,reservation_period::text FROM undergrid_sandbox.sandbox_payments WHERE id=$1", f.payment.ID).Scan(&key, &period); err != nil {
				t.Fatal(err)
			}
			if err := f.service.tickUSDC(ctx); err != nil {
				t.Fatal(err)
			}
			var state, actualKey, actualPeriod string
			var effects int
			if err := f.owner.db.QueryRow(ctx, "SELECT state,provider_key,reservation_period::text FROM undergrid_sandbox.sandbox_payments WHERE id=$1", f.payment.ID).Scan(&state, &actualKey, &actualPeriod); err != nil {
				t.Fatal(err)
			}
			if err := f.owner.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1", f.payment.ID).Scan(&effects); err != nil {
				t.Fatal(err)
			}
			expected, economicFacts := "settled", 2
			if tc.missing {
				expected, economicFacts = "unknown", 0
			}
			if state != expected || effects != economicFacts || key != actualKey || period != actualPeriod {
				t.Fatalf("historical nonce recovery state=%s effects=%d key_retained=%v period_retained=%v", state, effects, key == actualKey, period == actualPeriod)
			}
			if f.submits.Load() != 0 || f.verifies.Load() != 0 {
				t.Fatalf("historical proof disclosed to provider verify=%d settle=%d", f.verifies.Load(), f.submits.Load())
			}
		})
	}
}
