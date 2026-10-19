package network

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type OwnerAuthenticator func(*http.Request) (Owner, error)
type Service struct {
	semantic           *semanticProvider
	a2a                *networkA2A
	mcpHandler         http.Handler
	mcpOrigin          string
	dsn, environment   string
	ownerAuth          OwnerAuthenticator
	cursorKey          []byte
	issuanceKey        []byte
	issuanceKeyID      string
	credentialClock    func() time.Time
	termsVersion       string
	templates          map[string]ContributionTemplate
	artifacts          *localArtifactRuntime
	oauth              *oauthVerifier
	reportTriageWindow time.Duration
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
	return &Service{dsn: dsn, environment: environment, ownerAuth: auth, reportTriageWindow: 72 * time.Hour}
}

// ConfigureReportTriageWindow sets an internal queue target. It is not a
// reporter-facing service promise or a legal response deadline.
func (s *Service) ConfigureReportTriageWindow(window time.Duration) error {
	if window < time.Hour || window > 30*24*time.Hour {
		return errors.New("report triage window must be between one hour and thirty days")
	}
	s.reportTriageWindow = window
	return nil
}

func (s *Service) ConfigureOAuthResource(cfg OAuthResourceConfig) error {
	verifier, err := newOAuthVerifier(cfg)
	if err != nil {
		return err
	}
	s.oauth = verifier
	return nil
}

func decode(r *http.Request, v any) error {
	// Read one byte past the limit before decoding. A decoder over a limited
	// reader cannot distinguish EOF from a truncated oversized request.
	const maxCommandBytes = 64 * 1024
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxCommandBytes+1))
	if err != nil {
		return fail(400, "invalid_json")
	}
	if len(raw) > maxCommandBytes {
		return fail(413, "body_too_large")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
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
	w.Header().Set("X-Request-ID", NewID())
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.HasPrefix(r.URL.Path, "/skills/undergrid-network/") {
		s.serveAgentSkill(w, r)
		return
	}
	if r.URL.Path == "/.well-known/agent-card.json" || r.URL.Path == "/a2a" || strings.HasPrefix(r.URL.Path, "/a2a/") {
		if s.a2a == nil {
			writeA2AProtocolError(w, a2aUnavailable())
			return
		}
		s.a2a.serve(w, r)
		return
	}
	if r.URL.Path == "/mcp" {
		s.serveMCP(w, r)
		return
	}
	if r.URL.Path == "/openapi.json" {
		if r.Method != "GET" {
			s.writeProblem(w, fail(405, "method_not_allowed"))
			return
		}
		_, _ = w.Write(networkOpenAPI)
		return
	}
	if r.URL.Path == "/health/live" && r.Method == "GET" {
		w.Write([]byte(`{"status":"alive"}`))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/.well-known/oauth-protected-resource") {
		s.oauthMetadata(w, r)
		return
	}
	capabilityUpload := r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/artifacts/") && strings.HasSuffix(r.URL.Path, "/upload")
	var verificationErr error
	r, verificationErr = s.verifyNetworkRequest(r)
	if verificationErr != nil {
		s.oauthChallenge(w, false)
		s.writeProblem(w, verificationErr)
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
		if errors.As(err, &p) && !capabilityUpload && (p.Status == 401 || p.Status == 403) {
			s.oauthChallenge(w, p.Code == "insufficient_scope")
		}
		s.writeProblem(w, err)
		return
	}
	// The response owns its validator, including an original idempotent replay.
	// Lists, compound snapshots and errors do not acquire guessed validators.
	if resource, ok := result.(map[string]any); ok {
		if tag, ok := resource["etag"].(string); ok {
			w.Header().Set("ETag", tag)
		}
	}
	w.WriteHeader(status)
	if content, ok := result.(artifactContent); ok {
		_, _ = w.Write(content)
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Service) oauthChallenge(w http.ResponseWriter, insufficientScope bool) {
	if s.oauth == nil {
		return
	}
	resource, _ := url.Parse(s.oauth.resource)
	decodedPath, err := url.PathUnescape(s.oauth.metadataPath)
	if err != nil {
		return
	}
	u := &url.URL{Scheme: "https", Host: resource.Host, Path: decodedPath, RawPath: s.oauth.metadataPath}
	value := `Bearer resource_metadata="` + u.String() + `"`
	if insufficientScope {
		value += `, error="insufficient_scope"`
	}
	w.Header().Set("WWW-Authenticate", value)
}

func (s *Service) oauthMetadata(w http.ResponseWriter, r *http.Request) {
	if s.oauth == nil || r.Method != http.MethodGet || r.URL.EscapedPath() != s.oauth.metadataPath {
		http.NotFound(w, r)
		return
	}
	issuers := make([]string, 0, len(s.oauth.issuers))
	for issuer := range s.oauth.issuers {
		issuers = append(issuers, issuer)
	}
	slices.Sort(issuers)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"resource": s.oauth.resource, "resource_name": "Undergrid Network API", "authorization_servers": issuers, "scopes_supported": networkOAuthScopes(), "bearer_methods_supported": []string{"header"}})
}

