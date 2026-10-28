package network

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"testing"
)

func TestOwnerApplicationSubmitsReviewedRoleWithoutAdmission(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
	code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Owner application role review", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "catalog-builder", "schema_family": "source_catalog_v1", "description": "Build the declared catalogue"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	_, identity := builder.call("GET", "/v1/me", "", nil)
	agent := identity["agent_id"].(string)
	swarm := room["id"].(string)
	path := "/v1/owner/agents/" + agent + "/swarms/" + swarm + "/applications"
	body := map[string]any{"role_key": "catalog-builder", "capability_refs": []string{"source_catalog_v1"}}
	code, application := ownerCallMatch(builder, "POST", path, NewID(), room["etag"].(string), body)
	if code != 201 {
		t.Fatalf("reviewed owner application unavailable: %d %v", code, application)
	}
	if application["state"] != "applied" || application["version"] != float64(1) {
		t.Fatal("application implies admission", application)
	}
	var applied, accepted int
	var currentDeadline bool
	if err := db.QueryRow(context.Background(), `SELECT count(*) FILTER(WHERE state='applied'),count(*) FILTER(WHERE state='accepted'),bool_and(respond_by>clock_timestamp()) FROM undergrid.memberships WHERE swarm_id=$1 AND agent_id=$2`, swarm, agent).Scan(&applied, &accepted, &currentDeadline); err != nil {
		t.Fatal(err)
	}
	if applied != 1 || accepted != 0 || !currentDeadline {
		t.Fatal("wrong application persistence", applied, accepted, currentDeadline)
	}
	if code, out := ownerCallMatch(builder, "GET", "/v1/owner/agents/"+agent+"/swarms/"+swarm+"/context", "", "", nil); code != 404 {
		t.Fatal("application exposed private transcript", code, out)
	}
}

func TestAgentApplicationRetainsDeclaredClaimsInsteadOfDiscardingThem(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
	code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Retain explicit application claims", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "catalog-builder", "schema_family": "source_catalog_v1", "description": "Build the declared catalogue"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	code, application := builder.call("POST", "/v1/swarms/"+room["id"].(string)+"/applications", NewID(), map[string]any{"role_key": "catalog-builder", "capability_refs": []string{"source_catalog_v1", "unverified-reference"}})
	if code != 201 {
		t.Fatal(code, application)
	}
	var raw []byte
	if err := db.QueryRow(context.Background(), `SELECT COALESCE(to_jsonb(m)->'application_claims','null'::jsonb) FROM undergrid.memberships m WHERE id=$1`, application["id"]).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	if claims == nil || claims["schema"] != "application-claims-v1" || claims["role_key"] != "catalog-builder" || string(canonical(claims["capability_refs"])) != `["source_catalog_v1","unverified-reference"]` {
		t.Fatal("application dropped its unverified proposal claims", string(raw))
	}
}

func TestOwnerApplicationReviewReplayAndImmutableClaims(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	organizer, builder, foreign := collaborationClient(t, s), collaborationClient(t, s), client(t, s)
	code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Review and recover an application", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "catalog-builder", "schema_family": "source_catalog_v1", "description": "Build the declared catalogue"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	_, who := builder.call("GET", "/v1/me", "", nil)
	path := "/v1/owner/agents/" + who["agent_id"].(string) + "/swarms/" + room["id"].(string) + "/applications"
	body := map[string]any{"role_key": "catalog-builder", "capability_refs": []string{"source_catalog_v1"}}
	for _, check := range []struct {
		owner testClient
		tag   string
		want  int
	}{{builder, "", 428}, {builder, etag(999), 409}, {foreign, room["etag"].(string), 404}} {
		if code, out := ownerCallMatch(check.owner, "POST", path, NewID(), check.tag, body); code != check.want {
			t.Fatal("review/attribution precondition", code, check.want, out)
		}
	}
	key := NewID()
	code, first := ownerCallMatch(builder, "POST", path, key, room["etag"].(string), body)
	if code != 201 {
		t.Fatal(code, first)
	}
	if _, err := db.Exec(context.Background(), `UPDATE undergrid.swarms SET version=version+1 WHERE id=$1`, room["id"]); err != nil {
		t.Fatal(err)
	}
	code, replay := ownerCallMatch(builder, "POST", path, key, room["etag"].(string), body)
	if code != 201 || string(canonical(replay)) != string(canonical(first)) {
		t.Fatal("retained original application changed", code, replay, first)
	}
	if code, out := ownerCallMatch(builder, "POST", path, NewID(), room["etag"].(string), body); code != 409 {
		t.Fatal("stale review created new effect", code, out)
	}
	changed := map[string]any{"role_key": "catalog-builder", "capability_refs": []string{"different-unverified-reference"}}
	if code, out := ownerCallMatch(builder, "POST", path, key, room["etag"].(string), changed); code != 409 {
		t.Fatal("changed request reused receipt", code, out)
	}
	_, err := db.Exec(context.Background(), `UPDATE undergrid.memberships SET application_claims=application_claims||'{"capability_refs":["forged"]}'::jsonb WHERE id=$1`, first["id"])
	var pgError *pgconn.PgError
	if !errors.As(err, &pgError) || pgError.Code != "23514" {
		t.Fatal("application claims mutable", err)
	}
	var memberships, events int
	if err = db.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM undergrid.memberships WHERE id=$1),(SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='membership.applied')`, first["id"]).Scan(&memberships, &events); err != nil {
		t.Fatal(err)
	}
	if memberships != 1 || events != 1 {
		t.Fatal("duplicate application effects", memberships, events)
	}
	if _, err = db.Exec(context.Background(), `UPDATE undergrid.swarms SET state='archived',version=version+1 WHERE id=$1`, room["id"]); err != nil {
		t.Fatal(err)
	}
	if code, out := ownerCallMatch(builder, "POST", path, key, room["etag"].(string), body); code != 404 {
		t.Fatal("receipt resurrected archived application authority", code, out)
	}
}
