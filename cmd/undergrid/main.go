package main

import (
	"context"
	"encoding/base64"
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
	"undergrid/internal/network"
)

func main() {
	if err := run(); err != nil {
		slog.Error("undergrid stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	mode := flag.String("mode", "api", "api or migrate")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	if *mode == "migrate" {
		dsn := os.Getenv("UNDERGRID_MIGRATION_DATABASE_URL")
		if dsn == "" {
			return errors.New("migration database configuration required")
		}
		conn, err := pgx.Connect(ctx, dsn)
		if err != nil {
			return errors.New("migration database unavailable")
		}
		defer conn.Close(ctx)
		return network.Migrate(ctx, conn)
	}
	if *mode != "api" {
		return errors.New("unsupported process mode")
	}
	dsn := os.Getenv("UNDERGRID_DATABASE_URL")
	if dsn == "" {
		return errors.New("network database configuration required")
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return errors.New("network database unavailable")
	}
	var unsafe bool
	err = conn.QueryRow(ctx, `SELECT rolsuper OR rolbypassrls OR NOT pg_has_role(current_user,'undergrid_app','MEMBER') OR EXISTS(SELECT 1 FROM pg_tables WHERE schemaname='undergrid' AND tableowner=current_user) FROM pg_roles WHERE rolname=current_user`).Scan(&unsafe)
	conn.Close(ctx)
	if err != nil || unsafe {
		return errors.New("API requires a nonowner, non-superuser, non-bypass login with undergrid_app membership")
	}
	origin := os.Getenv("UNDERGRID_ORIGIN")
	sessionKey, err := base64.StdEncoding.DecodeString(os.Getenv("UNDERGRID_SESSION_KEY"))
	if err != nil {
		return errors.New("invalid session key")
	}
	sessions, err := identity.NewSessionManager(origin, sessionKey)
	if err != nil {
		return err
	}
	discoveryCtx, stop := context.WithTimeout(ctx, 15*time.Second)
	defer stop()
	login, err := identity.NewLogin(discoveryCtx, sessions, os.Getenv("UNDERGRID_OIDC_ISSUER"), os.Getenv("UNDERGRID_OIDC_CLIENT_ID"), os.Getenv("UNDERGRID_OIDC_CLIENT_SECRET"))
	if err != nil {
		return errors.New("OIDC discovery/configuration failed")
	}
	environment := os.Getenv("UNDERGRID_ENVIRONMENT")
	if environment != "development" && environment != "staging" && environment != "production" {
		return errors.New("explicit environment required")
	}
	service := network.New(dsn, environment, func(r *http.Request) (network.Owner, error) {
		o, err := sessions.Authenticate(r)
		return network.Owner{Issuer: o.Issuer, Subject: o.Subject, Verified: err == nil}, err
	})
	cursorKey, err := base64.StdEncoding.DecodeString(os.Getenv("UNDERGRID_CURSOR_KEY"))
	if err != nil {
		return errors.New("invalid cursor key")
	}
	if err = service.Configure(cursorKey, os.Getenv("UNDERGRID_TERMS_VERSION")); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/login", login.Start)
	mux.HandleFunc("/auth/callback", login.Callback)
	mux.HandleFunc("/auth/session", sessions.Session)
	mux.HandleFunc("/auth/logout", sessions.Logout)
	mux.Handle("/v1/", service)
	mux.Handle("/health/", service)
	mux.Handle("/", http.FileServer(http.Dir("web/dist")))
	address := os.Getenv("UNDERGRID_LISTEN")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	server := &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
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
