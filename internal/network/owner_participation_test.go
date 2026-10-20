package network

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestOwnerParticipationInvitationLifecycleWithoutAgentCredential(t *testing.T) {
	s, db := fixture(t)
	useOwnerParticipationLogin(t, s, db)
	ctx := context.Background()
	a, b, foreign := collaborationClient(t, s), collaborationClient(t, s), collaborationClient(t, s)
	_, identity := b.call("GET", "/v1/me", "", nil)
	agent := identity["agent_id"].(string)
	base := "/v1/owner/agents/" + agent
	var cases []map[string]any
	observe := func(r *http.Request, w *httptest.ResponseRecorder, body any) {
		// Unsupported verbs are asserted as405 below, not advertised as operations.
		if w.Code == 405 || !strings.HasPrefix(r.URL.Path, "/v1/owner/agents/") {
			return
		}
		var response any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"method": r.Method, "path": r.URL.String(), "request": body, "response": response, "status": w.Code, "request_id": w.Header().Get("X-Request-ID"), "etag": w.Header().Get("ETag")})
	}
	b.observe, foreign.observe = observe, observe
	defer func() {
		raw, err := json.Marshal(cases)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--owner-participation")
		cmd.Stdin = bytes.NewReader(raw)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("independent owner HTTP contract: %v %s", err, out)
		}
	}()

	code, room := a.call("POST", "/v1/swarms", "owner-participation-room", map[string]any{"objective": "Private collaboration", "visibility": "restricted", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	swarm := room["id"].(string)
	code, invite := a.call("POST", "/v1/swarms/"+swarm+"/invitations", "owner-participation-invite", map[string]any{"agent_id": agent, "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, invite)
	}
	code, out := a.call("POST", "/v1/swarms/"+swarm+"/messages", "private-history", map[string]any{"kind": "text", "body": "Private prior discussion"})
	if code != 201 {
		t.Fatal(code, out)
	}
	code, out = ownerCallMatch(b, "GET", base+"/participation", "", "", nil)
	if code != 200 {
		t.Fatalf("owner participation unavailable: %d %v", code, out)
	}
	if len(out["pending"].([]any)) != 1 || len(out["swarms"].([]any)) != 0 {
		t.Fatal(out)
	}
	code, out = ownerCallMatch(foreign, "GET", base+"/participation", "", "", nil)
	if code != 404 {
		t.Fatalf("foreign agent participation: %d %v", code, out)
	}
	code, out = ownerCallMatch(b, "GET", base+"/swarms/"+swarm+"/context", "", "", nil)
	if code != 404 {
		t.Fatalf("unaccepted history: %d %v", code, out)
	}
	code, out = ownerCallMatch(b, "GET", base+"/invitations/"+invite["id"].(string), "", "", nil)
	if code != 200 || out["objective"] != "Private collaboration" || out["history_policy"] != "full" || out["messages"] != nil {
		t.Fatal(code, out)
	}
	code, out = ownerCallMatch(foreign, "GET", base+"/invitations/"+invite["id"].(string), "", "", nil)
	if code != 404 {
		t.Fatal(code, out)
	}
	code, out = ownerCallMatch(b, "GET", base+"/participation?after=bad-cursor", "", "", nil)
	if code != 400 {
		t.Fatal(code, out)
	}
	code, out = ownerCallMatch(b, "POST", base+"/participation", "wrong-participation-method", "", map[string]any{})
	if code != 405 {
		t.Fatal(code, out)
	}
	body := map[string]any{"invitation_id": invite["id"], "decision": "accept"}
	path := base + "/swarms/" + swarm + "/membership-response"
	code, out = ownerCallMatch(b, "POST", path, "owner-no-match", "", body)
	if code != 428 {
		t.Fatal(code, out)
	}
	code, out = ownerCallMatch(b, "POST", path, "owner-stale-match", `"999"`, body)
	if code != 409 {
		t.Fatal(code, out)
	}
	code, out = ownerCallMatch(foreign, "POST", path, "foreign-decision", invite["etag"].(string), body)
	if code != 404 {
		t.Fatal(code, out)
	}
	for range 2 {
		code, out = ownerCallMatch(b, "POST", path, "owner-accept", invite["etag"].(string), body)
		if code != 200 || out["state"] != "accepted" {
			t.Fatal(code, out)
		}
	}
	var events int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='membership.accepted'", invite["id"]).Scan(&events); err != nil || events != 1 {
		t.Fatal(events, err)
	}
	code, out = ownerCallMatch(b, "GET", base+"/participation", "", "", nil)
	if code != 200 || len(out["pending"].([]any)) != 0 || len(out["swarms"].([]any)) != 1 {
		t.Fatal(code, out)
	}
	code, out = ownerCallMatch(b, "GET", base+"/swarms/"+swarm+"/context", "", "", nil)
	if code != 200 || len(out["messages"].([]any)) != 1 {
		t.Fatal(code, out)
	}
	code, out = b.call("POST", "/v1/swarms/"+swarm+"/withdraw", "withdraw-participation", map[string]any{})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = ownerCallMatch(b, "GET", base+"/swarms/"+swarm+"/context", "", "", nil)
	if code != 404 {
		t.Fatalf("withdrawn owner history: %d %v", code, out)
	}
	if _, err := db.Exec(ctx, "UPDATE undergrid.agents SET status='revoked' WHERE id=$1", agent); err != nil {
		t.Fatal(err)
	}
	code, out = ownerCallMatch(b, "GET", base+"/participation", "", "", nil)
	if code != 403 {
		t.Fatalf("revoked attribution agent accepted: %d %v", code, out)
	}

}

func useOwnerParticipationLogin(t *testing.T, s *Service, db *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Exec(ctx, `DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='undergrid_owner_participation_runner') THEN CREATE ROLE undergrid_owner_participation_runner LOGIN NOSUPERUSER NOBYPASSRLS; END IF;
 END $$; GRANT undergrid_app TO undergrid_owner_participation_runner;`); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgx.ParseConfig(s.dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.User, cfg.Password = "undergrid_owner_participation_runner", ""
	runtimeURL := url.URL{Scheme: "postgres", User: url.User(cfg.User), Path: "/" + cfg.Database, RawQuery: url.Values{"host": {cfg.Host}, "port": {strconv.Itoa(int(cfg.Port))}, "sslmode": {"disable"}}.Encode()}
	s.dsn = runtimeURL.String()
	check, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		t.Fatal(err)
	}
	var login string
	var unsafe bool
	err = check.QueryRow(ctx, "SELECT session_user,rolsuper OR rolbypassrls OR pg_has_role(current_user,'undergrid_worker','MEMBER') FROM pg_roles WHERE rolname=session_user").Scan(&login, &unsafe)
	check.Close(ctx)
	if err != nil || login != cfg.User || unsafe {
		t.Fatalf("unsafe runtime login: %s %v %v", login, unsafe, err)
	}
}
