package network

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPDiscoveryThroughNetworkService(t *testing.T) {
	s, _ := fixture(t)
	if err := s.ConfigureMCP("https://network.fixture.test"); err != nil {
		t.Fatal(err)
	}
	a := client(t, s)
	r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(mcpWireBody("server/discover", "2026-07-28", nil)))
	r.Header.Set("Authorization", "Bearer "+a.token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	r.Header.Set("Mcp-Method", "server/discover")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || out["result"] == nil {
		t.Fatalf("MCP discovery unavailable: %d %s", w.Code, w.Body.String())
	}
}

type mcpBearerTransport struct{ token string }

func (a mcpBearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	copy.Header = r.Header.Clone()
	copy.Header.Set("Authorization", "Bearer "+a.token)
	return http.DefaultTransport.RoundTrip(copy)
}
func mcpSDKClient(t *testing.T, s *Service, token string) *mcp.ClientSession {
	t.Helper()
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "independent-release-client", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp", HTTPClient: &http.Client{Transport: mcpBearerTransport{token}}, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
func mcpToolData(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	b, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err = json.Unmarshal(b, &envelope); err != nil {
		t.Fatal(err)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing structured data: %+v", result)
	}
	return data
}
func TestMCPIndependentSDKUsesDomainIdempotencyAndPrivacy(t *testing.T) {
	s, db := fixture(t)
	a, b := client(t, s), client(t, s)
	if err := s.ConfigureMCP("https://network.fixture.test"); err != nil {
		t.Fatal(err)
	}
	peer := mcpSDKClient(t, s, a.token)
	listed, err := peer.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != len(networkMCPOperations) {
		t.Fatalf("tool list mismatch %d", len(listed.Tools))
	}
	for _, tool := range listed.Tools {
		if strings.Contains(tool.Name, "owner") || strings.Contains(tool.Name, "financial") {
			t.Fatal("privileged tool exposed")
		}
	}
	args := map[string]any{"body": intentFixture(), "idempotency_key": "mcp-create-retry"}
	result, err := peer.CallTool(context.Background(), &mcp.CallToolParams{Name: "intention_create", Arguments: args})
	if err != nil || result.IsError {
		t.Fatalf("create: %v %+v", err, result)
	}
	created := mcpToolData(t, result)
	id := created["id"].(string)
	result, err = peer.CallTool(context.Background(), &mcp.CallToolParams{Name: "intention_create", Arguments: args})
	if err != nil || result.IsError || mcpToolData(t, result)["id"] != id {
		t.Fatalf("replay: %v %+v", err, result)
	}
	var events, jobs int
	if err = db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='intent.created'", id).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid_queue.river_job WHERE kind='intention_scan_v1' AND args->>'cause_id'=$1", id).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if events != 1 || jobs != 1 {
		t.Fatalf("retry duplicated effects events=%d jobs=%d", events, jobs)
	}
	result, err = peer.CallTool(context.Background(), &mcp.CallToolParams{Name: "intention_read", Arguments: map[string]any{"id": id}})
	if err != nil || result.IsError || mcpToolData(t, result)["id"] != id {
		t.Fatalf("read: %v %+v", err, result)
	}
	foreign := mcpSDKClient(t, s, b.token)
	result, err = foreign.CallTool(context.Background(), &mcp.CallToolParams{Name: "intention_read", Arguments: map[string]any{"id": id}})
	if err != nil || !result.IsError {
		t.Fatalf("foreign read succeeded: %v %+v", err, result)
	}
	invalid := map[string]any{"body": intentFixture(), "idempotency_key": "mcp-invalid", "arbitrary_action": "run"}
	result, err = peer.CallTool(context.Background(), &mcp.CallToolParams{Name: "intention_create", Arguments: invalid})
	if err != nil || !result.IsError {
		t.Fatalf("unknown arguments accepted: %v %+v", err, result)
	}
	var count int
	if err = db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.intents WHERE agent_id=(SELECT agent_id FROM undergrid.events WHERE aggregate_id=$1 LIMIT 1)", id).Scan(&count); err != nil || count != 1 {
		t.Fatalf("invalid schema side effect count=%d err=%v", count, err)
	}
}

