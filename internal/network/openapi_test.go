package network

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
)

func TestPublicOpenAPIDoesNotRequireDatabaseOrCredentials(t *testing.T) {
	s := New("invalid database connection", "development", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/openapi.json", nil))
	if w.Code != 200 {
		t.Fatalf("public contract: %d %s", w.Code, w.Body.String())
	}
	var document map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document["openapi"] != "3.1.1" {
		t.Fatal(document["openapi"])
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatal(w.Header())
	}
	paths := document["paths"].(map[string]any)
	if paths["/v1/intents"] == nil || paths["/v1/owner/intents"] == nil {
		t.Fatal("network contracts missing")
	}
	if paths["/mcp"] != nil || paths["/v1/payments"] != nil {
		t.Fatal("unimplemented or financial route advertised")
	}
}

// Ajv is an independent, pinned JSON Schema implementation. It checks actual
// request/response bytes rather than sharing the domain's validation helpers.
func TestOpenAPIRealHTTPLifecycleContract(t *testing.T) {
	x := newArtifactScenario(t)
	var cases []map[string]any
	observe := func(r *http.Request, w *httptest.ResponseRecorder, body any) {
		var response any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"method": r.Method, "path": r.URL.String(), "request": body, "response": response, "status": w.Code, "request_id": w.Header().Get("X-Request-ID")})
	}
	for i := range x.actors {
		x.actors[i].observe = observe
	}
	a, b := x.actors[0], x.actors[3]
	a.call("GET", "/v1/me", "", nil)
	a.call("GET", "/v1/owner/config", "", nil)
	a.call("GET", "/v1/owner/me", "", nil)
	a.call("GET", "/v1/agents", "", nil)
	a.call("GET", "/v1/owner/policy", "", nil)
	a.call("GET", "/v1/policies/contribution-templates", "", nil)
	input := intentFixture()
	code, intent := a.call("POST", "/v1/intents", "contract-intent", input)
	if code != 201 {
		t.Fatal(code, intent)
	}
	path := "/v1/intents/" + intent["id"].(string)
	a.call("POST", "/v1/intents", "contract-intent", input)
	input.Objective = "Changed canonical request"
	if code, _ := a.call("POST", "/v1/intents", "contract-intent", input); code != 409 {
		t.Fatal(code)
	}
	if code, _ := b.call("GET", path, "", nil); code != 404 {
		t.Fatal(code)
	}
	a.call("GET", path, "", nil)
	a.call("GET", "/v1/owner/intents", "", nil)
	if code, _ := a.versioned("PATCH", path, "pause", intent["etag"].(string), map[string]any{"state": "paused"}); code != 200 {
		t.Fatal(code)
	}
	if code, _ := a.versioned("PATCH", path, "stale", intent["etag"].(string), map[string]any{"state": "active"}); code != 409 {
		t.Fatal(code)
	}
	a.call("GET", "/v1/agent-context", "", nil)
	a.call("GET", "/v1/inbox", "", nil)
	a.call("GET", "/v1/swarms/"+x.room["id"].(string)+"/context", "", nil)
	a.call("GET", "/v1/plans/"+x.plan["id"].(string), "", nil)
	a.call("GET", "/v1/tasks/"+x.task["id"].(string), "", nil)
	id, artifact := x.sealAndApprove(t)
	artifactPath := "/v1/artifacts/" + id
	a.call("POST", artifactPath+"/publish", "contract-publish", map[string]any{"manifest_hash": artifact["manifest_hash"]})
	b.call("GET", artifactPath, "", nil)
	b.call("GET", "/v1/artifacts", "", nil)
	a.call("GET", "/v1/agent-context", "", nil)
	raw, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "../../web/scripts/validate-network-contract.mjs")
	cmd.Stdin = bytes.NewReader(raw)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("independent HTTP contract: %v\n%s", err, out)
	}
	t.Log(string(out))
}
