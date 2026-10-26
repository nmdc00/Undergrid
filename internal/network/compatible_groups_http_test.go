package network

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
)

func TestCompatibleGroupsHTTPUsesCurrentPublicDemandAndPrincipalDedupe(t *testing.T) {
	s, _, workerDSN := semanticFixture(t)
	owners := []testClient{client(t, s), client(t, s), client(t, s)}
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Group view " + NewID()
	cases := []any{}
	observe := func(r *http.Request, w *httptest.ResponseRecorder, _ any) {
		if r.Method != "GET" {
			return
		}
		var out any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"method": r.Method, "path": r.URL.Path, "status": w.Code, "response": out, "request_id": w.Header().Get("X-Request-ID")})
	}
	for i := range owners {
		owners[i].observe = observe
	}
	defer func() {
		cmd := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--compatible-groups")
		cmd.Stdin = bytes.NewReader(canonical(cases))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("actual group HTTP contract: %v %s", err, out)
		}
	}()
	rows := []map[string]any{}
	for i, owner := range []testClient{owners[0], owners[1], owners[1], owners[2]} {
		code, row := owner.call("POST", "/v1/intents", "group-create-"+string(rune('a'+i)), input)
		if code != 201 {
			t.Fatal(code, row)
		}
		rows = append(rows, row)
	}
	t.Cleanup(func() {
		for i, owner := range []testClient{owners[0], owners[1], owners[1], owners[2]} {
			row := rows[i]
			if row["state"] == "withdrawn" {
				continue
			}
			code, result := owner.versioned("PATCH", "/v1/intents/"+row["id"].(string), "retire-group-"+row["id"].(string), row["etag"].(string), map[string]any{"state": "withdrawn"})
			if code != 200 {
				t.Errorf("retire group fixture: %d %v", code, result)
			}
		}
	})
	runner := New(workerDSN, "development", nil)
	if err := runner.scanIntentions(context.Background(), rows[0]["id"].(string)); err != nil {
		t.Fatal(err)
	}
	path := "/v1/intents/" + rows[0]["id"].(string) + "/compatible-groups"
	code, page := owners[0].call("GET", path, "", nil)
	if code != 200 {
		t.Fatalf("compatible group view unavailable: %d %v", code, page)
	}
	groups, ok := page["items"].([]any)
	if !ok || len(groups) == 0 {
		t.Fatal("no current compatible group", page)
	}
	for _, value := range groups {
		group := value.(map[string]any)
		members := group["members"].([]any)
		if len(members) != 3 {
			t.Fatal("same principal counted twice or foreign demand omitted", group)
		}
		principals := map[string]bool{}
		for _, value := range members {
			member := value.(map[string]any)
			principal := member["principal_id"].(string)
			if principals[principal] {
				t.Fatal("principal duplication", group)
			}
			principals[principal] = true
		}
	}
	if err := s.ConfigureMCP("https://network.fixture.test"); err != nil {
		t.Fatal(err)
	}
	peer := mcpSDKClient(t, s, owners[0].token)
	result, err := peer.CallTool(context.Background(), &mcp.CallToolParams{Name: "intention_compatible_groups", Arguments: map[string]any{"id": rows[0]["id"]}})
	if err != nil || result.IsError {
		t.Fatalf("MCP compatible groups: %v %+v", err, result)
	}
	if len(mcpToolData(t, result)["items"].([]any)) == 0 {
		t.Fatal("MCP lost compatible groups")
	}
	if err := s.ConfigureA2ATransport("https://network.fixture.test"); err != nil {
		t.Fatal(err)
	}
	body := a2aCommandBody(NewID(), "intention_compatible_groups", "", nil)
	commandArgs(body)["id"] = rows[0]["id"]
	code, out, _ := a2aCall(owners[0], "POST", "/a2a/message:send", body)
	if code != 200 || out["task"] == nil || len(a2aLiveTaskResult(out)["items"].([]any)) == 0 {
		t.Fatal("A2A compatible groups", code, out)
	}
	ownerPath := "/v1/owner/intents/" + rows[0]["id"].(string) + "/compatible-groups"
	code, out = ownerCallMatch(owners[0], "GET", ownerPath, "", "", nil)
	if code != 200 || len(out["items"].([]any)) == 0 {
		t.Fatal("owner group view", code, out)
	}
	// Withdrawal must affect the view immediately, without waiting for a worker.
	code, withdrawn := owners[2].versioned("PATCH", "/v1/intents/"+rows[3]["id"].(string), "withdraw-group-member", rows[3]["etag"].(string), map[string]any{"state": "withdrawn"})
	if code != 200 {
		t.Fatal(code, withdrawn)
	}
	rows[3] = withdrawn
	code, out = owners[0].call("GET", path, "", nil)
	if code != 200 {
		t.Fatal(code, out)
	}
	for _, value := range out["items"].([]any) {
		for _, value := range value.(map[string]any)["members"].([]any) {
			if value.(map[string]any)["intent_id"] == rows[3]["id"] {
				t.Fatal("withdrawn group member remained visible", out)
			}
		}
	}
	code, out = owners[1].call("GET", path, "", nil)
	if code != 404 {
		t.Fatal("foreign source ownership exposed", code, out)
	}
}
