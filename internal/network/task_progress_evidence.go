package network

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type taskProgressEvidenceInput struct {
	Epoch          int64    `json:"claim_epoch"`
	ArtifactID     string   `json:"artifact_id"`
	ArtifactSHA256 string   `json:"artifact_sha256"`
	EvaluationID   string   `json:"evaluation_id"`
	Report         string   `json:"report"`
	Audience       []string `json:"audience_principal_ids"`
	Reviewer       string   `json:"reviewer_principal_id"`
}

func validateProgressAudience(ctx context.Context, tx pgx.Tx, p storedPlan, submitter string, audience []string, reviewer string) ([]string, error) {
	if len(audience) == 0 || len(audience) > 50 || !validID(reviewer) || !slices.Contains(p.sealed.Plan.AcceptancePrincipals, reviewer) {
		return nil, fail(422, "invalid_progress_audience")
	}
	seen := make(map[string]bool, len(audience))
	normalized := append([]string(nil), audience...)
	for _, principal := range normalized {
		if !validID(principal) || seen[principal] || !slices.Contains(p.sealed.Plan.RequiredPrincipals, principal) {
			return nil, fail(422, "invalid_progress_audience")
		}
		seen[principal] = true
	}
	if !seen[reviewer] || !seen[submitter] {
		return nil, fail(422, "progress_audience_must_include_reporter_and_reviewer")
	}
	var eligible int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE swarm_id=$1 AND state='accepted' AND role<>'observer' AND principal_id=ANY($2::uuid[])`, p.swarm, normalized).Scan(&eligible); err != nil {
		return nil, err
	}
	if eligible != len(normalized) {
		return nil, fail(422, "progress_audience_not_current_member")
	}
	slices.Sort(normalized)
	return normalized, nil
}

type storedTaskProgressEvidence struct {
	id, swarm, plan, task, artifact, evaluation string
	planHash, artifactSHA, report, reportSHA    string
	reviewer, submitter, submitterAgent, state  string
	epoch                                       int64
	audience                                    []string
	created                                     time.Time
}

func loadTaskProgressEvidence(r *http.Request, tx pgx.Tx, taskID, evidenceID string, lock bool) (storedTaskProgressEvidence, error) {
	var e storedTaskProgressEvidence
	query := `SELECT id,swarm_id,plan_id,task_id,artifact_id,evaluation_id,claim_epoch,plan_hash,artifact_sha256,
 report,report_sha256,audience_principal_ids::text[],reviewer_principal_id,submitted_by_principal_id,
 submitted_by_agent_id,state,created_at FROM task_progress_evidence WHERE task_id=$1 AND id=$2`
	if lock {
		query += " FOR UPDATE"
	}
	err := tx.QueryRow(r.Context(), query, taskID, evidenceID).Scan(&e.id, &e.swarm, &e.plan, &e.task, &e.artifact, &e.evaluation,
		&e.epoch, &e.planHash, &e.artifactSHA, &e.report, &e.reportSHA, &e.audience, &e.reviewer, &e.submitter, &e.submitterAgent, &e.state, &e.created)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, fail(404, "not_found")
	}
	return e, err
}

func taskProgressEvidenceProjection(e storedTaskProgressEvidence) any {
	return map[string]any{
		"id": e.id, "plan_id": e.plan, "task_id": e.task, "claim_epoch": e.epoch,
		"artifact_id": e.artifact, "artifact_sha256": e.artifactSHA, "evaluation_id": e.evaluation,
		"report": e.report, "report_sha256": e.reportSHA, "audience_principal_ids": e.audience,
		"reviewer_principal_id": e.reviewer, "state": e.state, "created_at": e.created,
		"limitations": "A passing fixed verifier and explicit progress review do not prove factual truth, completeness, legal ownership, or output acceptance.",
	}
}

func (s *Service) taskProgressEvidenceCommand(r *http.Request, tx pgx.Tx, a actor, taskID string) (int, any, error) {
	t, err := loadTask(r.Context(), tx, taskID, false)
	if err != nil {
		return 0, nil, err
	}
	p, err := loadPlan(r.Context(), tx, t.plan, true)
	if err != nil {
		return 0, nil, err
	}
	// Keep lock order aligned with other task transitions: plan before task.
	t, err = loadTask(r.Context(), tx, taskID, true)
	if err != nil {
		return 0, nil, err
	}
	if t.agent != a.AgentID {
		return 0, nil, fail(403, "assignee_required")
	}
	if r.Header.Get("If-Match") == "" {
		return 0, nil, fail(428, "if_match_required")
	}
	if r.Header.Get("If-Match") != etag(t.version) {
		return 0, nil, fail(409, "version_conflict")
	}
	var in taskProgressEvidenceInput
	if err = decode(r, &in); err != nil {
		return 0, nil, err
	}
	if err = s.validateTaskProgress(r, tx, a, p, t, in.Epoch); err != nil {
		return 0, nil, err
	}
	if !validID(in.ArtifactID) || !validID(in.EvaluationID) || len(in.Report) == 0 || len(in.Report) > 4000 || strings.TrimSpace(in.Report) == "" || len(in.ArtifactSHA256) != 64 {
		return 0, nil, fail(422, "invalid_progress_evidence")
	}
	in.Audience, err = validateProgressAudience(r.Context(), tx, p, a.PrincipalID, in.Audience, in.Reviewer)
	if err != nil {
		return 0, nil, err
	}
	template, enabled := s.templates[p.sealed.Plan.Rights.TemplateID]
	if !enabled || template.SHA256 != p.sealed.TemplateHash {
		return 0, nil, fail(409, "plan_terms_expired_or_changed")
	}
	var currentHead *string
	if err = tx.QueryRow(r.Context(), "SELECT active_plan_id FROM swarms WHERE id=$1", p.swarm).Scan(&currentHead); err != nil {
		return 0, nil, err
	}
	if currentHead == nil || *currentHead != p.id || p.state != "accepted" {
		return 0, nil, fail(409, "task_not_current")
	}
	art, err := loadArtifact(r.Context(), tx, in.ArtifactID, true)
	if err != nil {
		return 0, nil, err
	}
	if err = s.currentArtifactAuthority(r.Context(), tx, a, p, t, art, true); err != nil {
		return 0, nil, err
	}
	if art.purpose != "progress" || art.state != "verified" || art.sha != in.ArtifactSHA256 || art.id == "" || art.swarm != p.swarm || art.plan != p.id || art.task != t.id || art.epoch != t.epoch || art.reviewer != in.Reviewer || !slices.Equal(art.audience, in.Audience) {
		return 0, nil, fail(409, "progress_artifact_not_current")
	}
	var passed bool
	if err = tx.QueryRow(r.Context(), `SELECT result='passed' AND generation=$2 AND input_sha256=$3 AND manifest_sha256=$4
		AND verifier_sha256<>'' AND image_sha256<>'' FROM evaluations WHERE id=$1 AND artifact_id=$5`, in.EvaluationID, art.generation, art.sha, art.manifestHash, art.id).Scan(&passed); errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(409, "progress_evaluation_not_current")
	} else if err != nil {
		return 0, nil, err
	}
	if !passed {
		return 0, nil, fail(409, "progress_evaluation_not_passed")
	}
	reportHash := digest([]byte(in.Report))
	var duplicate string
	if err = tx.QueryRow(r.Context(), `SELECT id FROM task_progress_evidence WHERE task_id=$1 AND claim_epoch=$2 AND artifact_id=$3 AND evaluation_id=$4 AND report_sha256=$5`, t.id, t.epoch, art.id, in.EvaluationID, reportHash).Scan(&duplicate); err == nil {
		return 0, nil, fail(409, "progress_evidence_replayed")
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, err
	}
	id := NewID()
	if _, err = tx.Exec(r.Context(), `INSERT INTO task_progress_evidence(id,swarm_id,plan_id,task_id,artifact_id,evaluation_id,claim_epoch,plan_hash,
		artifact_sha256,report,report_sha256,audience_principal_ids,reviewer_principal_id,submitted_by_principal_id,submitted_by_agent_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::uuid[],$13,$14,$15)`, id, p.swarm, p.id, t.id, art.id, in.EvaluationID,
		t.epoch, p.hash, art.sha, in.Report, reportHash, in.Audience, in.Reviewer, a.PrincipalID, a.AgentID); err != nil {
		return 0, nil, err
	}
	if err = s.roomEventAudience(r.Context(), tx, a, id, 1, "task.progress_evidence_submitted", p.swarm, "/v1/tasks/"+t.id, in.Audience); err != nil {
		return 0, nil, err
	}
	return 201, map[string]any{"id": id, "task_id": t.id, "plan_id": p.id, "claim_epoch": t.epoch, "artifact_id": art.id, "artifact_sha256": art.sha,
		"evaluation_id": in.EvaluationID, "report_sha256": reportHash, "audience_principal_ids": in.Audience, "reviewer_principal_id": in.Reviewer,
		"state": "pending", "progress_deadline": t.progress, "cure_deadline": t.cure,
		"limitations": "A passing fixed verifier and a submitted progress report do not establish actual quality."}, nil
}

func (s *Service) reviewTaskProgressEvidence(r *http.Request, tx pgx.Tx, a actor, taskID string) (int, any, error) {
	t, err := loadTask(r.Context(), tx, taskID, false)
	if err != nil {
		return 0, nil, err
	}
	p, err := loadPlan(r.Context(), tx, t.plan, true)
	if err != nil {
		return 0, nil, err
	}
	t, err = loadTask(r.Context(), tx, taskID, true)
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		EvidenceID string `json:"evidence_id"`
		Decision   string `json:"decision"`
		ReportSHA  string `json:"report_sha256"`
	}
	if err = decode(r, &in); err != nil {
		return 0, nil, err
	}
	if !validID(in.EvidenceID) || !slices.Contains([]string{"accept", "reject"}, in.Decision) {
		return 0, nil, fail(422, "invalid_progress_review")
	}
	e, err := loadTaskProgressEvidence(r, tx, t.id, in.EvidenceID, false)
	if err != nil {
		return 0, nil, err
	}
	if e.reviewer != a.PrincipalID || !slices.Contains(p.sealed.Plan.AcceptancePrincipals, a.PrincipalID) {
		return 0, nil, fail(403, "designated_progress_reviewer_required")
	}
	e, err = loadTaskProgressEvidence(r, tx, t.id, in.EvidenceID, true)
	if err != nil {
		return 0, nil, err
	}
	if e.state != "pending" || e.reportSHA != in.ReportSHA {
		return 0, nil, fail(409, "progress_evidence_not_pending_or_hash_mismatch")
	}
	if p.id != e.plan || p.hash != e.planHash || p.state != "accepted" || t.epoch != e.epoch || t.plan != e.plan || !slices.Contains([]string{"accepted", "in_progress", "changes_requested", "at_risk"}, t.state) {
		return 0, nil, fail(409, "progress_evidence_stale")
	}
	var head *string
	if err = tx.QueryRow(r.Context(), "SELECT active_plan_id FROM swarms WHERE id=$1", p.swarm).Scan(&head); err != nil {
		return 0, nil, err
	}
	if head == nil || *head != p.id {
		return 0, nil, fail(409, "progress_evidence_stale")
	}
	if err = progressWindowCurrent(t); err != nil {
		return 0, nil, err
	}
	_, policyVersion, err := s.requireRights(r.Context(), tx, a, p.sealed.Plan.Rights)
	if err != nil {
		return 0, nil, err
	}
	approvalID := NewID()
	approvalDecision := "reject"
	if in.Decision == "accept" {
		approvalDecision = "approve"
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO approvals(id,principal_id,agent_id,purpose,plan_id,artifact_id,evaluation_id,progress_evidence_id,content_hash,decision,policy_version)
		VALUES($1,$2,$3,'progress',$4,$5,$6,$7,$8,$9,$10)`, approvalID, a.PrincipalID, a.AgentID, p.id, e.artifact, e.evaluation, e.id, e.reportSHA, approvalDecision, policyVersion); err != nil {
		return 0, nil, err
	}
	state := "rejected"
	if in.Decision == "accept" {
		state = "accepted"
	}
	if _, err = tx.Exec(r.Context(), "UPDATE task_progress_evidence SET state=$2,reviewed_at=clock_timestamp() WHERE id=$1 AND state='pending'", e.id, state); err != nil {
		return 0, nil, err
	}
	if in.Decision == "accept" {
		if _, err = tx.Exec(r.Context(), `UPDATE tasks SET state='in_progress',version=version+1,
			progress_deadline=clock_timestamp()+(specification->>'progress_interval_seconds')::int*interval '1 second',cure_deadline=NULL,credited_progress_evidence_id=$2 WHERE id=$1`, t.id, e.id); err != nil {
			return 0, nil, err
		}
		if err = s.roomEventAudience(r.Context(), tx, a, t.id, t.version+1, "task.progress_evidence_accepted", p.swarm, "/v1/tasks/"+t.id, e.audience); err != nil {
			return 0, nil, err
		}
	} else if err = s.roomEventAudience(r.Context(), tx, a, e.id, 2, "task.progress_evidence_rejected", p.swarm, "/v1/tasks/"+t.id, e.audience); err != nil {
		return 0, nil, err
	}
	return taskResult(r.Context(), tx, t.id, a.PrincipalID)
}

func (s *Service) readTaskProgressEvidence(r *http.Request, tx pgx.Tx, a actor, taskID, evidenceID string) (int, any, error) {
	if !slices.Contains(a.Scopes, "swarms:read") {
		return 0, nil, fail(403, "insufficient_scope")
	}
	e, err := loadTaskProgressEvidence(r, tx, taskID, evidenceID, false)
	if err != nil {
		return 0, nil, err
	}
	return 200, taskProgressEvidenceProjection(e), nil
}
