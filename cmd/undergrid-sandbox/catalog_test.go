package main

import (
	"testing"
	"undergrid/internal/sandbox"
)

func TestFinancialCatalogBoundsOneResourcePerRail(t *testing.T) {
	stripe := sandbox.Quote{Product: "stripe", Asset: "USD", Rail: "stripe_test"}
	usdc := sandbox.Quote{Product: "usdc", Asset: "USDC", Rail: "x402_test"}
	for _, catalog := range []map[string]sandbox.Quote{{"stripe": stripe}, {"usdc": usdc}, {"stripe": stripe, "usdc": usdc}} {
		if _, err := validateSandboxCatalog(catalog); err != nil {
			t.Fatal(err)
		}
	}
	duplicateStripe := stripe
	duplicateStripe.Product = "second"
	duplicateUSDC := usdc
	duplicateUSDC.Product = "second"
	for _, catalog := range []map[string]sandbox.Quote{{}, {"stripe": stripe, "second": duplicateStripe}, {"usdc": usdc, "second": duplicateUSDC}, {"stripe": stripe, "usdc": usdc, "second": duplicateUSDC}} {
		if _, err := validateSandboxCatalog(catalog); err == nil {
			t.Fatal("more than one product per rail accepted")
		}
	}
}
