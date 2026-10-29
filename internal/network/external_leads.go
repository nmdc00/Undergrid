package network

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// External lead retrieval deliberately makes no relevance, capability, identity
// or demand assertion. Future matching must persist independently proven evidence.
type externalLead struct {
	ID                string          `json:"id"`
	SourceID          string          `json:"source_id"`
	SourceVersion     string          `json:"source_version"`
	Origin            string          `json:"origin"`
	CanonicalURL      string          `json:"canonical_url"`
	Publisher         string          `json:"publisher"`
	PermissionNotice  string          `json:"permission_notice"`
	Metadata          json.RawMessage `json:"metadata"`
	ObservedAt        time.Time       `json:"observed_at"`
	CheckedAt         time.Time       `json:"checked_at"`
	FreshUntil        time.Time       `json:"fresh_until"`
	IdentityAssurance string          `json:"identity_assurance"`
	FitStatus         string          `json:"fit_status"`
	HardConstraints   string          `json:"hard_constraints"`
	ProjectionHash    string          `json:"projection_hash"`
}
type externalLeadCursor struct {
	Purpose, Environment, Principal, Agent, Credential, Channel, Intention, Kind, Query, After string
	Revision                                                                                   int
	Limit                                                                                      int
	Issued                                                                                     int64
}

func externalLeadIntentID(path string) (string, bool) {
	const prefix = "/v1/intents/"
	const suffix = "/external-leads"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	return id, validID(id)
}
func (s *Service) listExternalLeads(r *http.Request, tx pgx.Tx, a actor, intent, channel string) (int, any, error) {
	if r.Method != http.MethodGet {
		return 0, nil, fail(405, "method_not_allowed")
	}
	if !validID(intent) {
		return 0, nil, fail(404, "not_found")
	}
	var revision int
	err := tx.QueryRow(r.Context(), `SELECT i.current_revision FROM intents i JOIN agents g ON g.id=i.agent_id WHERE i.id=$1 AND i.principal_id=$2 AND i.state='active' AND i.expires_at>now() AND g.status='active'`, intent, a.PrincipalID).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	if len(s.cursorKey) != 32 {
		return 0, nil, fail(503, "cursor_signing_unconfigured")
	}
	if len(r.URL.RawQuery) > 8192 {
		return 0, nil, fail(400, "invalid_lead_query")
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return 0, nil, fail(400, "invalid_lead_query")
	}
	for key, v := range values {
		if len(v) != 1 || (key != "kind" && key != "q" && key != "limit" && key != "cursor") {
			return 0, nil, fail(400, "invalid_lead_query")
		}
	}
	kind := values.Get("kind")
	if kind != "" && kind != "external_request" && kind != "external_capability" {
		return 0, nil, fail(400, "invalid_lead_kind")
	}
	rawQuery := values.Get("q")
	if !utf8.ValidString(rawQuery) || utf8.RuneCountInString(rawQuery) > 200 || strings.ContainsRune(rawQuery, '\x00') {
		return 0, nil, fail(400, "invalid_lead_query")
	}
	query := strings.ToLower(strings.TrimSpace(rawQuery))
	limit := 20
	if text := values.Get("limit"); text != "" {
		limit, err = strconv.Atoi(text)
		if err != nil || limit < 1 || limit > 50 {
			return 0, nil, fail(400, "invalid_lead_limit")
		}
	}
	audience := externalLeadCursor{Purpose: "external-leads-v1", Environment: s.environment, Principal: a.PrincipalID, Agent: a.AgentID, Credential: a.CredentialID, Channel: channel, Intention: intent, Revision: revision, Kind: kind, Query: query, Limit: limit}
	after := ""
	if token := values.Get("cursor"); token != "" {
		after, err = s.decodeExternalLeadCursor(token, audience)
		if err != nil {
			return 0, nil, err
		}
	}
	var afterValue any
	if after != "" {
		afterValue = after
	}
	rows, err := tx.Query(r.Context(), `SELECT id,source_id,source_version,origin,canonical_url,publisher,permission_notice,metadata,observed_at,checked_at FROM external_leads_for_intention($1,$2,$3,$4,$5)`, intent, kind, query, afterValue, limit+1)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	items := []externalLead{}
	for rows.Next() {
		var item externalLead
		if err = rows.Scan(&item.ID, &item.SourceID, &item.SourceVersion, &item.Origin, &item.CanonicalURL, &item.Publisher, &item.PermissionNotice, &item.Metadata, &item.ObservedAt, &item.CheckedAt); err != nil {
			return 0, nil, err
		}
		item = externalLeadProjection(item)
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return 0, nil, err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		audience.After = items[len(items)-1].ID
		audience.Issued = time.Now().Unix()
		next = s.encodeExternalLeadCursor(audience)
	}
	return 200, map[string]any{"items": items, "next_cursor": next, "limit": limit, "intention_id": intent, "intention_revision": revision, "assessment": "not_assessed"}, nil
}
func (s *Service) encodeExternalLeadCursor(c externalLeadCursor) string {
	raw := canonical(c)
	mac := hmac.New(sha256.New, s.cursorKey)
	mac.Write([]byte("external-leads-v1\n"))
	mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Service) decodeExternalLeadCursor(token string, want externalLeadCursor) (string, error) {
	bad := func() (string, error) { return "", fail(400, "invalid_lead_cursor") }
	if len(token) > 4096 {
		return bad()
	}
	body, sig, ok := strings.Cut(token, ".")
	if !ok {
		return bad()
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return bad()
	}
	signature, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return bad()
	}
	mac := hmac.New(sha256.New, s.cursorKey)
	mac.Write([]byte("external-leads-v1\n"))
	mac.Write(raw)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return bad()
	}
	var got externalLeadCursor
	if json.Unmarshal(raw, &got) != nil || !validID(got.After) || got.Issued > time.Now().Unix()+60 {
		return bad()
	}
	after, issued := got.After, got.Issued
	got.After = ""
	got.Issued = 0
	if got != want {
		return bad()
	}
	if time.Since(time.Unix(issued, 0)) > 30*time.Minute {
		return "", fail(410, "lead_cursor_expired")
	}
	return after, nil
}

// externalLeadProjection annotates the already permission-filtered public row.
// Both list and declared-fit reads publish this exact assurance/hash contract.
func externalLeadProjection(lead externalLead) externalLead {
	lead.IdentityAssurance = "unknown"
	lead.FitStatus = "not_assessed"
	lead.HardConstraints = "unknown"
	lead.FreshUntil = lead.CheckedAt.Add(24 * time.Hour)
	// Hash the public projection only. Private approval files and mutable check
	// timestamps cannot reveal themselves through a supposedly public digest.
	lead.ProjectionHash = digest(canonical(map[string]any{"id": lead.ID, "source_id": lead.SourceID, "source_version": lead.SourceVersion, "origin": lead.Origin, "canonical_url": lead.CanonicalURL, "publisher": lead.Publisher, "permission_notice": lead.PermissionNotice, "metadata": lead.Metadata}))
	return lead
}
