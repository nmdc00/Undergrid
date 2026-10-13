package sandbox

import (
	"context"
	"github.com/jackc/pgx/v5"
	"os"
	"testing"
	"time"
)

func TestAuthorityReservationsAndReconciliation(t *testing.T) {
	dsn := os.Getenv("UNDERGRID_SANDBOX_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("CI requires separate sandbox database configuration")
		}
		t.Skip("requires separate real sandbox PostgreSQL")
	}
	ctx := context.Background()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	if err = Migrate(ctx, c); err != nil {
		t.Fatal(err)
	}
	runtimeDSN := os.Getenv("UNDERGRID_SANDBOX_TEST_RUNTIME_DATABASE_URL")
	if runtimeDSN == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("CI requires nonprivileged sandbox runtime database configuration")
		}
		runtimeDSN = dsn
	}
	s := New(runtimeDSN)
	owner := Owner{Issuer: "https://fixture.invalid", Subject: randomID()}
	actor, err := s.Enroll(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	policy := Policy{Asset: "USD", Cap: "1000", Products: []string{"platform-test"}, Payees: []string{"stripe-test-account"}, ExpiresAt: time.Now().Add(time.Hour)}
	if err = s.SetPolicy(ctx, owner, 1, policy); err != nil {
		t.Fatal(err)
	}
	token, err := s.IssueCredential(ctx, owner, "external-agent", []string{"payments:quote", "payments:authorize", "payments:read"})
	if err != nil {
		t.Fatal(err)
	}
	q := Quote{Product: "platform-test", Asset: "USD", Amount: "600", Fee: "0", Payee: "stripe-test-account", Rail: "stripe_test", TermsID: "test-terms", TermsHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExpiresAt: time.Now().Add(time.Hour)}
	first, err := s.CreateQuote(ctx, token, "quote-1", q)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.CreateQuote(ctx, token, "quote-1", q)
	if err != nil || replay.ID != first.ID {
		t.Fatalf("replay=%+v %v", replay, err)
	}
	q.Amount = "601"
	if _, err = s.CreateQuote(ctx, token, "quote-1", q); err != ErrConflict {
		t.Fatalf("changed idempotency payload: %v", err)
	}
	q.Amount = "600"
	if _, err = s.Authorize(ctx, token, first.ID, first.Hash, "authorize-1"); err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateQuote(ctx, token, "quote-2", q)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorize(ctx, token, second.ID, second.Hash, "authorize-2"); err != ErrLimit {
		t.Fatalf("cap bypass: %v", err)
	}
	dispatched, err := s.BeginDispatch(ctx, first.ID)
	if err != nil || dispatched.State != "submitting" {
		t.Fatalf("dispatch %+v %v", dispatched, err)
	}
	if err = s.MarkUnknown(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorize(ctx, token, second.ID, second.Hash, "authorize-2"); err != ErrLimit {
		t.Fatalf("unknown released reservation: %v", err)
	}
	effect := VerifiedEffect{Provider: "stripe_test", Account: "stripe-test-account", EventID: "evt_" + randomID(), FactID: "charge_" + randomID(), PaymentID: first.ID, Kind: "charge", Asset: "USD", Amount: "600", Evidence: []byte(`{"livemode":false,"verified_by":"fixture"}`)}
	if err = s.RecordEffect(ctx, effect); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordEffect(ctx, effect); err != nil {
		t.Fatalf("provider replay: %v", err)
	}
	changed := effect
	changed.Kind = "refund"
	changed.Amount = "1"
	if err = s.RecordEffect(ctx, changed); err != ErrConflict {
		t.Fatalf("event identity changed economic fact: %v", err)
	}
	effect.EventID = "evt_" + randomID()
	if err = s.RecordEffect(ctx, effect); err != nil {
		t.Fatalf("same charge new event: %v", err)
	}
	refund := effect
	refund.EventID = "evt_" + randomID()
	refund.FactID = "refund_" + randomID()
	refund.Kind = "refund"
	refund.Amount = "300"
	if err = s.RecordEffect(ctx, refund); err != nil {
		t.Fatal(err)
	}
	refund.EventID = "evt_" + randomID()
	refund.FactID = "refund_" + randomID()
	refund.Amount = "301"
	if err = s.RecordEffect(ctx, refund); err != ErrConflict {
		t.Fatalf("overrefund: %v", err)
	}
	if _, err = s.Authorize(ctx, token, second.ID, second.Hash, "authorize-2"); err != ErrLimit {
		t.Fatalf("refund reduced gross cap: %v", err)
	}
	if err = s.RevokeCredentials(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetPayment(ctx, token, first.ID); err != ErrUnauthorized {
		t.Fatalf("revoked token read: %v", err)
	}
	var count int
	if err = c.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1", first.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("effects=%d err=%v", count, err)
	}
	if actor == "" {
		t.Fatal("missing actor")
	}
}

func TestConcurrentCapAndCrossPrincipalIsolation(t *testing.T) {
	dsn := os.Getenv("UNDERGRID_SANDBOX_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("CI requires separate sandbox database configuration")
		}
		t.Skip("requires isolated PostgreSQL")
	}
	ctx := context.Background()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	if err = Migrate(ctx, c); err != nil {
		t.Fatal(err)
	}
	runtimeDSN := os.Getenv("UNDERGRID_SANDBOX_TEST_RUNTIME_DATABASE_URL")
	if runtimeDSN == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("CI requires nonprivileged sandbox runtime database configuration")
		}
		runtimeDSN = dsn
	}
	s := New(runtimeDSN)
	owner := Owner{Issuer: "fixture", Subject: randomID()}
	if _, err = s.Enroll(ctx, owner); err != nil {
		t.Fatal(err)
	}
	policy := Policy{Asset: "USD", Cap: "1000", Products: []string{"test"}, Payees: []string{"account"}, ExpiresAt: time.Now().Add(time.Hour)}
	if err = s.SetPolicy(ctx, owner, 1, policy); err != nil {
		t.Fatal(err)
	}
	token, err := s.IssueCredential(ctx, owner, "agent", []string{"payments:quote", "payments:authorize", "payments:read"})
	if err != nil {
		t.Fatal(err)
	}
	q := Quote{Product: "test", Asset: "USD", Amount: "600", Fee: "0", Payee: "account", Rail: "stripe_test", TermsID: "test", TermsHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExpiresAt: time.Now().Add(time.Hour)}
	payments := make([]Payment, 2)
	for i := range payments {
		payments[i], err = s.CreateQuote(ctx, token, randomID(), q)
		if err != nil {
			t.Fatal(err)
		}
	}
	results := make(chan error, 2)
	for _, p := range payments {
		go func(p Payment) { _, e := s.Authorize(ctx, token, p.ID, p.Hash, randomID()); results <- e }(p)
	}
	success, limits := 0, 0
	for range payments {
		e := <-results
		if e == nil {
			success++
		} else if e == ErrLimit {
			limits++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || limits != 1 {
		t.Fatalf("serialized cap: success=%d limits=%d", success, limits)
	}
	other := Owner{Issuer: owner.Issuer, Subject: randomID()}
	if _, err = s.Enroll(ctx, other); err != nil {
		t.Fatal(err)
	}
	otherToken, err := s.IssueCredential(ctx, other, "other", []string{"payments:read"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetPayment(ctx, otherToken, payments[0].ID); err != ErrUnauthorized {
		t.Fatalf("cross-principal read: %v", err)
	}
	var active string
	if err = c.QueryRow(ctx, "SELECT id::text FROM undergrid_sandbox.sandbox_payments WHERE id=ANY($1::uuid[]) AND state='authorized'", []string{payments[0].ID, payments[1].ID}).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if err = s.RevokeCredentials(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if p, e := s.BeginDispatch(ctx, active); e != ErrUnauthorized || p.State != "canceled" {
		t.Fatalf("revocation dispatch: %+v %v", p, e)
	}
	if err = s.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
		var super, bypass bool
		if e := tx.QueryRow(ctx, "SELECT rolsuper,rolbypassrls FROM pg_roles WHERE rolname=current_user").Scan(&super, &bypass); e != nil {
			return e
		}
		if super || bypass {
			t.Fatal("runtime role bypasses authorization")
		}
		var n int
		if e := tx.QueryRow(ctx, "SELECT count(*) FROM sandbox_payments").Scan(&n); e != nil {
			return e
		}
		if n != 0 {
			t.Fatalf("unset actor RLS leaked %d payments", n)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSandboxRoleCatalogSafety(t *testing.T) {
	dsn := os.Getenv("UNDERGRID_SANDBOX_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("CI requires sandbox PostgreSQL")
		}
		t.Skip("requires isolated PostgreSQL")
	}
	ctx := context.Background()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	if err = Migrate(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err = ValidateGroupRoles(ctx, c); err != nil {
		t.Fatal(err)
	}
	// A real fixture login is deliberately unsuitable as a group role; no shared
	// role attributes are mutated to manufacture a security test.
	runtimeDSN := os.Getenv("UNDERGRID_SANDBOX_TEST_RUNTIME_DATABASE_URL")
	if runtimeDSN == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("CI requires restricted runtime fixture")
		}
		t.Skip("requires runtime login fixture")
	}
	runtime, err := pgx.Connect(ctx, runtimeDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	var privileged bool
	if err = runtime.QueryRow(ctx, `SELECT rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication OR EXISTS(SELECT FROM pg_tables WHERE schemaname='undergrid_sandbox' AND tableowner=current_user) FROM pg_roles WHERE rolname=current_user`).Scan(&privileged); err != nil || privileged {
		t.Fatalf("runtime login must be nonprivileged nonowner: %v %v", privileged, err)
	}
	var login string
	if err = runtime.QueryRow(ctx, "SELECT current_user").Scan(&login); err != nil {
		t.Fatal(err)
	}
	safe, err := groupRoleSafe(ctx, c, login)
	if err != nil || safe {
		t.Fatalf("login accepted as group role: %v %v", safe, err)
	}
	safe, err = groupRoleSafe(ctx, c, "undergrid_sandbox_nonexistent_fixture_role")
	if err == nil || safe {
		t.Fatalf("missing role accepted: %v %v", safe, err)
	}
}
