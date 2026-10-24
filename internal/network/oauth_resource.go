package network

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type oauthContextKey string

const oauthResourceContextKey oauthContextKey = "undergrid-network-oauth"

type verifiedOAuth struct {
	Issuer, Subject, ClientID string
	Resource                  string
	Scopes                    []string
	Expires                   time.Time
}

type OAuthResourceConfig struct {
	Resource   string
	Issuers    []string
	HTTPClient *http.Client
}

type oauthIssuer struct {
	issuer             string
	metadataURL        string
	client             *http.Client
	mu                 sync.Mutex
	keys               *jose.JSONWebKeySet
	refreshed          time.Time
	lastRefreshAttempt time.Time
	lastUnknownRefresh time.Time
}

type oauthVerifier struct {
	resource     string
	metadataPath string
	issuers      map[string]*oauthIssuer
}

func newOAuthVerifier(cfg OAuthResourceConfig) (*oauthVerifier, error) {
	resource, err := url.Parse(cfg.Resource)
	if err != nil || resource.Scheme != "https" || resource.Host == "" || resource.User != nil || resource.Fragment != "" || resource.RawQuery != "" {
		return nil, errors.New("OAuth resource must be a canonical HTTPS URI without credentials, query or fragment")
	}
	if len(cfg.Issuers) == 0 || len(cfg.Issuers) > 8 {
		return nil, errors.New("between one and eight trusted OAuth authorization servers are required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	} else {
		copy := *client
		client = &copy
	}
	if client.Timeout == 0 {
		return nil, errors.New("OAuth discovery client must have a finite timeout")
	}
	if client.Timeout > 5*time.Second {
		client.Timeout = 5 * time.Second
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	v := &oauthVerifier{resource: cfg.Resource, metadataPath: protectedResourceMetadataPath(resource), issuers: map[string]*oauthIssuer{}}
	for _, issuer := range cfg.Issuers {
		u, parseErr := url.Parse(issuer)
		if parseErr != nil || len(issuer) > 512 || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("trusted OAuth issuer must be a canonical HTTPS URI")
		}
		if _, exists := v.issuers[issuer]; exists {
			return nil, errors.New("duplicate trusted OAuth issuer")
		}
		metadata := *u
		metadata.Path = "/.well-known/oauth-authorization-server" + strings.TrimSuffix(u.Path, "/")
		metadata.RawPath = ""
		entry := &oauthIssuer{issuer: issuer, metadataURL: metadata.String(), client: client}
		if err := entry.refresh(context.Background()); err != nil {
			return nil, fmt.Errorf("trusted OAuth issuer discovery failed: %w", err)
		}
		v.issuers[issuer] = entry
	}
	return v, nil
}

func protectedResourceMetadataPath(resource *url.URL) string {
	path := strings.TrimSuffix(resource.EscapedPath(), "/")
	return "/.well-known/oauth-protected-resource" + path
}

func (i *oauthIssuer) refresh(ctx context.Context) error {
	metaReq, err := http.NewRequestWithContext(ctx, http.MethodGet, i.metadataURL, nil)
	if err != nil {
		return err
	}
	metaResp, err := i.client.Do(metaReq)
	if err != nil {
		return err
	}
	defer metaResp.Body.Close()
	if metaResp.StatusCode != http.StatusOK {
		return fmt.Errorf("authorization-server metadata returned %d", metaResp.StatusCode)
	}
	var metadata struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err = decodeLimitedJSON(metaResp.Body, &metadata); err != nil {
		return err
	}
	if metadata.Issuer != i.issuer {
		return errors.New("authorization-server issuer mismatch")
	}
	jwksURL, err := url.Parse(metadata.JWKSURI)
	issuerURL, _ := url.Parse(i.issuer)
	if err != nil || jwksURL.Scheme != "https" || jwksURL.Host == "" || !strings.EqualFold(jwksURL.Hostname(), issuerURL.Hostname()) || jwksURL.Port() != issuerURL.Port() || jwksURL.User != nil || jwksURL.Fragment != "" {
		return errors.New("JWKS URI must be HTTPS on the configured issuer host")
	}
	jwksReq, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL.String(), nil)
	if err != nil {
		return err
	}
	jwksResp, err := i.client.Do(jwksReq)
	if err != nil {
		return err
	}
	defer jwksResp.Body.Close()
	if jwksResp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS returned %d", jwksResp.StatusCode)
	}
	var keys jose.JSONWebKeySet
	if err = decodeLimitedJSON(jwksResp.Body, &keys); err != nil {
		return err
	}
	if len(keys.Keys) == 0 || len(keys.Keys) > 100 {
		return errors.New("invalid JWKS size")
	}
	for _, key := range keys.Keys {
		if key.KeyID == "" || key.Use != "sig" && key.Use != "" || key.Algorithm != "" && !supportedOAuthAlgorithm(jose.SignatureAlgorithm(key.Algorithm)) {
			return errors.New("unsupported JWKS key")
		}
	}
	i.mu.Lock()
	i.keys = &keys
	i.refreshed = time.Now()
	i.mu.Unlock()
	return nil
}

