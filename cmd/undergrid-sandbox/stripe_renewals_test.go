package main

import (
	"context"
	"testing"
)

func TestStripeRenewalCollectionRequiresExplicitWorkerOptIn(t *testing.T) {
	for _, tc := range []struct {
		value          string
		enabled, valid bool
	}{{"", false, true}, {"manual_test_v1", true, true}, {"true", false, false}, {"automatic", false, false}} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("UNDERGRID_SANDBOX_STRIPE_RENEWALS", tc.value)
			c, err := stripeConfigFromEnvironment()
			if (err == nil) != tc.valid || c.RenewalsEnabled != tc.enabled {
				t.Fatalf("renewal opt-in %q enabled=%v err=%v", tc.value, c.RenewalsEnabled, err)
			}
		})
	}
}

func TestStripeRenewalRuntimeRejectsUnsupportedCollectionMode(t *testing.T) {
	t.Setenv("UNDERGRID_SANDBOX_STRIPE_RENEWALS", "automatic")
	err := runStripe(context.Background(), "unavailable fixture database")
	if err == nil || err.Error() != "explicit manual TEST renewal configuration required" {
		t.Fatalf("runtime did not enforce manual collection mode before provider IO: %v", err)
	}
}
