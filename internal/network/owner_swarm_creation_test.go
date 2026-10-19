package network

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func ownerSwarmCreationFixture(t *testing.T) (*Service, *pgx.Conn) {
	t.Helper()
	s, db := fixture(t)
	if _, err := db.Exec(context.Background(), `DO $$ BEGIN IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='undergrid_owner_swarm_create_runner') THEN CREATE ROLE undergrid_owner_swarm_create_runner LOGIN NOSUPERUSER NOBYPASSRLS; END IF; END $$; GRANT undergrid_app TO undergrid_owner_swarm_create_runner`); err != nil {
		t.Fatal(err)
	}
	s.dsn = networkFixtureRoleDSN(t, db, "undergrid_owner_swarm_create_runner")
	check, err := pgx.Connect(context.Background(), s.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close(context.Background())
	var worker bool
	if err = check.QueryRow(context.Background(), "SELECT pg_has_role(current_user,'undergrid_worker','MEMBER')").Scan(&worker); err != nil || worker {
		t.Fatal("API login unexpectedly has worker authority", worker, err)
	}
	return s, db
}

func TestOwnerSwarmCreationLifecycleWithoutAgentBearer(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	owner, foreign := client(t, s), client(t, s)
	_, identity := owner.call("GET", "/v1/me", "", nil)
	agent := identity["agent_id"].(string)
	path := "/v1/owner/agents/" + agent + "/swarms"
	var cases []map[string]any
	observe := func(r *http.Request, w *httptest.ResponseRecorder, body any) {
		if w.Code == 405 || !strings.HasPrefix(r.URL.Path, "/v1/owner/agents/") {
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
		cmd := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--owner-swarm-creation")
		cmd.Stdin = bytes.NewReader(raw)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("independent owner creation contract: %v %s", err, out)
		}
	}()
	body := map[string]any{"objective": "Private untrusted <script>coordination</script>", "visibility": "restricted", "history_policy": "full", "recruitment_roles": []any{}}
	code, room := ownerCallMatch(owner, "POST", path, "owner-create-room", "", body)
	if code != 201 {
		t.Fatalf("owner-session creation unavailable: %d %v", code, room)
	}
	id := room["id"].(string)
	if room["visibility"] != "restricted" || room["history_policy"] != "full" {
		t.Fatal(room)
	}
	code, replay := ownerCallMatch(owner, "POST", path, "owner-create-room", "", body)
	if code != 201 || string(canonical(replay)) != string(canonical(room)) {
		t.Fatal("replay changed creation", code, replay)
	}
	changed := map[string]any{"objective": "Changed command", "visibility": "restricted", "history_policy": "full"}
	if code, out := ownerCallMatch(owner, "POST", path, "owner-create-room", "", changed); code != 409 {
		t.Fatal("changed replay accepted", code, out)
	}
	if code, out := ownerCallMatch(foreign, "POST", path, NewID(), "", body); code != 404 {
		t.Fatal("foreign agent create", code, out)
	}
	for _, invalid := range []map[string]any{
		{"objective": "Missing disclosure", "visibility": "private"},
		{"objective": "Wrong disclosure", "visibility": "private", "history_policy": "from_join"},
		{"objective": "Public disabled", "visibility": "public", "history_policy": "full"},
		{"objective": "Forged principal", "visibility": "private", "history_policy": "full", "principal_id": identity["principal_id"]},
		{"objective": "No public consent option", "visibility": "private", "history_policy": "full", "public_transcript_consent": true},
		{"objective": "Invalid role", "visibility": "restricted", "history_policy": "full", "recruitment_roles": []any{map[string]any{"key": "builder", "schema_family": "arbitrary_code"}}},
	} {
		want := 422
		if invalid["principal_id"] != nil || invalid["public_transcript_consent"] != nil {
			want = 400
		} // Closed JSON body, before domain validation.
		if code, out := ownerCallMatch(owner, "POST", path, NewID(), "", invalid); code != want {
			t.Fatal("invalid owner room accepted", code, out)
		}
	}
	if code, out := ownerCallMatch(owner, "POST", path, "", "", body); code != 400 {
		t.Fatal("missing command key", code, out)
	}
	if code, out := ownerCallMatch(owner, "GET", path, "", "", nil); code != 405 {
		t.Fatal("unsupported collection method", code, out)
	}
	if code, out := ownerCallMatch(owner, "GET", path+"/"+id+"/context", "", "", nil); code != 200 || out["swarm"].(map[string]any)["objective"] != body["objective"] {
		t.Fatal("owner current history", code, out)
	}
	if code, out := ownerCallMatch(foreign, "GET", path+"/"+id+"/context", "", "", nil); code != 404 {
		t.Fatal("foreign private history", code, out)
	}
	code, page := ownerCallMatch(owner, "GET", "/v1/owner/agents/"+agent+"/participation", "", "", nil)
	if code != 200 || len(page["swarms"].([]any)) != 1 {
		t.Fatal("created room missing from participation", code, page)
	}
	private := map[string]any{"objective": "Confidential owner room", "visibility": "private", "history_policy": "full"}
	if code, out := ownerCallMatch(owner, "POST", path, "owner-private-room", "", private); code != 201 || out["visibility"] != "private" {
		t.Fatal("private creation unavailable", code, out)
	}
	unauthenticated := owner
	unauthenticated.owner = ""
	if code, out := ownerCallMatch(unauthenticated, "POST", path, NewID(), "", body); code != 401 {
		t.Fatal("missing session admitted", code, out)
	}
	var rooms, members, events, recruit, fanout int
	if err := db.QueryRow(context.Background(), `SELECT
 (SELECT count(*) FROM undergrid.swarms WHERE creator_principal_id=$1),
 (SELECT count(*) FROM undergrid.memberships WHERE swarm_id=$2 AND agent_id=$3 AND role='coordinator' AND state='accepted'),
 (SELECT count(*) FROM undergrid.events WHERE aggregate_id=$2 AND type='swarm.created'),
 (SELECT count(*) FROM undergrid_queue.river_job WHERE kind='builder_recruitment_v1' AND args->>'swarm_id'=$2::text),
 (SELECT count(*) FROM undergrid_queue.river_job j JOIN undergrid.events e ON j.args->>'event_id'=e.id::text WHERE e.aggregate_id=$2 AND j.kind='event_fanout_v1')`, identity["principal_id"], id, agent).Scan(&rooms, &members, &events, &recruit, &fanout); err != nil {
		t.Fatal(err)
	}
	if rooms != 2 || members != 1 || events != 1 || recruit != 1 || fanout != 1 {
		t.Fatal("creation effects/rejected writes mismatch", rooms, members, events, recruit, fanout)
	}
	// A principal decision does not expand the selected agent's bearer scopes.
	if code, _ := owner.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Bearer still narrow", "visibility": "private"}); code != 403 {
		t.Fatal("owner created agent authority", code)
	}
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.agents SET status='revoked' WHERE id=$1", agent); err != nil {
		t.Fatal(err)
	}
	if code, out := ownerCallMatch(owner, "POST", path, "owner-create-room", "", body); code != 403 {
		t.Fatal("replay retained revoked agent authority", code, out)
	}
}

