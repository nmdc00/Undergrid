package network

import (
	"slices"
	"time"
)

const (
	maxPlanTasks = 50
	maxPlanEdges = 100
)

type TaskSpec struct {
	Key                     string     `json:"key"`
	AssigneeAgentID         string     `json:"assignee_agent_id"`
	DependsOn               []string   `json:"depends_on"`
	OutputSchema            string     `json:"output_schema"`
	Checks                  []string   `json:"checks"`
	ProgressIntervalSeconds int        `json:"progress_interval_seconds"`
	Deadline                *time.Time `json:"deadline,omitempty"`
}
type PlanSpec struct {
	ParentVersion         *int64     `json:"parent_version"`
	Objective             string     `json:"objective"`
	RequiredPrincipals    []string   `json:"required_principals"`
	AcceptancePrincipals  []string   `json:"acceptance_principals"`
	PublicationPrincipals []string   `json:"publication_principals"`
	Rights                RightsSpec `json:"rights"`
	Tasks                 []TaskSpec `json:"tasks"`
	RespondBy             time.Time  `json:"respond_by"`
}
type sealedPlan struct {
	Plan                 PlanSpec `json:"plan"`
	TemplateHash         string   `json:"template_hash"`
	InputGrantPolicyHash string   `json:"input_grant_policy_hash,omitempty"`
}

func (p PlanSpec) validate() error {
	if len(p.Objective) < 3 || len(p.Objective) > 4000 || len(p.Tasks) < 1 || len(p.Tasks) > maxPlanTasks || len(p.RequiredPrincipals) < 1 || len(p.AcceptancePrincipals) < 1 || !p.RespondBy.After(time.Now()) || p.RespondBy.After(time.Now().Add(7*24*time.Hour)) {
		return fail(422, "invalid_plan")
	}
	for _, set := range [][]string{p.RequiredPrincipals, p.AcceptancePrincipals, p.PublicationPrincipals} {
		seen := map[string]bool{}
		if len(set) > 50 {
			return fail(422, "principal_limit")
		}
		for _, id := range set {
			if !validID(id) || seen[id] {
				return fail(422, "invalid_principal_set")
			}
			seen[id] = true
		}
	}
	for _, id := range p.AcceptancePrincipals {
		if !slices.Contains(p.RequiredPrincipals, id) {
			return fail(422, "acceptor_must_approve_plan")
		}
	}
	tasks := map[string]TaskSpec{}
	edges := 0
	for _, task := range p.Tasks {
		if len(task.Key) < 1 || len(task.Key) > 80 || !validID(task.AssigneeAgentID) || task.OutputSchema != "source_catalog_v1" || len(task.Checks) != 1 || task.Checks[0] != "json_schema_v1" || task.ProgressIntervalSeconds < 60 || task.ProgressIntervalSeconds > 7*86400 {
			return fail(422, "invalid_task")
		}
		if task.Deadline != nil && !task.Deadline.After(time.Now()) {
			return fail(422, "invalid_task_deadline")
		}
		if _, ok := tasks[task.Key]; ok {
			return fail(422, "duplicate_task_key")
		}
		tasks[task.Key] = task
		edges += len(task.DependsOn)
	}
	if edges > maxPlanEdges {
		return fail(422, "dependency_limit")
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) bool
	visit = func(key string) bool {
		if done[key] {
			return true
		}
		task, ok := tasks[key]
		if !ok || visiting[key] {
			return false
		}
		visiting[key] = true
		for _, dependency := range task.DependsOn {
			if !visit(dependency) {
				return false
			}
		}
		visiting[key] = false
		done[key] = true
		return true
	}
	for key := range tasks {
		if !visit(key) {
			return fail(422, "invalid_dependency_graph")
		}
	}
	return nil
}
