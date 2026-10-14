package network

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func listOwnerAgents(ctx context.Context, tx pgx.Tx, principal string) (int, any, error) {
	rows, err := tx.Query(ctx, `SELECT jsonb_build_object('id',a.id,'name',a.name,'status',a.status,'version',a.version,'etag','"'||a.version||'"','key_control_verified',a.key_control_verified_at IS NOT NULL,'endpoint_control_verified',a.endpoint_control_verified_at IS NOT NULL,'credentials',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',c.id,'scopes',c.scopes,'generation',c.generation,'etag','"'||c.generation||'"','expires_at',c.expires_at,'revoked_at',c.revoked_at) ORDER BY c.created_at) FROM credentials c WHERE c.agent_id=a.id),'[]'::jsonb)) FROM agents a WHERE a.principal_id=$1 ORDER BY a.created_at`, principal)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return 0, nil, err
		}
		var value any
		if err = json.Unmarshal(raw, &value); err != nil {
			return 0, nil, err
		}
		items = append(items, value)
	}
	return 200, map[string]any{"items": items}, rows.Err()
}
func revokeOwnerCredential(r *http.Request, tx pgx.Tx, principal, id, environment string) (int, any, error) {
	if !validID(id) {
		return 0, nil, fail(404, "not_found")
	}
	var input struct{}
	if err := decode(r, &input); err != nil {
		return 0, nil, err
	}
	var agent string
	ctx := r.Context()
	err := tx.QueryRow(ctx, "SELECT a.id FROM agents a JOIN credentials c ON c.agent_id=a.id WHERE c.id=$1 AND a.principal_id=$2 FOR UPDATE OF a", id, principal).Scan(&agent)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	tag, err := tx.Exec(ctx, "UPDATE credentials SET revoked_at=now() WHERE id=$1 AND revoked_at IS NULL", id)
	if err != nil {
		return 0, nil, err
	}
	if tag.RowsAffected() > 0 {
		if err = credentialEvent(ctx, tx, principal, id, 2, "credential.revoked", environment); err != nil {
			return 0, nil, err
		}
	}
	return 200, map[string]bool{"revoked": true}, nil
}
func credentialEvent(ctx context.Context, tx pgx.Tx, principal, id string, version int64, kind, environment string) error {
	origin := "registered"
	if environment != "production" {
		origin = "release_test"
	}
	_, err := tx.Exec(ctx, "INSERT INTO events(id,aggregate_kind,aggregate_id,version,principal_id,type,facts,origin) VALUES($1,'credential',$2,$3,$4,$5,'{}',$6)", NewID(), id, version, principal, kind, origin)
	return err
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
	if r.Method != "POST" && r.Method != "GET" && !(r.Method == "PATCH" && strings.HasPrefix(r.URL.Path, "/v1/owner/intents/")) {
		return 0, nil, fail(405, "method_not_allowed")
	}
	if r.Method == "GET" && r.URL.Path == "/v1/owner/config" {
		return 200, map[string]any{"terms_version": s.termsVersion, "contribution_templates": s.publicTemplates()}, nil
	}
	if r.URL.Path == "/v1/principals" {
		if r.Method != "POST" {
			return 0, nil, fail(405, "method_not_allowed")
		}
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
		if s.termsVersion == "" {
			return 0, nil, fail(503, "approved_terms_unconfigured")
		}
		if input.TermsVersion != s.termsVersion {
			return 0, nil, fail(422, "current_terms_required")
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
	if r.Method == "GET" && r.URL.Path == "/v1/owner/me" {
		return 200, map[string]any{"id": principal}, nil
	}
	if r.URL.Path == "/v1/owner/intents" || strings.HasPrefix(r.URL.Path, "/v1/owner/intents/") {
		return s.ownerIntents(r, tx, principal)
	}
	if id, action, ok := ownerAgentPath(r.URL.Path); ok {
		return s.ownerAgentControl(r, tx, principal, id, action)
	}
	if id, ok := ownerRotationPath(r.URL.Path); ok {
		if r.Method != "POST" {
			return 0, nil, fail(405, "method_not_allowed")
		}
		return s.ownerRotation(r, tx, principal, id)
	}
	if r.Method == "GET" && r.URL.Path == "/v1/agents" {
		return listOwnerAgents(ctx, tx, principal)
	}
	if r.URL.Path == "/v1/owner/policy" {
		return s.ownerPolicy(r, tx, principal)
	}
	if r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/v1/owner/credentials/") && strings.HasSuffix(r.URL.Path, "/revoke") {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/owner/credentials/"), "/revoke")
		return revokeOwnerCredential(r, tx, principal, id, s.environment)
	}
	if r.Method != "POST" || r.URL.Path != "/v1/agents" {
		return 0, nil, fail(404, "not_found")
	}
	var input struct {
		Name    string       `json:"name"`
		Scopes  []string     `json:"scopes"`
		Profile AgentProfile `json:"profile"`
	}
	if err = decode(r, &input); err != nil {
		return 0, nil, err
	}
	if len(input.Name) < 1 || len(input.Name) > 120 || len(input.Scopes) == 0 || len(input.Scopes) > 30 {
		return 0, nil, fail(422, "invalid_agent")
	}
	if err = s.validateAgentScopes(ctx, tx, principal, input.Scopes); err != nil {
		return 0, nil, err
	}
	if err = admitPrincipalResource(ctx, tx, principal, "agents"); err != nil {
		return 0, nil, err
	}
	id := NewID()
	if len(input.Profile.Capabilities) > 20 {
		return 0, nil, fail(422, "capability_limit")
	}
	if _, err = tx.Exec(ctx, "INSERT INTO agents(id,principal_id,name,profile) VALUES($1,$2,$3,$4)", id, principal, input.Name, canonical(input.Profile)); err != nil {
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
	if err == nil {
		err = credentialEvent(ctx, tx, principal, credentialID, 1, "credential.issued", s.environment)
	}
	if err == nil && input.Profile.Public && input.Profile.Recruitment {
		err = enqueueRecruitment(ctx, tx, "")
	}
	return 201, map[string]any{"id": id, "principal_id": principal, "credential_id": credentialID, "token": token, "expires_in": 2592000}, err
}

func (s *Service) validateAgentScopes(ctx context.Context, tx pgx.Tx, principal string, scopes []string) error {
	if len(scopes) == 0 || len(scopes) > 30 {
		return fail(422, "invalid_scopes")
	}
	var err error
	for _, scope := range scopes {
		ordinary := []string{"intents:read", "intents:write", "inbox:read", "swarms:read", "swarms:create", "swarms:apply", "members:invite", "members:respond", "members:withdraw", "messages:write", "plans:propose", "tasks:progress", "artifacts:submit", "artifacts:read", "artifacts:accept", "artifacts:reuse"}
		rightsScopes := []string{"plans:approve", "rights:accept", "tasks:accept", "publication:approve", "artifacts:grant"}
		if !slices.Contains(ordinary, scope) && !slices.Contains(rightsScopes, scope) {
			return fail(403, "scope_not_authorized")
		}
		if slices.Contains(rightsScopes, scope) {
			var raw []byte
			var policy CoordinationPolicy
			if err = tx.QueryRow(ctx, "SELECT policy FROM principals WHERE id=$1", principal).Scan(&raw); err != nil {
				return err
			}
			if json.Unmarshal(raw, &policy) != nil || !policy.ExpiresAt.After(time.Now()) || len(policy.TemplateIDs) == 0 {
				return fail(403, "owner_policy_required")
			}
		}
	}
	return nil
}
