package network

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"undergrid/internal/verification"
)

const artifactLimitations = "Fixed structural checks only: no factual truth, coverage, legal ownership or fitness proof; content is untrusted."

type storedArtifact struct {
	id, swarm, plan, task, principal, agent, generation, state, purpose, media string
	sha, manifestHash, planHash, agreement, license                            string
	size, epoch, version                                                       int64
	manifest                                                                   []byte
	rights                                                                     []string
}

func loadArtifact(ctx context.Context, tx pgx.Tx, id string, lock bool) (storedArtifact, error) {
	var a storedArtifact
	q := `SELECT id,swarm_id,plan_id,task_id,principal_id,agent_id,generation,state,purpose,media_type,size_bytes,claim_epoch,version,
 COALESCE(sha256,''),COALESCE(manifest_hash,''),COALESCE(plan_hash,''),COALESCE(agreement_hash,''),COALESCE(license_id,''),manifest_bytes,required_rights FROM artifacts WHERE id=$1`
	if lock {
		q += " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, q, id).Scan(&a.id, &a.swarm, &a.plan, &a.task, &a.principal, &a.agent, &a.generation, &a.state, &a.purpose, &a.media, &a.size, &a.epoch, &a.version, &a.sha, &a.manifestHash, &a.planHash, &a.agreement, &a.license, &a.manifest, &a.rights)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, fail(404, "not_found")
	}
	return a, err
}
func artifactProjection(a storedArtifact) any {
	out := map[string]any{"id": a.id, "state": a.state, "sha256": a.sha, "manifest_hash": a.manifestHash, "size_bytes": a.size, "media_type": a.media, "license_id": a.license, "limitations": artifactLimitations, "content_trust": "untrusted"}
	if len(a.manifest) > 0 {
		out["manifest"] = json.RawMessage(a.manifest)
	}
	// Public projection omits room, plan, task, policy and credential information.
	if a.state != "published" {
		out["swarm_id"] = a.swarm
		out["plan_id"] = a.plan
		out["task_id"] = a.task
		out["purpose"] = a.purpose
		out["claim_epoch"] = a.epoch
		out["agreement_hash"] = a.agreement
		out["submission_attestation"] = "authenticated_platform_submission_not_agent_signature_or_legal_ownership_proof"
	}
	return out
}
func authenticateArtifactUpload(r *http.Request, tx pgx.Tx) (actor, error) {
	var a actor
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 5 || !validID(parts[3]) {
		return a, fail(404, "not_found")
	}
	token := r.URL.Query().Get("token")
	b, err := hex.DecodeString(token)
	if err != nil || len(b) != 32 {
		return a, fail(401, "invalid_upload_capability")
	}
	hash := sha256.Sum256([]byte(token))
	err = tx.QueryRow(r.Context(), "SELECT principal_id,agent_id,credential_id FROM artifact_upload_identity($1,$2)", parts[3], hash[:]).Scan(&a.PrincipalID, &a.AgentID, &a.CredentialID)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, fail(401, "invalid_upload_capability")
	}
	if err != nil {
		return a, err
	}
	if _, err = tx.Exec(r.Context(), "SELECT set_config('undergrid.principal_id',$1,true),set_config('undergrid.agent_id',$2,true)", a.PrincipalID, a.AgentID); err != nil {
		return a, err
	}
	return lockActor(r.Context(), tx, a)
}
func (s *Service) artifactCommand(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	scope := "artifacts:read"
	switch {
	case r.Method == "PUT":
		scope = "artifacts:submit"
	case strings.HasSuffix(r.URL.Path, "/uploads") || strings.HasSuffix(r.URL.Path, "/finalize"):
		scope = "artifacts:submit"
	case strings.HasSuffix(r.URL.Path, "/acceptances"):
		scope = "artifacts:accept"
	case strings.HasSuffix(r.URL.Path, "/publication-approvals") || strings.HasSuffix(r.URL.Path, "/publish"):
		scope = "publication:approve"
	case strings.HasSuffix(r.URL.Path, "/reuse-receipts"):
		scope = "artifacts:reuse"
	}
	if !slices.Contains(a.Scopes, scope) {
		return 0, nil, fail(403, "insufficient_scope")
	}
	if s.artifacts == nil {
		return 0, nil, fail(503, "artifact_runtime_unavailable")
	}
	if r.URL.Path == "/v1/artifacts" && r.Method == "GET" {
		return listPublishedArtifacts(r, tx)
	}
	if r.URL.Path == "/v1/artifacts/uploads" && r.Method == "POST" {
		// Replay is a disclosure too. Recheck room, epoch, assignment and current
		// owner policy before consulting the retained command response.
		var in artifactUploadInput
		if err := decode(r, &in); err != nil {
			return 0, nil, err
		}
		if !validID(in.Swarm) || !validID(in.Plan) || !validID(in.Task) {
			return 0, nil, fail(422, "invalid_artifact_upload")
		}
		if err := roomAuthority(r.Context(), tx, a, in.Swarm, false); err != nil {
			return 0, nil, err
		}
		p, err := loadPlan(r.Context(), tx, in.Plan, true)
		if err != nil {
			return 0, nil, err
		}
		t, err := loadTask(r.Context(), tx, in.Task, true)
		if err != nil {
			return 0, nil, err
		}
		if p.swarm != in.Swarm {
			return 0, nil, fail(409, "artifact_assignment_not_current")
		}
		if err = s.currentArtifactAuthority(r.Context(), tx, a, p, t, storedArtifact{plan: in.Plan, task: in.Task, swarm: in.Swarm, agent: a.AgentID, epoch: in.Epoch, state: "quarantine"}, true); err != nil {
			return 0, nil, err
		}
		r.Body = io.NopCloser(bytes.NewReader(canonical(in)))
		return idempotent(r, tx, a, func() (int, any, error) { return s.createArtifactUpload(r, tx, a) })
	}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 || len(parts) > 5 || !validID(parts[3]) {
		return 0, nil, fail(404, "not_found")
	}
	art, err := loadArtifact(r.Context(), tx, parts[3], false)
	if err != nil {
		return 0, nil, err
	}
	if r.Method == "GET" {
		if len(parts) == 4 {
			return 200, artifactProjection(art), nil
		}
		switch parts[4] {
		case "evaluations":
			return artifactEvaluations(r.Context(), tx, art.id)
		case "content":
			if art.state == "quarantine" {
				return 0, nil, fail(409, "artifact_not_sealed")
			}
			data, err := s.artifactBytes(art)
			if err != nil {
				return 0, nil, err
			}
			if err = s.artifactActorEvent(r.Context(), tx, a, art, "artifact.retrieved", map[string]any{"artifact_sha256": art.sha, "evidence_level": "retrieved_not_reused"}); err != nil {
				return 0, nil, err
			}
			return 200, artifactContent(data), nil
		}
	}
	if len(parts) != 5 {
		return 0, nil, fail(404, "not_found")
	}
	if r.Method == "POST" && parts[4] == "reuse-receipts" {
		if art.state != "published" {
			return 0, nil, fail(404, "not_found")
		}
		return idempotent(r, tx, a, func() (int, any, error) { return s.recordArtifactReuse(r, tx, a, art) })
	}
	if r.Method != "POST" && r.Method != "PUT" {
		return 0, nil, fail(405, "method_not_allowed")
	}
	if err = roomAuthority(r.Context(), tx, a, art.swarm, false); err != nil {
		return 0, nil, err
	}
	p, err := loadPlan(r.Context(), tx, art.plan, true)
	if err != nil {
		return 0, nil, err
	}
	t, err := loadTask(r.Context(), tx, art.task, true)
	if err != nil {
		return 0, nil, err
	}
	art, err = loadArtifact(r.Context(), tx, art.id, true)
	if err != nil {
		return 0, nil, err
	}
	if err = s.currentArtifactAuthority(r.Context(), tx, a, p, t, art, scope == "artifacts:submit"); err != nil {
		return 0, nil, err
	}
	if r.Method == "PUT" && parts[4] == "upload" {
		if art.state != "quarantine" {
			return 0, nil, fail(409, "generation_is_sealed")
		}
		if err = s.artifacts.put(art.generation, art.size, r.Body); err != nil {
			return 0, nil, err
		}
		return 204, nil, nil
	}
	if r.Method != "POST" {
		return 0, nil, fail(404, "not_found")
	}
	return idempotent(r, tx, a, func() (int, any, error) {
		switch parts[4] {
		case "finalize":
			return s.finalizeArtifact(r, tx, a, p, t, art)
		case "acceptances":
			return s.acceptArtifact(r, tx, a, p, t, art)
		case "publication-approvals":
			return s.approveArtifactPublication(r, tx, a, p, art)
		case "publish":
			return s.publishArtifact(r, tx, a, p, t, art)
		}
		return 0, nil, fail(404, "not_found")
	})
}
func (s *Service) currentArtifactAuthority(ctx context.Context, tx pgx.Tx, a actor, p storedPlan, t storedTask, art storedArtifact, submit bool) error {
	template, ok := s.templates[p.sealed.Plan.Rights.TemplateID]
	if !ok || template.SHA256 != p.sealed.TemplateHash {
		return fail(409, "plan_terms_expired_or_changed")
	}
	var head *string
	if err := tx.QueryRow(ctx, "SELECT active_plan_id FROM swarms WHERE id=$1", p.swarm).Scan(&head); err != nil {
		return err
	}
	if head == nil || *head != p.id || p.state != "accepted" || t.epoch != art.epoch || t.plan != art.plan || t.agent != art.agent || !slices.Contains([]string{"accepted", "in_progress", "changes_requested", "at_risk", "submitted", "accepted_output"}, t.state) {
		return fail(409, "artifact_assignment_not_current")
	}
	if submit && art.agent != a.AgentID {
		return fail(403, "assignee_required")
	}
	if t.spec.Deadline != nil && !t.spec.Deadline.After(time.Now()) && art.state == "quarantine" {
		return fail(409, "task_deadline_passed")
	}
	_, _, err := s.requireRights(ctx, tx, a, p.sealed.Plan.Rights)
	return err
}

