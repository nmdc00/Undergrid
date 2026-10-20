package network

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Only the exact owner vote path participates in plan-wide policy locking.
// Unsupported paths and ordinary reads retain their existing authorization path.
func ownerPlanApprovalMutation(r *http.Request) (agent, plan string, ok bool) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if r.Method != http.MethodPost || len(parts) != 7 || parts[0] != "v1" || parts[1] != "owner" || parts[2] != "agents" || parts[4] != "plans" || parts[6] != "approvals" || !validID(parts[3]) || !validID(parts[5]) {
		return "", "", false
	}
	return parts[3], parts[5], true
}

func prelockOwnerPlanAuthorities(ctx context.Context, tx pgx.Tx, principal, agent, plan string) error {
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
	_, err = tx.Exec(ctx, "SELECT lock_plan_authorities($1)", plan)
	return err
}
