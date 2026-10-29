package network

import (
	"context"
	"testing"
)

func TestOwnerRecruitmentReviewPinsCurrentRoleAndRejectsStaleProfile(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	organizer, builder, foreign := collaborationClient(t, s), collaborationClient(t, s), client(t, s)
	observeOwnerApplicationContract(t, &builder, &foreign)
	code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Current owner recruitment review", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "builder", "schema_family": "source_catalog_v1", "description": "Build the declared catalogue"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	_, who := builder.call("GET", "/v1/me", "", nil)
	if _, err := db.Exec(context.Background(), `DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='undergrid_owner_recruitment_runner') THEN CREATE ROLE undergrid_owner_recruitment_runner LOGIN NOSUPERUSER NOBYPASSRLS; END IF; END $$; GRANT undergrid_worker TO undergrid_owner_recruitment_runner`); err != nil {
		t.Fatal(err)
	}
	worker := New(networkFixtureRoleDSN(t, db, "undergrid_owner_recruitment_runner"), "development", nil)
	if err := worker.ScanRecruitment(context.Background(), room["id"].(string)); err != nil {
		t.Fatal(err)
	}
	var opportunity string
	if err := db.QueryRow(context.Background(), `SELECT id FROM undergrid.opportunities WHERE recipient_agent_id=$1 AND source_swarm_id=$2 AND kind='builder_recruitment'`, who["agent_id"], room["id"]).Scan(&opportunity); err != nil {
		t.Fatal("actual recruitment scan did not produce fixture", err)
	}
	path := "/v1/owner/agents/" + who["agent_id"].(string) + "/recruitment"
	code, page := ownerCallMatch(builder, "GET", path, "", "", nil)
	if code != 200 {
		t.Fatalf("owner recruitment review unavailable: %d %v", code, page)
	}
	rows := page["items"].([]any)
	if len(rows) != 1 {
		t.Fatal("current recipient review missing", page)
	}
	row := rows[0].(map[string]any)
	if row["id"] != opportunity || row["swarm_id"] != room["id"] || row["history_policy"] != "full" || row["swarm_etag"] != room["etag"] {
		t.Fatal("unbound recruitment role review", row)
	}
	role := row["role"].(map[string]any)
	if role["key"] != "builder" || role["schema_family"] != "source_catalog_v1" {
		t.Fatal("missing exact role declaration", row)
	}
	if code, out := ownerCallMatch(foreign, "GET", path, "", "", nil); code != 404 {
		t.Fatal("foreign recipient disclosed", code, out)
	}
	if code, out := ownerCallMatch(builder, "GET", path+"/"+opportunity, "", "", nil); code != 200 {
		t.Fatal(code, out)
	}
	code, legacyPacket := builder.call("GET", "/v1/opportunities/"+opportunity, "", nil)
	if code != 200 || len(legacyPacket) != 7 {
		t.Fatal("legacy recruitment wire shape changed", code, legacyPacket)
	}
	for _, field := range []string{"id", "kind", "origin", "swarm_id", "role_key", "objective", "evidence"} {
		if _, ok := legacyPacket[field]; !ok {
			t.Fatal("missing published legacy field", field, legacyPacket)
		}
	}
	// Controlled profile change is an eligibility fault, not a discovery seed.
	if _, err := db.Exec(context.Background(), `UPDATE undergrid.agents SET profile=jsonb_set(profile,'{recruitment}','false') WHERE id=$1`, who["agent_id"]); err != nil {
		t.Fatal(err)
	}
	if code, out := ownerCallMatch(builder, "GET", path+"/"+opportunity, "", "", nil); code != 404 {
		t.Fatal("stale profile remained actionable", code, out)
	}
	code, page = ownerCallMatch(builder, "GET", path, "", "", nil)
	if code != 200 || len(page["items"].([]any)) != 0 {
		t.Fatal("stale item remained in owner list", code, page)
	}
	if code, out := builder.call("GET", "/v1/opportunities/"+opportunity, "", nil); code != 404 {
		t.Fatal("legacy agent packet retained stale profile evidence", code, out)
	}
	code, legacy := builder.call("GET", "/v1/opportunities?kind=builder_recruitment", "", nil)
	if code != 200 {
		t.Fatal(code, legacy)
	}
	for _, raw := range legacy["items"].([]any) {
		if raw.(map[string]any)["id"] == opportunity {
			t.Fatal("legacy agent listing retained stale evidence", legacy)
		}
	}

}
