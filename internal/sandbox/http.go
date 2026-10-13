package sandbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type OwnerAuthenticator func(*http.Request) (Owner, error)
type Handler struct {
	store    *Store
	owner    OwnerAuthenticator
	products map[string]Quote
}

func NewHandler(store *Store, owner OwnerAuthenticator, products map[string]Quote) *Handler {
	catalog := make(map[string]Quote, len(products))
	for k, v := range products {
		catalog[k] = v
	}
	return &Handler{store, owner, catalog}
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	respond := func(code int, v any) { w.WriteHeader(code); json.NewEncoder(w).Encode(v) }
	fail := func(err error) {
		code := 500
		switch {
		case errors.Is(err, ErrUnauthorized):
			code = 401
		case errors.Is(err, ErrInvalid):
			code = 400
		case errors.Is(err, errTooLarge):
			code = 413
		case errors.Is(err, ErrConflict):
			code = 409
		case errors.Is(err, ErrLimit):
			code = 403
		}
		message := "sandbox operation failed"
		if code != 500 {
			message = err.Error()
		}
		respond(code, map[string]string{"error": message, "designation": "TEST"})
	}
	decode := func(v any) bool {
		if err := decodeCommand(r, v); err != nil {
			fail(err)
			return false
		}
		return true
	}
	owner := func() (Owner, error) {
		if h.owner == nil {
			return Owner{}, ErrUnauthorized
		}
		return h.owner(r)
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == r.Header.Get("Authorization") {
		token = ""
	}
	switch {
	case r.Method == "GET" && r.URL.Path == "/health/live":
		respond(200, map[string]string{"status": "live", "designation": "TEST"})
	case r.Method == "POST" && r.URL.Path == "/v1/actors":
		var empty struct{}
		if !decode(&empty) {
			return
		}
		o, err := owner()
		if err != nil {
			fail(ErrUnauthorized)
			return
		}
		id, err := h.store.Enroll(r.Context(), o)
		if err != nil {
			fail(err)
			return
		}
		respond(201, map[string]string{"id": id, "designation": "TEST"})
	case r.Method == "GET" && r.URL.Path == "/v1/owner/policy":
		o, err := owner()
		if err != nil {
			fail(ErrUnauthorized)
			return
		}
		p, v, err := h.store.GetPolicy(r.Context(), o)
		if err != nil {
			fail(err)
			return
		}
		respond(200, map[string]any{"policy": p, "version": v, "designation": "TEST"})
	case r.Method == "PUT" && r.URL.Path == "/v1/owner/policy":
		var cmd struct {
			Version int64  `json:"version"`
			Policy  Policy `json:"policy"`
		}
		if !decode(&cmd) {
			return
		}
		o, err := owner()
		if err != nil {
			fail(ErrUnauthorized)
			return
		}
		if err = h.store.SetPolicy(r.Context(), o, cmd.Version, cmd.Policy); err != nil {
			fail(err)
			return
		}
		respond(200, map[string]any{"version": cmd.Version + 1, "designation": "TEST"})
	case r.Method == "POST" && r.URL.Path == "/v1/owner/credentials":
		var cmd struct {
			Name   string   `json:"name"`
			Scopes []string `json:"scopes"`
		}
		if !decode(&cmd) {
			return
		}
		o, err := owner()
		if err != nil {
			fail(ErrUnauthorized)
			return
		}
		credential, err := h.store.IssueCredential(r.Context(), o, cmd.Name, cmd.Scopes)
		if err != nil {
			fail(err)
			return
		}
		respond(201, map[string]string{"token": credential, "audience": "undergrid-sandbox", "designation": "TEST"})
	case r.Method == "POST" && r.URL.Path == "/v1/owner/credentials/revoke":
		var empty struct{}
		if !decode(&empty) {
			return
		}
		o, err := owner()
		if err != nil {
			fail(ErrUnauthorized)
			return
		}
		if err = h.store.RevokeCredentials(r.Context(), o); err != nil {
			fail(err)
			return
		}
		respond(200, map[string]string{"state": "revoked", "designation": "TEST"})
	case r.Method == "POST" && r.URL.Path == "/v1/refunds":
		var cmd struct {
			Payment string `json:"payment_id"`
			Amount  string `json:"amount_atomic"`
		}
		if !decode(&cmd) {
			return
		}
		refund, err := h.store.RequestRefund(r.Context(), token, r.Header.Get("Idempotency-Key"), cmd.Payment, cmd.Amount)
		if err != nil {
			fail(err)
			return
		}
		respond(201, refund)
	case r.Method == "POST" && r.URL.Path == "/v1/payments":
		var cmd struct {
			Product string `json:"product"`
		}
		if !decode(&cmd) {
			return
		}
		q, ok := h.products[cmd.Product]
		if !ok {
			fail(ErrInvalid)
			return
		}
		q.ExpiresAt = time.Now().UTC().Add(15 * time.Minute)
		p, err := h.store.createCatalogQuote(r.Context(), token, r.Header.Get("Idempotency-Key"), q)
		if err != nil {
			fail(err)
			return
		}
		respond(201, p)
	default:
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 3 && parts[0] == "v1" && parts[1] == "refunds" && validUUID(parts[2]) {
			if r.Method == "GET" && len(parts) == 3 {
				refund, err := h.store.GetRefund(r.Context(), token, parts[2])
				if err != nil {
					fail(err)
					return
				}
				respond(200, refund)
				return
			}
			if r.Method == "POST" && len(parts) == 4 && parts[3] == "authorize" {
				var cmd struct {
					Hash string `json:"refund_hash"`
				}
				if !decode(&cmd) {
					return
				}
				refund, err := h.store.AuthorizeRefund(r.Context(), token, parts[2], cmd.Hash, r.Header.Get("Idempotency-Key"))
				if err != nil {
					fail(err)
					return
				}
				respond(200, refund)
				return
			}
		}
		if len(parts) >= 3 && parts[0] == "v1" && parts[1] == "payments" && validUUID(parts[2]) {
			if r.Method == "GET" && len(parts) == 3 {
				p, err := h.store.GetPayment(r.Context(), token, parts[2])
				if err != nil {
					fail(err)
					return
				}
				respond(200, p)
				return
			}
			if r.Method == "POST" && len(parts) == 4 && parts[3] == "authorize" {
				var cmd struct {
					Hash string `json:"quote_hash"`
				}
				if !decode(&cmd) {
					return
				}
				p, err := h.store.Authorize(r.Context(), token, parts[2], cmd.Hash, r.Header.Get("Idempotency-Key"))
				if err != nil {
					fail(err)
					return
				}
				respond(200, p)
				return
			}
		}
		respond(404, map[string]string{"error": "not found", "designation": "TEST"})
	}
}
func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

var errTooLarge = errors.New("sandbox command exceeds 64 KiB")

func decodeCommand(r *http.Request, v any) error {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		return ErrInvalid
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 65537))
	if err != nil {
		return ErrInvalid
	}
	if len(b) > 65536 {
		return errTooLarge
	}
	check := json.NewDecoder(bytes.NewReader(b))
	if err = uniqueJSON(check); err != nil {
		return ErrInvalid
	}
	if _, err = check.Token(); err != io.EOF {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(v); err != nil {
		return ErrInvalid
	}
	return nil
}
func uniqueJSON(d *json.Decoder) error {
	tok, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := k.(string)
			if !ok || seen[key] {
				return ErrInvalid
			}
			seen[key] = true
			if err = uniqueJSON(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err = uniqueJSON(d); err != nil {
				return err
			}
		}
	default:
		return ErrInvalid
	}
	_, err = d.Token()
	return err
}
