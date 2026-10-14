package network

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/jackc/pgx/v5/pgconn"
)

type testOAuthAS struct {
	server    *httptest.Server
	issuer    string
	resource  string
	key       crypto.Signer
	keyID     string
	algorithm jose.SignatureAlgorithm
	jwksURI   string
}

func newTestOAuthAS(t *testing.T) *testOAuthAS {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	as := &testOAuthAS{resource: "https://undergrid.fixture.test/network", key: key, keyID: "test-key-1", algorithm: jose.RS256}
	as.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			jwksURI := as.jwksURI
			if jwksURI == "" {
				jwksURI = as.server.URL + "/jwks"
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": as.issuer, "jwks_uri": jwksURI})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: as.key.Public(), KeyID: as.keyID, Algorithm: string(as.algorithm), Use: "sig"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	as.issuer = as.server.URL
	t.Cleanup(as.server.Close)
	return as
}

func (as *testOAuthAS) configure(t *testing.T, s *Service) {
	t.Helper()
	client := as.server.Client()
	client.Timeout = 3 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if err := s.ConfigureOAuthResource(OAuthResourceConfig{Resource: as.resource, Issuers: []string{as.issuer}, HTTPClient: client}); err != nil {
		t.Fatal(err)
	}
}

func (as *testOAuthAS) token(t *testing.T, subject, clientID, scope string, expires time.Time, typ, audience string) string {
	return as.tokenIssuer(t, as.issuer, subject, clientID, scope, expires, typ, audience)
}

func (as *testOAuthAS) tokenIssuer(t *testing.T, issuer, subject, clientID, scope string, expires time.Time, typ, audience string) string {
	t.Helper()
	claims := map[string]any{"iss": issuer, "sub": subject, "aud": audience, "exp": expires.Unix(), "iat": time.Now().Add(-time.Second).Unix(), "jti": NewID(), "client_id": clientID, "scope": scope}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	options := (&jose.SignerOptions{}).WithType(jose.ContentType(typ)).WithHeader("kid", as.keyID)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: as.algorithm, Key: as.key}, options)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	compact, err := signed.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return compact
}

func oauthCall(t *testing.T, s *Service, owner, token, method, path, key, match string, body any) (int, map[string]any, http.Header) {
	t.Helper()
	var raw []byte
	if body != nil {
		raw = canonical(body)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	r.Header.Set("X-Test-Owner", owner)
	r.Header.Set("Authorization", "Bearer "+token)
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	if match != "" {
		r.Header.Set("If-Match", match)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	var out map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode HTTP %d: %v: %s", w.Code, err, w.Body.String())
		}
	}
	return w.Code, out, w.Header()
}

// OAuth grants must remain credential rows: artifact upload stores the
// credential ID as an FK, and the common actor lock/revoke paths operate on it.
func TestOAuthCredentialUsesNormalizedCredentialLifecycle(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	_, me := a.call("GET", "/v1/me", "", nil)
	var oauthID string
	err := db.QueryRow(context.Background(), `INSERT INTO undergrid.credentials
		(id,agent_id,kind,digest,oauth_issuer,oauth_subject,oauth_client_id,scopes,audience,expires_at)
		VALUES($1,$2,'oauth_grant',NULL,'https://as.fixture.test',$3,$4,
		ARRAY['intents:read'],'https://network.fixture.test',now()+interval '1 hour') RETURNING id`,
		NewID(), me["agent_id"], NewID(), NewID()).Scan(&oauthID)
	if err != nil {
		t.Fatalf("normalized OAuth credential insert: %v", err)
	}
	var kind string
	var digestIsNull bool
	if err = db.QueryRow(context.Background(), `SELECT kind,digest IS NULL FROM undergrid.credentials WHERE id=$1`, oauthID).Scan(&kind, &digestIsNull); err != nil {
		t.Fatal(err)
	}
	if kind != "oauth_grant" || !digestIsNull {
		t.Fatalf("OAuth credential variant = %q, digest null=%v", kind, digestIsNull)
	}
	digest := sha256.Sum256([]byte(a.token))
	var agent, credential string
	if err = db.QueryRow(context.Background(), `SELECT agent_id,credential_id FROM undergrid.identify_token($1)`, digest[:]).Scan(&agent, &credential); err != nil {
		t.Fatalf("existing API token lookup changed: %v", err)
	}
	if agent != me["agent_id"] || credential == oauthID {
		t.Fatalf("API token lookup resolved unexpected actor/credential: %s %s", agent, credential)
	}
}

