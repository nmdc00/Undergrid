package network

import (
	"context"
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
