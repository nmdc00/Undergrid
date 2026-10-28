package network

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestApplicationOfferTargetContentionRollsBackAndRecoversExactly(t *testing.T) {
	for _, owner := range []bool{false, true} {
		for _, target := range []string{"principal", "agent"} {
			label := "agent_" + target
			if owner {
				label = "owner_" + target
			}
			t.Run(label, func(t *testing.T) {
				s, db := ownerSwarmCreationFixture(t)
				organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
				code, room := organizer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Contended application target", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "builder", "schema_family": "source_catalog_v1", "description": "Declared role"}}})
				if code != 201 {
					t.Fatal(code, room)
				}
				_, who := builder.call("GET", "/v1/me", "", nil)
				_, coord := organizer.call("GET", "/v1/me", "", nil)
				swarm := room["id"].(string)
				code, application := builder.call("POST", "/v1/swarms/"+swarm+"/applications", NewID(), map[string]any{"role_key": "builder", "capability_refs": []string{"source_catalog_v1"}})
				if code != 201 {
					t.Fatal(code, application)
				}
				path := "/v1/swarms/" + swarm + "/invitations"
				body := map[string]any{"agent_id": who["agent_id"], "role": "contributor", "application_id": application["id"], "respond_by": time.Now().Add(time.Hour)}
				if owner {
					path = "/v1/owner/agents/" + coord["agent_id"].(string) + "/swarms/" + swarm + "/applications/" + application["id"].(string) + "/offer"
					body = map[string]any{"application_version": 1, "role": "contributor", "respond_by": body["respond_by"], "history_policy": "full"}
				}
				held, err := db.Begin(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				defer held.Rollback(context.Background())
				query := `SELECT 1 FROM undergrid.principals WHERE id=$1 FOR UPDATE`
				identity := who["principal_id"]
				if target == "agent" {
					query = `SELECT 1 FROM undergrid.agents WHERE id=$1 FOR UPDATE`
					identity = who["agent_id"]
				}
				if _, err = held.Exec(context.Background(), query, identity); err != nil {
					t.Fatal(err)
				}
				key := NewID()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(canonical(body))).WithContext(ctx)
				request.Header.Set("X-Test-Owner", organizer.owner)
				request.Header.Set("Idempotency-Key", key)
				if owner {
					request.Header.Set("If-Match", room["etag"].(string))
				} else {
					request.Header.Set("Authorization", "Bearer "+organizer.token)
				}
				recorder := httptest.NewRecorder()
				started := time.Now()
				s.ServeHTTP(recorder, request)
				var out map[string]any
				if err = json.Unmarshal(recorder.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				failure, ok := out["error"].(map[string]any)
				if recorder.Code != 500 || !ok || failure["retryable"] != true || time.Since(started) > 2*time.Second {
					t.Fatal("target contention did not fail promptly", recorder.Code, out, time.Since(started))
				}
				var state string
				var offered int
				if err = held.QueryRow(context.Background(), `SELECT state,(SELECT count(*) FROM undergrid.events WHERE aggregate_id=m.id AND type='membership.offered') FROM undergrid.memberships m WHERE id=$1`, application["id"]).Scan(&state, &offered); err != nil {
					t.Fatal(err)
				}
				if state != "applied" || offered != 0 {
					t.Fatal("contended command left partial effects", state, offered)
				}
				if err = held.Rollback(context.Background()); err != nil {
					t.Fatal(err)
				}
				var first map[string]any
				for range 2 {
					if owner {
						code, out = ownerCallMatch(organizer, "POST", path, key, room["etag"].(string), body)
					} else {
						code, out = organizer.call("POST", path, key, body)
					}
					if code != 201 || out["id"] != application["id"] || out["state"] != "offered" {
						t.Fatal("exact request did not recover", code, out)
					}
					if first != nil && string(canonical(first)) != string(canonical(out)) {
						t.Fatal("recovery receipt changed", first, out)
					}
					first = out
				}
				if err = db.QueryRow(context.Background(), `SELECT state,(SELECT count(*) FROM undergrid.events WHERE aggregate_id=m.id AND type='membership.offered') FROM undergrid.memberships m WHERE id=$1`, application["id"]).Scan(&state, &offered); err != nil {
					t.Fatal(err)
				}
				if state != "offered" || offered != 1 {
					t.Fatal("recovery duplicated offers/admission", state, offered)
				}
			})
		}
	}
}
