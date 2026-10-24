package network

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/riverqueue/river"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestOwnerInboxReceiptAndBoundCursor(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	owner, foreign := client(t, s), client(t, s)
	_, who := owner.call("GET", "/v1/me", "", nil)
	agent := who["agent_id"].(string)
	code, created := owner.call("POST", "/v1/intents", NewID(), intentFixture())
	if code != 201 {
		t.Fatal(code, created)
	}
	var cases []map[string]any
	observe := func(r *http.Request, w *httptest.ResponseRecorder, body any) {
		if !strings.Contains(r.URL.Path, "/inbox") {
			return
		}
		var response any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"method": r.Method, "path": r.URL.String(), "request": body, "response": response, "status": w.Code, "request_id": w.Header().Get("X-Request-ID"), "etag": w.Header().Get("ETag")})
	}
	owner.observe, foreign.observe = observe, observe
	defer func() {
		raw, err := json.Marshal(cases)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--owner-inbox")
		cmd.Stdin = bytes.NewReader(raw)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("owner inbox actual contract: %v %s", err, out)
		}
	}()
	path := "/v1/owner/agents/" + agent + "/inbox"
	code, page := ownerCallMatch(owner, "GET", path+"?limit=1", "", "", nil)
	if code != 200 {
		t.Fatalf("owner inbox unavailable: %d %v", code, page)
	}
	events := page["events"].([]any)
	if len(events) != 1 {
		t.Fatal("bounded page", page)
	}
	event := events[0].(map[string]any)
	if event["content_trust"] != "untrusted" {
		t.Fatal("trust", event)
	}
	if code, out := ownerCallMatch(foreign, "GET", path, "", "", nil); code != 404 {
		t.Fatal("foreign attribution", code, out)
	}
	_, peer := foreign.call("GET", "/v1/me", "", nil)
	if code, out := ownerCallMatch(foreign, "GET", "/v1/owner/agents/"+peer["agent_id"].(string)+"/inbox?cursor="+page["next_cursor"].(string), "", "", nil); code != 403 {
		t.Fatal("cursor audience", code, out)
	}
	for _, query := range []string{"?limit=51", "?wait_seconds=25"} {
		if code, out := ownerCallMatch(owner, "GET", path+query, "", "", nil); code != 400 {
			t.Fatal("bounded nonblocking inbox", code, out)
		}
	}
	ack := path + "/" + event["event_id"].(string) + "/ack"
	if code, out := ownerCallMatch(owner, "POST", ack, NewID(), "", map[string]any{"accept": true}); code != 400 {
		t.Fatal("closed receipt body", code, out)
	}
	for range 2 {
		if code, out := ownerCallMatch(owner, "POST", ack, "owner-receipt", "", map[string]any{}); code != 200 || out["acknowledged"] != true {
			t.Fatal("receipt", code, out)
		}
	}
	var acked time.Time
	var state string
	if err := db.QueryRow(context.Background(), "SELECT acked_at FROM undergrid.inbox WHERE agent_id=$1 AND event_id=$2", agent, event["event_id"]).Scan(&acked); err != nil || acked.IsZero() {
		t.Fatal("receipt persistence", acked, err)
	}
	if err := db.QueryRow(context.Background(), "SELECT state FROM undergrid.intents WHERE id=$1", created["id"]).Scan(&state); err != nil || state != "active" {
		t.Fatal("receipt changed intention", state, err)
	}
	old := s.encodeCursor(agent, 1, time.Now().Add(-31*24*time.Hour))
	if code, out := ownerCallMatch(owner, "GET", path+"?cursor="+old, "", "", nil); code != 410 {
		t.Fatal("expired cursor", code, out)
	}
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.principals SET status='suspended' WHERE id=$1", who["principal_id"]); err != nil {
		t.Fatal(err)
	}
	if code, out := ownerCallMatch(owner, "POST", ack, "owner-receipt", "", map[string]any{}); code != 403 {
		t.Fatal("suspended owner receipt", code, out)
	}
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.principals SET status='active' WHERE id=$1", who["principal_id"]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.agents SET status='revoked' WHERE id=$1", agent); err != nil {
		t.Fatal(err)
	}
	if code, out := ownerCallMatch(owner, "POST", ack, "owner-receipt", "", map[string]any{}); code != 403 {
		t.Fatal("revoked receipt replay", code, out)
	}
}

