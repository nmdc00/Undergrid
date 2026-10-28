package network

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"strings"
	"testing"
)

func TestCapabilityFitFiniteProtocolsUseCurrentAuthorizedRead(t *testing.T) {
	h := newClarificationFixture(t)
	path := "/v1/intents/" + h.source["id"].(string) + "/capability-fit"
	code, expected := h.owner.call("GET", path, "", nil)
	if code != 200 {
		t.Fatal(code, expected)
	}
	t.Run("MCP", func(t *testing.T) {
		if err := h.service.ConfigureMCP("https://network.fixture.test"); err != nil {
			t.Fatal(err)
		}
		peer := mcpSDKClient(t, h.service, h.owner.token)
		result, err := peer.CallTool(context.Background(), &mcp.CallToolParams{Name: "intention_capability_fit", Arguments: map[string]any{"id": h.source["id"]}})
		if err != nil || result == nil || result.IsError {
			t.Fatalf("MCP capability read unavailable: %v %+v", err, result)
		}
		if actual := mcpToolData(t, result); string(canonical(actual)) != string(canonical(expected)) {
			t.Fatal("MCP differs from current HTTP", actual, expected)
		}
		foreign := client(t, h.service)
		stranger := mcpSDKClient(t, h.service, foreign.token)
		denied, err := stranger.CallTool(context.Background(), &mcp.CallToolParams{Name: "intention_capability_fit", Arguments: map[string]any{"id": h.source["id"]}})
		if err != nil || denied == nil || !denied.IsError || strings.Contains(string(canonical(denied)), h.source["id"].(string)) {
			t.Fatal("foreign MCP source disclosed", err, denied)
		}
	})
	t.Run("A2A", func(t *testing.T) {
		if err := h.service.ConfigureA2ATransport("https://network.fixture.test"); err != nil {
			t.Fatal(err)
		}
		body := a2aCommandBody(NewID(), "intention_capability_fit", "", nil)
		commandArgs(body)["id"] = h.source["id"]
		code, out, _ := a2aCall(h.owner, "POST", "/a2a/message:send", body)
		if code != 200 || out["task"] == nil {
			t.Fatalf("A2A capability read unavailable: %d %v", code, out)
		}
		if actual := a2aLiveTaskResult(out); string(canonical(actual)) != string(canonical(expected)) {
			t.Fatal("A2A differs from current HTTP", actual, expected)
		}
		taskID := out["task"].(map[string]any)["id"].(string)
		foreign := client(t, h.service)
		if code, out, _ := a2aCall(foreign, "GET", "/a2a/tasks/"+taskID, nil); code != 404 {
			t.Fatal("foreign receipt readable", code, out)
		}
		// Recovery reauthorizes live resource access rather than storing report bodies.
		code, recovered, _ := a2aCall(h.owner, "GET", "/a2a/tasks/"+taskID, nil)
		if code != 200 || recovered["artifacts"] == nil {
			t.Fatal("live recovery absent", code, recovered)
		}
		var stored string
		if err := h.db.QueryRow(context.Background(), `SELECT to_jsonb(r)::text FROM undergrid.a2a_command_receipts r WHERE id=$1`, taskID).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(stored, "constraint_evidence") || strings.Contains(stored, "common_control_unknown") {
			t.Fatal("receipt retained resource body", stored)
		}
		code, out = h.owner.versioned("PATCH", "/v1/intents/"+h.source["id"].(string), NewID(), h.source["etag"].(string), map[string]any{"state": "paused"})
		if code != 200 {
			t.Fatal(code, out)
		}
		code, recovered, _ = a2aCall(h.owner, "GET", "/a2a/tasks/"+taskID, nil)
		if code != 200 || recovered["artifacts"] != nil || strings.Contains(string(canonical(recovered)), path) {
			t.Fatal("paused source resurrected", code, recovered)
		}
	})
}