func TestOAuthCredentialIdentityTupleRejectsNullMembersAsAppRole(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	_, me := a.call(http.MethodGet, "/v1/me", "", nil)
	principal := me["principal_id"].(string)
	agent := me["agent_id"].(string)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), "SET LOCAL ROLE undergrid_app; SET LOCAL search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), `SELECT set_config('undergrid.owner_issuer',$1,true),
		set_config('undergrid.owner_subject',$2,true),set_config('undergrid.principal_id',$3,true)`, "https://fixture.test", a.owner, principal); err != nil {
		t.Fatal(err)
	}
	var visibleAgent, visibleCredentials int
	if err = tx.QueryRow(context.Background(), "SELECT count(*) FROM agents WHERE id=$1", agent).Scan(&visibleAgent); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(context.Background(), "SELECT count(*) FROM credentials WHERE agent_id=$1", agent).Scan(&visibleCredentials); err != nil {
		t.Fatal(err)
	}
	if visibleAgent != 1 || visibleCredentials == 0 {
		t.Fatalf("app-role fixture lacks owner/agent access: agent=%d credentials=%d", visibleAgent, visibleCredentials)
	}
	for _, missing := range []string{"oauth_issuer", "oauth_subject", "oauth_client_id"} {
		issuer, subject, clientID := "https://as.fixture.test", "subject", "client"
		switch missing {
		case "oauth_issuer":
			issuer = ""
		case "oauth_subject":
			subject = ""
		case "oauth_client_id":
			clientID = ""
		}
		if _, err = tx.Exec(context.Background(), "SAVEPOINT oauth_identity_tuple_check"); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(context.Background(), `INSERT INTO credentials
			(id,agent_id,kind,digest,oauth_issuer,oauth_subject,oauth_client_id,scopes,audience,expires_at)
			VALUES($1,$2,'oauth_grant',NULL,$3,$4,$5,ARRAY['intents:read'],'https://network.fixture.test',now()+interval '1 hour')`,
			NewID(), agent, nullableIdentity(issuer), nullableIdentity(subject), nullableIdentity(clientID))
		if err == nil {
			t.Fatalf("app role inserted OAuth grant with NULL %s", missing)
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != "credentials_kind_identity_check" {
			t.Fatalf("NULL %s rejected for unrelated reason: %v", missing, err)
		}
		if _, err = tx.Exec(context.Background(), "ROLLBACK TO SAVEPOINT oauth_identity_tuple_check"); err != nil {
			t.Fatal(err)
		}
	}
}

func nullableIdentity(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func TestOAuthResourceVerifierRequiresRFC9068AccessToken(t *testing.T) {
	s, _ := fixture(t)
	as := newTestOAuthAS(t)
	as.configure(t, s)
	valid := as.token(t, "subject-a", "network-client", "intents:read", time.Now().Add(time.Hour), "at+jwt", as.resource)
	code, out, headers := oauthCall(t, s, "unused", valid, http.MethodGet, "/v1/me", "", "", nil)
	if code != 401 || out["error"] == nil || !strings.Contains(headers.Get("WWW-Authenticate"), "/.well-known/oauth-protected-resource") {
		t.Fatalf("unbound valid token did not fail closed with resource challenge: %d %v %v", code, out, headers)
	}
	tampered := []byte(valid)
	if tampered[len(tampered)-2] == 'A' {
		tampered[len(tampered)-2] = 'B'
	} else {
		tampered[len(tampered)-2] = 'A'
	}
	for name, token := range map[string]string{
		"ID token type":    as.token(t, "subject-a", "network-client", "intents:read", time.Now().Add(time.Hour), "JWT", as.resource),
		"wrong audience":   as.token(t, "subject-a", "network-client", "intents:read", time.Now().Add(time.Hour), "at+jwt", "https://other.fixture.test"),
		"untrusted issuer": as.tokenIssuer(t, "https://untrusted.fixture.test", "subject-a", "network-client", "intents:read", time.Now().Add(time.Hour), "at+jwt", as.resource),
		"expired":          as.token(t, "subject-a", "network-client", "intents:read", time.Now().Add(-time.Minute), "at+jwt", as.resource),
		"bad signature":    string(tampered),
	} {
		code, denial, challenge := oauthCall(t, s, "unused", token, http.MethodGet, "/v1/me", "", "", nil)
		assertOAuthErrorContract(t, denial, challenge, "invalid_credential")
		if code != 401 {
			t.Fatalf("%s accepted: %d", name, code)
		}
	}
	request := httptest.NewRequest(http.MethodGet, s.oauth.metadataPath, nil)
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	var metadata map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || metadata["resource"] != as.resource || metadata["authorization_servers"].([]any)[0] != as.issuer || metadata["bearer_methods_supported"].([]any)[0] != "header" {
		t.Fatalf("protected resource metadata: %d %v", response.Code, metadata)
	}

	supported := map[string]bool{}
	for _, scope := range metadata["scopes_supported"].([]any) {
		supported[scope.(string)] = true
	}
	if !supported["reports:read"] || !supported["reports:write"] {
		t.Fatalf("metadata omits live report scopes: %v", metadata)
	}

	oldKey, oldKeyID := as.key, as.keyID
	rotatedKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	as.key = rotatedKey
	as.keyID = "test-key-2"
	newKeyToken := as.token(t, "subject-a", "network-client", "intents:read", time.Now().Add(time.Hour), "at+jwt", as.resource)
	verified, err := s.oauth.verify(context.Background(), newKeyToken)
	if err != nil || verified.Subject != "subject-a" {
		t.Fatalf("JWKS key rotation refresh: claims=%+v err=%v", verified, err)
	}
	as.key, as.keyID = oldKey, oldKeyID
	retiredKeyToken := as.token(t, "subject-a", "network-client", "intents:read", time.Now().Add(time.Hour), "at+jwt", as.resource)
	as.key, as.keyID = rotatedKey, "test-key-2"
	if _, err = s.oauth.verify(context.Background(), retiredKeyToken); err == nil {
		t.Fatal("retired signing key remained trusted after JWKS refresh")
	}
}

func TestOAuthDiscoveryRejectsUntrustedJWKSHost(t *testing.T) {
	as := newTestOAuthAS(t)
	as.jwksURI = "https://attacker.fixture.test/jwks"
	client := as.server.Client()
	client.Timeout = time.Second
	service := New("postgres://unused", "test", nil)
	if err := service.ConfigureOAuthResource(OAuthResourceConfig{Resource: as.resource, Issuers: []string{as.issuer}, HTTPClient: client}); err == nil {
		t.Fatal("trusted issuer metadata redirected JWKS trust to another host")
	}
	if _, err := newOAuthVerifier(OAuthResourceConfig{Resource: "http://undergrid.fixture.test", Issuers: []string{as.issuer}, HTTPClient: client}); err == nil {
		t.Fatal("accepted non-HTTPS resource")
	}
}

func TestOAuthVerifierAcceptsES256FromConfiguredIssuer(t *testing.T) {
	s, _ := fixture(t)
	as := newTestOAuthAS(t)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	as.key = key
	as.algorithm = jose.ES256
	as.configure(t, s)
	token := as.token(t, "subject-es256", "network-client", "intents:read", time.Now().Add(time.Hour), "at+jwt", as.resource)
	claims, err := s.oauth.verify(context.Background(), token)
	if err != nil || claims.Subject != "subject-es256" {
		t.Fatalf("ES256 access token: claims=%+v err=%v", claims, err)
	}
}

func TestOAuthBindingScopesReplayOwnershipAndRevocation(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	as := newTestOAuthAS(t)
	as.configure(t, s)
	_, me := a.call(http.MethodGet, "/v1/me", "", nil)
	agent := me["agent_id"].(string)
	scopes := "intents:read intents:write inbox:read artifacts:grant"
	token := as.token(t, "oauth-subject", "oauth-client", scopes, time.Now().Add(time.Hour), "at+jwt", as.resource)
	path := "/v1/owner/agents/" + agent + "/oauth-bindings"
	body := map[string]any{"scopes": []string{"intents:read", "intents:write", "inbox:read"}}
	code, _, _ := oauthCall(t, s, a.owner, "", http.MethodPost, path, "missing-oauth-proof", "", body)
	if code != 401 {
		t.Fatalf("owner session alone created OAuth grant: %d", code)
	}
	code, denied, _ := oauthCall(t, s, a.owner, token, http.MethodPost, path, "unapproved-rights-scope", "", map[string]any{"scopes": []string{"artifacts:grant"}})
	if code != 403 || denied["error"].(map[string]any)["code"] != "owner_policy_required" {
		t.Fatalf("protected grant scope bypassed owner policy: %d %v", code, denied)
	}
	code, bound, _ := oauthCall(t, s, a.owner, token, http.MethodPost, path, "bind-oauth", "", body)
	if code != 201 || bound["kind"] != "oauth_grant" || bound["token"] != nil {
		t.Fatalf("bind: %d %v", code, bound)
	}
	code, replay, _ := oauthCall(t, s, a.owner, token, http.MethodPost, path, "bind-oauth", "", body)
	if code != 201 || replay["credential_id"] != bound["credential_id"] {
		t.Fatalf("idempotent ref replay: %d %v", code, replay)
	}
	narrowBindToken := as.token(t, "oauth-subject", "oauth-client", "intents:read", time.Now().Add(time.Hour), "at+jwt", as.resource)
	code, narrowReplay, _ := oauthCall(t, s, a.owner, narrowBindToken, http.MethodPost, path, "bind-oauth", "", body)
	if code != 403 || narrowReplay["error"].(map[string]any)["code"] != "oauth_scope_not_granted" {
		t.Fatalf("idempotent replay bypassed current token scope proof: %d %v", code, narrowReplay)
	}
	var count int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.credentials WHERE kind='oauth_grant' AND oauth_issuer=$1 AND oauth_subject='oauth-subject' AND oauth_client_id='oauth-client'`, as.issuer).Scan(&count); err != nil || count != 1 {
		t.Fatalf("binding duplicate count %d: %v", count, err)
	}
	var storedResponse []byte
	if err := db.QueryRow(context.Background(), `SELECT response FROM undergrid.idempotency WHERE owner_principal_id=$1 AND key='bind-oauth'`, me["principal_id"]).Scan(&storedResponse); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(storedResponse, []byte(token)) {
		t.Fatal("owner idempotency stored/replayed the OAuth bearer")
	}
	credentialID := bound["credential_id"].(string)
	var secretStored bool
	if err := db.QueryRow(context.Background(), `SELECT digest IS NOT NULL FROM undergrid.credentials WHERE id=$1`, credentialID).Scan(&secretStored); err != nil || secretStored {
		t.Fatalf("OAuth bearer digest stored=%v: %v", secretStored, err)
	}
	var facts []byte
	if err := db.QueryRow(context.Background(), `SELECT facts FROM undergrid.events WHERE aggregate_kind='credential' AND aggregate_id=$1 AND type='credential.oauth_bound'`, credentialID).Scan(&facts); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(facts, []byte(token)) || string(facts) != "{}" {
		t.Fatalf("credential event leaked token/claims: %s", facts)
	}

	code, listed, _ := oauthCall(t, s, a.owner, "", http.MethodGet, path, "", "", nil)
	if code != 200 {
		t.Fatalf("list binding: %d %v", code, listed)
	}
	items := listed["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["credential_id"] != credentialID || items[0].(map[string]any)["issuer"] != as.issuer || items[0].(map[string]any)["client_id"] != "oauth-client" {
		t.Fatalf("owner listing: %v", listed)
	}
	_, ownerAgentList := a.call(http.MethodGet, "/v1/agents", "", nil)
	listedCredentials := ownerAgentList["items"].([]any)[0].(map[string]any)["credentials"].([]any)
	foundOAuthKind := false
	for _, item := range listedCredentials {
		if item.(map[string]any)["id"] == credentialID && item.(map[string]any)["kind"] == "oauth_grant" {
			foundOAuthKind = true
		}
	}
	if !foundOAuthKind {
		t.Fatalf("owner agent listing omitted truthful OAuth credential kind: %v", listedCredentials)
	}
	outsider := client(t, s)
	code, _, _ = oauthCall(t, s, outsider.owner, "", http.MethodGet, path, "", "", nil)
	if code != 404 {
		t.Fatalf("other owner read binding: %d", code)
	}
	_, outsiderMe := outsider.call(http.MethodGet, "/v1/me", "", nil)
	code, rebound, _ := oauthCall(t, s, outsider.owner, token, http.MethodPost, "/v1/owner/agents/"+outsiderMe["agent_id"].(string)+"/oauth-bindings", "rebind-oauth-to-other-owner", "", map[string]any{"scopes": []string{"intents:read"}})
	if code != 409 || rebound["error"].(map[string]any)["code"] != "oauth_binding_exists" {
		t.Fatalf("OAuth identity was rebound to another owner: %d %v", code, rebound)
	}
	rotated := as.token(t, "oauth-subject", "oauth-client-rotated", scopes, time.Now().Add(time.Hour), "at+jwt", as.resource)
	code, staleRotation, _ := oauthCall(t, s, a.owner, rotated, http.MethodPost, "/v1/owner/oauth-bindings/"+credentialID+"/rotate", "stale-rotate-oauth", `"0"`, body)
	if code != 409 || staleRotation["error"].(map[string]any)["code"] != "version_conflict" {
		t.Fatalf("stale OAuth ETag accepted: %d %v", code, staleRotation)
	}
	code, missingETag, _ := oauthCall(t, s, a.owner, "", http.MethodPost, "/v1/owner/oauth-bindings/"+credentialID+"/revoke", "missing-etag-revoke", "", map[string]any{})
	if code != 428 || missingETag["error"].(map[string]any)["code"] != "if_match_required" {
		t.Fatalf("OAuth revoke omitted ETag requirement: %d %v", code, missingETag)
	}
	code, rotatedRef, _ := oauthCall(t, s, a.owner, rotated, http.MethodPost, "/v1/owner/oauth-bindings/"+credentialID+"/rotate", "rotate-oauth", `"1"`, body)
	if code != 201 || rotatedRef["rotated_from"] != credentialID {
		t.Fatalf("rotate binding: %d %v", code, rotatedRef)
	}
	code, rotationReplay, _ := oauthCall(t, s, a.owner, rotated, http.MethodPost, "/v1/owner/oauth-bindings/"+credentialID+"/rotate", "rotate-oauth", `"1"`, body)
	if code != 201 || rotationReplay["credential_id"] != rotatedRef["credential_id"] {
		t.Fatalf("rotation replay created a second binding: %d %v", code, rotationReplay)
	}
	newCredentialID := rotatedRef["credential_id"].(string)
	code, _, _ = oauthCall(t, s, a.owner, token, http.MethodGet, "/v1/me", "", "", nil)
	if code != 401 {
		t.Fatalf("rotated old OAuth grant remained usable: %d", code)
	}
	_, agentList := a.call(http.MethodGet, "/v1/agents", "", nil)
	ownerAgents := agentList["items"].([]any)
	agentVersion := ownerAgents[0].(map[string]any)["version"].(float64)
	code, _, _ = oauthCall(t, s, a.owner, "", http.MethodPost, "/v1/owner/agents/"+agent+"/recover-credential", "api-recovery", etag(int64(agentVersion)), map[string]any{"scopes": []string{"intents:read"}})
	if code != 201 {
		t.Fatalf("API credential recovery: %d", code)
	}

	code, me, _ = oauthCall(t, s, a.owner, rotated, http.MethodGet, "/v1/me", "", "", nil)
	if code != 200 {
		t.Fatalf("bound OAuth token did not resolve actor: %d %v", code, me)
	}
	narrow := as.token(t, "oauth-subject", "oauth-client-rotated", "intents:read", time.Now().Add(time.Hour), "at+jwt", as.resource)
	code, me, _ = oauthCall(t, s, a.owner, narrow, http.MethodGet, "/v1/me", "", "", nil)
	if code != 200 || len(me["scopes"].([]any)) != 1 || me["scopes"].([]any)[0] != "intents:read" {
		t.Fatalf("effective scopes not intersected: %d %v", code, me)
	}
	code, out, challenge := oauthCall(t, s, a.owner, narrow, http.MethodPost, "/v1/intents", "narrow-intent", "", intentFixture())
	if code != 403 || out["error"].(map[string]any)["code"] != "insufficient_scope" || !strings.Contains(challenge.Get("WWW-Authenticate"), `error="insufficient_scope"`) {
		t.Fatalf("token scope did not cap local grant: %d %v", code, out)
	}
	assertOAuthErrorContract(t, out, challenge, "insufficient_scope")
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.intents WHERE agent_id=$1`, agent).Scan(&count); err != nil || count != 0 {
		t.Fatalf("narrow token wrote intent: rows=%d err=%v", count, err)
	}

	code, revokedRef, _ := oauthCall(t, s, a.owner, "", http.MethodPost, "/v1/owner/oauth-bindings/"+newCredentialID+"/revoke", "revoke-oauth", `"1"`, map[string]any{})
	if code != 200 {
		t.Fatalf("owner revoke: %d", code)
	}
	code, revokeReplay, _ := oauthCall(t, s, a.owner, "", http.MethodPost, "/v1/owner/oauth-bindings/"+newCredentialID+"/revoke", "revoke-oauth", `"1"`, map[string]any{})
	if code != 200 || revokeReplay["credential_id"] != revokedRef["credential_id"] {
		t.Fatalf("revoke replay: %d %v", code, revokeReplay)
	}
	code, _, _ = oauthCall(t, s, a.owner, rotated, http.MethodGet, "/v1/me", "", "", nil)
	if code != 401 {
		t.Fatalf("revoked OAuth grant remained usable: %d", code)
	}
	// Rotation revokes the old immutable issuer/sub/client tuple. A later owner
	// cannot resurrect that same external identity against another agent, even
	// after the old credential is revoked; otherwise an old still-valid JWT
	// could become usable again through the replacement binding.
	code, rebound, _ = oauthCall(t, s, outsider.owner, token, http.MethodPost, "/v1/owner/agents/"+outsiderMe["agent_id"].(string)+"/oauth-bindings", "rebind-revoked-oauth", "", map[string]any{"scopes": []string{"intents:read"}})
	if code != 409 || rebound["error"].(map[string]any)["code"] != "oauth_binding_exists" {
		t.Fatalf("revoked OAuth identity was rebound: %d %v", code, rebound)
	}
	code, _, _ = oauthCall(t, s, a.owner, token, http.MethodGet, "/v1/me", "", "", nil)
	if code != 401 {
		t.Fatalf("old OAuth JWT became usable after attempted revoked-tuple rebind: %d", code)
	}
	shortToken := as.token(t, "oauth-subject", "oauth-client-short", "intents:read", time.Now().Add(time.Hour), "at+jwt", as.resource)
	shortExpiry := time.Now().Add(2 * time.Second).UTC()
	code, shortRef, _ := oauthCall(t, s, a.owner, shortToken, http.MethodPost, path, "bind-short-oauth", "", map[string]any{"scopes": []string{"intents:read"}, "expires_at": shortExpiry})
	if code != 201 {
		t.Fatalf("short binding: %d %v", code, shortRef)
	}
	time.Sleep(2100 * time.Millisecond)
	code, _, _ = oauthCall(t, s, a.owner, shortToken, http.MethodGet, "/v1/me", "", "", nil)
	if code != 401 {
		t.Fatalf("expired local OAuth binding remained usable: %d", code)
	}
}

func TestOAuthCredentialWritesArtifactThroughUploadCapability(t *testing.T) {
	x := newArtifactScenario(t)
	as := newTestOAuthAS(t)
	as.configure(t, x.s)
	actor := x.actors[2]
	_, me := actor.call(http.MethodGet, "/v1/me", "", nil)
	requestedScopes := "artifacts:submit rights:accept"
	token := as.token(t, "artifact-oauth-subject", "artifact-oauth-client", requestedScopes, time.Now().Add(time.Hour), "at+jwt", as.resource)
	bindPath := "/v1/owner/agents/" + me["agent_id"].(string) + "/oauth-bindings"
	code, binding, _ := oauthCall(t, x.s, actor.owner, token, http.MethodPost, bindPath, "bind-artifact-oauth", "", map[string]any{"scopes": []string{"artifacts:submit", "rights:accept"}})
	if code != 201 {
		t.Fatalf("bind artifact credential: %d %v", code, binding)
	}
	credentialID := binding["credential_id"].(string)
	data := []byte(`{"synthetic":"oauth artifact lifecycle"}`)
	body := map[string]any{"swarm_id": x.room["id"], "plan_id": x.plan["id"], "task_id": x.task["id"], "claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data), "purpose": "output"}
	code, upload, _ := oauthCall(t, x.s, actor.owner, token, http.MethodPost, "/v1/artifacts/uploads", "oauth-artifact-upload", "", body)
	if code != 201 {
		t.Fatalf("OAuth artifact upload authorization: %d %v", code, upload)
	}
	uploadURL, err := url.Parse(upload["upload_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPut, uploadURL.String(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := x.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("OAuth artifact quarantine upload: %d", response.StatusCode)
	}
	code, pending, _ := oauthCall(t, x.s, actor.owner, token, http.MethodPost, "/v1/artifacts/uploads", "oauth-artifact-revoked-capability", "", body)
	if code != 201 {
		t.Fatalf("second OAuth upload capability: %d %v", code, pending)
	}
	code, _, _ = oauthCall(t, x.s, actor.owner, "", http.MethodPost, "/v1/owner/oauth-bindings/"+credentialID+"/revoke", "revoke-artifact-oauth", `"1"`, map[string]any{})
	if code != 200 {
		t.Fatalf("revoke artifact OAuth grant: %d", code)
	}
	pendingURL, err := url.Parse(pending["upload_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	pendingRequest, err := http.NewRequest(http.MethodPut, pendingURL.String(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	pendingRequest.Header.Set("Content-Type", "application/json")
	pendingResponse, err := x.server.Client().Do(pendingRequest)
	if err != nil {
		t.Fatal(err)
	}
	pendingResponse.Body.Close()
	if pendingResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("preexisting capability survived OAuth grant revocation: %d", pendingResponse.StatusCode)
	}
	var storedCredential, state string
	if err = x.db.QueryRow(context.Background(), `SELECT upload_credential_id::text,state FROM undergrid.artifacts WHERE id=$1`, upload["id"]).Scan(&storedCredential, &state); err != nil {
		t.Fatal(err)
	}
	if storedCredential != credentialID || state != "quarantine" {
		t.Fatalf("artifact linked credential/state = %s/%s want %s/quarantine", storedCredential, state, credentialID)
	}
}

func assertOAuthErrorContract(t *testing.T, body map[string]any, headers http.Header, code string) {
	t.Helper()
	e, ok := body["error"].(map[string]any)
	if !ok || e["code"] != code || e["message"] == "" || e["request_id"] == "" || e["request_id"] != headers.Get("X-Request-ID") || e["retryable"] != false {
		t.Fatalf("OAuth denial violates shared error contract: %v %v", body, headers)
	}
	if !strings.Contains(headers.Get("WWW-Authenticate"), "resource_metadata=") {
		t.Fatalf("OAuth denial missing resource challenge: %v", headers)
	}
}
