package network

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestOwnerPlanProposalActualHTTPMatchesPublishedContract(t *testing.T) {
	s, _ := ownerSwarmCreationFixture(t)
	enableTestTerms(t, s)
	owner := client(t, s)
	_, identity := owner.call("GET", "/v1/me", "", nil)
	agent, principal := identity["agent_id"].(string), identity["principal_id"].(string)
	base := "/v1/owner/agents/" + agent
	code, room := ownerCallMatch(owner, "POST", base+"/swarms", NewID(), "", map[string]any{"objective": "Published owner plan contract", "visibility": "restricted", "history_policy": "full", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	var cases []map[string]any
	owner.observe = func(r *http.Request, w *httptest.ResponseRecorder, body any) {
		if !strings.HasSuffix(r.URL.Path, "/plan-context") && !strings.HasSuffix(r.URL.Path, "/plans") {
			return
		}
		var response any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"method": r.Method, "path": r.URL.String(), "request": body, "response": response, "status": w.Code, "request_id": w.Header().Get("X-Request-ID"), "etag": w.Header().Get("ETag")})
	}
	path := base + "/swarms/" + room["id"].(string)
	if code, out := ownerCallMatch(owner, "GET", path+"/plan-context", "", "", nil); code != 200 {
		t.Fatal(code, out)
	}
	if code, out := ownerCallMatch(owner, "POST", path+"/plans", NewID(), room["etag"].(string), ownerPlanProposalSpec(agent, principal)); code != 201 {
		t.Fatal(code, out)
	}
	if code, out := ownerCallMatch(owner, "GET", path+"/plan-context", "", "", nil); code != 200 {
		t.Fatal(code, out)
	}
	raw, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--owner-plan-proposal")
	cmd.Stdin = bytes.NewReader(raw)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("actual owner proposal/context contract:%v %s", err, out)
	}
}
