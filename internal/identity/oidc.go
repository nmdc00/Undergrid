package identity

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Login struct {
	sessions *SessionManager
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
}
type flow struct {
	State, Nonce, Verifier string
	Expires                int64
}

func NewLogin(ctx context.Context, sessions *SessionManager, issuer, clientID, secret string) (*Login, error) {
	u, err := url.Parse(issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || clientID == "" || secret == "" {
		return nil, errors.New("HTTPS OIDC issuer and confidential client configuration required")
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}
	for _, endpoint := range []string{provider.Endpoint().AuthURL, provider.Endpoint().TokenURL} {
		u, err := url.Parse(endpoint)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return nil, errors.New("OIDC endpoints must use HTTPS")
		}
	}
	return &Login{sessions, oauth2.Config{ClientID: clientID, ClientSecret: secret, Endpoint: provider.Endpoint(), RedirectURL: sessions.origin + "/auth/callback", Scopes: []string{oidc.ScopeOpenID, "email"}}, provider.Verifier(&oidc.Config{ClientID: clientID})}, nil
}
func (l *Login) Start(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	f := flow{oauth2.GenerateVerifier(), oauth2.GenerateVerifier(), oauth2.GenerateVerifier(), time.Now().Add(5 * time.Minute).Unix()}
	if l.sessions.writeCookie(w, flowCookie, f, 300) != nil {
		http.Error(w, "login unavailable", 503)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, l.oauth.AuthCodeURL(f.State, oauth2.S256ChallengeOption(f.Verifier), oidc.Nonce(f.Nonce), oauth2.SetAuthURLParam("max_age", "900")), http.StatusFound)
}
func (l *Login) Callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	var f flow
	if l.sessions.readCookie(r, flowCookie, &f) != nil || f.Expires <= time.Now().Unix() || f.State == "" || subtle.ConstantTimeCompare([]byte(f.State), []byte(r.URL.Query().Get("state"))) != 1 {
		http.Error(w, "invalid login state", 400)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: flowCookie, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	token, err := l.oauth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(f.Verifier))
	if err != nil {
		http.Error(w, "login exchange failed", 401)
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "identity token missing", 401)
		return
	}
	verified, err := l.verifier.Verify(ctx, raw)
	if err != nil || subtle.ConstantTimeCompare([]byte(verified.Nonce), []byte(f.Nonce)) != 1 {
		http.Error(w, "identity verification failed", 401)
		return
	}
	var claims struct {
		EmailVerified bool  `json:"email_verified"`
		AuthTime      int64 `json:"auth_time"`
	}
	if verified.Claims(&claims) != nil || !claims.EmailVerified || claims.AuthTime < time.Now().Add(-15*time.Minute).Unix() || claims.AuthTime > time.Now().Add(time.Minute).Unix() {
		http.Error(w, "recent verified owner login required", 401)
		return
	}
	v := session{verified.Issuer, verified.Subject, oauth2.GenerateVerifier(), time.Now().Add(15 * time.Minute).Unix()}
	if l.sessions.writeCookie(w, ownerCookie, v, 900) != nil {
		http.Error(w, "login unavailable", 503)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