func supportedOAuthAlgorithm(algorithm jose.SignatureAlgorithm) bool {
	return algorithm == jose.RS256 || algorithm == jose.PS256 || algorithm == jose.ES256
}

func decodeLimitedJSON(body io.Reader, out any) error {
	b, err := io.ReadAll(io.LimitReader(body, 1<<20+1))
	if err != nil {
		return err
	}
	if len(b) > 1<<20 {
		return errors.New("OAuth metadata exceeds 1 MiB")
	}
	return json.Unmarshal(b, out)
}

func (v *oauthVerifier) verify(ctx context.Context, raw string) (verifiedOAuth, error) {
	var out verifiedOAuth
	if len(raw) < 10 || len(raw) > 16*1024 {
		return out, errors.New("invalid access token size")
	}
	token, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{jose.RS256, jose.PS256, jose.ES256})
	if err != nil || len(token.Headers) != 1 {
		return out, errors.New("invalid signed access token")
	}
	header := token.Headers[0]
	typ, _ := header.ExtraHeaders[jose.HeaderKey("typ")].(string)
	if typ != "at+jwt" && typ != "application/at+jwt" || header.KeyID == "" || !supportedOAuthAlgorithm(jose.SignatureAlgorithm(header.Algorithm)) {
		return out, errors.New("invalid access token header")
	}
	var claims struct {
		jwt.Claims
		ClientID string `json:"client_id"`
		Scope    string `json:"scope"`
	}
	// Parse unverified only to choose among the fixed configured issuer records;
	// the exact issuer is checked again only after signature validation.
	if err = token.UnsafeClaimsWithoutVerification(&claims); err != nil {
		return out, errors.New("invalid access token claims")
	}
	entry := v.issuers[claims.Issuer]
	if entry == nil {
		return out, errors.New("untrusted OAuth issuer")
	}
	entry.mu.Lock()
	keys := entry.keys
	keysStale := keys != nil && time.Since(entry.refreshed) >= 5*time.Minute
	if keysStale && time.Since(entry.lastRefreshAttempt) < 10*time.Second {
		entry.mu.Unlock()
		return out, errors.New("stale OAuth signing-key refresh is in progress or retry-limited")
	}
	refreshStale := keysStale
	if refreshStale {
		entry.lastRefreshAttempt = time.Now()
	}
	entry.mu.Unlock()
	if keys == nil {
		return out, errors.New("OAuth signing keys unavailable")
	}
	if refreshStale {
		if err = entry.refresh(ctx); err != nil {
			return out, errors.New("stale OAuth signing keys could not be refreshed")
		}
		entry.mu.Lock()
		keys = entry.keys
		entry.mu.Unlock()
	}
	matched := keys.Key(header.KeyID)
	if len(matched) == 0 {
		entry.mu.Lock()
		refreshAllowed := time.Since(entry.lastUnknownRefresh) >= 10*time.Second
		if refreshAllowed {
			entry.lastUnknownRefresh = time.Now()
		}
		entry.mu.Unlock()
		if !refreshAllowed {
			return out, errors.New("OAuth signing-key refresh rate limited")
		}
		if err = entry.refresh(ctx); err != nil {
			return out, errors.New("OAuth signing-key refresh failed")
		}
		entry.mu.Lock()
		keys = entry.keys
		entry.mu.Unlock()
		matched = keys.Key(header.KeyID)
	}
	if len(matched) != 1 || matched[0].Algorithm != "" && matched[0].Algorithm != header.Algorithm || matched[0].Use != "" && matched[0].Use != "sig" {
		return out, errors.New("unknown or ambiguous OAuth signing key")
	}
	var key any
	switch jose.SignatureAlgorithm(header.Algorithm) {
	case jose.RS256, jose.PS256:
		if rsaKey, ok := matched[0].Key.(*rsa.PublicKey); ok && rsaKey.N.BitLen() >= 2048 {
			key = rsaKey
		}
	case jose.ES256:
		if ecdsaKey, ok := matched[0].Key.(*ecdsa.PublicKey); ok && ecdsaKey.Curve == elliptic.P256() {
			key = ecdsaKey
		}
	}
	if key == nil {
		return out, errors.New("unsupported OAuth signing key type")
	}
	if err = token.Claims(key, &claims); err != nil {
		return out, errors.New("access token signature invalid")
	}
	now := time.Now().UTC()
	if claims.Issuer != entry.issuer || claims.Subject == "" || len(claims.Subject) > 512 || claims.ClientID == "" || len(claims.ClientID) > 255 || claims.ID == "" || len(claims.ID) > 512 || claims.IssuedAt == nil || claims.Expiry == nil || !claims.Expiry.Time().After(now) || claims.IssuedAt.Time().After(now.Add(30*time.Second)) || !claims.Audience.Contains(v.resource) {
		return out, errors.New("access token claims invalid")
	}
	if claims.NotBefore != nil && claims.NotBefore.Time().After(now.Add(30*time.Second)) {
		return out, errors.New("access token is not active")
	}
	scopes := strings.Fields(claims.Scope)
	if len(scopes) == 0 || len(scopes) > 100 {
		return out, errors.New("access token has no usable scopes")
	}
	seen := map[string]bool{}
	for _, scope := range scopes {
		if seen[scope] || len(scope) > 128 {
			return out, errors.New("invalid access-token scope list")
		}
		seen[scope] = true
	}
	return verifiedOAuth{Issuer: claims.Issuer, Subject: claims.Subject, ClientID: claims.ClientID, Scopes: scopes, Expires: claims.Expiry.Time().UTC()}, nil
}

