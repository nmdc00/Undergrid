package network

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Exact owner plan votes participate in plan-wide policy locking.
// Unsupported paths and ordinary reads retain their existing authorization path.
func ownerPlanApprovalMutation(r *http.Request) (agent, plan string, ok bool) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if r.Method != http.MethodPost || len(parts) != 7 || parts[0] != "v1" || parts[1] != "owner" || parts[2] != "agents" || parts[4] != "plans" || parts[6] != "approvals" || !validID(parts[3]) || !validID(parts[5]) {
		return "", "", false
	}
	return parts[3], parts[5], true
}

func prelockOwnerPlanAuthorities(ctx context.Context, tx pgx.Tx, principal, agent, plan string, task bool) error {
	// Prove attribution before setting actor_agent: the privileged lock helper
	// may only resolve required principals for a room this owned agent can read.
	// The selected agent is deliberately not locked before the principal set.
	var state string
	err := tx.QueryRow(ctx, "SELECT status FROM agents WHERE id=$1 AND principal_id=$2", agent, principal).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(404, "not_found")
	}
	if err != nil {
		return err
	}
	if state != "active" {
		return fail(403, "inactive_agent")
	}
	if _, err = tx.Exec(ctx, "SELECT set_config('undergrid.agent_id',$1,true)", agent); err != nil {
		return err
	}
	if task {
		// RLS resolves only a current member-visible task after proven attribution.
		err = tx.QueryRow(ctx, "SELECT plan_id FROM tasks WHERE id=$1 AND assigned_agent_id=$2", plan, agent).Scan(&plan)
		if errors.Is(err, pgx.ErrNoRows) {
			return fail(404, "not_found")
		}
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, "SELECT lock_plan_authorities($1)", plan)
	return err
}

// Assignment responses may accept a plan and progress checks its effective
// policy, so both use the same canonical authority lock order as exact votes.
func ownerPlanAuthorityMutation(r *http.Request) (agent, resource string, task, ok bool) {
	if agent, plan, found := ownerPlanApprovalMutation(r); found {
		return agent, plan, false, true
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if r.Method == http.MethodPost && len(parts) == 7 && parts[0] == "v1" && parts[1] == "owner" && parts[2] == "agents" && parts[4] == "tasks" && (parts[6] == "assignment-response" || parts[6] == "progress" || parts[6] == "artifact-uploads") && validID(parts[3]) && validID(parts[5]) {
		return parts[3], parts[5], true, true
	}
	return "", "", false, false
}
