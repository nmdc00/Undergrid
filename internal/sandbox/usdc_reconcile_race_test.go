package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type usdcDelayedHeader struct {
	base    http.RoundTripper
	block   string
	calls   atomic.Int64
	ready   chan struct{}
	release chan struct{}
}

func (d *usdcDelayedHeader) RoundTrip(request *http.Request) (*http.Response, error) {
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	request.Body = io.NopCloser(bytes.NewReader(raw))
	var command struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	json.Unmarshal(raw, &command)
	response, err := d.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if command.Method != "eth_getBlockByNumber" || len(command.Params) == 0 {
		return response, nil
	}
	var block string
	json.Unmarshal(command.Params[0], &block)
	if block != d.block || d.calls.Add(1) != 3 {
		return response, nil
	}
	// Freeze the OLD final inclusion-header response after provider I/O and
	// before the service can commit it. A second worker can now commit a reorg.
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		return nil, err
	}
	close(d.ready)
	select {
	case <-d.release:
	case <-request.Context().Done():
		return nil, request.Context().Err()
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return response, nil
}
func TestUSDCStaleFinalizedProofCannotOverwriteNewerReorg(t *testing.T) {
	f := newUSDCChainFixture(t)
	f.phase.Store(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := f.service.VerifyConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.service.DispatchPayment(ctx, f.payment.ID); err != nil {
		t.Fatal(err)
	}
	p, err := f.owner.store.GetPayment(ctx, f.owner.token, f.payment.ID)
	if err != nil || p.State != "settled" || p.Settlement == nil {
		t.Fatalf("finalized setup %s %v", p.State, err)
	}
	block, _ := new(big.Int).SetString(p.Settlement.Block, 10)
	slow, err := NewUSDCService(f.service.store, f.service.config)
	if err != nil {
		t.Fatal(err)
	}
	slow.client = f.server.Client()
	if err = slow.VerifyConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	transport := &usdcDelayedHeader{base: f.server.Client().Transport, block: "0x" + block.Text(16), ready: make(chan struct{}), release: make(chan struct{})}
	client := *f.server.Client()
	client.Transport = transport
	slow.client = &client
	var once sync.Once
	release := func() { once.Do(func() { close(transport.release) }) }
	defer release()
	done := make(chan error, 1)
	go func() { done <- slow.ReconcilePayment(ctx, p.ID) }()
	select {
	case <-transport.ready:
	case err := <-done:
		t.Fatalf("old proof did not reach post-I/O seam: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("old proof did not reach bounded post-I/O seam")
	}
	f.phase.Store(2)
	if err = f.service.ReconcilePayment(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	current, err := f.owner.store.GetPayment(ctx, f.owner.token, p.ID)
	if err != nil || current.State != "unknown" {
		t.Fatalf("newer reorg did not commit %s %v", current.State, err)
	}
	release()
	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("old proof did not finish")
	}
	current, readErr := f.owner.store.GetPayment(ctx, f.owner.token, p.ID)
	if readErr != nil || current.State != "unknown" {
		t.Fatalf("stale finalized proof overwrote newer reorg: state=%s old_result=%v read=%v", current.State, err, readErr)
	}
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale proof not rejected against durable observation: %v", err)
	}
	if f.submits.Load() != 1 {
		t.Fatalf("reconciliation resubmitted payment %d times", f.submits.Load())
	}
}