func oauthClaims(r *http.Request) (verifiedOAuth, bool) {
	claims, ok := r.Context().Value(oauthResourceContextKey).(verifiedOAuth)
	return claims, ok
}

func (s *Service) insertOAuthGrant(ctx context.Context, tx pgx.Tx, principal, agent string, claims verifiedOAuth, scopes []string, expires time.Time) (string, error) {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM credentials WHERE kind='oauth_grant' AND oauth_issuer=$1 AND oauth_subject=$2 AND oauth_client_id=$3)`, claims.Issuer, claims.Subject, claims.ClientID).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return "", fail(409, "oauth_binding_exists")
	}
	id := NewID()
	_, err := tx.Exec(ctx, `INSERT INTO credentials(id,agent_id,kind,digest,oauth_issuer,oauth_subject,oauth_client_id,scopes,audience,expires_at)
		VALUES($1,$2,'oauth_grant',NULL,$3,$4,$5,$6,$7,$8)`, id, agent, claims.Issuer, claims.Subject, claims.ClientID, scopes, claims.Resource, expires)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "credentials_oauth_identity" {
			return "", fail(409, "oauth_binding_exists")
		}
		return "", err
	}
	if err = credentialEvent(ctx, tx, principal, id, 1, "credential.oauth_bound", s.environment); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Service) oauthBindingResult(ctx context.Context, tx pgx.Tx, id, principal, agent string) (map[string]any, error) {
	var scopes []string
	var generation int64
	var expires time.Time
	var issuer, clientID string
	err := tx.QueryRow(ctx, `SELECT c.scopes,c.generation,c.expires_at,c.oauth_issuer,c.oauth_client_id FROM credentials c JOIN agents a ON a.id=c.agent_id WHERE c.id=$1 AND c.agent_id=$2 AND c.kind='oauth_grant' AND a.principal_id=$3`, id, agent, principal).Scan(&scopes, &generation, &expires, &issuer, &clientID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": agent, "principal_id": principal, "credential_id": id, "kind": "oauth_grant", "issuer": issuer, "client_id": clientID, "scopes": scopes, "generation": generation, "etag": etag(generation), "expires_at": expires}, nil
}

func (s *Service) ownerOAuthBindings(r *http.Request, tx pgx.Tx, principal, agent string) (int, any, error) {
	if !validID(agent) {
		return 0, nil, fail(404, "not_found")
	}
	if r.Method == http.MethodGet {
		if _, err := ownerAgent(r.Context(), tx, principal, agent); err != nil {
			return 0, nil, err
		}
		rows, err := tx.Query(r.Context(), `SELECT id,kind,oauth_issuer,oauth_client_id,scopes,generation,expires_at,revoked_at FROM credentials WHERE agent_id=$1 AND kind='oauth_grant' ORDER BY created_at`, agent)
		if err != nil {
			return 0, nil, err
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var id, kind, issuer, clientID string
			var scopes []string
			var generation int64
			var expires time.Time
			var revoked *time.Time
			if err = rows.Scan(&id, &kind, &issuer, &clientID, &scopes, &generation, &expires, &revoked); err != nil {
				return 0, nil, err
			}
			items = append(items, map[string]any{"credential_id": id, "kind": kind, "issuer": issuer, "client_id": clientID, "scopes": scopes, "generation": generation, "etag": etag(generation), "expires_at": expires, "revoked_at": revoked})
		}
		return 200, map[string]any{"items": items}, rows.Err()
	}
	if r.Method != http.MethodPost {
		return 0, nil, fail(405, "method_not_allowed")
	}
	claims, ok := oauthClaims(r)
	if !ok || s.oauth == nil || claims.Resource != s.oauth.resource {
		return 0, nil, fail(401, "network_access_token_required")
	}
	_, err := ownerAgent(r.Context(), tx, principal, agent)
	if err != nil {
		return 0, nil, err
	}
	if _, err = tx.Exec(r.Context(), "SELECT set_config('undergrid.agent_id','',true)"); err != nil {
		return 0, nil, err
	}
	// Revalidate the current token's scope proof before idempotency replay. A
	// cached owner response must not let a narrower access token replay a
	// binding command whose requested scopes it no longer carries.
	request, err := readIdempotencyRequest(r, false)
	if err != nil {
		return 0, nil, err
	}
	restoreIdempotencyBody(r, request)
	var in struct {
		Scopes    []string  `json:"scopes"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if err := s.validateAgentScopes(r.Context(), tx, principal, in.Scopes); err != nil {
		return 0, nil, err
	}
	for _, scope := range in.Scopes {
		if !slices.Contains(claims.Scopes, scope) {
			return 0, nil, fail(403, "oauth_scope_not_granted")
		}
	}
	restoreIdempotencyBody(r, request)
	status, ref, err := idempotentOwner(r, tx, principal, func() (int, any, error) {
		now := time.Now().UTC()
		maxExpiry := now.Add(30 * 24 * time.Hour)
		if claims.Expires.Before(maxExpiry) {
			maxExpiry = claims.Expires
		}
		expires := in.ExpiresAt
		if expires.IsZero() {
			expires = maxExpiry
		}
		if !expires.After(now) || expires.After(maxExpiry) {
			return 0, nil, fail(422, "invalid_oauth_expiry")
		}
		id, err := s.insertOAuthGrant(r.Context(), tx, principal, agent, claims, in.Scopes, expires)
		if err != nil {
			return 0, nil, err
		}
		out, err := s.oauthBindingResult(r.Context(), tx, id, principal, agent)
		return 201, out, err
	})
	if err != nil {
		return 0, nil, err
	}
	return status, ref, nil
}

