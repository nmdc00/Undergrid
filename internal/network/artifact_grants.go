package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func inputGrantPolicyHash(t ContributionTemplate) string {
	if len(t.InputGrantPurposes) == 0 {
		return ""
	}
	purposes := append([]string(nil), t.InputGrantPurposes...)
	slices.Sort(purposes)
	return digest(canonical(map[string]any{"template_hash": t.SHA256, "purposes": purposes, "submitted_grants_survive_withdrawal": t.SubmittedGrantSurvivesWithdrawal}))
}
func loadLineageInput(ctx context.Context, tx pgx.Tx, output, id string) (storedArtifact, error) {
	var a storedArtifact
	err := tx.QueryRow(ctx, `SELECT id,swarm_id,plan_id,task_id,principal_id,agent_id,generation,state,media_type,size_bytes,claim_epoch,version,COALESCE(sha256,''),COALESCE(manifest_hash,''),COALESCE(plan_hash,''),COALESCE(agreement_hash,''),COALESCE(license_id,''),manifest_bytes,required_rights FROM lineage_input_context($1,$2)`, output, id).Scan(&a.id, &a.swarm, &a.plan, &a.task, &a.principal, &a.agent, &a.generation, &a.state, &a.media, &a.size, &a.epoch, &a.version, &a.sha, &a.manifestHash, &a.planHash, &a.agreement, &a.license, &a.manifest, &a.rights)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, fail(404, "not_found")
	}
	return a, err
}

type InputGrantScope struct {
	ArtifactSHA256     string    `json:"artifact_sha256"`
	ManifestHash       string    `json:"manifest_hash"`
	SourcePlanHash     string    `json:"source_plan_hash"`
	SourceEpoch        int64     `json:"source_claim_epoch"`
	TargetTaskID       string    `json:"target_task_id"`
	TargetPlanHash     string    `json:"target_plan_hash"`
	TargetEpoch        int64     `json:"target_claim_epoch"`
	RecipientAgent     string    `json:"recipient_agent_id"`
	RecipientPrincipal string    `json:"recipient_principal_id"`
	Purpose            string    `json:"purpose"`
	TemplateID         string    `json:"template_id"`
	TemplateHash       string    `json:"template_hash"`
	GrantPolicyHash    string    `json:"grant_policy_hash"`
	LicenseID          string    `json:"license_id"`
	SchemaFamily       string    `json:"schema_family"`
	ExpiresAt          time.Time `json:"expires_at"`
	SurviveWithdrawal  bool      `json:"survive_withdrawal"`
}
type grantTarget struct {
	Task         string `json:"task_id"`
	Epoch        int64  `json:"task_epoch"`
	Plan         string `json:"plan_id"`
	Hash         string `json:"plan_hash"`
	Agent        string `json:"agent_id"`
	Principal    string `json:"principal_id"`
	Template     string `json:"template_id"`
	TemplateHash string `json:"template_hash"`
	PolicyHash   string `json:"grant_policy_hash"`
	License      string `json:"license_id"`
	Schema       string `json:"schema_family"`
}