type artifactUploadInput struct {
	Swarm   string `json:"swarm_id"`
	Plan    string `json:"plan_id"`
	Task    string `json:"task_id"`
	Epoch   int64  `json:"claim_epoch"`
	Purpose string `json:"purpose,omitempty"`
	Media   string `json:"media_type"`
	Size    int64  `json:"size_bytes"`
}

func (s *Service) createArtifactUpload(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	var in artifactUploadInput
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if !validID(in.Swarm) || !validID(in.Plan) || !validID(in.Task) || in.Epoch < 1 || in.Media != "application/json" || in.Size < 1 || in.Size > verification.MaxDataBytes {
		return 0, nil, fail(422, "invalid_artifact_upload")
	}
	if in.Purpose == "" {
		in.Purpose = "output"
	}
	if in.Purpose != "output" && in.Purpose != "progress" {
		return 0, nil, fail(422, "invalid_artifact_purpose")
	}
	if err := roomAuthority(r.Context(), tx, a, in.Swarm, false); err != nil {
		return 0, nil, err
	}
	p, err := loadPlan(r.Context(), tx, in.Plan, true)
	if err != nil {
		return 0, nil, err
	}
	t, err := loadTask(r.Context(), tx, in.Task, true)
	if err != nil {
		return 0, nil, err
	}
	if p.swarm != in.Swarm || !slices.Contains([]string{"accepted", "in_progress", "changes_requested", "at_risk"}, t.state) {
		return 0, nil, fail(409, "artifact_assignment_not_current")
	}
	art := storedArtifact{plan: in.Plan, task: in.Task, swarm: in.Swarm, agent: a.AgentID, principal: a.PrincipalID, epoch: in.Epoch, state: "quarantine"}
	if err = s.currentArtifactAuthority(r.Context(), tx, a, p, t, art, true); err != nil {
		return 0, nil, err
	}
	var storage int64
	if err = tx.QueryRow(r.Context(), "SELECT principal_artifact_storage()").Scan(&storage); err != nil {
		return 0, nil, err
	}
	if storage+in.Size > 100<<20 {
		return 0, nil, fail(429, "artifact_storage_quota_exceeded")
	}
	id, generation := NewID(), NewID()
	capability := make([]byte, 32)
	if _, err = rand.Read(capability); err != nil {
		return 0, nil, err
	}
	token := hex.EncodeToString(capability)
	hash := sha256.Sum256([]byte(token))
	expires := time.Now().UTC().Add(60 * time.Second)
	if _, err = tx.Exec(r.Context(), `INSERT INTO artifacts(id,swarm_id,plan_id,task_id,principal_id,agent_id,claim_epoch,generation,purpose,size_bytes,media_type,upload_digest,upload_credential_id,upload_expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, id, in.Swarm, in.Plan, in.Task, a.PrincipalID, a.AgentID, in.Epoch, generation, in.Purpose, in.Size, in.Media, hash[:], a.CredentialID, expires); err != nil {
		return 0, nil, err
	}
	return 201, map[string]any{"id": id, "state": "quarantine", "purpose": in.Purpose, "generation": generation, "upload_url": s.artifacts.config.Origin + "/v1/artifacts/" + id + "/upload?token=" + token, "expires_at": expires, "size_bytes": in.Size, "media_type": in.Media}, nil
}
func (s *Service) artifactBytes(a storedArtifact) ([]byte, error) {
	b, err := s.artifacts.read(a.generation, a.size)
	if err != nil {
		return nil, err
	}
	if a.sha != "" && digest(b) != a.sha {
		return nil, fail(409, "artifact_hash_mismatch")
	}
	return b, nil
}
func (s *Service) finalizeArtifact(r *http.Request, tx pgx.Tx, a actor, p storedPlan, t storedTask, art storedArtifact) (int, any, error) {
	var in struct {
		SHA      string                `json:"sha256"`
		PlanHash string                `json:"plan_hash"`
		Epoch    int64                 `json:"claim_epoch"`
		Manifest verification.Manifest `json:"manifest"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if art.state != "quarantine" || !slices.Contains([]string{"accepted", "in_progress", "changes_requested", "at_risk"}, t.state) || in.PlanHash != p.hash || in.Epoch != t.epoch {
		return 0, nil, fail(409, "artifact_not_current")
	}
	data, err := s.artifactBytes(art)
	if err != nil {
		return 0, nil, err
	}
	sha := digest(data)
	if !artifactHash(in.SHA) || sha != in.SHA {
		return 0, nil, fail(422, "artifact_hash_mismatch")
	}
	m := in.Manifest
	if len(m.Title) < 1 || len(m.Title) > 300 || m.SchemaFamily != t.spec.OutputSchema || m.TemplateID != p.sealed.Plan.Rights.TemplateID || m.LicenseID != p.sealed.Plan.Rights.LicenseID || m.LicenseID != "CC0-1.0" || len(m.SourceDeclaration) < 1 || len(m.SourceDeclaration) > 4000 || len(m.Inputs) > 50 || len(m.Attribution) < 1 || len(m.Attribution) > 50 {
		return 0, nil, fail(422, "manifest_contract_or_unknown_license_compatibility")
	}
	rights := append([]string(nil), p.sealed.Plan.PublicationPrincipals...)
	rights = append(rights, art.principal)
	seen := map[string]bool{}
	for _, input := range m.Inputs {
		if !validID(input.ArtifactID) || seen[input.ArtifactID] || input.ArtifactID == art.id || !artifactHash(input.SHA256) {
			return 0, nil, fail(422, "invalid_input_lineage")
		}
		seen[input.ArtifactID] = true
		source, err := loadArtifact(r.Context(), tx, input.ArtifactID, false)
		if err != nil {
			return 0, nil, err
		}
		if source.state != "published" || source.sha != input.SHA256 || source.license != input.LicenseID || source.license != m.LicenseID {
			return 0, nil, fail(409, "input_rights_not_established")
		}
		if _, err = s.artifactBytes(source); err != nil {
			return 0, nil, err
		}
		rights = append(rights, source.rights...)
		if _, err = tx.Exec(r.Context(), "INSERT INTO artifact_inputs(artifact_id,input_artifact_id,input_sha256,purpose,rights_grant_hash) VALUES($1,$2,$3,'assembly',$4)", art.id, source.id, source.sha, source.manifestHash); err != nil {
			return 0, nil, err
		}
	}
	// Only earlier sealed published versions are eligible inputs. A quarantined
	// output cannot be an input, so every accepted edge strictly precedes sealing.
	slices.Sort(rights)
	rights = slices.Compact(rights)
	attributed := map[string]string{}
	for _, at := range m.Attribution {
		if !validID(at.PrincipalID) || len(at.Text) < 1 || len(at.Text) > 1000 || attributed[at.PrincipalID] != "" {
			return 0, nil, fail(422, "invalid_attribution")
		}
		attributed[at.PrincipalID] = at.Text
	}
	if attributed[art.principal] == "" {
		return 0, nil, fail(422, "contributor_attribution_required")
	}
	for _, input := range m.Inputs {
		source, _ := loadArtifact(r.Context(), tx, input.ArtifactID, false)
		var sm verification.Manifest
		if err = json.Unmarshal(source.manifest, &sm); err != nil {
			return 0, nil, err
		}
		for _, at := range sm.Attribution {
			if attributed[at.PrincipalID] != at.Text {
				return 0, nil, fail(422, "input_attribution_required")
			}
		}
	}
	manifest := canonical(m)
	if len(manifest) > verification.MaxManifestBytes {
		return 0, nil, fail(422, "manifest_limit")
	}
	manifestHash := digest(manifest)
	if _, err = tx.Exec(r.Context(), `UPDATE artifacts SET state='sealed',sha256=$2,manifest=$3,manifest_bytes=$4,manifest_hash=$5,plan_hash=$6,agreement_hash=$7,license_id=$8,required_rights=$9,sealed_at=now(),version=version+1 WHERE id=$1`, art.id, sha, manifest, manifest, manifestHash, p.hash, p.sealed.TemplateHash, m.LicenseID, rights); err != nil {
		return 0, nil, err
	}
	if art.purpose == "output" {
		if _, err = tx.Exec(r.Context(), "UPDATE tasks SET state='submitted',submitted_artifact_id=$2,version=version+1 WHERE id=$1", t.id, art.id); err != nil {
			return 0, nil, err
		}
	}
	if err = enqueueArtifactEvaluation(r.Context(), tx, art.id, art.generation, manifestHash); err != nil {
		return 0, nil, err
	}
	if err = s.roomEvent(r.Context(), tx, a, art.id, 2, "artifact.sealed", art.swarm, "/v1/artifacts/"+art.id); err != nil {
		return 0, nil, err
	}
	art, err = loadArtifact(r.Context(), tx, art.id, false)
	return 200, artifactProjection(art), err
}
func artifactEvaluations(ctx context.Context, tx pgx.Tx, id string) (int, any, error) {
	rows, err := tx.Query(ctx, "SELECT id,evidence FROM evaluations WHERE artifact_id=$1 ORDER BY created_at,id", id)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id string
		var raw []byte
		if err = rows.Scan(&id, &raw); err != nil {
			return 0, nil, err
		}
		var item map[string]any
		if err = json.Unmarshal(raw, &item); err != nil {
			return 0, nil, err
		}
		item["id"] = id
		item["limitations"] = artifactLimitations
		item["runtime_isolation"] = "development_process_not_qualified"
		item["image_identity_evidence"] = "operator_configured_fixture_not_container_attestation"
		items = append(items, item)
	}
	return 200, map[string]any{"items": items}, rows.Err()
}
func (s *Service) acceptArtifact(r *http.Request, tx pgx.Tx, a actor, p storedPlan, t storedTask, art storedArtifact) (int, any, error) {
	var in struct {
		Evaluation string `json:"evaluation_id"`
		SHA        string `json:"artifact_sha256"`
		PlanHash   string `json:"plan_hash"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if !slices.Contains(p.sealed.Plan.AcceptancePrincipals, a.PrincipalID) {
		return 0, nil, fail(403, "designated_acceptance_principal_required")
	}
	if art.purpose != "output" || !validID(in.Evaluation) || in.SHA != art.sha || in.PlanHash != p.hash || !slices.Contains([]string{"verified", "published"}, art.state) {
		return 0, nil, fail(409, "exact_passing_evaluation_required")
	}
	var passed bool
	err := tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM evaluations WHERE id=$1 AND artifact_id=$2 AND generation=$3 AND input_sha256=$4 AND manifest_sha256=$5 AND result='passed')", in.Evaluation, art.id, art.generation, art.sha, art.manifestHash).Scan(&passed)
	if err != nil {
		return 0, nil, err
	}
	if !passed {
		return 0, nil, fail(409, "exact_passing_evaluation_required")
	}
	_, policyVersion, err := s.requireRights(r.Context(), tx, a, p.sealed.Plan.Rights)
	if err != nil {
		return 0, nil, err
	}
	if err = s.artifactApproval(r.Context(), tx, a, p, art, "acceptance", art.sha, &in.Evaluation, policyVersion); err != nil {
		return 0, nil, err
	}
	var count int
	if err = tx.QueryRow(r.Context(), "SELECT artifact_current_approval_count($1,'acceptance')", art.id).Scan(&count); err != nil {
		return 0, nil, err
	}
	if count == len(p.sealed.Plan.AcceptancePrincipals) && t.state != "accepted_output" {
		if _, err = tx.Exec(r.Context(), "UPDATE tasks SET state='accepted_output',version=version+1 WHERE id=$1 AND submitted_artifact_id=$2", t.id, art.id); err != nil {
			return 0, nil, err
		}
		if err = s.roomEvent(r.Context(), tx, a, t.id, t.version+1, "task.output_accepted", p.swarm, "/v1/tasks/"+t.id); err != nil {
			return 0, nil, err
		}
	}
	return 200, map[string]any{"artifact_id": art.id, "accepted": true, "evaluation_id": in.Evaluation, "artifact_sha256": art.sha}, nil
}
func (s *Service) artifactApproval(ctx context.Context, tx pgx.Tx, a actor, p storedPlan, art storedArtifact, purpose, hash string, evaluation *string, version int64) error {
	id := NewID()
	tag, err := tx.Exec(ctx, `INSERT INTO approvals(id,principal_id,agent_id,purpose,plan_id,artifact_id,evaluation_id,content_hash,decision,policy_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'approve',$9) ON CONFLICT DO NOTHING`, id, a.PrincipalID, a.AgentID, purpose, p.id, art.id, evaluation, hash, version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var recorded int64
		var previous *string
		if err = tx.QueryRow(ctx, "SELECT policy_version,evaluation_id FROM approvals WHERE artifact_id=$1 AND principal_id=$2 AND purpose=$3 AND content_hash=$4", art.id, a.PrincipalID, purpose, hash).Scan(&recorded, &previous); err != nil {
			return err
		}
		if recorded != version || (evaluation != nil && (previous == nil || *previous != *evaluation)) {
			return fail(409, "approval_requires_new_artifact")
		}
		return nil
	}
	return s.roomEvent(ctx, tx, a, id, 1, "artifact."+purpose+"_approved", art.swarm, "/v1/artifacts/"+art.id)
}
func (s *Service) approveArtifactPublication(r *http.Request, tx pgx.Tx, a actor, p storedPlan, art storedArtifact) (int, any, error) {
	var in struct {
		Hash    string `json:"manifest_hash"`
		License string `json:"license_id"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if !slices.Contains(art.rights, a.PrincipalID) {
		return 0, nil, fail(403, "required_rights_principal_required")
	}
	if art.purpose != "output" || in.Hash != art.manifestHash || in.License != art.license || !slices.Contains([]string{"sealed", "verified", "published"}, art.state) {
		return 0, nil, fail(409, "exact_manifest_required")
	}
	_, version, err := s.requireRights(r.Context(), tx, a, p.sealed.Plan.Rights)
	if err != nil {
		return 0, nil, err
	}
	if err = s.artifactApproval(r.Context(), tx, a, p, art, "publication", art.manifestHash, nil, version); err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"artifact_id": art.id, "manifest_hash": art.manifestHash, "approved": true}, nil
}
func (s *Service) publishArtifact(r *http.Request, tx pgx.Tx, a actor, p storedPlan, t storedTask, art storedArtifact) (int, any, error) {
	var in struct {
		Hash string `json:"manifest_hash"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	var coordinator bool
	if err := tx.QueryRow(r.Context(), "SELECT swarm_coordinator($1)", p.swarm).Scan(&coordinator); err != nil {
		return 0, nil, err
	}
	if !coordinator {
		return 0, nil, fail(403, "coordinator_required")
	}
	if art.purpose != "output" || in.Hash != art.manifestHash || art.state != "verified" || t.state != "accepted_output" {
		return 0, nil, fail(409, "publication_not_ready")
	}
	if _, err := s.artifactBytes(art); err != nil {
		return 0, nil, err
	}
	if err := publicationGates(r.Context(), tx, p, art); err != nil {
		return 0, nil, err
	}
	// Revalidate immutable input grants and bytes immediately before the public
	// transition. A missing, tampered or held input never gets laundered by output.
	rows, err := tx.Query(r.Context(), "SELECT input_artifact_id,input_sha256,rights_grant_hash FROM artifact_inputs WHERE artifact_id=$1 ORDER BY input_artifact_id", art.id)
	if err != nil {
		return 0, nil, err
	}
	type link struct{ id, sha, grant string }
	links := []link{}
	for rows.Next() {
		var x link
		if err = rows.Scan(&x.id, &x.sha, &x.grant); err != nil {
			rows.Close()
			return 0, nil, err
		}
		links = append(links, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, nil, err
	}
	for _, x := range links {
		input, err := loadArtifact(r.Context(), tx, x.id, false)
		if err != nil {
			return 0, nil, err
		}
		if input.state != "published" || input.sha != x.sha || input.manifestHash != x.grant || input.license != art.license {
			return 0, nil, fail(409, "input_rights_not_established")
		}
		if _, err = s.artifactBytes(input); err != nil {
			return 0, nil, err
		}
	}
	if _, err = tx.Exec(r.Context(), "UPDATE artifacts SET state='published',published_at=now(),version=version+1 WHERE id=$1", art.id); err != nil {
		return 0, nil, err
	}
	if err = s.roomEvent(r.Context(), tx, a, art.id, art.version+1, "artifact.published", art.swarm, "/v1/artifacts/"+art.id); err != nil {
		return 0, nil, err
	}
	art.state = "published"
	return 200, artifactProjection(art), nil
}
func publicationGates(ctx context.Context, tx pgx.Tx, p storedPlan, art storedArtifact) error {
	for purpose, set := range map[string][]string{"acceptance": p.sealed.Plan.AcceptancePrincipals, "publication": art.rights} {
		var count int
		err := tx.QueryRow(ctx, "SELECT artifact_current_approval_count($1,$2)", art.id, purpose).Scan(&count)
		if err != nil {
			return err
		}
		if count != len(set) {
			return fail(409, "required_"+purpose+"_approvals_missing_or_stale")
		}
	}
	var planRights bool
	if err := tx.QueryRow(ctx, "SELECT artifact_current_plan_rights($1)", art.id).Scan(&planRights); err != nil {
		return err
	}
	if !planRights {
		return fail(409, "plan_rights_missing_or_stale")
	}
	var passing bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM evaluations WHERE artifact_id=$1 AND generation=$2 AND input_sha256=$3 AND manifest_sha256=$4 AND result='passed')", art.id, art.generation, art.sha, art.manifestHash).Scan(&passing); err != nil {
		return err
	}
	if !passing {
		return fail(409, "passing_evaluation_required")
	}
	return nil
}
func listPublishedArtifacts(r *http.Request, tx pgx.Tx) (int, any, error) {
	rows, err := tx.Query(r.Context(), "SELECT id FROM artifacts WHERE state='published' ORDER BY published_at DESC,id LIMIT 100")
	if err != nil {
		return 0, nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, nil, err
	}
	items := []any{}
	for _, id := range ids {
		a, err := loadArtifact(r.Context(), tx, id, false)
		if err != nil {
			return 0, nil, err
		}
		items = append(items, artifactProjection(a))
	}
	return 200, map[string]any{"items": items}, nil
}
func (s *Service) artifactActorEvent(ctx context.Context, tx pgx.Tx, a actor, art storedArtifact, kind string, facts map[string]any) error {
	facts["artifact_id"] = art.id
	origin := "registered"
	if s.environment != "production" {
		origin = "release_test"
	}
	_, err := tx.Exec(ctx, "INSERT INTO events(id,aggregate_kind,aggregate_id,version,principal_id,agent_id,type,facts,origin) VALUES($1,'artifact_receipt',$1,1,$2,$3,$4,$5,$6)", NewID(), a.PrincipalID, a.AgentID, kind, canonical(facts), origin)
	return err
}
func (s *Service) recordArtifactReuse(r *http.Request, tx pgx.Tx, a actor, art storedArtifact) (int, any, error) {
	var in struct {
		SHA         string `json:"artifact_sha256"`
		Kind        string `json:"use_kind"`
		Declaration string `json:"declaration"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if in.SHA != art.sha || !slices.Contains([]string{"schema_validation", "analysis", "assembly", "other"}, in.Kind) || len(in.Declaration) < 1 || len(in.Declaration) > 2000 {
		return 0, nil, fail(422, "invalid_reuse_receipt")
	}
	var retrieved bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM events WHERE principal_id=$1 AND type='artifact.retrieved' AND facts->>'artifact_id'=$2 AND facts->>'artifact_sha256'=$3)`, a.PrincipalID, art.id, art.sha).Scan(&retrieved); err != nil {
		return 0, nil, err
	}
	if !retrieved {
		return 0, nil, fail(409, "exact_version_retrieval_required")
	}
	var thirdParty bool
	if err := tx.QueryRow(r.Context(), "SELECT artifact_third_party($1)", art.id).Scan(&thirdParty); err != nil {
		return 0, nil, err
	}
	out := map[string]any{"artifact_id": art.id, "artifact_sha256": art.sha, "principal_id": a.PrincipalID, "use_kind": in.Kind, "declaration": in.Declaration, "third_party": thirdParty, "evidence_level": "self_reported", "control_independence": "not_established_by_account_ids"}
	if err := s.artifactActorEvent(r.Context(), tx, a, art, "artifact.reuse_reported", out); err != nil {
		return 0, nil, err
	}
	return 201, out, nil
}
