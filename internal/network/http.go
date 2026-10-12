package network

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type OwnerAuthenticator func(*http.Request) (Owner, error)
type Service struct {
	dsn, environment string
	ownerAuth        OwnerAuthenticator
	cursorKey        []byte
	termsVersion     string
}

func (s *Service) Configure(cursorKey []byte, termsVersion string) error {
	if len(cursorKey) != 32 || termsVersion == "" {
		return errors.New("32-byte cursor key and approved terms version required")
	}
	s.cursorKey = append([]byte(nil), cursorKey...)
	s.termsVersion = termsVersion
	return nil
}

func New(dsn, environment string, auth OwnerAuthenticator) *Service {
	return &Service{dsn: dsn, environment: environment, ownerAuth: auth}
}

func decode(r *http.Request, v any) error {
	d := json.NewDecoder(io.LimitReader(r.Body, 65537))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fail(400, "invalid_json")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fail(400, "invalid_json")
	}
	return nil
}
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Path == "/health/live" && r.Method == "GET" {
		w.Write([]byte(`{"status":"alive"}`))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	status, result, err := s.execute(r)
	if err == nil && r.Method == "GET" && r.URL.Path == "/v1/inbox" {
		status, result, err = s.waitInbox(r, status, result)
	}
	if err != nil {
		var p problem
		if !errors.As(err, &p) {
			slog.Error("request failed", "path", r.URL.Path, "error", err)
			p = problem{500, "internal_error"}
		}
		status = p.Status
		result = map[string]any{"error": map[string]string{"code": p.Code}}
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(result)
}
func (s *Service) execute(r *http.Request) (int, any, error) {
	ctx := r.Context()
	db, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		return 0, nil, err
	}
	defer db.Close(ctx)
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET LOCAL ROLE undergrid_app; SET LOCAL search_path=undergrid,pg_catalog; SET LOCAL statement_timeout='5s'"); err != nil {
		return 0, nil, err
	}
	var status int
	var result any
	if r.URL.Path == "/v1/principals" || r.URL.Path == "/v1/agents" || strings.HasPrefix(r.URL.Path, "/v1/owner/") {
		status, result, err = s.ownerCommand(r, tx)
	} else {
		var a actor
		a, err = authenticate(ctx, tx, r.Header.Get("Authorization"))
		if err == nil {
			status, result, err = s.agentCommand(r, tx, a)
		}
	}
	if err != nil {
		return 0, nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, nil, err
	}
	return status, result, nil
}
func authenticate(ctx context.Context, tx pgx.Tx, authorization string) (actor, error) {
	var a actor
	token, ok := strings.CutPrefix(authorization, "Bearer ")
	if !ok || !strings.HasPrefix(token, "ug_") {
		return a, fail(401, "invalid_credential")
	}
	h := sha256.Sum256([]byte(token))
	err := tx.QueryRow(ctx, "SELECT principal_id,agent_id,credential_id FROM identify_token($1)", h[:]).Scan(&a.PrincipalID, &a.AgentID, &a.CredentialID)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, fail(401, "invalid_credential")
	}
	if err != nil {
		return a, err
	}
	if _, err = tx.Exec(ctx, "SELECT set_config('undergrid.principal_id',$1,true),set_config('undergrid.agent_id',$2,true)", a.PrincipalID, a.AgentID); err != nil {
		return a, err
	}
	var state string
	if err = tx.QueryRow(ctx, "SELECT status FROM principals WHERE id=$1 FOR UPDATE", a.PrincipalID).Scan(&state); err != nil {
		return a, err
	}
	if state != "active" {
		return a, fail(401, "inactive_principal")
	}
	if err = tx.QueryRow(ctx, "SELECT status FROM agents WHERE id=$1 FOR UPDATE", a.AgentID).Scan(&state); err != nil {
		return a, err
	}
	if state != "active" {
		return a, fail(401, "inactive_agent")
	}
	err = tx.QueryRow(ctx, "SELECT scopes FROM credentials WHERE id=$1 AND revoked_at IS NULL AND expires_at>now() FOR UPDATE", a.CredentialID).Scan(&a.Scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, fail(401, "invalid_credential")
	}
	return a, err
}
func (s *Service) agentCommand(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/invitations/") {
		if !slices.Contains(a.Scopes, "swarms:read") {
			return 0, nil, fail(403, "insufficient_scope")
		}
		return invitationPacket(r, tx, a)
	}
	if r.URL.Path == "/v1/swarms" || strings.HasPrefix(r.URL.Path, "/v1/swarms/") {
		return s.swarmCommand(r, tx, a)
	}
	if r.Method == "GET" && (r.URL.Path == "/v1/opportunities" || strings.HasPrefix(r.URL.Path, "/v1/opportunities/") || (strings.HasPrefix(r.URL.Path, "/v1/intents/") && strings.HasSuffix(r.URL.Path, "/opportunities"))) {
		if !slices.Contains(a.Scopes, "intents:read") {
			return 0, nil, fail(403, "insufficient_scope")
		}
		return listOpportunities(r, tx, a)
	}
	if r.URL.Path == "/v1/inbox" || r.URL.Path == "/v1/agent-context" || strings.HasPrefix(r.URL.Path, "/v1/inbox/") {
		if !slices.Contains(a.Scopes, "inbox:read") {
			return 0, nil, fail(403, "insufficient_scope")
		}
		return s.inboxCommand(r, tx, a)
	}
	if r.Method == "GET" && r.URL.Path == "/v1/me" {
		return 200, map[string]any{"agent_id": a.AgentID, "principal_id": a.PrincipalID, "environment": s.environment}, nil
	}
	if r.Method == "DELETE" && r.URL.Path == "/v1/credentials/current" {
		_, err := tx.Exec(r.Context(), "UPDATE credentials SET revoked_at=now() WHERE id=$1", a.CredentialID)
		if err == nil {
			err = credentialEvent(r.Context(), tx, a.PrincipalID, a.CredentialID, 2, "credential.revoked", s.environment)
		}
		return 200, map[string]bool{"revoked": true}, err
	}
	if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/intents/") {
		if !slices.Contains(a.Scopes, "intents:read") {
			return 0, nil, fail(403, "insufficient_scope")
		}
		return getIntent(r.Context(), tx, a, strings.TrimPrefix(r.URL.Path, "/v1/intents/"))
	}
	if (r.Method == "POST" && r.URL.Path == "/v1/intents") || (r.Method == "PATCH" && strings.HasPrefix(r.URL.Path, "/v1/intents/")) {
		if !slices.Contains(a.Scopes, "intents:write") {
			return 0, nil, fail(403, "insufficient_scope")
		}
		return mutateIntent(r, tx, a, s.environment)
	}
	return 0, nil, fail(404, "not_found")
}
