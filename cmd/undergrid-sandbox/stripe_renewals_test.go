package main

import "testing"

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
