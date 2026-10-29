package network

import (
	"bytes"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"strings"
)

func ownerContributionRoute(path string) bool {
	p := strings.Split(strings.Trim(path, "/"), "/")
	return len(p) == 7 && p[0] == "v1" && p[1] == "owner" && p[2] == "agents" && validID(p[3]) && p[4] == "tasks" && validID(p[5]) && (p[6] == "artifact-uploads" || p[6] == "artifacts")
}
func (s *Service) ownerContribution(r *http.Request, tx pgx.Tx, principal string) (int, any, error) {
	p := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	listing := p[6] == "artifacts"
	if listing && r.Method != http.MethodGet || !listing && r.Method != http.MethodPost {
		return 0, nil, fail(405, "method_not_allowed")
	}
	a, err := lockOwnerAgent(r.Context(), tx, principal, p[3])
	if err != nil {
		return 0, nil, err
	}
	if listing {
		return ownerTaskArtifacts(r, tx, a, p[5])
	}
	if r.Header.Get("If-Match") == "" {
		return 0, nil, fail(428, "if_match_required")
	}
	var input struct {
		Epoch    int64    `json:"claim_epoch"`
		Media    string   `json:"media_type"`
		Size     int64    `json:"size_bytes"`
		Purpose  string   `json:"purpose,omitempty"`
		Audience []string `json:"audience_principal_ids,omitempty"`
		Reviewer string   `json:"reviewer_principal_id,omitempty"`
	}
	if err = decode(r, &input); err != nil {
		return 0, nil, err
	}
	task, err := loadTask(r.Context(), tx, p[5], false)
	if err != nil {
		return 0, nil, err
	}
	plan, err := loadPlan(r.Context(), tx, task.plan, false)
	if err != nil {
		return 0, nil, err
	}
	a.ownerAuthenticated = true
	a.Scopes = []string{"artifacts:submit", "rights:accept"}
	command := r.Clone(r.Context())
	command.URL.Path = "/v1/artifacts/uploads"
	command.Body = io.NopCloser(bytes.NewReader(canonical(artifactUploadInput{Swarm: plan.swarm, Plan: plan.id, Task: task.id, Epoch: input.Epoch, Media: input.Media, Size: input.Size, Purpose: input.Purpose, Audience: input.Audience, Reviewer: input.Reviewer})))
	return s.artifactCommand(command, tx, a)
}

// Discovery exposes only RLS-visible metadata, never an upload capability or a
// renewed generation. Recovery of byte transfer is a separate authorized action.
func ownerTaskArtifacts(r *http.Request, tx pgx.Tx, a actor, taskID string) (int, any, error) {
	task, err := loadTask(r.Context(), tx, taskID, false)
	if err != nil {
		return 0, nil, err
	}
	if task.agent != a.AgentID {
		return 0, nil, fail(403, "assignee_required")
	}
	plan, err := loadPlan(r.Context(), tx, task.plan, false)
	if err != nil {
		return 0, nil, err
	}
	if err = roomAuthority(r.Context(), tx, a, plan.swarm, false); err != nil {
		return 0, nil, err
	}
	after := r.URL.Query().Get("after_id")
	if after != "" && !validID(after) {
		return 0, nil, fail(400, "invalid_cursor")
	}
	var rows pgx.Rows
	if after == "" {
		rows, err = tx.Query(r.Context(), "SELECT id FROM artifacts WHERE task_id=$1 AND agent_id=$2 ORDER BY id LIMIT 101", taskID, a.AgentID)
	} else {
		rows, err = tx.Query(r.Context(), "SELECT id FROM artifacts WHERE task_id=$1 AND agent_id=$2 AND id>$3 ORDER BY id LIMIT 101", taskID, a.AgentID, after)
	}
	if err != nil {
		return 0, nil, err
	}
	// Close the bounded result before loading projections on this connection.
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, nil, err
	}
	next := ""
	if len(ids) > 100 {
		next = ids[99]
		ids = ids[:100]
	}
	items := []any{}
	for _, id := range ids {
		artifact, err := loadArtifact(r.Context(), tx, id, false)
		if err != nil {
			return 0, nil, err
		}
		items = append(items, artifactProjectionWithContext(artifact, true))
	}
	return 200, map[string]any{"items": items, "next_after_id": next}, nil
}

func ownerArtifactRoute(path string) bool {
	p := strings.Split(strings.Trim(path, "/"), "/")
	if (len(p) != 6 && len(p) != 7) || p[0] != "v1" || p[1] != "owner" || p[2] != "agents" || !validID(p[3]) || p[4] != "artifacts" || !validID(p[5]) {
		return false
	}
	return len(p) == 6 || p[6] == "upload" || p[6] == "finalize" || p[6] == "evaluations" || p[6] == "content"
}
func (s *Service) ownerArtifact(r *http.Request, tx pgx.Tx, principal string) (int, any, error) {
	p := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	valid := len(p) == 6 && r.Method == http.MethodGet
	if len(p) == 7 {
		valid = p[6] == "upload" && r.Method == http.MethodPut || p[6] == "finalize" && r.Method == http.MethodPost || (p[6] == "evaluations" || p[6] == "content") && r.Method == http.MethodGet
	}
	if !valid {
		return 0, nil, fail(405, "method_not_allowed")
	}
	a, err := lockOwnerAgent(r.Context(), tx, principal, p[3])
	if err != nil {
		return 0, nil, err
	}
	a.ownerAuthenticated = true
	a.Scopes = []string{"artifacts:read", "artifacts:submit", "rights:accept"}
	command := r.Clone(r.Context())
	command.URL.Path = "/v1/artifacts/" + p[5]
	if len(p) == 7 {
		command.URL.Path += "/" + p[6]
	}
	return s.artifactCommand(command, tx, a)
}
