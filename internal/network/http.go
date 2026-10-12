package network

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
}

func New(dsn, environment string, auth OwnerAuthenticator) *Service {
	return &Service{dsn, environment, auth}
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
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	status, result, err := s.execute(r)
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
	if r.URL.Path == "/v1/principals" || r.URL.Path == "/v1/agents" {
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
func (s *Service) ownerCommand(r *http.Request, tx pgx.Tx) (int, any, error) {
	if s.ownerAuth == nil {
		return 0, nil, fail(503, "identity_provider_unconfigured")
	}
	o, err := s.ownerAuth(r)
	if err != nil || !o.Verified || o.Issuer == "" || o.Subject == "" {
		return 0, nil, fail(401, "verified_owner_required")
	}
	ctx := r.Context()
	if _, err = tx.Exec(ctx, "SELECT set_config('undergrid.owner_issuer',$1,true),set_config('undergrid.owner_subject',$2,true)", o.Issuer, o.Subject); err != nil {
		return 0, nil, err
	}
	if r.Method != "POST" {
		return 0, nil, fail(405, "method_not_allowed")
	}
	if r.URL.Path == "/v1/principals" {
		var input struct {
			Name         string `json:"name"`
			TermsVersion string `json:"terms_version"`
		}
		if err = decode(r, &input); err != nil {
			return 0, nil, err
		}
		if len(input.Name) < 1 || len(input.Name) > 120 || input.TermsVersion == "" {
			return 0, nil, fail(422, "name_and_terms_required")
		}
		id := NewID()
		err = tx.QueryRow(ctx, "INSERT INTO principals(id,issuer,subject,name,terms_version) VALUES($1,$2,$3,$4,$5) ON CONFLICT(issuer,subject) DO UPDATE SET subject=excluded.subject RETURNING id", id, o.Issuer, o.Subject, input.Name, input.TermsVersion).Scan(&id)
		return 201, map[string]any{"id": id}, err
	}
	var principal string
	if err = tx.QueryRow(ctx, "SELECT id FROM principals WHERE issuer=$1 AND subject=$2 AND status='active' FOR UPDATE", o.Issuer, o.Subject).Scan(&principal); errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(403, "registration_required")
	}
	if err != nil {
		return 0, nil, err
	}
	var input struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if err = decode(r, &input); err != nil {
		return 0, nil, err
	}
	if len(input.Name) < 1 || len(input.Name) > 120 || len(input.Scopes) == 0 || len(input.Scopes) > 20 {
		return 0, nil, fail(422, "invalid_agent")
	}
	for _, scope := range input.Scopes {
		if !slices.Contains([]string{"intents:read", "intents:write", "inbox:read"}, scope) {
			return 0, nil, fail(403, "scope_not_authorized")
		}
	}
	id := NewID()
	if _, err = tx.Exec(ctx, "INSERT INTO agents(id,principal_id,name) VALUES($1,$2,$3)", id, principal, input.Name); err != nil {
		return 0, nil, err
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return 0, nil, err
	}
	token := "ug_" + base64.RawURLEncoding.EncodeToString(secret)
	hash := sha256.Sum256([]byte(token))
	credentialID := NewID()
	_, err = tx.Exec(ctx, "INSERT INTO credentials(id,agent_id,digest,scopes,audience,expires_at) VALUES($1,$2,$3,$4,'undergrid-network',now()+interval '30 days')", credentialID, id, hash[:], input.Scopes)
	return 201, map[string]any{"id": id, "principal_id": principal, "credential_id": credentialID, "token": token, "expires_in": 2592000}, err
}
func (s *Service) agentCommand(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	if r.Method == "GET" && r.URL.Path == "/v1/me" {
		return 200, map[string]any{"agent_id": a.AgentID, "principal_id": a.PrincipalID, "environment": s.environment}, nil
	}
	if r.Method == "DELETE" && r.URL.Path == "/v1/credentials/current" {
		_, err := tx.Exec(r.Context(), "UPDATE credentials SET revoked_at=now() WHERE id=$1", a.CredentialID)
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
