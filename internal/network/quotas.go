package network

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// Admission runs inside the command transaction after its principal lock. Counts
// cover all agents of that principal, not just the credential issuing the command.
func admitPrincipalResource(ctx context.Context, tx pgx.Tx, principal, resource string) error {
	var query string
	var limit int
	switch resource {
	case "agents":
		query = "SELECT count(*) FROM agents WHERE principal_id=$1"
		limit = 10
	case "active_intents":
		query = "SELECT count(*) FROM intents WHERE principal_id=$1 AND state='active'"
		limit = 25
	case "active_swarms":
		query = "SELECT principal_active_swarm_count($1)"
		limit = 10
	default:
		panic("unknown admission resource")
	}
	var count int
	if err := tx.QueryRow(ctx, query, principal).Scan(&count); err != nil {
		return err
	}
	if count >= limit {
		return fail(429, resource+"_quota_exceeded")
	}
	return nil
}
