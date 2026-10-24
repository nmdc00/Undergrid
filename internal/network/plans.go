package network

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type storedPlan struct {
	id, swarm, state, hash string
	version                int64
	sealed                 sealedPlan
	parent                 *string
}

func loadPlan(ctx context.Context, tx pgx.Tx, id string, lock bool) (storedPlan, error) {
	var p storedPlan
	var raw []byte
	query := "SELECT id,swarm_id,state,content_hash,version,specification,parent_id FROM plan_versions WHERE id=$1"
	if lock {
		query += " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, query, id).Scan(&p.id, &p.swarm, &p.state, &p.hash, &p.version, &raw, &p.parent)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, fail(404, "not_found")
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(raw, &p.sealed)
	return p, err
}
func planResult(ctx context.Context, tx pgx.Tx, p storedPlan) (any, error) {
	rows, err := tx.Query(ctx, "SELECT id,state,version,claim_epoch,response_deadline,progress_deadline,cure_deadline,dependency_blocked FROM tasks WHERE plan_id=$1 ORDER BY task_key", p.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []any{}
	for rows.Next() {
		var id, state string
		var version, epoch int64
		var response time.Time
		var progress, cure *time.Time
		var blocked bool
		if err = rows.Scan(&id, &state, &version, &epoch, &response, &progress, &cure, &blocked); err != nil {
			return nil, err
		}
		tasks = append(tasks, map[string]any{"id": id, "state": state, "version": version, "etag": etag(version), "claim_epoch": epoch, "response_deadline": response, "progress_deadline": progress, "cure_deadline": cure, "dependency_blocked": blocked})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"id": p.id, "swarm_id": p.swarm, "state": p.state, "version": p.version, "content_hash": p.hash, "canonical_json": string(canonical(p.sealed)), "specification": p.sealed.Plan, "tasks": tasks}, nil
}
func (s *Service) proposePlan(r *http.Request, tx pgx.Tx, a actor, swarm string) (int, any, error) {
	if err := roomAuthority(r.Context(), tx, a, swarm, false); err != nil {
		return 0, nil, err
	}
	var contributor bool
	if err := tx.QueryRow(r.Context(), "SELECT swarm_contributor($1)", swarm).Scan(&contributor); err != nil {
		return 0, nil, err
	}
	if !contributor {
		return 0, nil, fail(403, "contributor_required")
	}
	var in PlanSpec
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if err := in.validate(); err != nil {
		return 0, nil, err
	}
	template, ok := s.templates[in.Rights.TemplateID]
	if !ok || !slices.Contains(template.LicenseIDs, in.Rights.LicenseID) || in.Rights.Publication != "exact_manifest_approval" {
		return 0, nil, fail(422, "terms_not_enabled")
	}
	var roomVersion, nextVersion int64
	if err := tx.QueryRow(r.Context(), "SELECT version FROM swarms WHERE id=$1", swarm).Scan(&roomVersion); err != nil {
		return 0, nil, err
	}
	if r.Header.Get("If-Match") == "" {
		return 0, nil, fail(428, "if_match_required")
	}
	if r.Header.Get("If-Match") != etag(roomVersion) {
		return 0, nil, fail(409, "version_conflict")
	}
	if err := tx.QueryRow(r.Context(), "SELECT COALESCE(max(version),0)+1 FROM plan_versions WHERE swarm_id=$1", swarm).Scan(&nextVersion); err != nil {
		return 0, nil, err
	}
	var parent *string
	if nextVersion == 1 {
		if in.ParentVersion != nil {
			return 0, nil, fail(409, "parent_version_conflict")
		}
	} else {
		if in.ParentVersion == nil || *in.ParentVersion != nextVersion-1 {
			return 0, nil, fail(409, "parent_version_conflict")
		}
		var id string
		if err := tx.QueryRow(r.Context(), "SELECT id FROM plan_versions WHERE swarm_id=$1 AND version=$2", swarm, *in.ParentVersion).Scan(&id); err != nil {
			return 0, nil, err
		}
		parent = &id
	}
	rows, err := tx.Query(r.Context(), "SELECT agent_id,principal_id,role FROM memberships WHERE swarm_id=$1 AND state='accepted'", swarm)
	if err != nil {
		return 0, nil, err
	}
	agents := map[string]string{}
	members := map[string]bool{}
	principals := map[string]bool{}
	for rows.Next() {
		var agent, principal, role string
		if err = rows.Scan(&agent, &principal, &role); err != nil {
			rows.Close()
			return 0, nil, err
		}
		members[principal] = true
		if role != "observer" {
			agents[agent] = principal
			principals[principal] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, nil, err
	}
	for _, principal := range in.RequiredPrincipals {
		if !principals[principal] {
			return 0, nil, fail(422, "principal_not_contributor")
		}
	}
	for _, principal := range in.PublicationPrincipals {
		if !members[principal] {
			return 0, nil, fail(422, "publication_principal_not_member")
		}
	}
	for _, task := range in.Tasks {
		principal, ok := agents[task.AssigneeAgentID]
		if !ok || !slices.Contains(in.RequiredPrincipals, principal) {
			return 0, nil, fail(422, "assignee_must_approve_plan")
		}
	}
	in.RespondBy = in.RespondBy.UTC()
	sealed := sealedPlan{Plan: in, TemplateHash: template.SHA256, InputGrantPolicyHash: inputGrantPolicyHash(template)}
	hash := digest(canonical(sealed))
	id := NewID()
	if _, err = tx.Exec(r.Context(), "INSERT INTO plan_versions(id,swarm_id,parent_id,version,specification,content_hash,respond_by) VALUES($1,$2,$3,$4,$5,$6,$7)", id, swarm, parent, nextVersion, canonical(sealed), hash, in.RespondBy); err != nil {
		return 0, nil, err
	}
	deadline := time.Now().Add(48 * time.Hour)
	if in.RespondBy.Before(deadline) {
		deadline = in.RespondBy
	}
	for _, task := range in.Tasks {
		if _, err = tx.Exec(r.Context(), "INSERT INTO tasks(id,plan_id,task_key,specification,specification_hash,assigned_agent_id,assigned_principal_id,response_deadline,final_deadline) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)", NewID(), id, task.Key, canonical(task), digest(canonical(task)), task.AssigneeAgentID, agents[task.AssigneeAgentID], deadline, task.Deadline); err != nil {
			return 0, nil, err
		}
	}
	if _, err = tx.Exec(r.Context(), "UPDATE swarms SET version=version+1 WHERE id=$1", swarm); err != nil {
		return 0, nil, err
	}
	if err = s.roomEvent(r.Context(), tx, a, id, 1, "plan.proposed", swarm, "/v1/plans/"+id); err != nil {
		return 0, nil, err
	}
	result, err := planResult(r.Context(), tx, storedPlan{id: id, swarm: swarm, state: "proposed", hash: hash, version: nextVersion, sealed: sealed, parent: parent})
	return 201, result, err
}
func (s *Service) planCommand(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 || len(parts) > 6 || !validID(parts[3]) {
		return 0, nil, fail(404, "not_found")
	}
	id := parts[3]
	if r.Method == "GET" && parts[2] == "tasks" && len(parts) == 6 && parts[4] == "progress-evidence" && validID(parts[5]) {
		// The evidence row supplies the room binding; row security additionally
		// restricts report reads to its finite named audience.
		return s.readTaskProgressEvidence(r, tx, a, id, parts[5])
	}
	if r.Method == "GET" && len(parts) == 4 {
		if !slices.Contains(a.Scopes, "swarms:read") {
			return 0, nil, insufficientScope(a, "swarms:read")
		}
		if parts[2] == "tasks" {
			return taskResult(r.Context(), tx, id, a.PrincipalID)
		}
		p, err := loadPlan(r.Context(), tx, id, false)
		if err != nil {
			return 0, nil, err
		}
		result, err := planResult(r.Context(), tx, p)
		return 200, result, err
	}
	if r.Method != "POST" || len(parts) != 5 {
		return 0, nil, fail(404, "not_found")
	}
	scope := ""
	if parts[2] == "plans" && parts[4] == "approvals" {
		scope = "plans:approve"
	}
	if parts[2] == "tasks" {
		scope = map[string]string{"assignment-response": "tasks:accept", "progress": "tasks:progress", "progress-evidence": "tasks:progress", "progress-evidence-review": "tasks:accept"}[parts[4]]
	}
	if scope == "" {
		return 0, nil, fail(404, "not_found")
	}
	if !slices.Contains(a.Scopes, scope) {
		return 0, nil, insufficientScope(a, scope)
	}
	planID := id
	if parts[2] == "tasks" {
		if err := tx.QueryRow(r.Context(), "SELECT plan_id FROM tasks WHERE id=$1", id).Scan(&planID); errors.Is(err, pgx.ErrNoRows) {
			return 0, nil, fail(404, "not_found")
		} else if err != nil {
			return 0, nil, err
		}
	}
	p, err := loadPlan(r.Context(), tx, planID, false)
	if err != nil {
		return 0, nil, err
	}
	if err = roomAuthority(r.Context(), tx, a, p.swarm, false); err != nil {
		return 0, nil, err
	}
	return idempotent(r, tx, a, func() (int, any, error) {
		if parts[2] == "plans" {
			return s.approvePlan(r, tx, a, planID)
		}
		if parts[4] == "progress-evidence" {
			return s.taskProgressEvidenceCommand(r, tx, a, id)
		}
		if parts[4] == "progress-evidence-review" {
			return s.reviewTaskProgressEvidence(r, tx, a, id)
		}
		return s.updateTask(r, tx, a, id, parts[4])
	})
}
func (s *Service) approvePlan(r *http.Request, tx pgx.Tx, a actor, id string) (int, any, error) {
	p, err := loadPlan(r.Context(), tx, id, true)
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		Decision string `json:"decision"`
		Hash     string `json:"content_hash"`
	}
	if err = decode(r, &in); err != nil {
		return 0, nil, err
	}
	if !slices.Contains([]string{"approve", "reject"}, in.Decision) {
		return 0, nil, fail(422, "invalid_decision")
	}
	if p.hash != in.Hash || p.state != "proposed" || !p.sealed.Plan.RespondBy.After(time.Now()) {
		return 0, nil, fail(409, "plan_not_current")
	}
	if !slices.Contains(p.sealed.Plan.RequiredPrincipals, a.PrincipalID) {
		return 0, nil, fail(403, "not_required_approver")
	}
	_, policyVersion, err := s.requireRights(r.Context(), tx, a, p.sealed.Plan.Rights)
	if err != nil {
		return 0, nil, err
	}
	for _, purpose := range []string{"plan", "rights"} {
		approvalID := NewID()
		tag, err := tx.Exec(r.Context(), "INSERT INTO approvals(id,principal_id,agent_id,purpose,plan_id,content_hash,decision,policy_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING", approvalID, a.PrincipalID, a.AgentID, purpose, id, p.hash, in.Decision, policyVersion)
		if err != nil {
			return 0, nil, err
		}
		if tag.RowsAffected() > 0 {
			if err = s.roomEvent(r.Context(), tx, a, approvalID, 1, "plan."+purpose+"_"+in.Decision, p.swarm, "/v1/plans/"+p.id); err != nil {
				return 0, nil, err
			}
		}
		if tag.RowsAffected() == 0 {
			var decision string
			var recordedVersion int64
			if err = tx.QueryRow(r.Context(), "SELECT decision,policy_version FROM approvals WHERE plan_id=$1 AND principal_id=$2 AND purpose=$3 AND content_hash=$4", id, a.PrincipalID, purpose, p.hash).Scan(&decision, &recordedVersion); err != nil {
				return 0, nil, err
			}
			if decision != in.Decision || recordedVersion != policyVersion {
				return 0, nil, fail(409, "approval_requires_new_revision")
			}
		}
	}
	if in.Decision == "reject" {
		if _, err = tx.Exec(r.Context(), "UPDATE plan_versions SET state='rejected' WHERE id=$1", id); err != nil {
			return 0, nil, err
		}
		if err = s.roomEvent(r.Context(), tx, a, p.id, 2, "plan.rejected", p.swarm, "/v1/plans/"+p.id); err != nil {
			return 0, nil, err
		}
	} else {
		if err = s.maybeAcceptPlan(r.Context(), tx, a, p); err != nil {
			return 0, nil, err
		}
	}
	p, err = loadPlan(r.Context(), tx, id, false)
	if err != nil {
		return 0, nil, err
	}
	result, err := planResult(r.Context(), tx, p)
	return 200, result, err
}
func (s *Service) maybeAcceptPlan(ctx context.Context, tx pgx.Tx, a actor, p storedPlan) error {
	if p.state != "proposed" {
		return nil
	}
	template, enabled := s.templates[p.sealed.Plan.Rights.TemplateID]
	if !enabled || template.SHA256 != p.sealed.TemplateHash || !p.sealed.Plan.RespondBy.After(time.Now()) {
		return fail(409, "plan_terms_expired_or_changed")
	}
	var votes, rights, unaccepted, missingMembers int
	if err := tx.QueryRow(ctx, "SELECT count(*) FILTER(WHERE purpose='plan'),count(*) FILTER(WHERE purpose='rights') FROM approvals WHERE plan_id=$1 AND content_hash=$2 AND principal_id=ANY($3) AND decision='approve' AND policy_version=current_policy_version(principal_id)", p.id, p.hash, p.sealed.Plan.RequiredPrincipals).Scan(&votes, &rights); err != nil {
		return err
	}
	if votes != len(p.sealed.Plan.RequiredPrincipals) || rights != votes {
		return nil
	}
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM tasks WHERE plan_id=$1 AND (state<>'accepted' OR dependency_blocked)", p.id).Scan(&unaccepted); err != nil {
		return err
	}
	if unaccepted != 0 {
		return nil
	}
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM unnest($1::uuid[]) p WHERE NOT EXISTS(SELECT 1 FROM memberships m WHERE m.swarm_id=$2 AND m.principal_id=p AND m.state='accepted' AND m.role<>'observer')", p.sealed.Plan.RequiredPrincipals, p.swarm).Scan(&missingMembers); err != nil {
		return err
	}
	if missingMembers != 0 {
		return fail(409, "approver_membership_changed")
	}
	var head *string
	if err := tx.QueryRow(ctx, "SELECT active_plan_id FROM swarms WHERE id=$1", p.swarm).Scan(&head); err != nil {
		return err
	}
	if head != nil {
		var ancestor bool
		if err := tx.QueryRow(ctx, "WITH RECURSIVE lineage AS (SELECT id,parent_id FROM plan_versions WHERE id=$1 UNION ALL SELECT p.id,p.parent_id FROM plan_versions p JOIN lineage l ON p.id=l.parent_id) SELECT EXISTS(SELECT 1 FROM lineage WHERE id=$2)", p.id, *head).Scan(&ancestor); err != nil {
			return err
		}
		if !ancestor {
			return fail(409, "plan_head_changed")
		}
		if _, err := tx.Exec(ctx, "UPDATE plan_versions SET state='superseded' WHERE id=$1", *head); err != nil {
			return err
		}
		if err := s.roomEvent(ctx, tx, a, *head, 3, "plan.superseded", p.swarm, "/v1/plans/"+*head); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE tasks SET state='cancelled',version=version+1,claim_epoch=claim_epoch+1 WHERE plan_id=$1 AND state NOT IN ('accepted_output','abandoned','cancelled')", *head); err != nil {
			return err
		}
	}
	// Execution starts only after every exact plan and assignment approval.
	if _, err := tx.Exec(ctx, "UPDATE tasks SET progress_deadline=$2::timestamptz+(specification->>'progress_interval_seconds')::int*interval '1 second' WHERE plan_id=$1 AND state='accepted'", p.id, time.Now().UTC()); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE plan_versions SET state='accepted' WHERE id=$1", p.id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE swarms SET active_plan_id=$2,version=version+1 WHERE id=$1", p.swarm, p.id); err != nil {
		return err
	}
	return s.roomEvent(ctx, tx, a, p.id, 2, "plan.accepted", p.swarm, "/v1/plans/"+p.id)
}
