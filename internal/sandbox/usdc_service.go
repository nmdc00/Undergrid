package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// This identifies explicit operator TEST configuration. It is not provider
// qualification, wallet custody, or permission to enable production payments.
type USDCConfig struct {
	RPCURL, FacilitatorURL, ApprovalID, Product, Payee, ResourceURL, TokenName, TokenVersion string
}
type USDCService struct {
	store    *Store
	config   USDCConfig
	client   *http.Client
	verified atomic.Bool
	rpcID    atomic.Int64
}

func NewUSDCService(store *Store, c USDCConfig) (*USDCService, error) {
	for _, raw := range []string{c.RPCURL, c.FacilitatorURL, c.ResourceURL} {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
			return nil, ErrInvalid
		}
	}
	if store == nil || strings.TrimSpace(c.ApprovalID) == "" || len(c.ApprovalID) > 500 || c.Product == "" || c.TokenName == "" || c.TokenVersion == "" {
		return nil, ErrInvalid
	}
	address, err := normalizeWalletAddress(c.Payee)
	if err != nil || address != c.Payee {
		return nil, ErrInvalid
	}
	if store.usdcTokenDomain == nil || store.usdcTokenDomain.Name != c.TokenName || store.usdcTokenDomain.Version != c.TokenVersion || len(store.usdcVerification) == 0 {
		return nil, ErrInvalid
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &USDCService{store: store, config: c, client: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// No network/provider error text escapes into public payment objects.
func (s *USDCService) request(ctx context.Context, method, endpoint string, payload, output any) error {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(encode(payload))
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return ErrInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(req)
	if err != nil {
		return errors.New("USDC provider unavailable")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
	if err != nil || len(raw) > 1024*1024 || response.StatusCode != 200 || !json.Valid(raw) {
		return errors.New("invalid USDC provider response")
	}
	if err = json.Unmarshal(raw, output); err != nil {
		return errors.New("invalid USDC provider response")
	}
	return nil
}
func (s *USDCService) rpc(ctx context.Context, method string, params any, output any) error {
	id := s.rpcID.Add(1)
	var response struct {
		Version string          `json:"jsonrpc"`
		ID      int64           `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	if err := s.request(ctx, "POST", s.config.RPCURL, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}, &response); err != nil {
		return err
	}
	if response.Version != "2.0" || response.ID != id || len(response.Result) == 0 || (len(response.Error) > 0 && string(response.Error) != "null") {
		return errors.New("invalid USDC RPC envelope")
	}
	if err := json.Unmarshal(response.Result, output); err != nil {
		return errors.New("invalid USDC RPC result")
	}
	return nil
}
func chainQuantity(raw string) (*big.Int, error) {
	if !strings.HasPrefix(raw, "0x") || len(raw) < 3 || len(raw) > 66 || !validLowerHex(raw[2:]) {
		return nil, ErrInvalid
	}
	if len(raw) > 3 && raw[2] == '0' {
		return nil, ErrInvalid
	}
	number, ok := new(big.Int).SetString(raw[2:], 16)
	if !ok || number.Sign() < 0 || number.BitLen() > 256 {
		return nil, ErrInvalid
	}
	return number, nil
}
func chainHash(raw string) bool {
	return len(raw) == 66 && strings.HasPrefix(raw, "0x") && validLowerHex(raw[2:])
}
func validLowerHex(raw string) bool {
	for _, c := range raw {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
