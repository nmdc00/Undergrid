package network

import (
	"bytes"
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"net/url"
	"reflect"
	"testing"
)

func TestOwnerContributionCreatesSessionBoundQuarantineWithoutBorrowingCredential(t *testing.T) {
	x := newArtifactScenario(t)
	owner := x.actors[2]
	_, who := owner.call("GET", "/v1/me", "", nil)
	path := "/v1/owner/agents/" + who["agent_id"].(string) + "/tasks/" + x.task["id"].(string) + "/artifact-uploads"
	data := []byte(`{"sources":[{"source_id":"owner-fixture","source_url":"https://example.org/"}]}`)
	body := map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data)}
	key := NewID()
	code, upload := ownerCallMatch(owner, "POST", path, key, x.task["etag"].(string), body)
	if code != 201 {
		t.Fatalf("owner contribution unavailable after normal accepted plan: %d %v", code, upload)
	}
	if upload["state"] != "quarantine" || upload["purpose"] != "output" || upload["upload_authentication"] != "owner_session" {
		t.Fatal("missing explicit owner quarantine mode")
	}
	parsed, err := url.Parse(upload["upload_url"].(string))
	if err != nil || parsed.Path != "/v1/owner/agents/"+who["agent_id"].(string)+"/artifacts/"+upload["id"].(string)+"/upload" || len(parsed.Query().Get("token")) != 64 {
		t.Fatal("invalid owner upload capability route")
	}
	if code, replayed := ownerCallMatch(owner, "POST", path, key, x.task["etag"].(string), body); code != 201 || !reflect.DeepEqual(upload, replayed) {
		t.Fatal("owner quarantine creation did not recover exactly", code)
	}
	var mode, agent, principal, state string
	var noCredential bool
	var artifacts, approvals int
	if err := x.db.QueryRow(context.Background(), `SELECT upload_authentication,upload_credential_id IS NULL,agent_id,principal_id,(SELECT state FROM undergrid.tasks WHERE id=a.task_id),(SELECT count(*) FROM undergrid.artifacts WHERE task_id=a.task_id),(SELECT count(*) FROM undergrid.approvals WHERE plan_id=a.plan_id) FROM undergrid.artifacts a WHERE id=$1`, upload["id"]).Scan(&mode, &noCredential, &agent, &principal, &state, &artifacts, &approvals); err != nil {
		t.Fatal(err)
	}
	if mode != "owner_session" || !noCredential || agent != who["agent_id"] || principal != who["principal_id"] || state != "accepted" || artifacts != 1 || approvals != 6 {
		t.Fatal("owner upload borrowed authority or submitted work", mode, noCredential, agent, principal, state, artifacts, approvals)
	}
}

func TestOwnerContributionRecoveryCannotBorrowCredentialGeneration(t *testing.T) {
	x := newArtifactScenario(t)
	owner := x.actors[2]
	_, who := owner.call("GET", "/v1/me", "", nil)
	data := []byte(`{"sources":[{"source_id":"separate","source_url":"https://example.org/"}]}`)
	key := NewID()
	legacy := x.upload(t, data, key)
	path := "/v1/owner/agents/" + who["agent_id"].(string) + "/tasks/" + x.task["id"].(string) + "/artifact-uploads"
	body := map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data), "purpose": "output"}
	code, owned := ownerCallMatch(owner, "POST", path, key, x.task["etag"].(string), body)
	if code != 201 || owned["id"] == legacy["id"] || owned["upload_authentication"] != "owner_session" {
		t.Fatal("owner reused credential-bound generation", code)
	}
	agentBody := map[string]any{"swarm_id": x.room["id"], "plan_id": x.plan["id"], "task_id": x.task["id"], "claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data), "purpose": "output"}
	if code, replayed := owner.call("POST", "/v1/artifacts/uploads", key, agentBody); code != 201 || !reflect.DeepEqual(legacy, replayed) {
		t.Fatal("owner changed credential recovery", code)
	}
	parsed, err := url.Parse(owned["upload_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	// Owner capabilities cannot authenticate through the anonymous credential path.
	target := x.server.URL + "/v1/artifacts/" + owned["id"].(string) + "/upload?" + parsed.RawQuery
	request, err := http.NewRequest("PUT", target, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	response, err := x.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("owner capability authenticated without owner session", response.StatusCode)
	}
}

func TestOwnerContributionCreationRequiresCurrentTaskAndOwner(t *testing.T) {
	x := newArtifactScenario(t)
	owner := x.actors[2]
	_, who := owner.call("GET", "/v1/me", "", nil)
	path := "/v1/owner/agents/" + who["agent_id"].(string) + "/tasks/" + x.task["id"].(string) + "/artifact-uploads"
	body := map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": 12}
	for _, scenario := range []struct {
		name, match string
		actor       testClient
		body        map[string]any
		status      int
	}{
		{"missing task version", "", owner, body, 428},
		{"stale task version", "\"999\"", owner, body, 409},
		{"stale epoch", x.task["etag"].(string), owner, map[string]any{"claim_epoch": x.task["claim_epoch"].(float64) + 1, "media_type": "application/json", "size_bytes": 12}, 409},
		{"foreign owner", x.task["etag"].(string), x.actors[3], body, 404},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if code, _ := ownerCallMatch(scenario.actor, "POST", path, NewID(), scenario.match, scenario.body); code != scenario.status {
				t.Fatal("unexpected quarantine guard", code)
			}
		})
	}
	var count int
	if err := x.db.QueryRow(context.Background(), `SELECT count(*) FROM undergrid.artifacts WHERE task_id=$1`, x.task["id"]).Scan(&count); err != nil || count != 0 {
		t.Fatal("denied upload allocated storage", count, err)
	}
}

func TestOwnerContributionDoesNotRequireLiveNetworkCredential(t *testing.T) {
	x := newArtifactScenario(t)
	owner := x.actors[2]
	_, who := owner.call("GET", "/v1/me", "", nil)
	// Controlled credential revocation leaves the current owner and agent active.
	if _, err := x.db.Exec(context.Background(), `UPDATE undergrid.credentials SET revoked_at=clock_timestamp() WHERE agent_id=$1`, who["agent_id"]); err != nil {
		t.Fatal(err)
	}
	if code, _ := owner.call("GET", "/v1/me", "", nil); code != 401 {
		t.Fatal("revoked credential remained valid", code)
	}
	path := "/v1/owner/agents/" + who["agent_id"].(string) + "/tasks/" + x.task["id"].(string) + "/artifact-uploads"
	code, upload := ownerCallMatch(owner, "POST", path, NewID(), x.task["etag"].(string), map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": 12})
	if code != 201 || upload["upload_authentication"] != "owner_session" {
		t.Fatal("owner improperly depended on revoked network credential", code)
	}
	for _, change := range []string{"upload_authentication='credential'", "generation=gen_random_uuid()", "upload_digest=decode(repeat('00',32),'hex')", "upload_expires_at=upload_expires_at+interval '1 minute'"} {
		_, err := x.db.Exec(context.Background(), "UPDATE undergrid.artifacts SET "+change+" WHERE id=$1", upload["id"])
		var denied *pgconn.PgError
		if !errors.As(err, &denied) || denied.Code != "23514" {
			t.Fatal("upload binding not protected by constraint", change, err)
		}
	}
}
