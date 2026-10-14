package network

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

type ContributionTemplate struct {
	ID                               string   `json:"id"`
	SHA256                           string   `json:"sha256"`
	LicenseIDs                       []string `json:"license_ids"`
	TermsURL                         string   `json:"terms_url"`
	ApprovalEvidence                 string   `json:"approval_evidence"`
	TestOnly                         bool     `json:"test_only"`
	InputGrantPurposes               []string `json:"input_grant_purposes,omitempty"`
	SubmittedGrantSurvivesWithdrawal bool     `json:"submitted_grant_survives_withdrawal,omitempty"`
}
type CoordinationPolicy struct {
	TemplateIDs    []string  `json:"template_ids"`
	LicenseIDs     []string  `json:"license_ids"`
	TaskCategories []string  `json:"task_categories"`
	MaxActiveTasks int       `json:"max_active_tasks"`
	ExpiresAt      time.Time `json:"expires_at"`
}
type RightsSpec struct {
	TemplateID  string `json:"template_id"`
	LicenseID   string `json:"license_id"`
	Publication string `json:"publication"`
}

// Configure once before serving. No template is enabled by default. Test-only
// templates cannot cross the production boundary; legal review remains external.
func (s *Service) SetContributionTemplates(templates []ContributionTemplate) error {
	next := map[string]ContributionTemplate{}
	for _, v := range templates {
		for _, purpose := range v.InputGrantPurposes {
			if purpose != "assembly" && purpose != "evaluation" {
				return errors.New("unsupported input grant purpose")
			}
		}
		if len(v.InputGrantPurposes) > 2 || (v.SubmittedGrantSurvivesWithdrawal && len(v.InputGrantPurposes) == 0) {
			return errors.New("explicit bounded input grant template required")
		}
		u, err := url.Parse(v.TermsURL)
		hash, hashErr := hex.DecodeString(v.SHA256)
		if v.ID == "" || len(v.ID) > 100 || err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || hashErr != nil || len(hash) != 32 || len(v.LicenseIDs) == 0 || v.ApprovalEvidence == "" || (s.environment == "production" && v.TestOnly) {
			return errors.New("invalid or unapproved contribution template")
		}
		if _, exists := next[v.ID]; exists {
			return errors.New("duplicate contribution template ID")
		}
		v.LicenseIDs = append([]string(nil), v.LicenseIDs...)
		v.InputGrantPurposes = append([]string(nil), v.InputGrantPurposes...)
		next[v.ID] = v
	}
	s.templates = next
	return nil
}
func (s *Service) publicTemplates() any {
	items := []any{}
	for _, t := range s.templates {
		items = append(items, map[string]any{"id": t.ID, "sha256": t.SHA256, "license_ids": t.LicenseIDs, "terms_url": t.TermsURL, "test_only": t.TestOnly, "input_grant_purposes": t.InputGrantPurposes, "submitted_grant_survives_withdrawal": t.SubmittedGrantSurvivesWithdrawal, "input_grant_policy_hash": inputGrantPolicyHash(t)})
	}
	return map[string]any{"templates": items}
}
func (s *Service) ownerPolicy(r *http.Request, tx pgx.Tx, principal string) (int, any, error) {
	var version int64
	var raw []byte
	if err := tx.QueryRow(r.Context(), "SELECT version,policy FROM principals WHERE id=$1", principal).Scan(&version, &raw); err != nil {
		return 0, nil, err
	}
	if r.Method == "GET" {
		return 200, map[string]any{"version": version, "etag": etag(version), "policy": json.RawMessage(raw)}, nil
	}
	if r.Method != "POST" {
		return 0, nil, fail(405, "method_not_allowed")
	}
	if r.Header.Get("If-Match") == "" {
		return 0, nil, fail(428, "if_match_required")
	}
	if r.Header.Get("If-Match") != etag(version) {
		return 0, nil, fail(409, "version_conflict")
	}
	var in CoordinationPolicy
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if len(in.TemplateIDs) > 10 || len(in.LicenseIDs) > 20 || in.MaxActiveTasks < 0 || in.MaxActiveTasks > 50 || !in.ExpiresAt.After(time.Now()) || in.ExpiresAt.After(time.Now().Add(90*24*time.Hour)) {
		return 0, nil, fail(422, "invalid_coordination_policy")
	}
	for _, id := range in.TemplateIDs {
		if _, ok := s.templates[id]; !ok {
			return 0, nil, fail(422, "template_not_enabled")
		}
	}
	for _, category := range in.TaskCategories {
		if category != "json_dataset" {
			return 0, nil, fail(422, "unsupported_task_category")
		}
	}
	for _, license := range in.LicenseIDs {
		found := false
		for _, id := range in.TemplateIDs {
			if slices.Contains(s.templates[id].LicenseIDs, license) {
				found = true
			}
		}
		if !found {
			return 0, nil, fail(422, "license_not_enabled")
		}
	}
	version++
	if _, err := tx.Exec(r.Context(), "UPDATE principals SET policy=$2,version=$3 WHERE id=$1", principal, canonical(in), version); err != nil {
		return 0, nil, err
	}
	origin := "registered"
	if s.environment != "production" {
		origin = "release_test"
	}
	if _, err := tx.Exec(r.Context(), "INSERT INTO events(id,aggregate_kind,aggregate_id,version,principal_id,type,facts,origin) VALUES($1,'principal',$2,$3,$2,'principal.policy_updated',$4,$5)", NewID(), principal, version, canonical(map[string]any{"policy_hash": digest(canonical(in))}), origin); err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"version": version, "etag": etag(version), "policy": in}, nil
}
func (s *Service) requireRights(ctx context.Context, tx pgx.Tx, a actor, rights RightsSpec) (CoordinationPolicy, int64, error) {
	var policy CoordinationPolicy
	var version int64
	if !slices.Contains(a.Scopes, "rights:accept") {
		return policy, 0, fail(403, "rights_scope_required")
	}
	template, ok := s.templates[rights.TemplateID]
	if !ok || !slices.Contains(template.LicenseIDs, rights.LicenseID) || rights.Publication != "exact_manifest_approval" {
		return policy, 0, fail(403, "terms_not_enabled")
	}
	var raw []byte
	if err := tx.QueryRow(ctx, "SELECT policy,version FROM principals WHERE id=$1", a.PrincipalID).Scan(&raw, &version); err != nil {
		return policy, 0, err
	}
	if err := json.Unmarshal(raw, &policy); err != nil {
		return policy, 0, err
	}
	if !policy.ExpiresAt.After(time.Now()) || !slices.Contains(policy.TemplateIDs, rights.TemplateID) || !slices.Contains(policy.LicenseIDs, rights.LicenseID) || !slices.Contains(policy.TaskCategories, "json_dataset") {
		return policy, 0, fail(403, "owner_policy_denied")
	}
	return policy, version, nil
}
