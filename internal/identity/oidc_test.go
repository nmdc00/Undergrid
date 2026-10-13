package identity

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// A cryptographic protocol fixture, explicitly not selected-provider acceptance.
func TestOIDCCodeFlowNonceAudienceAndVerifiedEmail(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "fixture"))
	if err != nil {
		t.Fatal(err)
	}
	var issuer, nonce, verifier string
	emailVerified := true
	audience := "undergrid-owner"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "fixture", Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			r.ParseForm()
			if r.Form.Get("code_verifier") != verifier || r.Form.Get("code") != "single-use-fixture" {
				http.Error(w, "invalid code", 400)
				return
			}
			token, err := jwt.Signed(signer).Claims(map[string]any{"iss": issuer, "sub": "owner-a", "aud": audience, "exp": time.Now().Add(time.Minute).Unix(), "iat": time.Now().Unix(), "auth_time": time.Now().Unix(), "nonce": nonce, "email_verified": emailVerified}).Serialize()
			if err != nil {
				t.Error(err)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "not-an-undergrid-credential", "token_type": "Bearer", "id_token": token})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL
	ctx := oidc.ClientContext(context.Background(), server.Client())
	sessions, _ := NewSessionManager("https://undergrid.test", bytes.Repeat([]byte{9}, 32))
	login, err := NewLogin(ctx, sessions, issuer, "undergrid-owner", "confidential-secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"valid", "nonce", "audience", "unverified"} {
		t.Run(scenario, func(t *testing.T) {
			start := httptest.NewRecorder()
			login.Start(start, httptest.NewRequest("GET", "https://undergrid.test/auth/login", nil))
			location, _ := url.Parse(start.Header().Get("Location"))
			cookie := start.Result().Cookies()[0]
			probe := httptest.NewRequest("GET", "/", nil)
			probe.AddCookie(cookie)
			var f flow
			if err := sessions.readCookie(probe, flowCookie, &f); err != nil {
				t.Fatal(err)
			}
			nonce = f.Nonce
			verifier = f.Verifier
			audience = "undergrid-owner"
			emailVerified = true
			if scenario == "nonce" {
				nonce = "wrong"
			}
			if scenario == "audience" {
				audience = "another-service"
			}
			if scenario == "unverified" {
				emailVerified = false
			}
			request := httptest.NewRequest("GET", "https://undergrid.test/auth/callback?state="+location.Query().Get("state")+"&code=single-use-fixture", nil).WithContext(ctx)
			request.AddCookie(cookie)
			callback := httptest.NewRecorder()
			login.Callback(callback, request)
			want := 401
			if scenario == "valid" {
				want = 303
			}
			if callback.Code != want {
				t.Fatalf("callback %d: %s", callback.Code, callback.Body.String())
			}
		})
	}
}
