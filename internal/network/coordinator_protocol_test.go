package network

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http/httptest"
	"testing"
	"time"
)

// Durable A2A receipt recovery must re-read the proposal through current room
// authority rather than returning a stale embedded command response.
func TestCoordinatorA2AReceiptRecoversProposalResource(t *testing.T) {
	const proposal = "0193db4d-7a46-7000-8000-000000000001"
	const room = "0193db4d-7a46-7000-8000-000000000002"
	for _, tc := range []struct{ name, path string }{
		{"coordinator_propose", "/v1/swarms/" + room + "/coordinator-proposals"},
		{"coordinator_approve", "/v1/coordinator-proposals/" + proposal + "/approvals"},
		{"coordinator_accept", "/v1/coordinator-proposals/" + proposal + "/accept"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := a2aResourcePath(networkOperation{name: tc.name, method: "POST"}, httptest.NewRequest("POST", tc.path, nil), map[string]any{"id": proposal})
			if want := "/v1/coordinator-proposals/" + proposal; got != want {
				t.Fatalf("receipt resource=%q want %q", got, want)
			}
		})
	}
}

func TestCoordinatorSuccessionMCPA2ALifecycleAndReceiptRecovery(t *testing.T) {
	f := newSuccessionTestRoom(t)
	if err := f.s.ConfigureMCP("https://network.fixture.test"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ConfigureA2ATransport("https://network.fixture.test"); err != nil {
		t.Fatal(err)
	}
	peer := mcpSDKClient(t, f.s, f.successor.token)
	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		result, err := peer.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if result.IsError {
			t.Fatalf("%s: %+v", name, result)
		}
		return mcpToolData(t, result)
	}
	_, current := f.successor.call("GET", f.path+"/context", "", nil)
	p := call("coordinator_propose", map[string]any{"id": f.roomID, "if_match": current["swarm"].(map[string]any)["etag"], "idempotency_key": NewID(), "body": map[string]any{"successor_agent_id": f.successorID["agent_id"], "respond_by": time.Now().Add(time.Hour).UTC()}})
	body := a2aCommandBody(NewID(), "coordinator_accept", NewID(), map[string]any{"content_hash": p["content_hash"]})
	commandArgs(body)["id"], commandArgs(body)["if_match"] = p["id"], p["etag"]
	code, out, _ := a2aCall(f.successor, "POST", "/a2a/message:send", body)
	if code != 200 || out["task"] == nil {
		t.Fatal(code, out)
	}
	receipt := out["task"].(map[string]any)["id"].(string)
	if a2aLiveTaskResult(out)["state"] != "proposed" {
		t.Fatal("acceptance alone appointed successor", out)
	}
	f.assertOldCoordinator(t)
	call("coordinator_approve", map[string]any{"id": p["id"], "if_match": p["etag"], "idempotency_key": NewID(), "body": map[string]any{"decision": "approve", "content_hash": p["content_hash"]}})
	vote := a2aCommandBody(NewID(), "coordinator_approve", NewID(), map[string]any{"decision": "approve", "content_hash": p["content_hash"]})
	commandArgs(vote)["id"], commandArgs(vote)["if_match"] = p["id"], p["etag"]
	code, out, _ = a2aCall(f.peer, "POST", "/a2a/message:send", vote)
	if code != 200 || a2aLiveTaskResult(out)["state"] != "applied" {
		t.Fatal(code, out)
	}
	code, recovered, _ := a2aCall(f.successor, "GET", "/a2a/tasks/"+receipt, nil)
	if code != 200 {
		t.Fatal(code, recovered)
	}
	resource := recovered["artifacts"].([]any)[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["data"].(map[string]any)["resource"].(map[string]any)
	if resource["id"] != p["id"] || resource["state"] != "applied" {
		t.Fatal("receipt did not recover current proposal", code, recovered)
	}
	if got := call("coordinator_read", map[string]any{"id": p["id"]}); got["state"] != "applied" {
		t.Fatal(got)
	}
	foreign := successionClient(t, f.s)
	if code, out, _ := a2aCall(foreign, "GET", "/a2a/tasks/"+receipt, nil); code != 404 {
		t.Fatal("foreign receipt disclosure", code, out)
	}
}
