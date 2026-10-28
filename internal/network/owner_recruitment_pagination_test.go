package network

import (
	"context"
	"testing"
)

func TestOwnerRecruitmentWindowsAdvancePastStaleRoleEvidence(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	ctx := context.Background()
	builder := collaborationClient(t, s)
	_, who := builder.call("GET", "/v1/me", "", nil)
	if _, err := db.Exec(ctx, `DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='undergrid_owner_recruitment_runner') THEN CREATE ROLE undergrid_owner_recruitment_runner LOGIN NOSUPERUSER NOBYPASSRLS; END IF; END $$; GRANT undergrid_worker TO undergrid_owner_recruitment_runner`); err != nil {
		t.Fatal(err)
	}
	worker := New(networkFixtureRoleDSN(t, db, "undergrid_owner_recruitment_runner"), "development", nil)
	type roomSource struct {
		actor testClient
		room  string
		etag  string
	}
	sources := map[string]roomSource{}
	for range 51 {
		organizer := client(t, s)
		code, agent := organizer.call("POST", "/v1/agents", NewID(), map[string]any{"name": "Private coordinator", "scopes": []string{"swarms:create", "swarms:read", "swarms:archive"}})
		if code != 201 {
			t.Fatal(code, agent)
		}
		organizer.token = agent["token"].(string)
		code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Bounded current recruitment review", "visibility": "restricted", "recruitment_roles": []RecruitmentRole{{Key: "builder", SchemaFamily: "source_catalog_v1", Description: "Original declaration"}}})
		if code != 201 {
			t.Fatal(code, room)
		}
		if err := worker.ScanRecruitment(ctx, room["id"].(string)); err != nil {
			t.Fatal(err)
		}
		var opportunity string
		if err := db.QueryRow(ctx, `SELECT id FROM undergrid.opportunities WHERE source_swarm_id=$1 AND recipient_agent_id=$2 AND kind='builder_recruitment'`, room["id"], who["agent_id"]).Scan(&opportunity); err != nil {
			t.Fatal("normal scan failed", err)
		}
		sources[opportunity] = roomSource{organizer, room["id"].(string), room["etag"].(string)}
	}
	path := "/v1/owner/agents/" + who["agent_id"].(string) + "/recruitment"
	read := func(url string) map[string]any {
		t.Helper()
		code, out := ownerCallMatch(builder, "GET", url, "", "", nil)
		if code != 200 {
			t.Fatal(code, out)
		}
		return out
	}
	first := read(path)
	rows := first["items"].([]any)
	if len(rows) != 50 || first["candidate_limit"] != float64(50) || first["next_after"] == "" {
		t.Fatal("incorrect candidate bound", len(rows), first["next_after"])
	}
	previous := ""
	seen := map[string]bool{}
	for _, raw := range rows {
		row := raw.(map[string]any)
		id := row["id"].(string)
		source, ok := sources[id]
		if !ok || seen[id] || id <= previous || row["swarm_id"] != source.room {
			t.Fatal("misbound/unsorted/duplicate candidate", row)
		}
		seen[id] = true
		previous = id
		// Eligibility fault only: all opportunities were normally emitted above.
		if _, err := db.Exec(ctx, `UPDATE undergrid.swarms SET recruitment_roles='[{"key":"builder","schema_family":"source_catalog_v1","description":"Changed after recruitment"}]' WHERE id=$1`, source.room); err != nil {
			t.Fatal(err)
		}
	}
	if first["next_after"] != previous {
		t.Fatal("cursor does not bind scanned window", first)
	}
	stale := read(path)
	if len(stale["items"].([]any)) != 0 || stale["next_after"] != previous {
		t.Fatal("stale window prevented continuation", stale)
	}
	tail := read(path + "?after=" + previous)
	remaining := tail["items"].([]any)
	if len(remaining) != 1 || tail["next_after"] != "" {
		t.Fatal("incorrect terminal window", tail)
	}
	last := remaining[0].(map[string]any)
	lastID := last["id"].(string)
	if seen[lastID] || lastID <= previous || last["swarm_id"] != sources[lastID].room {
		t.Fatal("terminal window repeated or misbound candidate", last)
	}
	if code, out := ownerCallMatch(builder, "GET", path+"/"+previous, "", "", nil); code != 404 {
		t.Fatal("changed role retained selected review", code, out)
	}
	if code, out := ownerCallMatch(builder, "GET", path+"?after=invalid", "", "", nil); code != 400 {
		t.Fatal("invalid cursor accepted", code, out)
	}
	source := sources[lastID]
	if code, out := source.actor.versioned("POST", "/v1/swarms/"+source.room+"/archive", NewID(), source.etag, map[string]any{}); code != 200 {
		t.Fatal("normal archive failed", code, out)
	}
	if code, out := ownerCallMatch(builder, "GET", path+"/"+lastID, "", "", nil); code != 404 {
		t.Fatal("archived room remained actionable", code, out)
	}
	if page := read(path + "?after=" + previous); len(page["items"].([]any)) != 0 || page["next_after"] != "" {
		t.Fatal("archived recruitment survived list", page)
	}
}
