package network

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestConstraintAssessmentFiniteProtocolsUseAuthorizedDomainRead(t *testing.T) {
	h := newClarificationFixture(t)
	expected := h.read(t, false)
	t.Run("MCP", func(t *testing.T) {
		if err := h.service.ConfigureMCP("https://network.fixture.test"); err != nil {
			t.Fatal(err)
		}
		peer := mcpSDKClient(t, h.service, h.owner.token)
		result, err := peer.CallTool(context.Background(), &mcp.CallToolParams{Name: "intention_constraint_assessments", Arguments: map[string]any{"id": h.source["id"]}})
		if err != nil || result == nil || result.IsError {
			t.Fatalf("MCP hard-evidence read unavailable: %v %+v", err, result)
		}
		if actual := mcpToolData(t, result); string(canonical(actual)) != string(canonical(expected)) {
			t.Fatal("MCP diverged from current authorized HTTP report", actual, expected)
		}
	})
	t.Run("A2A", func(t *testing.T) {
		if err := h.service.ConfigureA2ATransport("https://network.fixture.test"); err != nil {
			t.Fatal(err)
		}
		body := a2aCommandBody(NewID(), "intention_constraint_assessments", "", nil)
		commandArgs(body)["id"] = h.source["id"]
		code, out, _ := a2aCall(h.owner, "POST", "/a2a/message:send", body)
		if code != 200 || out["task"] == nil {
			t.Fatalf("A2A hard-evidence read unavailable: %d %v", code, out)
		}
		if actual := a2aLiveTaskResult(out); string(canonical(actual)) != string(canonical(expected)) {
			t.Fatal("A2A diverged from current authorized HTTP report", actual, expected)
		}
	})
}
