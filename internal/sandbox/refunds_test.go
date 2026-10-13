package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

func refundFixture(t *testing.T) (*Store, *pgx.Conn, Owner, string, Payment, *Handler) {
	t.Helper()
	ctx := context.Background()
	dsn := os.Getenv("UNDERGRID_SANDBOX_TEST_DATABASE_URL")
	runtime := os.Getenv("UNDERGRID_SANDBOX_TEST_RUNTIME_DATABASE_URL")
	if dsn == "" || runtime == "" {
		t.Fatal("real separate sandbox admin/runtime fixtures required")
	}
	db, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(context.Background()) })
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	s := New(runtime)
	owner := Owner{Issuer: "https://refund-fixture.invalid", Subject: randomID()}
	if _, err = s.Enroll(ctx, owner); err != nil {
		t.Fatal(err)
	}
	policy := Policy{Asset: "USD", Cap: "1000", Products: []string{"platform-test"}, Payees: []string{"stripe-test-account"}, ExpiresAt: time.Now().Add(time.Hour)}
	if err = s.SetPolicy(ctx, owner, 1, policy); err != nil {
		t.Fatal(err)
	}
	token, err := s.IssueCredential(ctx, owner, "payment-agent", []string{"payments:quote", "payments:authorize", "payments:read"})
	if err != nil {
		t.Fatal(err)
	}
	q := Quote{Product: "platform-test", Asset: "USD", Amount: "600", Fee: "0", Payee: "stripe-test-account", Rail: "stripe_test", TermsID: "fixture-terms", TermsHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExpiresAt: time.Now().Add(time.Hour)}
	p, err := s.CreateQuote(ctx, token, "refund-payment", q)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorize(ctx, token, p.ID, p.Hash, "pay"); err != nil {
		t.Fatal(err)
	}
	effect := VerifiedEffect{Provider: "stripe_test", Account: q.Payee, EventID: "evt_" + randomID(), FactID: "charge_" + randomID(), PaymentID: p.ID, Kind: "charge", Asset: "USD", Amount: "600", Evidence: json.RawMessage(`{"verified_by":"fixture","livemode":false}`)}
	if err = s.RecordEffect(ctx, effect); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(s, func(r *http.Request) (Owner, error) {
		if r.Header.Get("X-Fixture-Owner") != owner.Subject {
			return Owner{}, ErrUnauthorized
		}
		return owner, nil
	}, nil)
	return s, db, owner, token, p, h
}
func refundCall(t *testing.T, h *Handler, token, owner, method, path, key string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("X-Fixture-Owner", owner)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return w.Code, out
}
func TestRefundHTTPRequiresSeparateOwnerAuthorityAndExactRetry(t *testing.T) {
	s, _, owner, paymentToken, p, h := refundFixture(t)
	body := map[string]any{"payment_id": p.ID, "amount_atomic": "300"}
	code, out := refundCall(t, h, paymentToken, "", "POST", "/v1/refunds", "scope-denied", body)
	if code != 401 {
		t.Fatalf("payment scope granted refund: %d %v", code, out)
	}
	if _, err := s.IssueCredential(context.Background(), owner, "refund-agent", []string{"refunds:request", "refunds:authorize", "refunds:read"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("refund scope issued without owner grant: %v", err)
	}
	grant := map[string]any{"version": 2, "policy": map[string]any{"asset": "USD", "cap_atomic": "1000", "products": []string{"platform-test"}, "payees": []string{"stripe-test-account"}, "expires_at": time.Now().Add(time.Hour), "allow_refunds": true}}
	code, out = refundCall(t, h, "", owner.Subject, "PUT", "/v1/owner/policy", "", grant)
	if code != 200 {
		t.Fatalf("owner refund grant: %d %v", code, out)
	}
	token, err := s.IssueCredential(context.Background(), owner, "refund-agent", []string{"refunds:request", "refunds:authorize", "refunds:read"})
	if err != nil {
		t.Fatal(err)
	}
	code, first := refundCall(t, h, token, "", "POST", "/v1/refunds", "refund-one", body)
	if code != 201 || first["state"] != "requested" {
		t.Fatalf("refund request: %d %v", code, first)
	}
	otherOwner := Owner{Issuer: "https://refund-fixture.invalid", Subject: randomID()}
	if _, err = s.Enroll(context.Background(), otherOwner); err != nil {
		t.Fatal(err)
	}
	otherToken, err := s.IssueCredential(context.Background(), otherOwner, "read-only", []string{"refunds:read"})
	if err != nil {
		t.Fatalf("read scope unexpectedly required spending/refund grant: %v", err)
	}
	if _, err = s.GetRefund(context.Background(), otherToken, first["id"].(string)); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("cross-actor refund read: %v", err)
	}
	code, again := refundCall(t, h, token, "", "POST", "/v1/refunds", "refund-one", body)
	if code != 201 || again["id"] != first["id"] {
		t.Fatalf("refund replay: %d %v", code, again)
	}
	changed := map[string]any{"payment_id": p.ID, "amount_atomic": "301"}
	code, out = refundCall(t, h, token, "", "POST", "/v1/refunds", "refund-one", changed)
	if code != 409 {
		t.Fatalf("changed refund retry: %d %v", code, out)
	}
	path := "/v1/refunds/" + first["id"].(string) + "/authorize"
	code, out = refundCall(t, h, token, "", "POST", path, "wrong-hash", map[string]any{"refund_hash": "0000000000000000000000000000000000000000000000000000000000000000"})
	if code != 409 {
		t.Fatalf("wrong refund hash accepted: %d %v", code, out)
	}
	code, out = refundCall(t, h, token, "", "POST", path, "approve", map[string]any{"refund_hash": first["refund_hash"]})
	if code != 200 || out["state"] != "authorized" {
		t.Fatalf("refund approval: %d %v", code, out)
	}
	if err = s.RevokeCredentials(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	code, out = refundCall(t, h, token, "", "POST", path, "approve", map[string]any{"refund_hash": first["refund_hash"]})
	if code != 401 {
		t.Fatalf("revoked refund replay: %d %v", code, out)
	}
}

func TestRefundReservationsUnknownReconciliationAndNoGrossCredit(t *testing.T) {
	s, db, owner, paymentToken, p, h := refundFixture(t)
	ctx := context.Background()
	policy := map[string]any{"version": 2, "policy": map[string]any{"asset": "USD", "cap_atomic": "1000", "products": []string{"platform-test"}, "payees": []string{"stripe-test-account"}, "expires_at": time.Now().Add(time.Hour), "allow_refunds": true}}
	if code, out := refundCall(t, h, "", owner.Subject, "PUT", "/v1/owner/policy", "", policy); code != 200 {
		t.Fatal(code, out)
	}
	token, err := s.IssueCredential(ctx, owner, "refunds", []string{"refunds:request", "refunds:authorize", "refunds:read"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.RequestRefund(ctx, token, "one", p.ID, "400")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.RequestRefund(ctx, token, "two", p.ID, "400")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, r := range []Refund{a, b} {
		wg.Add(1)
		go func(r Refund) {
			defer wg.Done()
			_, err := s.AuthorizeRefund(ctx, token, r.ID, r.Hash, "authorize-"+r.ID)
			results <- err
		}(r)
	}
	wg.Wait()
	close(results)
	passed, rejected := 0, 0
	for err := range results {
		if err == nil {
			passed++
		} else if errors.Is(err, ErrLimit) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if passed != 1 || rejected != 1 {
		t.Fatalf("concurrent over-refund admitted: %d/%d", passed, rejected)
	}
	var authorized string
	if err = db.QueryRow(ctx, "SELECT id::text FROM undergrid_sandbox.sandbox_refunds WHERE payment_id=$1 AND state='authorized'", p.ID).Scan(&authorized); err != nil {
		t.Fatal(err)
	}
	dispatched, err := s.BeginRefundDispatch(ctx, authorized)
	if err != nil || dispatched.State != "pending" {
		t.Fatalf("refund dispatch: %+v %v", dispatched, err)
	}
	if err = s.MarkRefundUnknown(ctx, authorized); err != nil {
		t.Fatal(err)
	}
	again, err := s.GetRefund(ctx, token, authorized)
	if err != nil || again.State != "unknown" || again.ProviderKey != dispatched.ProviderKey {
		t.Fatalf("unknown identity: %+v %v", again, err)
	}
	if _, err = s.BeginRefundDispatch(ctx, authorized); !errors.Is(err, ErrConflict) {
		t.Fatalf("unknown redispatched: %v", err)
	}
	other := a
	if authorized == a.ID {
		other = b
	}
	if _, err = s.AuthorizeRefund(ctx, token, other.ID, other.Hash, "retry-other"); !errors.Is(err, ErrLimit) {
		t.Fatalf("unknown released refund reservation: %v", err)
	}
	if err = s.RevokeCredentials(ctx, owner); err != nil {
		t.Fatal(err)
	}
	e := VerifiedEffect{Provider: "stripe_test", Account: p.Quote.Payee, EventID: "evt_" + randomID(), FactID: "refund_" + randomID(), PaymentID: p.ID, RefundID: authorized, Kind: "refund", Asset: "USD", Amount: "400", Evidence: json.RawMessage(`{"verified_by":"fixture","livemode":false}`)}
	if err = s.RecordEffect(ctx, e); err != nil {
		t.Fatalf("late valid refund lost on revoke: %v", err)
	}
	if err = s.RecordEffect(ctx, e); err != nil {
		t.Fatal(err)
	}
	var state string
	var count int
	if err = db.QueryRow(ctx, "SELECT state FROM undergrid_sandbox.sandbox_refunds WHERE id=$1", authorized).Scan(&state); err != nil || state != "succeeded" {
		t.Fatalf("refund result not persisted: %s %v", state, err)
	}
	if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE refund_id=$1", authorized).Scan(&count); err != nil || count != 1 {
		t.Fatalf("refund effect duplicated: %d %v", count, err)
	}
	if err = s.MarkRefundUnknown(ctx, authorized); !errors.Is(err, ErrConflict) {
		t.Fatalf("late unknown overwrote success: %v", err)
	}
	fresh, err := s.IssueCredential(ctx, owner, "new-payment", []string{"payments:quote", "payments:authorize", "payments:read"})
	if err != nil {
		t.Fatal(err)
	}
	q := p.Quote
	q.ExpiresAt = time.Now().Add(time.Hour)
	next, err := s.CreateQuote(ctx, fresh, "next", q)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorize(ctx, fresh, next.ID, next.Hash, "next"); !errors.Is(err, ErrLimit) {
		t.Fatalf("refund replenished gross spending cap: %v", err)
	}
	if _, err = s.GetPayment(ctx, paymentToken, p.ID); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked payment credential remained usable")
	}
}

func TestRefundRetryMetadataHasActorRLS(t *testing.T) {
	_, db, _, _, p, _ := refundFixture(t)
	ctx := context.Background()
	var persisted int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_idempotency WHERE payment_id=$1", p.ID).Scan(&persisted); err != nil || persisted != 2 {
		t.Fatalf("missing admin-visible retry fixture: %d %v", persisted, err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET LOCAL ROLE undergrid_sandbox_app; SET LOCAL search_path=undergrid_sandbox,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	var visible int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM sandbox_idempotency WHERE payment_id=$1", p.ID).Scan(&visible); err != nil {
		t.Fatal(err)
	}
	if visible != 0 {
		t.Fatalf("financial retry metadata visible with no actor: %d", visible)
	}
}

func TestRefundDispatchRechecksRevocationBeforeProviderClaim(t *testing.T) {
	s, db, owner, _, p, _ := refundFixture(t)
	ctx := context.Background()
	policy := Policy{Asset: "USD", Cap: "1000", Products: []string{"platform-test"}, Payees: []string{"stripe-test-account"}, ExpiresAt: time.Now().Add(time.Hour), AllowRefunds: true}
	if err := s.SetPolicy(ctx, owner, 2, policy); err != nil {
		t.Fatal(err)
	}
	token, err := s.IssueCredential(ctx, owner, "refund", []string{"refunds:request", "refunds:authorize", "refunds:read"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.RequestRefund(ctx, token, "request", p.ID, "100")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthorizeRefund(ctx, token, r.ID, r.Hash, "approve"); err != nil {
		t.Fatal(err)
	}
	if err = s.RevokeCredentials(ctx, owner); err != nil {
		t.Fatal(err)
	}
	canceled, err := s.BeginRefundDispatch(ctx, r.ID)
	if !errors.Is(err, ErrUnauthorized) || canceled.State != "failed" {
		t.Fatalf("revoked refund dispatched: %+v %v", canceled, err)
	}
	var state string
	if err = db.QueryRow(ctx, "SELECT state FROM undergrid_sandbox.sandbox_refunds WHERE id=$1", r.ID).Scan(&state); err != nil || state != "failed" {
		t.Fatalf("failed pre-dispatch state not committed: %s %v", state, err)
	}
	var facts int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE refund_id=$1", r.ID).Scan(&facts); err != nil || facts != 0 {
		t.Fatalf("pre-dispatch cancellation created financial fact: %d %v", facts, err)
	}
}
