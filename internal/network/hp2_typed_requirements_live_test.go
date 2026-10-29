//go:build hp1_live && hp2_live

package network

import (
	"context"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestHP2LiveOwnerCreatesAndRevisesTypedRequirements(t *testing.T) {
	hp2LiveOwnerSetup(t, hp2TypedRequirements)
}
func assertHP2TypedRequirementState(t *testing.T, ctx context.Context, db *pgx.Conn) {
	t.Helper()
	var revisions, head, version, opportunities int
	var originals, current bool
	if err := db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM undergrid.intent_revisions),
 i.current_revision,i.version,
 (SELECT count(*) FROM undergrid.opportunities),
 (SELECT r.document->'hard_constraints'->'freshness'->>'max_age_seconds'='3600' AND r.document->'hard_constraints'->'delivery'->>'not_before'='2027-01-02T10:00:00.123456789+01:00' AND r.document->'hard_constraints'->'input_schema_families'='["source_catalog_v1","reference_v1"]'::jsonb FROM undergrid.intent_revisions r WHERE r.intent_id=i.id AND revision=1),
 (SELECT NOT(r.document->'hard_constraints' ? 'freshness') AND r.document->'hard_constraints'->'delivery'->>'deliver_by'='2027-01-04T09:00:00.250000001-02:00' AND r.document->'hard_constraints'->'input_schema_families'='["source_catalog_v1"]'::jsonb AND r.document->'hard_constraints'->'coverage'->'required'='["region:eu"]'::jsonb FROM undergrid.intent_revisions r WHERE r.intent_id=i.id AND revision=3)
 FROM undergrid.intents i JOIN undergrid.principals p ON p.id=i.principal_id WHERE p.subject='hp2-controlled-buyer-a'`).Scan(&revisions, &head, &version, &opportunities, &originals, &current); err != nil {
		t.Fatal(err)
	}
	if revisions != 5 || head != 3 || version != 3 || opportunities != 0 || !originals || !current {
		t.Fatalf("typed immutable requirement effects: revisions=%d head=%d version=%d opportunities=%d original=%v current=%v", revisions, head, version, opportunities, originals, current)
	}
}