func TestMCPOAuthCurrentScopeAndRevocation(t *testing.T) {
	s, db := fixture(t)
	a := client(t, s)
	as := newTestOAuthAS(t)
	as.configure(t, s)
	_, me := a.call("GET", "/v1/me", "", nil)
	agent := me["agent_id"].(string)
	token := as.token(t, "mcp-subject", "mcp-client", "intents:read intents:write", time.Now().Add(time.Hour), "at+jwt", as.resource)
	code, binding, _ := oauthCall(t, s, a.owner, token, "POST", "/v1/owner/agents/"+agent+"/oauth-bindings", "mcp-oauth-bind", "", map[string]any{"scopes": []string{"intents:read", "intents:write"}})
	if code != 201 {
		t.Fatalf("bind: %d %v", code, binding)
	}
	if err := s.ConfigureMCP("https://undergrid.fixture.test"); err != nil {
		t.Fatal(err)
	}
	narrow := as.token(t, "mcp-subject", "mcp-client", "intents:read", time.Now().Add(time.Hour), "at+jwt", as.resource)
	peer := mcpSDKClient(t, s, narrow)
	result, err := peer.CallTool(context.Background(), &mcp.CallToolParams{Name: "intention_create", Arguments: map[string]any{"body": intentFixture(), "idempotency_key": "mcp-narrow"}})
	if err != nil || !result.IsError {
		t.Fatalf("narrow token wrote: %v %+v", err, result)
	}
	var count int
	if err = db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.intents WHERE agent_id=$1", agent).Scan(&count); err != nil || count != 0 {
		t.Fatalf("narrow side effect: %d %v", count, err)
	}
	code, _, _ = oauthCall(t, s, a.owner, "", "POST", "/v1/owner/oauth-bindings/"+binding["credential_id"].(string)+"/revoke", "mcp-revoke", `"1"`, map[string]any{})
	if code != 200 {
		t.Fatalf("revoke: %d", code)
	}
	// Raw HTTP proves every later request reauthenticates; an SDK connection
	// and cached tool discovery cannot preserve authority after revocation.
	request := httptest.NewRequest("POST", "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != 401 || !strings.Contains(response.Header().Get("WWW-Authenticate"), "resource_metadata=") {
		t.Fatalf("revoked OAuth retained MCP: %d %s", response.Code, response.Body.String())
	}
}

func TestMCPHTTPProtocolAndOriginGuards(t *testing.T) {
	s, _ := fixture(t)
	a := client(t, s)
	if err := s.ConfigureMCP("https://network.fixture.test"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, origin, version, method, body string
		status                              int
		needle                              string
	}{
		{"foreign origin", "https://foreign.fixture.test", "2026-07-28", "server/discover", `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`, 403, "invalid_origin"},
		{"header method mismatch", "", "2026-07-28", "tools/list", `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`, 400, ""},
		{"unsupported version", "", "2099-01-01", "tools/list", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`, 400, ""},
		{"bounded body", "", "2026-07-28", "server/discover", strings.Repeat("x", 64*1024+1), 413, "body_too_large"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/mcp", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+a.token)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Accept", "application/json, text/event-stream")
			r.Header.Set("Mcp-Protocol-Version", tc.version)
			r.Header.Set("Mcp-Method", tc.method)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.needle) {
				t.Fatalf("guard: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func mcpWireBody(method, version string, params map[string]any) []byte {
	if params == nil {
		params = map[string]any{}
	}
	params["_meta"] = map[string]any{mcp.MetaKeyProtocolVersion: version, mcp.MetaKeyClientInfo: map[string]any{"name": "raw-independent-client", "version": "1"}, mcp.MetaKeyClientCapabilities: map[string]any{}}
	return canonical(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
}
func TestMCPInsufficientScopeReturnsHTTPChallenge(t *testing.T) {
	s, _ := fixture(t)
	a := client(t, s)
	as := newTestOAuthAS(t)
	as.configure(t, s)
	_, me := a.call("GET", "/v1/me", "", nil)
	agent := me["agent_id"].(string)
	full := as.token(t, "mcp-challenge-subject", "mcp-challenge-client", "intents:read intents:write", time.Now().Add(time.Hour), "at+jwt", as.resource)
	code, _, _ := oauthCall(t, s, a.owner, full, "POST", "/v1/owner/agents/"+agent+"/oauth-bindings", "bind-scope-challenge", "", map[string]any{"scopes": []string{"intents:read", "intents:write"}})
	if code != 201 {
		t.Fatal(code)
	}
	if err := s.ConfigureMCP("https://undergrid.fixture.test"); err != nil {
		t.Fatal(err)
	}
	narrow := as.token(t, "mcp-challenge-subject", "mcp-challenge-client", "intents:read", time.Now().Add(time.Hour), "at+jwt", as.resource)
	r := httptest.NewRequest("POST", "/mcp", bytes.NewReader(mcpWireBody("tools/call", "2026-07-28", map[string]any{"name": "intention_create", "arguments": map[string]any{"body": intentFixture(), "idempotency_key": "scope-challenge"}})))
	r.Header.Set("Authorization", "Bearer "+narrow)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("Mcp-Method", "tools/call")
	r.Header.Set("Mcp-Name", "intention_create")
	r.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	challenge := w.Header().Get("WWW-Authenticate")
	if w.Code != 403 || !strings.Contains(challenge, `error="insufficient_scope"`) || !strings.Contains(challenge, `scope="intents:write"`) || !strings.Contains(challenge, "resource_metadata=") {
		t.Fatalf("scope challenge: %d %v %s", w.Code, w.Header(), w.Body.String())
	}
}
