package network

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"
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
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		denied := httptest.NewRecorder()
		s.ServeHTTP(denied, httptest.NewRequest(method, "/openapi.json", nil))
		if denied.Code != 405 {
			t.Fatalf("document mutation %s: %d", method, denied.Code)
		}
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
		cases = append(cases, map[string]any{"method": r.Method, "path": r.URL.String(), "request": body, "response": response, "status": w.Code, "request_id": w.Header().Get("X-Request-ID"), "etag": w.Header().Get("ETag")})
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

	// Rights-scoped sharing and private report receipt remain different authority paths.
	_, recipient := b.call("GET", "/v1/me", "", nil)
	code, restricted := a.call("POST", "/v1/intents", "contract-restricted", sharingIntent())
	if code != 201 {
		t.Fatal(code, restricted)
	}
	restrictedPath := "/v1/intents/" + restricted["id"].(string)
	grantPath := "/v1/owner/intents/" + restricted["id"].(string) + "/grants"
	code, grant := a.versioned("POST", grantPath, "contract-grant", restricted["etag"].(string), map[string]any{"principal_id": recipient["principal_id"], "fields": []string{"objective"}, "expires_at": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, grant)
	}
	code, shared := b.call("GET", restrictedPath, "", nil)
	if code != 200 || shared["budget"] != nil || shared["hard_constraints"] != nil {
		t.Fatal(code, shared)
	}
	a.call("GET", grantPath, "", nil)
	code, _ = a.versioned("POST", grantPath+"/"+recipient["principal_id"].(string)+"/revoke", "contract-grant-revoke", grant["etag"].(string), map[string]any{})
	if code != 200 {
		t.Fatal(code)
	}
	if code, _ = b.call("GET", restrictedPath, "", nil); code != 404 {
		t.Fatal(code)
	}
	code, reporter := a.call("POST", "/v1/agents", "contract-reporter", map[string]any{"name": "Report client", "scopes": []string{"reports:read", "reports:write"}})
	if code != 201 {
		t.Fatal(code, reporter)
	}
	reportClient := a
	reportClient.token = reporter["token"].(string)
	code, receipt := reportClient.call("POST", "/v1/reports", "contract-report", map[string]any{"kind": "privacy", "subject": map[string]any{"kind": "artifact", "id": NewID()}, "statement": "Private report reference is opaque"})
	if code != 201 {
		t.Fatal(code, receipt)
	}
	reportClient.call("GET", "/v1/reports/"+receipt["id"].(string), "", nil)
	if code, _ = b.call("GET", "/v1/reports/"+receipt["id"].(string), "", nil); code != 403 {
		t.Fatal(code)
	}
	// Local trusted AS fixture proves resource contract, not external provider compatibility.
	as := newTestOAuthAS(t)
	as.configure(t, x.s)
	_, actor := a.call("GET", "/v1/me", "", nil)
	oauthClient := a
	oauthClient.token = as.token(t, NewID(), "contract-client", "intents:read inbox:read", time.Now().Add(time.Hour), "at+jwt", as.resource)
	bindingsPath := "/v1/owner/agents/" + actor["agent_id"].(string) + "/oauth-bindings"
	code, binding := oauthClient.versioned("POST", bindingsPath, "contract-oauth-binding", "", map[string]any{"scopes": []string{"intents:read", "inbox:read"}})
	if code != 201 {
		t.Fatal(code, binding)
	}
	oauthClient.call("GET", "/v1/me", "", nil)
	a.call("GET", bindingsPath, "", nil)
	code, _ = a.versioned("POST", "/v1/owner/oauth-bindings/"+binding["credential_id"].(string)+"/revoke", "contract-oauth-revoke", binding["etag"].(string), map[string]any{})
	if code != 200 {
		t.Fatal(code)
	}
	if code, _ = oauthClient.call("GET", "/v1/me", "", nil); code != 401 {
		t.Fatal(code)
	}
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
