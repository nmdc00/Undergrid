package sandbox

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Controlled HTTPS/RPC data are fixture evidence, never actual provider proof.
// Calldata and events are independently encoded from the signed test command.
type usdcChainFixture struct {
	owner                 *walletHTTPFixture
	payment               Payment
	command               payerTestCommand
	service               *USDCService
	server                *httptest.Server
	phase                 atomic.Int64
	fault                 atomic.Value
	submits               atomic.Int64
	verifies              atomic.Int64
	providerInTransaction atomic.Bool
}

func usdcFixtureRoleDSN(t *testing.T, db *pgx.Conn, group string) string {
	t.Helper()
	role := "usdc_chain_" + strings.ReplaceAll(randomID(), "-", "")
	identifier := pgx.Identifier{role}.Sanitize()
	if _, err := db.Exec(context.Background(), "CREATE ROLE "+identifier+" LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION;GRANT "+pgx.Identifier{group}.Sanitize()+" TO "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(context.Background(), "DROP ROLE "+identifier) })
	config := db.Config()
	dsn := fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=disable", config.Host, config.Port, role, config.Database)
	actual, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	var user string
	var super, bypass, app, worker bool
	err = actual.QueryRow(context.Background(), `SELECT current_user,rolsuper,rolbypassrls,pg_has_role(current_user,'undergrid_sandbox_app','MEMBER'),pg_has_role(current_user,'undergrid_sandbox_worker','MEMBER') FROM pg_roles WHERE rolname=current_user`).Scan(&user, &super, &bypass, &app, &worker)
	actual.Close(context.Background())
	if err != nil || user != role || super || bypass || app != (group == "undergrid_sandbox_app") || worker != (group == "undergrid_sandbox_worker") {
		t.Fatalf("not isolated %s login %s super=%v bypass=%v app=%v worker=%v err=%v", group, user, super, bypass, app, worker, err)
	}
	return dsn
}

