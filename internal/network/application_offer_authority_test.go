package network

import (
	"context"
	"testing"
	"time"
)

func TestApplicationOfferRechecksApplicantBeforeWriteAndReceiptRecovery(t *testing.T) {
	for _, check := range []struct{ phase, identity string }{{"before_offer", "agent"}, {"before_recovery", "agent"}, {"before_offer", "principal"}, {"before_recovery", "principal"}} {
		t.Run(check.phase+"_"+check.identity, func(t *testing.T) {
			phase := check.phase
			s, db := ownerSwarmCreationFixture(t)
			organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
			code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Current application target", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "builder", "schema_family": "source_catalog_v1", "description": "Declared role"}}})
			if code != 201 {
				t.Fatal(code, room)
			}
			_, who := builder.call("GET", "/v1/me", "", nil)
			path := "/v1/swarms/" + room["id"].(string)
			code, application := builder.call("POST", path+"/applications", NewID(), map[string]any{"role_key": "builder", "capability_refs": []string{"source_catalog_v1"}})
			if code != 201 {
				t.Fatal(code, application)
			}
			key := NewID()
			body := map[string]any{"agent_id": who["agent_id"], "role": "contributor", "application_id": application["id"], "respond_by": time.Now().Add(time.Hour)}
			if phase == "before_recovery" {
				if code, out := organizer.call("POST", path+"/invitations", key, body); code != 201 {
					t.Fatal(code, out)
				}
			}
			query := `UPDATE undergrid.agents SET status='revoked' WHERE id=$1`
			target := who["agent_id"]
			if check.identity == "principal" {
				query = `UPDATE undergrid.principals SET status='suspended' WHERE id=$1`
				target = who["principal_id"]
			}
			if _, err := db.Exec(context.Background(), query, target); err != nil {
				t.Fatal(err)
			}
			if code, out := organizer.call("POST", path+"/invitations", key, body); code != 409 {
				t.Fatal("revoked applicant authorized offer or receipt", code, out)
			}
			var state string
			var offered int
			if err := db.QueryRow(context.Background(), `SELECT state,(SELECT count(*) FROM undergrid.events WHERE aggregate_id=m.id AND type='membership.offered') FROM undergrid.memberships m WHERE id=$1`, application["id"]).Scan(&state, &offered); err != nil {
				t.Fatal(err)
			}
			if phase == "before_offer" && (state != "applied" || offered != 0) {
				t.Fatal("denial changed application", state, offered)
			}
			if phase == "before_recovery" && (state != "offered" || offered != 1) {
				t.Fatal("denied replay duplicated effect", state, offered)
			}
		})
	}
}