func (s *Service) ownerOAuthRevoke(r *http.Request, tx pgx.Tx, principal, id string) (int, any, error) {
	if !validID(id) {
		return 0, nil, fail(404, "not_found")
	}
	if r.Method != http.MethodPost {
		return 0, nil, fail(405, "method_not_allowed")
	}
	var agent string
	var generation int64
	err := tx.QueryRow(r.Context(), `SELECT c.agent_id FROM credentials c JOIN agents a ON a.id=c.agent_id WHERE c.id=$1 AND c.kind='oauth_grant' AND a.principal_id=$2`, id, principal).Scan(&agent)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	if _, err = ownerAgent(r.Context(), tx, principal, agent); err != nil {
		return 0, nil, err
	}
	err = tx.QueryRow(r.Context(), `SELECT generation FROM credentials WHERE id=$1 AND agent_id=$2 AND kind='oauth_grant' FOR UPDATE`, id, agent).Scan(&generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	if _, err = tx.Exec(r.Context(), "SELECT set_config('undergrid.agent_id','',true)"); err != nil {
		return 0, nil, err
	}
	return idempotentOwner(r, tx, principal, func() (int, any, error) {
		if err := requireMatch(r, generation); err != nil {
			return 0, nil, err
		}
		var in struct{}
		if err := decode(r, &in); err != nil {
			return 0, nil, err
		}
		tag, err := tx.Exec(r.Context(), "UPDATE credentials SET revoked_at=now() WHERE id=$1 AND kind='oauth_grant' AND revoked_at IS NULL", id)
		if err != nil {
			return 0, nil, err
		}
		if tag.RowsAffected() > 0 {
			if err = credentialEvent(r.Context(), tx, principal, id, 2, "credential.revoked", s.environment); err != nil {
				return 0, nil, err
			}
		}
		return 200, map[string]any{"credential_id": id, "revoked": true, "generation": generation, "etag": etag(generation)}, nil
	})
}

func (s *Service) ownerOAuthRotate(r *http.Request, tx pgx.Tx, principal, id string) (int, any, error) {
	if !validID(id) {
		return 0, nil, fail(404, "not_found")
	}
	if r.Method != http.MethodPost {
		return 0, nil, fail(405, "method_not_allowed")
	}
	claims, ok := oauthClaims(r)
	if !ok || s.oauth == nil || claims.Resource != s.oauth.resource {
		return 0, nil, fail(401, "network_access_token_required")
	}
	var agent string
	var generation int64
	var oldScopes []string
	var revoked *time.Time
	var expires time.Time
	err := tx.QueryRow(r.Context(), `SELECT c.agent_id FROM credentials c JOIN agents a ON a.id=c.agent_id WHERE c.id=$1 AND c.kind='oauth_grant' AND a.principal_id=$2`, id, principal).Scan(&agent)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	if _, err = ownerAgent(r.Context(), tx, principal, agent); err != nil {
		return 0, nil, err
	}
	err = tx.QueryRow(r.Context(), `SELECT generation,scopes,revoked_at,expires_at FROM credentials WHERE id=$1 AND agent_id=$2 AND kind='oauth_grant' FOR UPDATE`, id, agent).Scan(&generation, &oldScopes, &revoked, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	if _, err = tx.Exec(r.Context(), "SELECT set_config('undergrid.agent_id','',true)"); err != nil {
		return 0, nil, err
	}
	request, err := readIdempotencyRequest(r, false)
	if err != nil {
		return 0, nil, err
	}
	restoreIdempotencyBody(r, request)
	var in struct {
		Scopes    []string  `json:"scopes"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if err := s.validateAgentScopes(r.Context(), tx, principal, in.Scopes); err != nil {
		return 0, nil, err
	}
	for _, scope := range in.Scopes {
		if !slices.Contains(oldScopes, scope) || !slices.Contains(claims.Scopes, scope) {
			return 0, nil, fail(403, "oauth_scope_escalation")
		}
	}
	restoreIdempotencyBody(r, request)
	return idempotentOwner(r, tx, principal, func() (int, any, error) {
		if err := requireMatch(r, generation); err != nil {
			return 0, nil, err
		}
		if revoked != nil || !expires.After(time.Now()) {
			return 0, nil, fail(409, "credential_inactive")
		}
		now := time.Now().UTC()
		maxExpiry := now.Add(30 * 24 * time.Hour)
		if claims.Expires.Before(maxExpiry) {
			maxExpiry = claims.Expires
		}
		nextExpiry := in.ExpiresAt
		if nextExpiry.IsZero() {
			nextExpiry = maxExpiry
		}
		if !nextExpiry.After(now) || nextExpiry.After(maxExpiry) {
			return 0, nil, fail(422, "invalid_oauth_expiry")
		}
		newID, err := s.insertOAuthGrant(r.Context(), tx, principal, agent, claims, in.Scopes, nextExpiry)
		if err != nil {
			return 0, nil, err
		}
		tag, err := tx.Exec(r.Context(), "UPDATE credentials SET revoked_at=now() WHERE id=$1 AND revoked_at IS NULL", id)
		if err != nil {
			return 0, nil, err
		}
		if tag.RowsAffected() != 1 {
			return 0, nil, fail(409, "credential_inactive")
		}
		if err = credentialEvent(r.Context(), tx, principal, id, 2, "credential.revoked", s.environment); err != nil {
			return 0, nil, err
		}
		out, err := s.oauthBindingResult(r.Context(), tx, newID, principal, agent)
		if err != nil {
			return 0, nil, err
		}
		out["rotated_from"] = id
		return 201, out, nil
	})
}
