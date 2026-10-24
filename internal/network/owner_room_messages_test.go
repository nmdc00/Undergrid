package network

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestOwnerRoomMessageAttributionAndRecovery(t *testing.T) {
	s, db := fixture(t)
	useOwnerParticipationLogin(t, s, db)
	a, foreign := collaborationClient(t, s), collaborationClient(t, s)
	_, identity := a.call("GET", "/v1/me", "", nil)
	code, room := a.call("POST", "/v1/swarms", "owner-message-room", map[string]any{"objective": "Private message proof", "visibility": "private", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	swarm := room["id"].(string)
	var cases []map[string]any
	observe := func(r *http.Request, w *httptest.ResponseRecorder, body any) {
		var response any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"method": r.Method, "path": r.URL.String(), "request": body, "response": response, "status": w.Code, "request_id": w.Header().Get("X-Request-ID"), "etag": w.Header().Get("ETag")})
	}
	a.observe, foreign.observe = observe, observe
	defer func() {
		raw, err := json.Marshal(cases)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--owner-room-messages")
		cmd.Stdin = bytes.NewReader(raw)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("actual message HTTP contract %v %s", err, out)
		}
	}()
	path := "/v1/owner/agents/" + identity["agent_id"].(string) + "/swarms/" + swarm + "/messages"
	body := map[string]any{"body": "<script>untrusted</script> https://example.invalid/agent"}
	code, out := ownerCallMatch(a, "POST", path, "owner-message", "", body)
	if code != 201 || out["content_trust"] != "untrusted" {
		t.Fatal(code, out)
	}
	first := out["id"]
	code, out = ownerCallMatch(a, "POST", path, "owner-message", "", body)
	if code != 201 || out["id"] != first {
		t.Fatal(code, out)
	}
	code, out = ownerCallMatch(foreign, "POST", path, "foreign-message", "", body)
	if code != 404 {
		t.Fatal(code, out)
	}
	code, out = ownerCallMatch(a, "POST", path, "edit-not-supported", "", map[string]any{"body": "rewrite", "replaces_id": first})
	if code != 400 {
		t.Fatal(code, out)
	}
	var count, events, members int
	var agent, principal, stored string
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.messages WHERE swarm_id=$1", swarm).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT agent_id,principal_id,body FROM undergrid.messages WHERE id=$1", first).Scan(&agent, &principal, &stored); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='swarm.message'", swarm).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.memberships WHERE swarm_id=$1", swarm).Scan(&members); err != nil {
		t.Fatal(err)
	}
	var eventID string
	if err := db.QueryRow(context.Background(), "SELECT id FROM undergrid.events WHERE aggregate_id=$1 AND type='swarm.message'", swarm).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	var queued int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE kind='event_fanout_v1' AND args->>'event_id'=$1", eventID).Scan(&queued); err != nil || queued != 1 {
		t.Fatal(queued, err)
	}
	if _, err := db.Exec(context.Background(), `DO $$ BEGIN IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='undergrid_owner_message_worker') THEN CREATE ROLE undergrid_owner_message_worker LOGIN NOSUPERUSER NOBYPASSRLS; END IF; END $$; GRANT undergrid_worker TO undergrid_owner_message_worker;`); err != nil {
		t.Fatal(err)
	}
	w := fanoutWorker{dsn: networkFixtureRoleDSN(t, db, "undergrid_owner_message_worker")}
	for range 2 {
		if err := w.Work(context.Background(), &river.Job[fanoutArgs]{Args: fanoutArgs{EventID: eventID}}); err != nil {
			t.Fatal(err)
		}
	}
	var delivered int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.inbox WHERE event_id=$1 AND agent_id=$2", eventID, agent).Scan(&delivered); err != nil || delivered != 1 {
		t.Fatal(delivered, err)
	}
	if count != 1 || events != 1 || members != 1 || agent != identity["agent_id"] || principal != identity["principal_id"] || stored != body["body"] {
		t.Fatal(count, events, members, agent, principal, stored, identity)
	}
}

