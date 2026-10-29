package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"undergrid/internal/sandbox"
)

func runUSDC(ctx context.Context, dsn string) error {
	invalid := errors.New("invalid isolated USDC TEST worker configuration")
	var encoded map[string]string
	if json.Unmarshal([]byte(os.Getenv("UNDERGRID_SANDBOX_USDC_SIGNING_PUBLIC_KEYS")), &encoded) != nil {
		return invalid
	}
	keys := map[string]ed25519.PublicKey{}
	for id, value := range encoded {
		key, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return invalid
		}
		keys[id] = key
	}
	store, err := sandbox.New(dsn).WithUSDCVerification(keys)
	if err != nil {
		return invalid
	}
	domain := sandbox.USDCTokenDomain{Name: os.Getenv("UNDERGRID_SANDBOX_USDC_TOKEN_NAME"), Version: os.Getenv("UNDERGRID_SANDBOX_USDC_TOKEN_VERSION")}
	store, err = store.WithUSDCTokenDomain(domain)
	if err != nil {
		return invalid
	}
	config := sandbox.USDCConfig{RPCURL: os.Getenv("UNDERGRID_SANDBOX_USDC_RPC_URL"), FacilitatorURL: os.Getenv("UNDERGRID_SANDBOX_USDC_FACILITATOR_URL"), ApprovalID: os.Getenv("UNDERGRID_SANDBOX_USDC_PROVIDER_APPROVAL"), Product: os.Getenv("UNDERGRID_SANDBOX_USDC_PRODUCT"), Payee: os.Getenv("UNDERGRID_SANDBOX_USDC_PAYEE"), ResourceURL: os.Getenv("UNDERGRID_SANDBOX_USDC_RESOURCE_URL"), TokenName: domain.Name, TokenVersion: domain.Version}
	service, err := sandbox.NewUSDCService(store, config)
	if err != nil {
		return invalid
	}
	return service.RunWorker(ctx)
}
