package network

import (
	"context"
	"strings"
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

		foreign := client(t, h.service)
		stranger := mcpSDKClient(t, h.service, foreign.token)
		denied, err := stranger.CallTool(context.Background(), &mcp.CallToolParams{Name: "intention_constraint_assessments", Arguments: map[string]any{"id": h.source["id"]}})
		if err != nil || denied == nil || !denied.IsError || strings.Contains(string(canonical(denied)), "registered_demand_hard_evidence_v1") {
			t.Fatal("MCP foreign source disclosed", err, denied)
		}
		forged, err := peer.CallTool(context.Background(), &mcp.CallToolParams{Name: "intention_constraint_assessments", Arguments: map[string]any{"id": h.source["id"], "principal_id": NewID()}})
		if err != nil || forged == nil || !forged.IsError {
			t.Fatal("MCP accepted authority-shaped extra arguments", err, forged)
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

		taskID := out["task"].(map[string]any)["id"].(string)
		foreign := client(t, h.service)
		if code, denied, _ := a2aCall(foreign, "GET", "/a2a/tasks/"+taskID, nil); code != 404 || strings.Contains(string(canonical(denied)), "registered_demand_hard_evidence_v1") {
			t.Fatal("foreign A2A receipt disclosure", code, denied)
		}
		code, recovered, _ := a2aCall(h.owner, "GET", "/a2a/tasks/"+taskID, nil)
		if code != 200 {
			t.Fatal(code, recovered)
		}
		snapshot := func(task map[string]any) map[string]any {
			t.Helper()
			raw, ok := task["artifacts"].([]any)
			if !ok || len(raw) != 1 {
				t.Fatal("missing live resource snapshot", task)
			}
			artifact := raw[0].(map[string]any)
			part := artifact["parts"].([]any)[0].(map[string]any)
			data := part["data"].(map[string]any)
			if data["kind"] != "current_resource_snapshot" || data["resource_path"] != h.path(false) {
				t.Fatal("wrong resource reference", data)
			}
			return data["resource"].(map[string]any)
		}
		if actual := snapshot(recovered); string(canonical(actual)) != string(canonical(expected)) {
			t.Fatal("A2A initial current snapshot diverged", actual)
		}
		changed := h.publishedInput
		changed.HardConstraints.Coverage = &CoverageConstraint{Required: []string{"region:eu"}, Excluded: []string{}}
		code, updated := h.publisher.versioned("PATCH", "/v1/intents/"+h.candidate["id"].(string), NewID(), h.candidate["etag"].(string), map[string]any{"revision": changed})
		if code != 200 {
			t.Fatal(code, updated)
		}
		code, recovered, _ = a2aCall(h.owner, "GET", "/a2a/tasks/"+taskID, nil)
		if code != 200 {
			t.Fatal(code, recovered)
		}
		row := reportedCandidate(t, snapshot(recovered), h.candidate["id"].(string))
		if row == nil || row["hard_status"] != "aligned" || row["candidate_version"] != float64(2) || row["clarification_required"] != false {
			t.Fatal("A2A retained historical unknown declaration", row)
		}
		var stored string
		if err := h.db.QueryRow(context.Background(), `SELECT to_jsonb(r)::text FROM undergrid.a2a_command_receipts r WHERE id=$1`, taskID).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(stored, "registered_demand_hard_evidence_v1") || strings.Contains(stored, "coverage_unspecified") {
			t.Fatal("receipt stored report body", stored)
		}
		code, paused := h.owner.versioned("PATCH", "/v1/intents/"+h.source["id"].(string), NewID(), h.source["etag"].(string), map[string]any{"state": "paused"})
		if code != 200 {
			t.Fatal(code, paused)
		}
		code, recovered, _ = a2aCall(h.owner, "GET", "/a2a/tasks/"+taskID, nil)
		if code != 200 || recovered["artifacts"] != nil || strings.Contains(string(canonical(recovered)), h.path(false)) {
			t.Fatal("receipt resurrected noncurrent source", code, recovered)
		}
	})
}
