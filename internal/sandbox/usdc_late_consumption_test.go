package sandbox

import (
	"context"
	"errors"
	"testing"
)

// An unused nonce observation cannot revoke an externally held signed proof.
// Local revocation blocks new disclosure; it cannot prove no later chain spend.
func TestUSDCLateNonceConsumptionAfterLocalRevocationRetainsHistoricalRecovery(t *testing.T) {
	f := newUSDCChainFixture(t)
	ctx := context.Background()
	f.phase.Store(2)
	f.prebroadcastHead.Store(100)
	if err := f.service.VerifyConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	var key, period string
	if err := f.owner.db.QueryRow(ctx, "SELECT provider_key,reservation_period::text FROM undergrid_sandbox.sandbox_payments WHERE id=$1", f.payment.ID).Scan(&key, &period); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.db.Exec(ctx, "UPDATE undergrid_sandbox.sandbox_credentials SET revoked_at=now() WHERE actor_id=$1", f.owner.actorID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.DispatchPayment(ctx, f.payment.ID); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked current dispatch: %v", err)
	}
	assertState := func(want string, facts int) {
		t.Helper()
		var state, actualKey, actualPeriod string
		var effects int
		if err := f.owner.db.QueryRow(ctx, "SELECT state,provider_key,reservation_period::text FROM undergrid_sandbox.sandbox_payments WHERE id=$1", f.payment.ID).Scan(&state, &actualKey, &actualPeriod); err != nil {
			t.Fatal(err)
		}
		if err := f.owner.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1", f.payment.ID).Scan(&effects); err != nil {
			t.Fatal(err)
		}
		if state != want || effects != facts || actualKey != key || actualPeriod != period || f.verifies.Load() != 0 || f.submits.Load() != 0 {
			t.Fatalf("state=%s facts=%d key=%v period=%v verify=%d submit=%d", state, effects, actualKey == key, actualPeriod == period, f.verifies.Load(), f.submits.Load())
		}
	}
	assertState("unknown", 0)
	// A later externally broadcast canonical transfer uses the original proof.
	f.nonceConsumed.Store(true)
	f.phase.Store(1)
	if err := f.service.tickUSDC(ctx); err != nil {
		t.Fatal(err)
	}
	assertState("settled", 2)
}
