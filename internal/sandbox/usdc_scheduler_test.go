package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Historical settled rows are trusted fixture facts, not asserted live provider
// results. Their now-unavailable receipts must not starve a new valid payment.
func TestUSDCWorkerHistoricalFailuresDoNotStarveNewPayment(t *testing.T) {
	f := newUSDCChainFixture(t)
	ctx := context.Background()
	f.phase.Store(1)
	if err := f.service.VerifyConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	// Each row has a real independently signed quote/payer authorization and nonce
	// claim. Only its historical finalized observation is fixture-seeded.
	old := []string{}
	for n := 0; n < 101; n++ {
		quote := f.payment.Quote
		p, err := f.owner.store.CreateQuote(ctx, f.owner.token, fmt.Sprintf("old-quote-%d", n), quote)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.owner.store.Authorize(ctx, f.owner.token, p.ID, p.Hash, fmt.Sprintf("old-reserve-%d", n)); err != nil {
			t.Fatal(err)
		}
		// Existing wallet fixture uses the deterministic EOA at key7.
		private, _ := walletTestKey(t, 7)
		command := payerCommandFor(t, p, private)
		response := f.owner.call(t, "", http.MethodPost, "/v1/payments/"+p.ID+"/wallet-authorization", fmt.Sprintf("old-payer-%d", n), command, f.owner.token)
		if response.Code != 200 {
			t.Fatalf("historical payer %d %s", response.Code, response.Body.String())
		}
		if _, err = f.service.store.BeginDispatch(ctx, p.ID); err != nil {
			t.Fatal(err)
		}
		proof := usdcObservation{Transaction: "0x" + hash([]byte(p.ID)), BlockHash: "0x" + strings.Repeat("b", 64), Block: "100", Finality: "finalized", TransferIndex: "1", AuthorizationIndex: "0", Evidence: encode(map[string]any{"transaction_hash": "0x" + hash([]byte(p.ID)), "block_hash": "0x" + strings.Repeat("b", 64), "designation": "TEST HISTORICAL FIXTURE"})}
		if err = f.service.store.recordUSDC(ctx, p.ID, proof); err != nil {
			t.Fatal(err)
		}
		old = append(old, p.ID)
	}
	if _, err := f.owner.db.Exec(ctx, `UPDATE undergrid_sandbox.sandbox_payments SET updated_at=now()-interval '2 hours',usdc_next_check=now()-interval '1 hour' WHERE id=ANY($1::uuid[])`, old); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		if err := f.service.tickUSDC(ctx); err != nil {
			t.Fatal(err)
		}
		p, err := f.owner.store.GetPayment(ctx, f.owner.token, f.payment.ID)
		if err != nil {
			t.Fatal(err)
		}
		if p.State == "settled" {
			var failures int
			if err = f.owner.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_payments WHERE id=ANY($1::uuid[]) AND usdc_last_error='reconciliation_failed'", old).Scan(&failures); err != nil || failures == 0 {
				t.Fatalf("silent historical failures %d %v", failures, err)
			}
			if f.submits.Load() != 1 {
				t.Fatalf("scheduler resubmitted old money: %d", f.submits.Load())
			}
			return
		}
	}
	t.Fatal("more than100 settled rows delayed a new authorized payment beyond two bounded ticks")
}
func TestUSDCWorkerContinuesCanonicalRechecksWithoutRepeatingTransfer(t *testing.T) {
	f := newUSDCChainFixture(t)
	ctx := context.Background()
	f.phase.Store(1)
	if err := f.service.VerifyConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.service.DispatchPayment(ctx, f.payment.ID); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := f.owner.db.QueryRow(ctx, "SELECT jsonb_agg(to_jsonb(e))::text FROM undergrid_sandbox.sandbox_effects e WHERE payment_id=$1", f.payment.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []int64{1, 2} {
		if _, err := f.owner.db.Exec(ctx, "UPDATE undergrid_sandbox.sandbox_payments SET usdc_next_check=now()-interval '1 second' WHERE id=$1", f.payment.ID); err != nil {
			t.Fatal(err)
		}
		f.phase.Store(phase)
		if err := f.service.tickUSDC(ctx); err != nil {
			t.Fatal(err)
		}
	}
	p, err := f.owner.store.GetPayment(ctx, f.owner.token, f.payment.ID)
	if err != nil || p.State != "unknown" {
		t.Fatalf("settled payment stopped reorg monitoring: %s %v", p.State, err)
	}
	var charges int
	if err = f.owner.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1 AND kind IN('charge','fee')", p.ID).Scan(&charges); err != nil || charges != 2 {
		t.Fatalf("recheck duplicated transfer: %d %v", charges, err)
	}
	var snapshot []json.RawMessage
	if json.Unmarshal([]byte(before), &snapshot) != nil || len(snapshot) != 2 || f.submits.Load() != 1 {
		t.Fatal("invalid original transfer fixture")
	}
	var scheduled bool
	if err = f.owner.db.QueryRow(ctx, "SELECT usdc_next_check>now() FROM undergrid_sandbox.sandbox_payments WHERE id=$1", p.ID).Scan(&scheduled); err != nil || !scheduled {
		t.Fatalf("recheck not durably scheduled %v %v", scheduled, err)
	}
}
