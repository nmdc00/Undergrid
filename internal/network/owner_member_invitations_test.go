package network

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"
)

func TestOwnerMemberInvitationReviewedRoomAndSeparateAdmission(t *testing.T) {
	s, db := fixture(t)
	useOwnerParticipationLogin(t, s, db)
	a, b, foreign := collaborationClient(t, s), collaborationClient(t, s), collaborationClient(t, s)
	var cases []map[string]any
	observe := func(r *http.Request, w *httptest.ResponseRecorder, body any) {
		var response any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"method": r.Method, "path": r.URL.String(), "request": body, "response": response, "status": w.Code, "request_id": w.Header().Get("X-Request-ID"), "etag": w.Header().Get("ETag")})
	}
	defer func() {
		raw, err := json.Marshal(cases)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--owner-member-invitations")
		cmd.Stdin = bytes.NewReader(raw)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("actual owner invitation contract: %v %s", err, out)
		}
	}()

	_, identity := a.call("GET", "/v1/me", "", nil)
	_, target := b.call("GET", "/v1/me", "", nil)
	code, room := a.call("POST", "/v1/swarms", "invitation-owner-room", map[string]any{"objective": "Reviewed private collaboration", "visibility": "private", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	swarm := room["id"].(string)
	path := "/v1/owner/agents/" + identity["agent_id"].(string) + "/swarms/" + swarm + "/invitations"
	body := map[string]any{"agent_id": target["agent_id"], "role": "contributor", "respond_by": time.Now().Add(time.Hour), "history_policy": "full"}
	a.observe, foreign.observe = observe, observe
	for _, tc := range []struct {
		match  string
		status int
	}{{"", 428}, {`"999"`, 409}} {
		code, out := ownerCallMatch(a, "POST", path, "denied-"+tc.match, tc.match, body)
		if code != tc.status {
			t.Fatalf("review precondition: %d %v", code, out)
		}
	}
	code, out := ownerCallMatch(foreign, "POST", path, "foreign-invite", room["etag"].(string), body)
	if code != 404 {
		t.Fatal(code, out)
	}
	delete(body, "history_policy")
	code, out = ownerCallMatch(a, "POST", path, "missing-history", room["etag"].(string), body)
	if code != 422 {
		t.Fatal(code, out)
	}
	body["history_policy"] = "full"
	for range 2 {
		code, out = ownerCallMatch(a, "POST", path, "owner-invite", room["etag"].(string), body)
		if code != 201 || out["state"] != "offered" {
			t.Fatal(code, out)
		}
	}
	var offered, events int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.memberships WHERE swarm_id=$1 AND agent_id=$2 AND state='offered'", swarm, target["agent_id"]).Scan(&offered); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='membership.offered'", out["id"]).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if offered != 1 || events != 1 {
		t.Fatal(offered, events)
	}
	code, out = ownerCallMatch(b, "GET", "/v1/owner/agents/"+target["agent_id"].(string)+"/swarms/"+swarm+"/context", "", "", nil)
	if code != 404 {
		t.Fatalf("invitation granted history: %d %v", code, out)
	}
}