func newUSDCChainFixture(t *testing.T) *usdcChainFixture {
	t.Helper()
	f, key, original := newPayerFixture(t)
	api, err := New(usdcFixtureRoleDSN(t, f.db, "undergrid_sandbox_app")).WithUSDCSigning(*f.store.usdcSigning)
	if err != nil {
		t.Fatal(err)
	}
	api, err = api.WithUSDCTokenDomain(*f.store.usdcTokenDomain)
	if err != nil {
		t.Fatal(err)
	}
	f.store = api
	f.handler = mustWalletHandler(t, api, nil, map[string]Quote{original.Quote.Product: original.Quote})

	quote := original.Quote
	quote.Product = "usdc-chain-" + randomID()
	if err := f.store.SetPolicy(context.Background(), f.owner, 3, Policy{Asset: "USDC", Cap: "9999999999999999", Products: []string{quote.Product}, Payees: []string{quote.Payee}, ExpiresAt: time.Now().Add(time.Hour), AllowRefunds: true}); err != nil {
		t.Fatal(err)
	}
	p, err := f.store.CreateQuote(context.Background(), f.token, "chain-quote", quote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Authorize(context.Background(), f.token, p.ID, p.Hash, "chain-reserve"); err != nil {
		t.Fatal(err)
	}
	command := payerCommandFor(t, p, key)
	response := f.call(t, "", http.MethodPost, "/v1/payments/"+p.ID+"/wallet-authorization", "chain-payer-proof", command, f.token)
	if response.Code != 200 {
		t.Fatalf("payer proof %d %s", response.Code, response.Body.String())
	}
	fixture := &usdcChainFixture{owner: f, payment: p, command: command}
	fixture.fault.Store("")
	fixture.server = httptest.NewTLSServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(fixture.server.Close)
	workerDSN := usdcFixtureRoleDSN(t, f.db, "undergrid_sandbox_worker")
	worker, err := New(workerDSN).WithUSDCVerification(f.store.usdcVerification)
	if err != nil {
		t.Fatal(err)
	}
	worker, err = worker.WithUSDCTokenDomain(*f.store.usdcTokenDomain)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewUSDCService(worker, USDCConfig{RPCURL: fixture.server.URL + "/rpc", FacilitatorURL: fixture.server.URL, ApprovalID: "CONTROLLED-FIXTURE-NOT-PROVIDER-APPROVAL", Product: p.Quote.Product, Payee: p.Quote.Payee, ResourceURL: "https://sandbox-fixture.invalid/usdc", TokenName: "USDC", TokenVersion: "2"})
	if err != nil {
		t.Fatal(err)
	}
	service.client = fixture.server.Client()
	fixture.service = service
	return fixture
}
func chainWord(value string) string {
	n, ok := new(big.Int).SetString(value, 10)
	if !ok {
		panic("invalid fixture integer")
	}
	return fmt.Sprintf("%064x", n)
}
func chainAddress(value string) string {
	return strings.Repeat("0", 24) + strings.TrimPrefix(value, "0x")
}
func (f *usdcChainFixture) calldata() string {
	c := f.command
	signature, _ := hex.DecodeString(strings.TrimPrefix(c.Signature, "0x"))
	selector := hex.EncodeToString(keccak256([]byte("transferWithAuthorization(address,address,uint256,uint256,uint256,bytes32,uint8,bytes32,bytes32)"))[:4])
	return "0x" + selector + chainAddress(c.Authorization.From) + chainAddress(c.Authorization.To) + chainWord(c.Authorization.Value) + chainWord(c.Authorization.ValidAfter) + chainWord(c.Authorization.ValidBefore) + strings.TrimPrefix(c.Authorization.Nonce, "0x") + chainWord(fmt.Sprint(signature[64])) + hex.EncodeToString(signature[:64])
}
func (f *usdcChainFixture) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var transactions int
	err := f.owner.db.QueryRow(r.Context(), `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename LIKE 'usdc_chain_%' AND state='idle in transaction'`).Scan(&transactions)
	if err == nil && transactions > 0 {
		f.providerInTransaction.Store(true)
	}
	if r.URL.Path == "/supported" {
		io.WriteString(w, `{"kinds":[{"x402Version":2,"scheme":"exact","network":"eip155:84532"}],"extensions":[],"signers":{}}`)
		return
	}
	if r.URL.Path == "/verify" || r.URL.Path == "/settle" {
		var payload map[string]any
		if json.NewDecoder(r.Body).Decode(&payload) != nil {
			w.WriteHeader(400)
			return
		}
		requirements, _ := payload["paymentRequirements"].(map[string]any)
		if requirements["amount"] != f.command.Authorization.Value || requirements["payTo"] != f.command.Authorization.To || requirements["network"] != "eip155:84532" {
			w.WriteHeader(400)
			return
		}
		if r.URL.Path == "/verify" {
			f.verifies.Add(1)
			json.NewEncoder(w).Encode(map[string]any{"isValid": true, "payer": f.command.Authorization.From})
			return
		}
		f.submits.Add(1)
		if f.fault.Load().(string) == "settle-response-lost" {
			w.WriteHeader(504)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "payer": f.command.Authorization.From, "network": "eip155:84532", "transaction": "0x" + strings.Repeat("a", 64)})
		return
	}
	var request struct {
		ID     any               `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil {
		w.WriteHeader(400)
		return
	}
	phase := f.phase.Load()
	hash := "0x" + strings.Repeat("b", 64)
	if phase == 2 {
		hash = "0x" + strings.Repeat("c", 64)
	}
	txHash := "0x" + strings.Repeat("a", 64)
	fault := f.fault.Load().(string)
	result := any(nil)
	switch request.Method {
	case "eth_chainId":
		result = "0x14a34"
		if fault == "wrong-chain" {
			result = "0x2105"
		}
	case "eth_getCode":
		result = "0x60006000"
	case "eth_blockNumber":
		result = "0x63"
		if f.submits.Load() > 0 {
			result = "0x64"
		}
	case "eth_call":
		var call map[string]any
		json.Unmarshal(request.Params[0], &call)
		data, _ := call["data"].(string)
		switch data {
		case "0x06fdde03":
			result = "0x" + chainWord("32") + chainWord("4") + hex.EncodeToString([]byte("USDC")) + strings.Repeat("0", 56)
		case "0x54fd4d50":
			result = "0x" + chainWord("32") + chainWord("1") + "32" + strings.Repeat("0", 62)
		case "0x313ce567":
			result = "0x" + chainWord("6")
		default:
			if strings.HasPrefix(data, "0x"+hex.EncodeToString(keccak256([]byte("authorizationState(address,bytes32)"))[:4])) {
				used := "0"
				if f.submits.Load() > 0 && phase != 2 {
					used = "1"
				}
				result = "0x" + chainWord(used)
			} else {
				result = "0x" + chainWord("999999999999")
			}

		}
	case "eth_getBlockByNumber":
		number := "0x64"
		var tag string
		json.Unmarshal(request.Params[0], &tag)
		if (phase == 0 && (tag == "safe" || tag == "finalized")) || (phase == 3 && tag == "finalized") {
			number = "0x63"
		}
		result = map[string]any{"number": number, "hash": hash, "timestamp": fmt.Sprintf("0x%x", time.Now().Unix())}
	case "eth_getTransactionByHash":
		result = map[string]any{"hash": txHash, "to": USDCTestToken, "input": f.calldata(), "blockHash": hash, "blockNumber": "0x64", "chainId": "0x14a34"}
	case "eth_getTransactionReceipt":
		if phase == 2 {
			break
		}
		token := USDCTestToken
		from := f.command.Authorization.From
		to := f.command.Authorization.To
		value := f.command.Authorization.Value
		nonce := f.command.Authorization.Nonce
		if fault == "wrong-token" {
			token = "0x" + strings.Repeat("1", 40)
		}
		if fault == "wrong-payer" {
			from = "0x" + strings.Repeat("2", 40)
		}
		if fault == "wrong-payee" {
			to = "0x" + strings.Repeat("3", 40)
		}
		if fault == "wrong-value" {
			value = "1"
		}
		if fault == "wrong-nonce" {
			nonce = "0x" + strings.Repeat("4", 64)
		}
		logs := []any{
			map[string]any{"address": token, "topics": []string{"0x" + hex.EncodeToString(keccak256([]byte("AuthorizationUsed(address,bytes32)"))), "0x" + chainAddress(from), nonce}, "data": "0x", "logIndex": "0x0", "transactionHash": txHash, "blockHash": hash, "blockNumber": "0x64", "removed": false},
			map[string]any{"address": token, "topics": []string{"0x" + hex.EncodeToString(keccak256([]byte("Transfer(address,address,uint256)"))), "0x" + chainAddress(from), "0x" + chainAddress(to)}, "data": "0x" + chainWord(value), "logIndex": "0x1", "transactionHash": txHash, "blockHash": hash, "blockNumber": "0x64", "removed": false},
		}
		status := "0x1"
		if fault == "reverted" {
			status = "0x0"
		}
		result = map[string]any{"transactionHash": txHash, "blockHash": hash, "blockNumber": "0x64", "status": status, "logs": logs, "to": token}
	case "eth_getLogs":
		if phase == 2 {
			result = []any{}
		} else {
			result = []any{map[string]any{"address": USDCTestToken, "topics": []string{"0x" + hex.EncodeToString(keccak256([]byte("AuthorizationUsed(address,bytes32)"))), "0x" + chainAddress(f.command.Authorization.From), f.command.Authorization.Nonce}, "data": "0x", "transactionHash": txHash, "blockHash": hash, "blockNumber": "0x64", "logIndex": "0x0", "removed": false}}
		}
	default:
		w.WriteHeader(400)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
}
func TestUSDCCanonicalSettlementRequiresFinalityAndReorgCorrectsFacts(t *testing.T) {
	f := newUSDCChainFixture(t)
	ctx := context.Background()
	if err := f.service.VerifyConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.service.DispatchPayment(ctx, f.payment.ID); err != nil {
		t.Fatal(err)
	}
	p, err := f.owner.store.GetPayment(ctx, f.owner.token, f.payment.ID)
	if err != nil || p.State != "pending" {
		t.Fatalf("provisional transfer mislabeled %s %v", p.State, err)
	}
	var effects int
	if err = f.owner.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1", p.ID).Scan(&effects); err != nil || effects != 0 {
		t.Fatalf("unfinalized money %d %v", effects, err)
	}
	f.phase.Store(3)
	if err = f.service.ReconcilePayment(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	var safe int
	if err = f.owner.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_usdc_observations WHERE payment_id=$1 AND finality='safe'", p.ID).Scan(&safe); err != nil || safe != 1 {
		t.Fatalf("safe evidence missing %d %v", safe, err)
	}
	f.phase.Store(1)
	if err = f.service.ReconcilePayment(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	p, err = f.owner.store.GetPayment(ctx, f.owner.token, p.ID)
	if err != nil || p.State != "settled" {
		t.Fatalf("canonical finalized transfer not settled %s %v", p.State, err)
	}
	var net, fee string
	if err = f.owner.db.QueryRow(ctx, `SELECT COALESCE(sum(amount) FILTER(WHERE kind='charge'),0)::text,COALESCE(sum(amount) FILTER(WHERE kind='fee'),0)::text FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1`, p.ID).Scan(&net, &fee); err != nil || net != p.Quote.Amount || fee != p.Quote.Fee {
		t.Fatalf("gross breakdown %s %s %v", net, fee, err)
	}
	if err = f.service.ReconcilePayment(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.owner.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1", p.ID).Scan(&effects); err != nil || effects != 2 {
		t.Fatalf("replay repeated economic effects %d %v", effects, err)
	}
	f.phase.Store(0)
	if err = f.service.ReconcilePayment(ctx, p.ID); err == nil {
		t.Fatal("stale RPC finality regression accepted")
	}
	still, err := f.owner.store.GetPayment(ctx, f.owner.token, p.ID)
	if err != nil || still.State != "settled" {
		t.Fatalf("stale RPC erased finalized truth %s %v", still.State, err)
	}
	f.phase.Store(2)
	if err = f.service.ReconcilePayment(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	p, err = f.owner.store.GetPayment(ctx, f.owner.token, p.ID)
	if err != nil || p.State != "unknown" {
		t.Fatalf("reorg failed to reopen reconciliation %s %v", p.State, err)
	}
	var correction int
	if err = f.owner.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1 AND kind='reorg'", p.ID).Scan(&correction); err != nil || correction != 2 {
		t.Fatalf("append-only gross correction %d %v", correction, err)
	}
	var refundable bool
	capacity := func() bool {
		t.Helper()
		err := f.owner.store.transaction(ctx, "undergrid_sandbox_app", func(tx pgx.Tx) error {
			if _, err := agentContext(ctx, tx, f.owner.token, "payments:read"); err != nil {
				return err
			}
			var err error
			refundable, err = refundCapacity(ctx, tx, p.ID, p.Quote.Amount, "")
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return refundable
	}
	if capacity() {
		t.Fatal("orphaned transfer remained refundable")
	}
	f.phase.Store(1)
	if err = f.service.ReconcilePayment(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	p, err = f.owner.store.GetPayment(ctx, f.owner.token, p.ID)
	if err != nil || p.State != "settled" {
		t.Fatalf("same transaction re-finalization failed %s %v", p.State, err)
	}
	if err = f.owner.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1 AND kind IN('charge','fee')", p.ID).Scan(&effects); err != nil || effects != 2 {
		t.Fatalf("re-finalization invented a transfer %d %v", effects, err)
	}
	if !capacity() {
		t.Fatal("re-finalized original transfer lost refundable capacity")
	}
	if f.submits.Load() != 1 || f.providerInTransaction.Load() {
		t.Fatalf("provider effects violated submits=%d open_transaction=%v", f.submits.Load(), f.providerInTransaction.Load())
	}
}
func TestUSDCUnknownReconcilesHistoricalProofWithoutResubmission(t *testing.T) {
	f := newUSDCChainFixture(t)
	ctx := context.Background()
	if err := f.service.VerifyConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	f.fault.Store("settle-response-lost")
	if err := f.service.DispatchPayment(ctx, f.payment.ID); err == nil {
		t.Fatal("lost provider response not reported")
	}
	p, err := f.owner.store.GetPayment(ctx, f.owner.token, f.payment.ID)
	if err != nil || p.State != "unknown" {
		t.Fatalf("uncertain submission %s %v", p.State, err)
	}
	var originalKey, period string
	if err = f.owner.db.QueryRow(ctx, "SELECT provider_key,reservation_period::text FROM undergrid_sandbox.sandbox_payments WHERE id=$1", p.ID).Scan(&originalKey, &period); err != nil {
		t.Fatal(err)
	}
	if _, err = f.owner.db.Exec(ctx, "UPDATE undergrid_sandbox.sandbox_credentials SET revoked_at=now() WHERE actor_id=$1", f.owner.actorID); err != nil {
		t.Fatal(err)
	}
	f.fault.Store("")
	f.phase.Store(1)
	if err = f.service.ReconcilePayment(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	var state, key, retainedPeriod string
	if err = f.owner.db.QueryRow(ctx, "SELECT state,provider_key,reservation_period::text FROM undergrid_sandbox.sandbox_payments WHERE id=$1", p.ID).Scan(&state, &key, &retainedPeriod); err != nil || state != "settled" || key != originalKey || period != retainedPeriod {
		t.Fatalf("late financial truth lost %s %v", state, err)
	}
	if f.submits.Load() != 1 {
		t.Fatalf("unknown resubmitted %d times", f.submits.Load())
	}
}
func TestUSDCRejectsMismatchedCanonicalEvidence(t *testing.T) {
	for _, fault := range []string{"wrong-chain", "wrong-token", "wrong-payer", "wrong-payee", "wrong-value", "wrong-nonce", "reverted"} {
		t.Run(fault, func(t *testing.T) {
			f := newUSDCChainFixture(t)
			ctx := context.Background()
			if err := f.service.VerifyConfiguration(ctx); err != nil {
				t.Fatal(err)
			}
			f.phase.Store(1)
			f.fault.Store(fault)
			if err := f.service.DispatchPayment(ctx, f.payment.ID); err == nil {
				t.Fatal("mismatched evidence admitted")
			}
			var effects int
			if err := f.owner.db.QueryRow(ctx, "SELECT count(*) FROM undergrid_sandbox.sandbox_effects WHERE payment_id=$1", f.payment.ID).Scan(&effects); err != nil || effects != 0 {
				t.Fatalf("mismatched evidence created money %d %v", effects, err)
			}
		})
	}
}

func TestUSDCRuntimeWorkerSettlesAndStopsWithIsolatedIdentity(t *testing.T) {
	f := newUSDCChainFixture(t)
	f.phase.Store(1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.service.RunWorker(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("USDC worker failed bounded shutdown")
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		p, err := f.owner.store.GetPayment(context.Background(), f.owner.token, f.payment.ID)
		if err != nil {
			t.Fatal(err)
		}
		if p.State == "settled" {
			if f.submits.Load() != 1 {
				t.Fatal("runtime duplicate submission", f.submits.Load())
			}
			return
		}
		select {
		case err := <-done:
			t.Fatal("runtime stopped before settlement", err)
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("real USDC worker never settled its configured test product")
}
