// The sandbox is a separate process with no network-domain dependency.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"undergrid/internal/identity"
	"undergrid/internal/sandbox"
)

func main() {
	if err := run(); err != nil {
		slog.Error("TEST sandbox stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	mode := flag.String("mode", "api", "api, renewal-worker, stripe-worker or migrate; TEST only")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	dsn := os.Getenv("UNDERGRID_SANDBOX_DATABASE_URL")
	if *mode == "migrate" {
		dsn = os.Getenv("UNDERGRID_SANDBOX_MIGRATION_DATABASE_URL")
	}
	if dsn == "" {
		return errors.New("separate sandbox database configuration required")
	}
	if *mode == "api" && (os.Getenv("UNDERGRID_SANDBOX_STRIPE_SECRET_KEY") != "" || os.Getenv("UNDERGRID_SANDBOX_STRIPE_WEBHOOK_SECRET") != "") {
		return errors.New("sandbox API must not carry provider worker credentials")
	}
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return errors.New("sandbox database unavailable")
	}
	defer c.Close(ctx)
	if *mode == "migrate" {
		return sandbox.Migrate(ctx, c)
	}
	if *mode != "api" && *mode != "renewal-worker" && *mode != "stripe-worker" {
		return errors.New("unsupported sandbox mode")
	}
	if err = sandbox.ValidateGroupRoles(ctx, c); err != nil {
		return err
	}
	if *mode == "renewal-worker" || *mode == "stripe-worker" {
		var unsafe bool
		err = c.QueryRow(ctx, `SELECT rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication OR CASE WHEN to_regrole('undergrid_app') IS NOT NULL THEN pg_has_role(current_user,'undergrid_app','MEMBER') ELSE false END OR CASE WHEN to_regrole('undergrid_worker') IS NOT NULL THEN pg_has_role(current_user,'undergrid_worker','MEMBER') ELSE false END OR NOT pg_has_role(current_user,'undergrid_sandbox_worker','MEMBER') OR pg_has_role(current_user,'undergrid_sandbox_app','MEMBER') OR EXISTS(SELECT FROM pg_tables WHERE schemaname='undergrid_sandbox' AND tableowner=current_user) OR to_regnamespace('undergrid') IS NOT NULL FROM pg_roles WHERE rolname=current_user`).Scan(&unsafe)
		if err != nil || unsafe {
			return errors.New("sandbox renewal worker requires isolated nonowner worker login without app privileges")
		}
		c.Close(ctx)
		if *mode == "stripe-worker" {
			return runStripe(ctx, dsn)
		}
		return sandbox.New(dsn).RunRenewalWorker(ctx)
	}
	var unsafe bool
	err = c.QueryRow(ctx, `SELECT rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication OR CASE WHEN to_regrole('undergrid_app') IS NOT NULL THEN pg_has_role(current_user,'undergrid_app','MEMBER') ELSE false END OR CASE WHEN to_regrole('undergrid_worker') IS NOT NULL THEN pg_has_role(current_user,'undergrid_worker','MEMBER') ELSE false END OR NOT pg_has_role(current_user,'undergrid_sandbox_app','MEMBER') OR pg_has_role(current_user,'undergrid_sandbox_worker','MEMBER') OR EXISTS(SELECT FROM pg_tables WHERE schemaname='undergrid_sandbox' AND tableowner=current_user) OR to_regnamespace('undergrid') IS NOT NULL FROM pg_roles WHERE rolname=current_user`).Scan(&unsafe)
	if err != nil || unsafe {
		return errors.New("sandbox API requires isolated nonowner app login without worker privileges")
	}
	c.Close(ctx)
	key, err := base64.StdEncoding.DecodeString(os.Getenv("UNDERGRID_SANDBOX_SESSION_KEY"))
	if err != nil {
		return errors.New("invalid sandbox session key")
	}
	sessions, err := identity.NewSessionManager(os.Getenv("UNDERGRID_SANDBOX_ORIGIN"), key)
	if err != nil {
		return err
	}
	discovery, stop := context.WithTimeout(ctx, 15*time.Second)
	defer stop()
	login, err := identity.NewLogin(discovery, sessions, os.Getenv("UNDERGRID_SANDBOX_OIDC_ISSUER"), os.Getenv("UNDERGRID_SANDBOX_OIDC_CLIENT_ID"), os.Getenv("UNDERGRID_SANDBOX_OIDC_CLIENT_SECRET"))
	if err != nil {
		return errors.New("sandbox OIDC configuration failed")
	}
	var catalog map[string]sandbox.Quote
	if err = json.Unmarshal([]byte(os.Getenv("UNDERGRID_SANDBOX_TEST_CATALOG")), &catalog); err != nil {
		return errors.New("invalid TEST catalog")
	}
	usdcConfigured, err := validateSandboxCatalog(catalog)
	if err != nil {
		return err
	}
	store := sandbox.New(dsn)
	if usdcConfigured {
		config, e := sandbox.ParseUSDCSigningConfig(os.Getenv("UNDERGRID_SANDBOX_USDC_SIGNING_KEY_ID"), os.Getenv("UNDERGRID_SANDBOX_USDC_SIGNING_PRIVATE_KEY"), os.Getenv("UNDERGRID_SANDBOX_USDC_SIGNING_PUBLIC_KEYS"))
		if e != nil {
			return errors.New("explicit trusted USDC signing configuration required")
		}
		store, e = store.WithUSDCSigning(config)
		if e != nil {
			return e
		}
	}
	service, err := sandbox.NewHandler(store, func(r *http.Request) (sandbox.Owner, error) {
		o, e := sessions.Authenticate(r)
		return sandbox.Owner{Issuer: o.Issuer, Subject: o.Subject}, e
	}, catalog, os.Getenv("UNDERGRID_SANDBOX_ORIGIN"))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/login", login.Start)
	mux.HandleFunc("/auth/callback", login.Callback)
	mux.HandleFunc("/auth/session", sessions.Session)
	mux.HandleFunc("/auth/logout", sessions.Logout)
	mux.Handle("/", service)
	addr := os.Getenv("UNDERGRID_SANDBOX_LISTEN")
	if addr == "" {
		addr = "127.0.0.1:8081"
	}
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	stopped := make(chan error, 1)
	go func() { stopped <- server.ListenAndServe() }()
	select {
	case err = <-stopped:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdown, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()
	return server.Shutdown(shutdown)
}

func validateSandboxCatalog(catalog map[string]sandbox.Quote) (bool, error) {
	if len(catalog) < 1 || len(catalog) > 2 {
		return false, errors.New("one Stripe product and one USDC resource at most required")
	}
	usdc := false
	seen := map[string]bool{}
	for id, q := range catalog {
		if id != q.Product || !(q.Rail == "stripe_test" && q.Asset == "USD" || q.Rail == "x402_test" && q.Asset == "USDC") {
			return false, errors.New("invalid TEST catalog")
		}
		if seen[q.Rail] {
			return false, errors.New("one product per TEST rail required")
		}
		seen[q.Rail] = true
		if q.Asset == "USDC" {
			usdc = true
		}
	}
	return usdc, nil
}
