//go:build hp1_live && hp2_live

package network

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestHP2LiveOwnerInspectsNontransitiveGroupsAndCurrentRevision(t *testing.T) {
	hp2LiveOwnerSetup(t, hp2CompatibleGroups)
}

// All owners/agents/intents and opportunities come from normal browser commands
// and the actual restricted worker. This oracle reads normalized persistence only.
func assertHP2GroupInspectionState(t *testing.T, ctx context.Context, db *pgx.Conn) {
	t.Helper()
	var revisions, sourceRevision, shared, completed int
	var sourceLicenses, bLicenses, cLicenses bool
	if err := db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM undergrid.intent_revisions),
 (SELECT i.current_revision FROM undergrid.intents i JOIN undergrid.principals p ON p.id=i.principal_id WHERE p.subject='hp2-controlled-buyer-a'),
 (SELECT count(*) FROM undergrid.opportunities o JOIN undergrid.intents i ON i.id=o.source_intent_id JOIN undergrid.principals p ON p.id=i.principal_id WHERE p.subject='hp2-controlled-buyer-a' AND o.kind='shared_demand'),
 (SELECT count(*) FROM undergrid_queue.river_job WHERE kind='intention_scan_v1' AND state='completed'),
 (SELECT r.document->'hard_constraints'->'license_ids'='["CC0-1.0"]'::jsonb FROM undergrid.intents i JOIN undergrid.principals p ON p.id=i.principal_id JOIN undergrid.intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE p.subject='hp2-controlled-buyer-a'),
 (SELECT r.document->'hard_constraints'->'license_ids'='["CC0-1.0"]'::jsonb FROM undergrid.intents i JOIN undergrid.principals p ON p.id=i.principal_id JOIN undergrid.intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE p.subject='hp2-controlled-buyer-b'),
 (SELECT r.document->'hard_constraints'->'license_ids'='["ODC-BY-1.0"]'::jsonb FROM undergrid.intents i JOIN undergrid.principals p ON p.id=i.principal_id JOIN undergrid.intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE p.subject='hp2-controlled-builder-c')`).Scan(&revisions, &sourceRevision, &shared, &completed, &sourceLicenses, &bLicenses, &cLicenses); err != nil {
		t.Fatal(err)
	}
	if revisions != 4 || sourceRevision != 2 || shared < 2 || completed < 3 || !sourceLicenses || !bLicenses || !cLicenses {
		t.Fatalf("actual group discovery/revision effects: revisions=%d source=%d shared=%d completed=%d licenses=%v/%v/%v", revisions, sourceRevision, shared, completed, sourceLicenses, bLicenses, cLicenses)
	}
}
