package main

import (
	"context"
	"flag"
	"os"
	"testing"
)

func TestSandboxAPIRejectsWorkerProviderCredentials(t *testing.T) {
	oldArgs, oldFlags := os.Args, flag.CommandLine
	defer func() { os.Args = oldArgs; flag.CommandLine = oldFlags }()
	os.Args = []string{"undergrid-sandbox"}
	flag.CommandLine = flag.NewFlagSet("sandbox", flag.ContinueOnError)
	t.Setenv("UNDERGRID_SANDBOX_DATABASE_URL", "postgres://api-fixture.invalid/separate-sandbox")
	t.Setenv("UNDERGRID_SANDBOX_STRIPE_SECRET_KEY", "sk_test_fixture")
	if err := run(); err == nil || err.Error() != "sandbox API must not carry provider worker credentials" {
		t.Fatalf("worker key admitted to owner API: %v", err)
	}
}
func TestStripeWorkerRejectsLiveKeyBeforeProviderIO(t *testing.T) {
	t.Setenv("UNDERGRID_SANDBOX_STRIPE_SECRET_KEY", "sk_live_disallowed")
	if err := runStripe(context.Background(), "postgres://worker-fixture.invalid/separate-sandbox"); err == nil || err.Error() != "invalid isolated Stripe TEST worker configuration" {
		t.Fatalf("live credentials admitted: %v", err)
	}
}

func TestSandboxRequiresDedicatedConfiguration(t *testing.T) {
	oldArgs, oldFlags := os.Args, flag.CommandLine
	defer func() { os.Args = oldArgs; flag.CommandLine = oldFlags }()
	os.Args = []string{"undergrid-sandbox"}
	flag.CommandLine = flag.NewFlagSet("sandbox", flag.ContinueOnError)
	t.Setenv("UNDERGRID_SANDBOX_DATABASE_URL", "")
	t.Setenv("UNDERGRID_DATABASE_URL", "postgres://network-credentials.invalid/network")
	if err := run(); err == nil || err.Error() != "separate sandbox database configuration required" {
		t.Fatalf("network config substituted: %v", err)
	}
}

func TestSandboxRejectsPrivilegedAndCombinedRuntimeLogin(t *testing.T) {
	for _, name := range []string{"UNDERGRID_SANDBOX_TEST_DATABASE_URL", "UNDERGRID_SANDBOX_TEST_RUNTIME_DATABASE_URL"} {
		dsn := os.Getenv(name)
		if dsn == "" {
			if os.Getenv("CI") != "" {
				t.Fatal("CI requires sandbox startup fixture configuration")
			}
			t.Skip("requires sandbox PostgreSQL fixture")
		}
		t.Run(name, func(t *testing.T) {
			oldArgs, oldFlags := os.Args, flag.CommandLine
			defer func() { os.Args = oldArgs; flag.CommandLine = oldFlags }()
			os.Args = []string{"undergrid-sandbox"}
			flag.CommandLine = flag.NewFlagSet("sandbox", flag.ContinueOnError)
			t.Setenv("UNDERGRID_SANDBOX_DATABASE_URL", dsn)
			if err := run(); err == nil || err.Error() != "sandbox API requires isolated nonowner app login without worker privileges" {
				t.Fatalf("unsafe runtime login accepted: %v", err)
			}
		})
	}
}

func TestRenewalWorkerRejectsPrivilegedAndCombinedRuntimeLogin(t *testing.T) {
	for _, mode := range []string{"renewal-worker", "stripe-worker"} {
		for _, name := range []string{"UNDERGRID_SANDBOX_TEST_DATABASE_URL", "UNDERGRID_SANDBOX_TEST_RUNTIME_DATABASE_URL"} {
			dsn := os.Getenv(name)
			if dsn == "" {
				if os.Getenv("CI") != "" {
					t.Fatal("CI requires separate sandbox fixture")
				}
				t.Skip("requires sandbox PostgreSQL fixture")
			}
			t.Run(mode+"/"+name, func(t *testing.T) {
				oldArgs, oldFlags := os.Args, flag.CommandLine
				defer func() { os.Args = oldArgs; flag.CommandLine = oldFlags }()
				os.Args = []string{"undergrid-sandbox", "-mode", mode}
				flag.CommandLine = flag.NewFlagSet("sandbox", flag.ContinueOnError)
				t.Setenv("UNDERGRID_SANDBOX_DATABASE_URL", dsn)
				if err := run(); err == nil || err.Error() != "sandbox renewal worker requires isolated nonowner worker login without app privileges" {
					t.Fatalf("unsafe renewal worker login accepted: %v", err)
				}
			})
		}
	}
}
