package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Exercise the actual app LOGIN/RLS boundary, not an HTTP-reachable SQL command.
// A supplied actor must not consume another actor's unique subscription slot.
func TestStripeBillingSetupDatabaseRejectsMixedActorSubscription(t *testing.T) {
	f := newStripeRecoveryFixture(t)
	ctx := context.Background()
	var credential string
	if err := f.db.QueryRow(ctx, "SELECT id::text FROM undergrid_sandbox.sandbox_credentials WHERE digest=$1", hash([]byte(f.token))).Scan(&credential); err != nil {
		t.Fatal(err)
	}
	app := New(usdcFixtureRoleDSN(t, f.db, "undergrid_sandbox_app"))
	sub, err := app.CreateSubscription(ctx, f.owner, "actor-bound-consent", SubscriptionConsent{CredentialID: credential, Maximum: "100", PeriodSeconds: 86400, RenewalCount: 2, ExpiresAt: time.Now().Add(72 * time.Hour)}, f.p.Quote)
	if err != nil {
		t.Fatal(err)
	}
	other := Owner{Issuer: f.owner.Issuer, Subject: "other-setup-actor-" + randomID()}
	otherActor, err := app.Enroll(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	accepted := errors.New("fixture rollback: mixed-actor insert was accepted")
	err = app.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		if _, err := ownerContext(ctx, tx, other); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO sandbox_stripe_billing_setups(id,subscription_id,actor_id,consent_hash) VALUES($1,$2,$3,$4)`, randomID(), sub.ID, otherActor, hash([]byte("arbitrary non-owner consent"))); err != nil {
			return err
		}
		return accepted // Always roll back a successful attack oracle.
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("mixed-actor setup not rejected by ownership FK: %v", err)
	}
	var count int
	if err := f.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_stripe_billing_setups WHERE subscription_id=$1", sub.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("foreign setup consumed owner's slot", count)
	}
	if setup, err := app.RequestStripeBillingSetup(ctx, f.owner, sub.ID, sub.Hash, "legitimate-owner-setup"); err != nil || setup.State != "requested" {
		t.Fatal("owner setup blocked after rejected insert", setup, err)
	}
}

// A corrupted/stale worker snapshot must be rejected before the transition
// callback. This does not assert that ordinary HTTP can construct that snapshot.
func TestStripeBillingWorkerRejectsActorSnapshotDrift(t *testing.T) {
	f := newRenewalProviderFixture(t)
	ctx := context.Background()
	other := Owner{Issuer: f.base.owner.Issuer, Subject: "other-billing-snapshot-" + randomID()}
	otherActor, err := f.app.Enroll(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.service.billing(ctx, f.sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	b.actor = otherActor
	entered := false
	err = f.service.billingMutation(ctx, b, true, func(pgx.Tx, StripeBillingSetup) error { entered = true; return ErrInvalid })
	if !errors.Is(err, ErrConflict) || entered {
		t.Fatalf("wrong actor reached billing transition: entered=%v err=%v", entered, err)
	}
	if f.posts("/v1/customers") != 0 || f.posts("/v1/checkout/sessions") != 0 {
		t.Fatal("wrong actor caused provider work")
	}
}