func TestOwnerInboxRechecksPrivateResourceAndReceiptRemainsSeparate(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	owner, peer := collaborationClient(t, s), collaborationClient(t, s)
	_, who := owner.call("GET", "/v1/me", "", nil)
	agent := who["agent_id"].(string)
	code, room := peer.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Private inbox resource", "visibility": "private", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	swarm := room["id"].(string)
	roomPath := "/v1/swarms/" + swarm
	code, offer := peer.call("POST", roomPath+"/invitations", NewID(), map[string]any{"agent_id": agent, "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, offer)
	}
	code, out := owner.versioned("POST", roomPath+"/membership-response", NewID(), offer["etag"].(string), map[string]any{"invitation_id": offer["id"], "decision": "accept"})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = peer.call("POST", roomPath+"/messages", NewID(), map[string]any{"body": "Private untrusted message", "kind": "text"})
	if code != 201 {
		t.Fatal(code, out)
	}
	var eventID string
	if err := db.QueryRow(context.Background(), "SELECT id FROM undergrid.events WHERE aggregate_id=$1 AND type='swarm.message'", swarm).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(context.Background(), `DO $$ BEGIN IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='undergrid_owner_inbox_worker') THEN CREATE ROLE undergrid_owner_inbox_worker LOGIN NOSUPERUSER NOBYPASSRLS; END IF; END $$; GRANT undergrid_worker TO undergrid_owner_inbox_worker`); err != nil {
		t.Fatal(err)
	}
	worker := fanoutWorker{dsn: networkFixtureRoleDSN(t, db, "undergrid_owner_inbox_worker")}
	for range 2 {
		if err := worker.Work(context.Background(), &river.Job[fanoutArgs]{Args: fanoutArgs{EventID: eventID}}); err != nil {
			t.Fatal(err)
		}
	}
	path := "/v1/owner/agents/" + agent + "/inbox"
	code, page := ownerCallMatch(owner, "GET", path, "", "", nil)
	if code != 200 || len(page["events"].([]any)) != 1 {
		t.Fatal("visible private reference", code, page)
	}
	if _, ok := page["events"].([]any)[0].(map[string]any)["body"]; ok {
		t.Fatal("inbox exposed message body", page)
	}
	code, out = owner.call("POST", roomPath+"/withdraw", NewID(), map[string]any{})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, page = ownerCallMatch(owner, "GET", path+"?limit=1", "", "", nil)
	if code != 200 || len(page["events"].([]any)) != 0 {
		t.Fatal("withdrawn private reference", code, page)
	}
	if sequence, err := s.decodeCursor(page["next_cursor"].(string), agent); err != nil || sequence != 1 {
		t.Fatal("hidden event cursor did not advance", sequence, err)
	}
	code, out = owner.call("GET", roomPath+"/context", "", nil)
	if code != 404 {
		t.Fatal("withdrawn private resource", code, out)
	}
	if code, out := ownerCallMatch(owner, "POST", path+"/"+eventID+"/ack", NewID(), "", map[string]any{}); code != 200 || out["acknowledged"] != true {
		t.Fatal("receipt of retained own event", code, out)
	}
	var state string
	var count int
	if err := db.QueryRow(context.Background(), "SELECT state,(SELECT count(*) FROM undergrid.inbox WHERE agent_id=$1 AND event_id=$3) FROM undergrid.memberships WHERE swarm_id=$2 AND agent_id=$1", agent, swarm, eventID).Scan(&state, &count); err != nil || state != "withdrawn" || count != 1 {
		t.Fatal("receipt changed authority or duplicate fanout", state, count, err)
	}
}