// One renderer keeps transaction errors and adapter-level authentication
// denials on the same public contract. Internal errors are logged, never sent.
func (s *Service) writeProblem(w http.ResponseWriter, err error) {
	var p problem
	requestID := w.Header().Get("X-Request-ID")
	if !errors.As(err, &p) {
		slog.Error("request failed", "request_id", requestID, "error", err)
		p = problem{500, "internal_error"}
	}
	if p.Status == 429 {
		w.Header().Set("Retry-After", "60")
	}
	message := strings.ReplaceAll(p.Code, "_", " ")
	if p.Status >= 500 {
		message = "The service could not complete the request."
	}
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
		"code": p.Code, "message": message, "request_id": requestID,
		"retryable": p.Status == 429 || p.Status == 500 || p.Status == 502 || p.Status == 503 || p.Status == 504,
	}})

}
func (s *Service) execute(r *http.Request) (int, any, error) {
	ctx := r.Context()
	db, tx, err := s.beginNetworkTransaction(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer db.Close(ctx)
	defer tx.Rollback(ctx)

	var status int
	var result any
	if r.URL.Path == "/v1/principals" || r.URL.Path == "/v1/agents" || strings.HasPrefix(r.URL.Path, "/v1/owner/") {
		status, result, err = s.ownerCommand(r, tx)
	} else {
		_, status, result, err = s.executeAgentCommandTx(r, tx)
	}
	if err != nil {
		return 0, nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, nil, err
	}
	return status, result, nil
}
func authenticate(ctx context.Context, tx pgx.Tx, authorization, method, path string) (actor, error) {
	var a actor
	token, ok := strings.CutPrefix(authorization, "Bearer ")
	if !ok || token == "" {
		return a, fail(401, "invalid_credential")
	}
	var err error
	if strings.HasPrefix(token, "ug_") {
		h := sha256.Sum256([]byte(token))
		err = tx.QueryRow(ctx, "SELECT principal_id,agent_id,credential_id FROM identify_token($1)", h[:]).Scan(&a.PrincipalID, &a.AgentID, &a.CredentialID)
	} else {
		claims, found := ctx.Value(oauthResourceContextKey).(verifiedOAuth)
		if !found {
			return a, fail(401, "invalid_credential")
		}
		err = tx.QueryRow(ctx, "SELECT principal_id,agent_id,credential_id FROM identify_oauth($1,$2,$3,$4)", claims.Issuer, claims.Subject, claims.ClientID, claims.Resource).Scan(&a.PrincipalID, &a.AgentID, &a.CredentialID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return a, fail(401, "invalid_credential")
	}
	if err != nil {
		return a, err
	}
	if _, err = tx.Exec(ctx, "SELECT set_config('undergrid.principal_id',$1,true),set_config('undergrid.agent_id',$2,true)", a.PrincipalID, a.AgentID); err != nil {
		return a, err
	}
	if (method == "POST" || method == "GET") && strings.HasPrefix(path, "/v1/artifacts/") && !strings.HasSuffix(path, "/reuse-receipts") {
		parts := strings.Split(path, "/")
		if (len(parts) == 5 || len(parts) == 4) && validID(parts[3]) {
			if _, err = tx.Exec(ctx, "SELECT lock_artifact_authorities($1)", parts[3]); err != nil {
				return a, err
			}
		}
	}
	if method == "POST" && (strings.HasPrefix(path, "/v1/plans/") || strings.HasPrefix(path, "/v1/tasks/")) {
		parts := strings.Split(path, "/")
		if len(parts) == 5 && validID(parts[3]) {
			planID := parts[3]
			if parts[2] == "tasks" {
				err = tx.QueryRow(ctx, "SELECT plan_id FROM tasks WHERE id=$1", parts[3]).Scan(&planID)
				if errors.Is(err, pgx.ErrNoRows) {
					return a, fail(404, "not_found")
				}
				if err != nil {
					return a, err
				}
			}
			if _, err = tx.Exec(ctx, "SELECT lock_plan_authorities($1)", planID); err != nil {
				return a, err
			}
		}
	}
	if err = prelockCoordinatorRequest(ctx, tx, a, method, path); err != nil {
		return a, err
	}
	a, err = lockActor(ctx, tx, a)
	if err != nil {
		return a, err
	}
	if claims, ok := ctx.Value(oauthResourceContextKey).(verifiedOAuth); ok && !strings.HasPrefix(token, "ug_") {
		// Signature verification precedes database waits. A later access token
		// can expire before its longer-lived local grant, so recheck its own
		// deadline after acquiring the complete actor authority.
		if !claims.Expires.After(time.Now()) {
			return a, fail(401, "invalid_credential")
		}
		a.grantedScopes = append([]string(nil), a.Scopes...)
		a.Scopes = scopeIntersection(a.Scopes, claims.Scopes)
	}
	return a, nil
}
func lockActor(ctx context.Context, tx pgx.Tx, a actor) (actor, error) {
	var state string
	var err error
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
	// Lock first, then evaluate real server time. now() is fixed at transaction
	// start; even clock_timestamp() in the locking WHERE may run before a row
	// lock wait. MATERIALIZED prevents the expiry predicate moving ahead of it.
	err = tx.QueryRow(ctx, `WITH locked_credential AS MATERIALIZED (
 SELECT scopes,revoked_at,expires_at FROM credentials WHERE id=$1 FOR UPDATE
 ) SELECT scopes FROM locked_credential
 WHERE revoked_at IS NULL AND expires_at>clock_timestamp()`, a.CredentialID).Scan(&a.Scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, fail(401, "invalid_credential")
	}
	return a, err
}

func scopeIntersection(current, token []string) []string {
	out := make([]string, 0, len(current))
	for _, scope := range current {
		if slices.Contains(token, scope) {
			out = append(out, scope)
		}
	}
	return out
}

func networkOAuthScopes() []string {
	return []string{"intents:read", "intents:write", "inbox:read", "swarms:read", "swarms:create", "swarms:archive", "swarms:apply", "members:invite", "members:respond", "members:withdraw", "coordinator:propose", "coordinator:approve", "coordinator:accept", "messages:write", "plans:propose", "plans:approve", "tasks:progress", "tasks:accept", "artifacts:submit", "artifacts:read", "artifacts:accept", "artifacts:reuse", "artifacts:grant", "rights:accept", "publication:approve", "reports:read", "reports:write"}
}
func (s *Service) agentCommand(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	if _, _, _, ok := coordinatorRequest(r.URL.Path); ok {
		return s.coordinatorCommand(r, tx, a)
	}
	if r.URL.Path == "/v1/reports" || strings.HasPrefix(r.URL.Path, "/v1/reports/") {
		return s.reportCommand(r, tx, a)
	}
	if r.Method == "GET" && r.URL.Path == "/v1/policies/contribution-templates" {
		return 200, s.publicTemplates(), nil
	}
	if r.URL.Path == "/v1/artifacts" || strings.HasPrefix(r.URL.Path, "/v1/artifacts/") {
		return s.artifactCommand(r, tx, a)
	}
	if strings.HasPrefix(r.URL.Path, "/v1/artifact-input-grants/") {
		return s.inputGrantCommand(r, tx, a)
	}
	if strings.HasPrefix(r.URL.Path, "/v1/plans/") || strings.HasPrefix(r.URL.Path, "/v1/tasks/") {
		return s.planCommand(r, tx, a)
	}
	if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/invitations/") {
		if !slices.Contains(a.Scopes, "swarms:read") {
			return 0, nil, insufficientScope(a, "swarms:read")
		}
		return invitationPacket(r, tx, a)
	}
	if r.URL.Path == "/v1/swarms" || strings.HasPrefix(r.URL.Path, "/v1/swarms/") {
		return s.swarmCommand(r, tx, a)
	}
	if id, ok := externalFitIntentID(r.URL.Path); ok {
		if !slices.Contains(a.Scopes, "intents:read") {
			return 0, nil, insufficientScope(a, "intents:read")
		}
		return s.externalFitReport(r, tx, a, id)
	}
	if id, ok := externalLeadIntentID(r.URL.Path); ok {
		if !slices.Contains(a.Scopes, "intents:read") {
			return 0, nil, insufficientScope(a, "intents:read")
		}
		return s.listExternalLeads(r, tx, a, id, "agent")
	}
	if r.Method == "GET" && (r.URL.Path == "/v1/opportunities" || strings.HasPrefix(r.URL.Path, "/v1/opportunities/") || (strings.HasPrefix(r.URL.Path, "/v1/intents/") && strings.HasSuffix(r.URL.Path, "/opportunities"))) {
		if !slices.Contains(a.Scopes, "intents:read") {
			return 0, nil, insufficientScope(a, "intents:read")
		}
		return listOpportunities(r, tx, a)
	}
	if r.URL.Path == "/v1/inbox" || r.URL.Path == "/v1/agent-context" || strings.HasPrefix(r.URL.Path, "/v1/inbox/") {
		if !slices.Contains(a.Scopes, "inbox:read") {
			return 0, nil, insufficientScope(a, "inbox:read")
		}
		return s.inboxCommand(r, tx, a)
	}
	if r.Method == "GET" && r.URL.Path == "/v1/me" {
		return 200, map[string]any{"agent_id": a.AgentID, "principal_id": a.PrincipalID, "environment": s.environment, "scopes": a.Scopes}, nil
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
			return 0, nil, insufficientScope(a, "intents:read")
		}
		return getIntent(r.Context(), tx, a, strings.TrimPrefix(r.URL.Path, "/v1/intents/"))
	}
	if (r.Method == "POST" && r.URL.Path == "/v1/intents") || (r.Method == "PATCH" && strings.HasPrefix(r.URL.Path, "/v1/intents/")) {
		if !slices.Contains(a.Scopes, "intents:write") {
			return 0, nil, insufficientScope(a, "intents:write")
		}
		return mutateIntent(r, tx, a, s.environment)
	}
	return 0, nil, fail(404, "not_found")
}
