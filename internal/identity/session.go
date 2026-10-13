// Package identity owns interactive owner sessions. Agent bearer authentication
// belongs to the network domain; an OIDC ID token is never an API credential.
package identity

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"
)

const ownerCookie = "__Host-undergrid-owner"
const flowCookie = "__Host-undergrid-flow"

var errSession = errors.New("valid interactive session required")

type session struct {
	Issuer, Subject, CSRF string
	Expires               int64
}
type Owner struct{ Issuer, Subject string }
type SessionManager struct {
	origin string
	aead   cipher.AEAD
}

func NewSessionManager(origin string, key []byte) (*SessionManager, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || len(key) != 32 {
		return nil, errors.New("HTTPS origin and 32-byte session key required")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SessionManager{origin, aead}, nil
}
func (s *SessionManager) writeCookie(w http.ResponseWriter, name string, value any, maxAge int) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	sealed := s.aead.Seal(nonce, nonce, raw, []byte(name+"|"+s.origin))
	http.SetCookie(w, &http.Cookie{Name: name, Value: base64.RawURLEncoding.EncodeToString(sealed), Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
	return nil
}
func (s *SessionManager) readCookie(r *http.Request, name string, value any) error {
	cookie, err := r.Cookie(name)
	if err != nil || len(cookie.Value) > 4096 {
		return errSession
	}
	sealed, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil || len(sealed) < s.aead.NonceSize() {
		return errSession
	}
	nonce := sealed[:s.aead.NonceSize()]
	raw, err := s.aead.Open(nil, nonce, sealed[s.aead.NonceSize():], []byte(name+"|"+s.origin))
	if err != nil {
		return errSession
	}
	if json.Unmarshal(raw, value) != nil {
		return errSession
	}
	return nil
}
func (s *SessionManager) Authenticate(r *http.Request) (Owner, error) {
	var v session
	if err := s.readCookie(r, ownerCookie, &v); err != nil || v.Expires <= time.Now().Unix() || v.Subject == "" || v.Issuer == "" {
		return Owner{}, errSession
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		if r.Header.Get("Origin") != s.origin || v.CSRF == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(v.CSRF)) != 1 {
			return Owner{}, errSession
		}
	}
	return Owner{v.Issuer, v.Subject}, nil
}
func (s *SessionManager) Session(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if _, err := s.Authenticate(r); err != nil {
		http.Error(w, "authentication required", 401)
		return
	}
	var v session
	if s.readCookie(r, ownerCookie, &v) != nil {
		http.Error(w, "authentication required", 401)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"csrf_token": v.CSRF, "expires_at": v.Expires})
}
func (s *SessionManager) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if _, err := s.Authenticate(r); err != nil {
		http.Error(w, "authentication required", 401)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: ownerCookie, Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.WriteHeader(204)
}
