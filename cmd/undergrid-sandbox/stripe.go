package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"undergrid/internal/sandbox"
)

func runStripe(ctx context.Context, dsn string) error {
	config, err := stripeConfigFromEnvironment()
	if err != nil {
		return err
	}
	service, err := sandbox.NewStripeService(sandbox.New(dsn), config)
	if err != nil {
		return errors.New("invalid isolated Stripe TEST worker configuration")
	}
	startup, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err = service.VerifyAccount(startup); err != nil {
		return err
	}
	addr := os.Getenv("UNDERGRID_SANDBOX_STRIPE_LISTEN")
	if addr == "" {
		addr = "127.0.0.1:8082"
	}
	server := &http.Server{Addr: addr, Handler: service, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	workerCtx, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan error, 2)
	go func() { done <- server.ListenAndServe() }()
	go func() { done <- service.RunWorker(workerCtx) }()
	select {
	case err = <-done:
	case <-ctx.Done():
		err = nil
	}
	stop()
	shutdown, finish := context.WithTimeout(context.Background(), 10*time.Second)
	defer finish()
	shutdownErr := server.Shutdown(shutdown)
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return errors.Join(err, shutdownErr)
}

func stripeConfigFromEnvironment() (sandbox.StripeConfig, error) {
	mode := os.Getenv("UNDERGRID_SANDBOX_STRIPE_RENEWALS")
	if mode != "" && mode != "manual_test_v1" {
		return sandbox.StripeConfig{}, errors.New("explicit manual TEST renewal configuration required")
	}
	config := sandbox.StripeConfig{SecretKey: os.Getenv("UNDERGRID_SANDBOX_STRIPE_SECRET_KEY"), WebhookSecret: os.Getenv("UNDERGRID_SANDBOX_STRIPE_WEBHOOK_SECRET"), AccountID: os.Getenv("UNDERGRID_SANDBOX_STRIPE_ACCOUNT_ID"), Product: os.Getenv("UNDERGRID_SANDBOX_STRIPE_PRODUCT"), PriceID: os.Getenv("UNDERGRID_SANDBOX_STRIPE_PRICE_ID"), StripeProductID: os.Getenv("UNDERGRID_SANDBOX_STRIPE_PROVIDER_PRODUCT_ID"), SuccessURL: os.Getenv("UNDERGRID_SANDBOX_STRIPE_SUCCESS_URL"), CancelURL: os.Getenv("UNDERGRID_SANDBOX_STRIPE_CANCEL_URL")}
	config.RenewalsEnabled = mode == "manual_test_v1"
	return config, nil
}
