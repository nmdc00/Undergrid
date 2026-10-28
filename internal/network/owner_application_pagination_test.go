package network

import (
	"context"
	"testing"
)

func TestOwnerApplicationQueuePagesNormalSubmissionsAndExcludesExpired(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	organizer := collaborationClient(t, s)
	code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Bounded coordinator application queue", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "builder", "schema_family": "source_catalog_v1", "description": "Declared role"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	_, coordinator := organizer.call("GET", "/v1/me", "", nil)
	path := "/v1/owner/agents/" + coordinator["agent_id"].(string) + "/swarms/" + room["id"].(string) + "/applications"
	type applicant struct {
		client testClient
		agent  string
	}
	applicants := map[string]applicant{}
	for range 101 {
		builder := collaborationClient(t, s)
		_, who := builder.call("GET", "/v1/me", "", nil)
		code, application := builder.call("POST", "/v1/swarms/"+room["id"].(string)+"/applications", NewID(), map[string]any{"role_key": "builder", "capability_refs": []string{"source_catalog_v1"}})
		if code != 201 {
			t.Fatal(code, application)
		}
		applicants[application["id"].(string)] = applicant{builder, who["agent_id"].(string)}
	}
	const counts = `SELECT (SELECT count(*) FROM undergrid.events)+(SELECT count(*) FROM undergrid.inbox)+(SELECT count(*) FROM undergrid_queue.river_job)`
	var before, after int
	if err := db.QueryRow(context.Background(), counts).Scan(&before); err != nil {
		t.Fatal(err)
	}
	read := func(actor testClient, url string) map[string]any {
		t.Helper()
		code, out := ownerCallMatch(actor, "GET", url, "", "", nil)
		if code != 200 {
			t.Fatal(code, out)
		}
		return out
	}
	first := read(organizer, path)
	rows := first["items"].([]any)
	if len(rows) != 100 || first["next_after"] == "" {
		t.Fatal("unbounded/missing first application page", len(rows), first["next_after"])
	}
	seen := map[string]bool{}
	previous := ""
	for _, raw := range rows {
		row := raw.(map[string]any)
		id := row["id"].(string)
		if _, ok := applicants[id]; !ok || seen[id] || id < previous {
			t.Fatal("misbound/duplicate/unsorted application", row)
		}
		seen[id] = true
		previous = id
	}
	second := read(organizer, path+"?after="+first["next_after"].(string))
	tail := second["items"].([]any)
	if len(tail) != 1 || second["next_after"] != "" {
		t.Fatal("incorrect terminal page", second)
	}
	id := tail[0].(map[string]any)["id"].(string)
	if seen[id] {
		t.Fatal("page repeated application", id)
	}
	selected, ok := applicants[id]
	if !ok {
		t.Fatal("unknown application on second page", id)
	}
	ownPath := "/v1/owner/agents/" + selected.agent + "/applications"
	if items := read(selected.client, ownPath)["items"].([]any); len(items) != 1 || items[0].(map[string]any)["id"] != id {
		t.Fatal("applicant saw foreign queue", items)
	}
	if err := db.QueryRow(context.Background(), counts).Scan(&after); err != nil || before != after {
		t.Fatal("read created events/inbox/jobs", before, after, err)
	}
	if _, err := db.Exec(context.Background(), `UPDATE undergrid.memberships SET respond_by=clock_timestamp()-interval '1 second' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if items := read(selected.client, ownPath)["items"].([]any); len(items) != 0 {
		t.Fatal("expired pending application visible", items)
	}
	current := read(organizer, path)
	if len(current["items"].([]any)) != 100 || current["next_after"] != "" {
		t.Fatal("expired item still consumed queue page", current["next_after"])
	}
	if code, out := ownerCallMatch(organizer, "GET", path+"?after=invalid", "", "", nil); code != 400 {
		t.Fatal("malformed application cursor accepted", code, out)
	}
}