func (s *Service) createInputGrant(r *http.Request, tx pgx.Tx, a actor, p storedPlan, t storedTask, source storedArtifact) (int, any, error) {
	var rawScope map[string]json.RawMessage
	if err := decode(r, &rawScope); err != nil {
		return 0, nil, err
	}
	survival := bytes.TrimSpace(rawScope["survive_withdrawal"])
	if !bytes.Equal(survival, []byte("true")) && !bytes.Equal(survival, []byte("false")) {
		return 0, nil, fail(422, "explicit_withdrawal_scope_required")
	}
	var scope InputGrantScope
	decoder := json.NewDecoder(bytes.NewReader(canonical(rawScope)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&scope); err != nil {
		return 0, nil, fail(400, "invalid_json")
	}
	template, enabled := s.templates[scope.TemplateID]
	if !enabled || len(template.InputGrantPurposes) == 0 || !slices.Contains(template.InputGrantPurposes, scope.Purpose) || scope.GrantPolicyHash == "" || scope.GrantPolicyHash != inputGrantPolicyHash(template) || scope.GrantPolicyHash != p.sealed.InputGrantPolicyHash || scope.TemplateHash != template.SHA256 || scope.TemplateHash != source.agreement || scope.TemplateID != p.sealed.Plan.Rights.TemplateID || scope.LicenseID != source.license || scope.SchemaFamily != "source_catalog_v1" || (scope.SurviveWithdrawal && !template.SubmittedGrantSurvivesWithdrawal) {
		return 0, nil, fail(409, "exact_scoped_agreement_required")
	}
	if !slices.Contains(source.rights, a.PrincipalID) {
		return 0, nil, fail(403, "required_rights_principal_required")
	}
	if !slices.Contains([]string{"sealed", "verified"}, source.state) || scope.ArtifactSHA256 != source.sha || scope.ManifestHash != source.manifestHash || scope.SourcePlanHash != source.planHash || scope.SourceEpoch != source.epoch || t.epoch != source.epoch || !validID(scope.TargetTaskID) || !validID(scope.RecipientAgent) || !validID(scope.RecipientPrincipal) || scope.TargetEpoch < 1 || !scope.ExpiresAt.After(time.Now()) || scope.ExpiresAt.After(time.Now().Add(30*24*time.Hour)) {
		return 0, nil, fail(409, "input_grant_not_current")
	}
	var raw []byte
	err := tx.QueryRow(r.Context(), "SELECT input_grant_target($1,$2)", source.id, scope.TargetTaskID).Scan(&raw)
	if err != nil {
		return 0, nil, err
	}
	if len(raw) == 0 {
		return 0, nil, fail(409, "named_current_recipient_required")
	}
	var target grantTarget
	if err = json.Unmarshal(raw, &target); err != nil {
		return 0, nil, err
	}
	if target.Agent != scope.RecipientAgent || target.Principal != scope.RecipientPrincipal || target.Hash != scope.TargetPlanHash || target.Epoch != scope.TargetEpoch || target.Template != scope.TemplateID || target.TemplateHash != scope.TemplateHash || target.PolicyHash != scope.GrantPolicyHash || target.License != scope.LicenseID || target.Schema != scope.SchemaFamily {
		return 0, nil, fail(409, "exact_target_scope_required")
	}
	_, version, err := s.requireRights(r.Context(), tx, a, p.sealed.Plan.Rights)
	if err != nil {
		return 0, nil, err
	}
	// PostgreSQL timestamptz stores microseconds; hash the same instant that the
	// normalized expiry column can retain, including across a request retry.
	scope.ExpiresAt = scope.ExpiresAt.UTC().Truncate(time.Microsecond)
	hash := digest(canonical(scope))
	id := NewID()
	tag, err := tx.Exec(r.Context(), `INSERT INTO approvals(id,principal_id,agent_id,purpose,plan_id,artifact_id,content_hash,decision,policy_version,grant_recipient_agent_id,grant_recipient_principal_id,grant_target_task_id,grant_target_plan_id,grant_scope,grant_expires_at,grant_survives_withdrawal,grant_state) VALUES($1,$2,$3,$4,$5,$6,$7,'approve',$8,$9,$10,$11,$12,$13,$14,$15,'active') ON CONFLICT DO NOTHING`, id, a.PrincipalID, a.AgentID, scope.Purpose+"_grant", p.id, source.id, hash, version, scope.RecipientAgent, scope.RecipientPrincipal, scope.TargetTaskID, target.Plan, canonical(scope), scope.ExpiresAt, scope.SurviveWithdrawal)
	if err != nil {
		return 0, nil, err
	}
	if tag.RowsAffected() == 0 {
		var state string
		var oldVersion int64
		if err = tx.QueryRow(r.Context(), "SELECT id,grant_state,policy_version FROM approvals WHERE artifact_id=$1 AND principal_id=$2 AND purpose=$3 AND content_hash=$4", source.id, a.PrincipalID, scope.Purpose+"_grant", hash).Scan(&id, &state, &oldVersion); err != nil {
			return 0, nil, err
		}
		if state != "active" || oldVersion != version {
			return 0, nil, fail(409, "input_grant_requires_new_scope")
		}
	}
	if tag.RowsAffected() > 0 {
		origin := "registered"
		if s.environment != "production" {
			origin = "release_test"
		}
		if _, err = tx.Exec(r.Context(), "INSERT INTO events(id,aggregate_kind,aggregate_id,version,principal_id,agent_id,type,facts,origin) VALUES($1,'artifact_grant',$2,1,$3,$4,'artifact.input_grant_created',$5,$6)", NewID(), id, a.PrincipalID, a.AgentID, canonical(map[string]any{"grant_id": id, "artifact_id": source.id}), origin); err != nil {
			return 0, nil, err
		}
		if err = enqueueInputGrantDelivery(r.Context(), tx, id); err != nil {
			return 0, nil, err
		}
	}
	return 201, map[string]any{"id": id, "artifact_id": source.id, "content_hash": hash, "scope": scope, "state": "active", "evidence_level": "authenticated_agreement_not_legal_ownership_proof"}, nil
}
func (s *Service) inputGrantCommand(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	if !slices.Contains(a.Scopes, "artifacts:grant") && !slices.Contains(a.Scopes, "artifacts:read") {
		return 0, nil, fail(403, "insufficient_scope")
	}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 || len(parts) > 5 || !validID(parts[3]) {
		return 0, nil, fail(404, "not_found")
	}
	var principal, artifact, state, hash string
	var scope []byte
	err := tx.QueryRow(r.Context(), "SELECT principal_id,artifact_id,grant_state,content_hash,grant_scope FROM approvals WHERE id=$1 AND grant_scope IS NOT NULL", parts[3]).Scan(&principal, &artifact, &state, &hash, &scope)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	if r.Method == "GET" && len(parts) == 4 {
		return 200, map[string]any{"id": parts[3], "artifact_id": artifact, "content_hash": hash, "scope": json.RawMessage(scope), "state": state}, nil
	}
	if r.Method != "POST" || len(parts) != 5 || parts[4] != "revoke" {
		return 0, nil, fail(404, "not_found")
	}
	if !slices.Contains(a.Scopes, "artifacts:grant") || principal != a.PrincipalID {
		return 0, nil, fail(403, "grantor_required")
	}
	return idempotent(r, tx, a, func() (int, any, error) {
		var in struct{}
		if err := decode(r, &in); err != nil {
			return 0, nil, err
		}
		tag, err := tx.Exec(r.Context(), "UPDATE approvals SET grant_state='revoked' WHERE id=$1 AND principal_id=$2 AND grant_state='active'", parts[3], a.PrincipalID)
		if err != nil {
			return 0, nil, err
		}
		if tag.RowsAffected() > 0 {
			origin := "registered"
			if s.environment != "production" {
				origin = "release_test"
			}
			if _, err = tx.Exec(r.Context(), "INSERT INTO events(id,aggregate_kind,aggregate_id,version,principal_id,agent_id,type,facts,origin) VALUES($1,'artifact_grant',$2,2,$3,$4,'artifact.input_grant_revoked',$5,$6)", NewID(), parts[3], a.PrincipalID, a.AgentID, canonical(map[string]any{"grant_id": parts[3], "artifact_id": artifact}), origin); err != nil {
				return 0, nil, err
			}
		}
		return 200, map[string]any{"id": parts[3], "state": "revoked"}, nil
	})
}
func (s *Service) inputGrantSnapshot(ctx context.Context, tx pgx.Tx, a actor) ([]any, error) {
	rows, err := tx.Query(ctx, `SELECT g.id,g.artifact_id,g.grant_scope FROM approvals g JOIN artifacts source ON source.id=g.artifact_id WHERE g.grant_recipient_agent_id=$1 AND g.grant_state='active' AND g.grant_expires_at>clock_timestamp() AND effective_input_grant_count(g.artifact_id,g.grant_target_task_id,split_part(g.purpose,'_',1))=cardinality(source.required_rights) ORDER BY g.id`, a.AgentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, input string
		var raw []byte
		if err = rows.Scan(&id, &input, &raw); err != nil {
			return nil, err
		}
		var scope InputGrantScope
		if err = json.Unmarshal(raw, &scope); err != nil {
			return nil, err
		}
		template, ok := s.templates[scope.TemplateID]
		if !ok || inputGrantPolicyHash(template) != scope.GrantPolicyHash {
			continue
		}
		items = append(items, map[string]any{"id": id, "artifact_id": input, "scope": scope, "resource_path": "/v1/artifact-input-grants/" + id, "content_trust": "untrusted"})
	}
	return items, rows.Err()
}
func (s *Service) privateInputRead(r *http.Request, tx pgx.Tx, a actor, source storedArtifact) error {
	purpose, task := r.URL.Query().Get("purpose"), r.URL.Query().Get("task_id")
	if !slices.Contains([]string{"assembly", "evaluation"}, purpose) || !validID(task) {
		return fail(403, "named_input_purpose_required")
	}
	if _, _, err := s.inputGrantEvidence(r.Context(), tx, a, source, task, purpose, nil); err != nil {
		return err
	}
	return nil
}
func (s *Service) inputGrantEvidence(ctx context.Context, tx pgx.Tx, a actor, source storedArtifact, task, purpose string, selected []string) (string, []string, error) {
	var m struct {
		Template string `json:"template_id"`
	}
	if err := json.Unmarshal(source.manifest, &m); err != nil {
		return "", nil, err
	}
	template, ok := s.templates[m.Template]
	if !ok || template.SHA256 != source.agreement || !slices.Contains(template.InputGrantPurposes, purpose) {
		return "", nil, fail(409, "exact_scoped_agreement_required")
	}
	var raw []byte
	if err := tx.QueryRow(ctx, "SELECT input_grant_evidence($1,$2,$3,$4)", source.id, task, purpose, selected).Scan(&raw); err != nil {
		return "", nil, err
	}
	var proof struct {
		IDs          []string `json:"ids"`
		Hashes       []string `json:"hashes"`
		PolicyHashes []string `json:"policy_hashes"`
		Count        int      `json:"count"`
	}
	if err := json.Unmarshal(raw, &proof); err != nil {
		return "", nil, err
	}
	if proof.Count != len(source.rights) || proof.Count == 0 || (selected != nil && len(selected) != proof.Count) {
		return "", nil, fail(403, "scoped_input_grants_required")
	}
	for _, hash := range proof.PolicyHashes {
		if hash != inputGrantPolicyHash(template) {
			return "", nil, fail(409, "exact_scoped_agreement_required")
		}
	}
	return digest(canonical(proof.Hashes)), proof.IDs, nil
}