// Fault injection after room/membership/event/fanout proves queue failure rolls back creation;
// it is not claimed as an ordinary API-reachable database failure.
func TestOwnerSwarmCreationRollbackAfterRoomInsert(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	owner := client(t, s)
	_, identity := owner.call("GET", "/v1/me", "", nil)
	agent := identity["agent_id"].(string)
	name := "owner_create_fault_" + strings.ReplaceAll(NewID(), "-", "")
	quoted := pgx.Identifier{"undergrid", name}.Sanitize()
	sql := "CREATE FUNCTION " + quoted + "() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='builder_recruitment_v1' AND EXISTS(SELECT 1 FROM undergrid.swarms s WHERE s.id::text=NEW.args->>'swarm_id' AND s.coordinator_agent_id::text='" + agent + "') THEN RAISE EXCEPTION 'fixture recruitment failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER " + pgx.Identifier{name}.Sanitize() + " BEFORE INSERT ON undergrid_queue.river_job FOR EACH ROW EXECUTE FUNCTION " + quoted + "()"
	if _, err := db.Exec(context.Background(), sql); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), "DROP TRIGGER "+pgx.Identifier{name}.Sanitize()+" ON undergrid_queue.river_job; DROP FUNCTION "+quoted+"()")
	path := "/v1/owner/agents/" + agent + "/swarms"
	if code, out := ownerCallMatch(owner, "POST", path, NewID(), "", map[string]any{"objective": "Atomic room rollback", "visibility": "private", "history_policy": "full"}); code != 500 {
		t.Fatal("injected failure missing", code, out)
	}
	var rooms, events, commands int
	if err := db.QueryRow(context.Background(), `SELECT
 (SELECT count(*) FROM undergrid.swarms WHERE creator_principal_id=$1),
 (SELECT count(*) FROM undergrid.events WHERE principal_id=$1 AND type='swarm.created'),
 (SELECT count(*) FROM undergrid.idempotency WHERE actor_id=$2 AND operation LIKE '%swarms%')`, identity["principal_id"], agent).Scan(&rooms, &events, &commands); err != nil {
		t.Fatal(err)
	}
	if rooms != 0 || events != 0 || commands != 0 {
		t.Fatal("failed transaction retained creation", rooms, events, commands)
	}
}

func TestOwnerSwarmCreationSharesPrincipalQuotaWithAgentCommands(t *testing.T) {
	s, db := ownerSwarmCreationFixture(t)
	owner := collaborationClient(t, s)
	_, identity := owner.call("GET", "/v1/me", "", nil)
	agent := identity["agent_id"].(string)
	path := "/v1/owner/agents/" + agent + "/swarms"
	body := map[string]any{"objective": "Shared principal room quota", "visibility": "private", "history_policy": "full"}
	var first string
	for index := range 9 {
		key := NewID()
		if index == 0 {
			first = key
		}
		if code, out := ownerCallMatch(owner, "POST", path, key, "", body); code != 201 {
			t.Fatal(code, out)
		}
	}
	if code, out := owner.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "Agent uses the same quota", "visibility": "private"}); code != 201 {
		t.Fatal(code, out)
	}
	if code, out := ownerCallMatch(owner, "POST", path, NewID(), "", body); code != 429 {
		t.Fatal("owner bypassed principal room quota", code, out)
	}
	if code, out := ownerCallMatch(owner, "POST", path, first, "", body); code != 201 {
		t.Fatal("quota blocked committed replay", code, out)
	}
	var rooms int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.swarms WHERE creator_principal_id=$1", identity["principal_id"]).Scan(&rooms); err != nil || rooms != 10 {
		t.Fatal("quota failure retained room", rooms, err)
	}
}
