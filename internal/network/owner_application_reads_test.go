package network

import (
	"context"
	"strings"
	"testing"
)

func TestOwnerApplicationReadsSeparateApplicantAndCoordinatorViews(t *testing.T) {
	s, _ := ownerSwarmCreationFixture(t)
	organizer, builder, foreign := collaborationClient(t, s), collaborationClient(t, s), client(t, s)
	code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Public application summary", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "builder", "schema_family": "source_catalog_v1", "description": "Declared role"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	_, applicant := builder.call("GET", "/v1/me", "", nil)
	_, coordinator := organizer.call("GET", "/v1/me", "", nil)
	code, application := builder.call("POST", "/v1/swarms/"+room["id"].(string)+"/applications", NewID(), map[string]any{"role_key": "builder", "capability_refs": []string{"source_catalog_v1"}})
	if code != 201 {
		t.Fatal(code, application)
	}
	own := "/v1/owner/agents/" + applicant["agent_id"].(string) + "/applications"
	review := "/v1/owner/agents/" + coordinator["agent_id"].(string) + "/swarms/" + room["id"].(string) + "/applications"
	for _, view := range []struct {
		actor testClient
		path  string
	}{{builder, own}, {organizer, review}} {
		code, out := ownerCallMatch(view.actor, "GET", view.path, "", "", nil)
		if code != 200 {
			t.Fatalf("current application view unavailable: %d %v", code, out)
		}
		rows, ok := out["items"].([]any)
		if !ok || len(rows) != 1 {
			t.Fatal("missing bounded own application", out)
		}
		item := rows[0].(map[string]any)
		if item["id"] != application["id"] || item["state"] != "applied" || item["history_policy"] != "full" {
			t.Fatal("misbound application view", item)
		}
		claims := item["application_claims"].(map[string]any)
		if claims["role_key"] != "builder" || string(canonical(claims["capability_refs"])) != `["source_catalog_v1"]` {
			t.Fatal("lost application proposal", item)
		}
		for _, key := range []string{`"messages"`, `"profile"`, `"token"`, `"owner"`} {
			if strings.Contains(string(canonical(out)), key) {
				t.Fatal("private application view data", key, out)
			}
		}
	}
	if code, out := ownerCallMatch(foreign, "GET", own, "", "", nil); code != 404 {
		t.Fatal("foreign applicant page exposed", code, out)
	}
	builderReview := "/v1/owner/agents/" + applicant["agent_id"].(string) + "/swarms/" + room["id"].(string) + "/applications"
	if code, out := ownerCallMatch(builder, "GET", builderReview, "", "", nil); code == 200 {
		t.Fatal("applicant obtained coordinator queue", code, out)
	}
}

func TestOwnerApplicationReadBoundsLargeClaimsWithoutLosingStoredEvidence(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
	code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Bounded application claim projection", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "builder", "schema_family": "source_catalog_v1", "description": "Declared role"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	reference := strings.Repeat("unverified-reference", 2000)
	code, application := builder.call("POST", "/v1/swarms/"+room["id"].(string)+"/applications", NewID(), map[string]any{"role_key": "builder", "capability_refs": []string{reference}})
	if code != 201 {
		t.Fatal("valid legacy claim input rejected", code, application)
	}
	_, who := builder.call("GET", "/v1/me", "", nil)
	code, out := ownerCallMatch(builder, "GET", "/v1/owner/agents/"+who["agent_id"].(string)+"/applications", "", "", nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	row := out["items"].([]any)[0].(map[string]any)
	if row["application_claims"] != nil || row["claims_state"] != "retained_over_budget" {
		t.Fatal("unbounded claims presented as normal review payload")
	}
	var retained, document string
	if err := db.QueryRow(context.Background(), `SELECT application_claims->'capability_refs'->>0,application_claims::text FROM undergrid.memberships WHERE id=$1`, application["id"]).Scan(&retained, &document); err != nil {
		t.Fatal(err)
	}
	if retained != reference || row["application_claims_sha256"] != digest([]byte(document)) || len(canonical(out)) > 32768 {
		t.Fatal("claim evidence was lost or projection unbounded", len(retained), len(canonical(out)))
	}
}
