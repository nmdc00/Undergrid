//go:build hp1_live && hp2_live

package network

import (
	"context"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestHP2LiveOwnerRecruitmentApplicationOfferAndAdmission(t *testing.T) {
	hp2LiveOwnerSetup(t, hp2Applications)
}
func assertHP2ApplicationState(t *testing.T, ctx context.Context, db *pgx.Conn) {
	t.Helper()
	var applications, accepted, claims, appliedEvents, offeredEvents, plans, tasks, approvals int
	if err := db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM undergrid.memberships WHERE origin='application'),
 (SELECT count(*) FROM undergrid.memberships m JOIN undergrid.principals p ON p.id=m.principal_id WHERE m.origin='application' AND m.state='accepted' AND m.version=3 AND p.subject='hp2-controlled-builder-c'),
 (SELECT count(*) FROM undergrid.memberships WHERE origin='application' AND application_claims->>'schema'='application-claims-v1' AND application_claims->>'role_key'='catalog-builder' AND application_claims->'capability_refs'='["owner-declared-catalogue"]'::jsonb),
 (SELECT count(*) FROM undergrid.events WHERE type='membership.applied'),
 (SELECT count(*) FROM undergrid.events WHERE type='membership.offered'),
 (SELECT count(*) FROM undergrid.plan_versions),(SELECT count(*) FROM undergrid.tasks),(SELECT count(*) FROM undergrid.approvals)`).Scan(&applications, &accepted, &claims, &appliedEvents, &offeredEvents, &plans, &tasks, &approvals); err != nil {
		t.Fatal(err)
	}
	if applications != 1 || accepted != 1 || claims != 1 || appliedEvents != 1 || offeredEvents != 1 || plans != 0 || tasks != 0 || approvals != 0 {
		t.Fatalf("application admission effects: applications=%d accepted=%d claims=%d applied/offered=%d/%d plans/tasks/approvals=%d/%d/%d", applications, accepted, claims, appliedEvents, offeredEvents, plans, tasks, approvals)
	}
}