func TestOwnerRoomMessageCurrentMembershipAndRoomRequired(t *testing.T) {
	s, db := fixture(t)
	useOwnerParticipationLogin(t, s, db)
	owner, observer, contributor, outsider := client(t, s), collaborationClient(t, s), collaborationClient(t, s), collaborationClient(t, s)
	code, credential := owner.call("POST", "/v1/agents", "message-coordinator", map[string]any{"name": "message coordinator", "scopes": []string{"swarms:read", "swarms:create", "members:invite", "swarms:archive"}, "profile": map[string]any{"public": true, "accept_invitations": true}})
	if code != 201 {
		t.Fatal(code, credential)
	}
	owner.token = credential["token"].(string)
	_, room := owner.call("POST", "/v1/swarms", "message-authority-room", map[string]any{"objective": "Current authority", "visibility": "private", "recruitment_roles": []any{}})
	swarm := room["id"].(string)
	path := func(c testClient) string {
		_, id := c.call("GET", "/v1/me", "", nil)
		return "/v1/owner/agents/" + id["agent_id"].(string) + "/swarms/" + swarm + "/messages"
	}
	body := map[string]any{"body": "hello"}
	for _, member := range []struct {
		client testClient
		role   string
	}{{observer, "observer"}, {contributor, "contributor"}} {
		_, id := member.client.call("GET", "/v1/me", "", nil)
		code, inv := owner.call("POST", "/v1/swarms/"+swarm+"/invitations", NewID(), map[string]any{"agent_id": id["agent_id"], "role": member.role, "respond_by": time.Now().Add(time.Hour)})
		if code != 201 {
			t.Fatal(code, inv)
		}
		code, out := ownerCallMatch(member.client, "POST", path(member.client), "unaccepted", "", body)
		if code != 404 {
			t.Fatal(code, out)
		}
		code, out = member.client.versioned("POST", "/v1/swarms/"+swarm+"/membership-response", NewID(), inv["etag"].(string), map[string]any{"invitation_id": inv["id"], "decision": "accept"})
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	code, out := ownerCallMatch(observer, "POST", path(observer), "observer", "", body)
	if code != 403 {
		t.Fatal(code, out)
	}
	code, out = ownerCallMatch(outsider, "POST", path(outsider), "outsider", "", body)
	if code != 404 {
		t.Fatal(code, out)
	}
	code, out = ownerCallMatch(contributor, "POST", path(contributor), "contributor", "", body)
	if code != 201 {
		t.Fatal(code, out)
	}
	code, out = contributor.call("POST", "/v1/swarms/"+swarm+"/withdraw", NewID(), map[string]any{})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = ownerCallMatch(contributor, "POST", path(contributor), "contributor", "", body)
	if code != 404 {
		t.Fatal("withdrawn replay", code, out)
	}
	_, current := owner.call("GET", "/v1/swarms/"+swarm+"/context", "", nil)
	code, out = owner.versioned("POST", "/v1/swarms/"+swarm+"/archive", NewID(), current["swarm"].(map[string]any)["etag"].(string), map[string]any{})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = ownerCallMatch(owner, "POST", path(owner), "archived", "", body)
	if code != 409 {
		t.Fatal(code, out)
	}
}

// Controlled database fault after message/event writes proves transaction rollback;
// this is not an ordinary externally reachable API request failure.
func TestOwnerRoomMessageQueueFailureRollsBack(t *testing.T) {
	s, db := fixture(t)
	useOwnerParticipationLogin(t, s, db)
	owner := collaborationClient(t, s)
	_, id := owner.call("GET", "/v1/me", "", nil)
	_, room := owner.call("POST", "/v1/swarms", "message-rollback-room", map[string]any{"objective": "Atomic text", "visibility": "private", "recruitment_roles": []any{}})
	swarm := room["id"].(string)
	name := "owner_message_fault_" + strings.ReplaceAll(NewID(), "-", "")
	quoted := pgx.Identifier{"undergrid", name}.Sanitize()
	sql := "CREATE FUNCTION " + quoted + "() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='event_fanout_v1' AND EXISTS(SELECT 1 FROM undergrid.events e WHERE e.id::text=NEW.args->>'event_id' AND e.aggregate_id::text='" + swarm + "' AND e.type='swarm.message') THEN RAISE EXCEPTION 'fixture message fanout failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER " + pgx.Identifier{name}.Sanitize() + " BEFORE INSERT ON undergrid_queue.river_job FOR EACH ROW EXECUTE FUNCTION " + quoted + "()"
	if _, err := db.Exec(context.Background(), sql); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), "DROP TRIGGER "+pgx.Identifier{name}.Sanitize()+" ON undergrid_queue.river_job; DROP FUNCTION "+quoted+"()")
	path := "/v1/owner/agents/" + id["agent_id"].(string) + "/swarms/" + swarm + "/messages"
	code, out := ownerCallMatch(owner, "POST", path, "rollback-text", "", map[string]any{"body": "must roll back"})
	if code != 500 {
		t.Fatal(code, out)
	}
	var count, events, commands int
	var sequence, version int64
	err := db.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM undergrid.messages WHERE swarm_id=$1),(SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='swarm.message'),(SELECT count(*) FROM undergrid.idempotency WHERE actor_id=$2 AND key='rollback-text'),sequence,version FROM undergrid.swarms WHERE id=$1`, swarm, id["agent_id"]).Scan(&count, &events, &commands, &sequence, &version)
	if err != nil || count != 0 || events != 0 || commands != 0 || sequence != 0 || version != 1 {
		t.Fatal(count, events, commands, sequence, version, err)
	}
}
