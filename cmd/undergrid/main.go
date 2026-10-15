package main

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	mode := flag.String("mode", "api", "api, worker or migrate")
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
	if *mode != "api" && *mode != "worker" {
		return errors.New("unsupported process mode")
	}
	dsn := os.Getenv("UNDERGRID_DATABASE_URL")
	role := "undergrid_app"
	if *mode == "worker" {
		dsn = os.Getenv("UNDERGRID_WORKER_DATABASE_URL")
		role = "undergrid_worker"
	}
	if dsn == "" {
		return errors.New("network database configuration required")
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return errors.New("network database unavailable")
	}
	var unsafe bool
	err = conn.QueryRow(ctx, `SELECT rolsuper OR rolbypassrls OR NOT pg_has_role(current_user,$1,'MEMBER') OR EXISTS(SELECT 1 FROM pg_tables WHERE schemaname='undergrid' AND tableowner=current_user) FROM pg_roles WHERE rolname=current_user`, role).Scan(&unsafe)
	var ready bool
	if err == nil {
		err = conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM public.undergrid_migrations WHERE name='river-0.49.0-ready')").Scan(&ready)
	}
	conn.Close(ctx)
	if err != nil || unsafe || !ready {
		return errors.New("API requires a nonowner, non-superuser, non-bypass login with undergrid_app membership")
	}
	environment := os.Getenv("UNDERGRID_ENVIRONMENT")
	if environment != "development" && environment != "staging" && environment != "production" {
		return errors.New("explicit environment required")
	}
	if *mode == "worker" {
		return network.RunWorker(ctx, dsn, environment)
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
	service := network.New(dsn, environment, func(r *http.Request) (network.Owner, error) {
		o, err := sessions.Authenticate(r)
		return network.Owner{Issuer: o.Issuer, Subject: o.Subject, Verified: err == nil}, err
	})
	if err = service.ConfigureA2ATransport(origin); err != nil {
		return err
	}

	cursorKey, err := base64.StdEncoding.DecodeString(os.Getenv("UNDERGRID_CURSOR_KEY"))
	if err != nil {
		return errors.New("invalid cursor key")
	}
	if err = service.Configure(cursorKey, os.Getenv("UNDERGRID_TERMS_VERSION")); err != nil {
		return err
	}
	if value := os.Getenv("UNDERGRID_REPORT_TRIAGE_WINDOW"); value != "" {
		window, parseErr := time.ParseDuration(value)
		if parseErr != nil {
			return errors.New("invalid report triage window")
		}
		if err = service.ConfigureReportTriageWindow(window); err != nil {
			return err
		}
	}
	issuanceKey, err := base64.StdEncoding.DecodeString(os.Getenv("UNDERGRID_CREDENTIAL_ISSUANCE_KEY"))
	if err != nil || subtle.ConstantTimeCompare(issuanceKey, sessionKey) == 1 {
		return errors.New("dedicated credential issuance key required")
	}
	if err = service.ConfigureCredentialIssuance(os.Getenv("UNDERGRID_CREDENTIAL_ISSUANCE_KEY_ID"), issuanceKey); err != nil {
		return err
	}
	oauthResource := os.Getenv("UNDERGRID_NETWORK_OAUTH_RESOURCE")
	oauthIssuers := strings.TrimSpace(os.Getenv("UNDERGRID_NETWORK_OAUTH_ISSUERS"))
	if oauthResource != "" || oauthIssuers != "" {
		if oauthResource == "" || oauthIssuers == "" {
			return errors.New("network OAuth resource and trusted issuer configuration must be set together")
		}
		issuers := []string{}
		for _, issuer := range strings.Split(oauthIssuers, ",") {
			if issuer = strings.TrimSpace(issuer); issuer != "" {
				issuers = append(issuers, issuer)
			}
		}
		if err = service.ConfigureOAuthResource(network.OAuthResourceConfig{Resource: oauthResource, Issuers: issuers}); err != nil {
			return errors.New("network OAuth resource discovery/configuration failed")
		}
	}
	if err = service.ConfigureArtifactsFromEnvironment(); err != nil {
		return err
	}
	if path := os.Getenv("UNDERGRID_CONTRIBUTION_TEMPLATES_FILE"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return errors.New("contribution template configuration unavailable")
		}
		var templates []network.ContributionTemplate
		if err = json.Unmarshal(raw, &templates); err != nil {
			return errors.New("invalid contribution template configuration")
		}
		if err = service.SetContributionTemplates(templates); err != nil {
			return err
		}
	}
	if err = service.ConfigureMCP(origin); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/login", login.Start)
	mux.HandleFunc("/auth/callback", login.Callback)
	mux.HandleFunc("/auth/session", sessions.Session)
	mux.HandleFunc("/auth/logout", sessions.Logout)
	registerNetworkRoutes(mux, service)
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

// Keep network route wiring together so public documentation reaches the same
// handler as the domain API, instead of falling through to the static console.
func registerNetworkRoutes(mux *http.ServeMux, service *network.Service) {
	mux.Handle("/mcp", service)
	mux.Handle("/v1/", service)
	mux.Handle("/.well-known/", service)
	mux.Handle("/health/", service)
	mux.Handle("/openapi.json", service)
	mux.Handle("/a2a/", service)
}
