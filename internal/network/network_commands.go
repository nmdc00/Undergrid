package network

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Protocol adapters share this finite typed command registry. Owner and
// financial commands are deliberately outside the agent network surface.
type networkOperation struct{ name, method, path string }
type networkCommandArguments struct {
	ID             string          `json:"id,omitempty"`
	SecondaryID    string          `json:"secondary_id,omitempty"`
	Body           json.RawMessage `json:"body,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	IfMatch        string          `json:"if_match,omitempty"`
	Query          map[string]any  `json:"query,omitempty"`
}

var networkOperations = []networkOperation{
	{"credential_revoke_current", "DELETE", "/v1/credentials/current"},
	{"contribution_templates", "GET", "/v1/policies/contribution-templates"},
	{"opportunities_list", "GET", "/v1/opportunities"},
	{"opportunity_read", "GET", "/v1/opportunities/{id}"},
	{"invitation_read", "GET", "/v1/invitations/{id}"},
	{"artifacts_list", "GET", "/v1/artifacts"},
	{"artifact_input_grant_read", "GET", "/v1/artifact-input-grants/{id}"},
	{"artifact_input_grant_revoke", "POST", "/v1/artifact-input-grants/{id}/revoke"},
	{"inbox_acknowledge", "POST", "/v1/inbox/{id}/ack"},
	{"agent_context", "GET", "/v1/agent-context"},
	{"network_me", "GET", "/v1/me"},
	{"intention_create", "POST", "/v1/intents"},
	{"intention_read", "GET", "/v1/intents/{id}"},
	{"intention_revise", "PATCH", "/v1/intents/{id}"},
	{"intention_opportunities", "GET", "/v1/intents/{id}/opportunities"},
	{"intention_external_leads", "GET", "/v1/intents/{id}/external-leads"},
	{"intention_external_fit_report", "GET", "/v1/intents/{id}/external-fit-report"},
	{"inbox_read", "GET", "/v1/inbox"},
	{"swarm_create", "POST", "/v1/swarms"},
	{"swarm_context", "GET", "/v1/swarms/{id}/context"},
	{"swarm_invite", "POST", "/v1/swarms/{id}/invitations"},
	{"swarm_apply", "POST", "/v1/swarms/{id}/applications"},
	{"swarm_membership_respond", "POST", "/v1/swarms/{id}/membership-response"},
	{"swarm_message", "POST", "/v1/swarms/{id}/messages"},
	{"swarm_withdraw", "POST", "/v1/swarms/{id}/withdraw"},
	{"plan_propose", "POST", "/v1/swarms/{id}/plans"},
	{"plan_read", "GET", "/v1/plans/{id}"},
	{"plan_approve", "POST", "/v1/plans/{id}/approvals"},
	{"task_read", "GET", "/v1/tasks/{id}"},
	{"task_respond", "POST", "/v1/tasks/{id}/assignment-response"},
	{"task_heartbeat", "POST", "/v1/tasks/{id}/progress"},
	{"task_progress_submit", "POST", "/v1/tasks/{id}/progress-evidence"},
	{"task_progress_review", "POST", "/v1/tasks/{id}/progress-evidence-review"},
	{"task_progress_read", "GET", "/v1/tasks/{id}/progress-evidence/{evidence_id}"},
	{"artifact_upload_request", "POST", "/v1/artifacts/uploads"},
	{"artifact_read", "GET", "/v1/artifacts/{id}"},
	{"artifact_evaluations", "GET", "/v1/artifacts/{id}/evaluations"},
	{"artifact_finalize", "POST", "/v1/artifacts/{id}/finalize"},
	{"artifact_accept", "POST", "/v1/artifacts/{id}/acceptances"},
	{"artifact_publication_approve", "POST", "/v1/artifacts/{id}/publication-approvals"},
	{"artifact_publish", "POST", "/v1/artifacts/{id}/publish"},
	{"artifact_reuse", "POST", "/v1/artifacts/{id}/reuse-receipts"},
	{"artifact_input_grant", "POST", "/v1/artifacts/{id}/input-grants"},
	{"report_create", "POST", "/v1/reports"},
	{"report_read", "GET", "/v1/reports/{id}"},
}

func lookupNetworkOperation(name string) (networkOperation, bool) {
	for _, op := range networkOperations {
		if op.name == name {
			return op, true
		}
	}
	return networkOperation{}, false
}
func buildNetworkCommandRequest(ctx context.Context, authorization string, op networkOperation, in networkCommandArguments) (*http.Request, error) {
	path := op.path
	for marker, id := range map[string]string{"{id}": in.ID, "{evidence_id}": in.SecondaryID} {
		if strings.Contains(path, marker) {
			if !validID(id) {
				return nil, fail(400, "invalid_id")
			}
			path = strings.ReplaceAll(path, marker, id)
		}
	}
	if len(in.Query) > 0 {
		q := url.Values{}
		for k, v := range in.Query {
			q.Set(k, fmt.Sprint(v))
		}
		path += "?" + q.Encode()
	}
	r, err := http.NewRequestWithContext(ctx, op.method, path, bytes.NewReader(in.Body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Authorization", authorization)
	r.Header.Set("Idempotency-Key", in.IdempotencyKey)
	r.Header.Set("If-Match", in.IfMatch)
	return r, nil
}

// Reuse the published wire schemas rather than duplicating domain knowledge.
func networkOperationInputSchema(op networkOperation) (any, error) {
	var doc map[string]any
	if err := json.Unmarshal(NetworkOpenAPI(), &doc); err != nil {
		return nil, err
	}
	paths := doc["paths"].(map[string]any)
	route, ok := paths[op.path].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing network command canonical path %s", op.path)
	}
	operation, ok := route[strings.ToLower(op.method)].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing network command canonical method %s", op.name)
	}
	components := doc["components"].(map[string]any)
	properties := map[string]any{}
	required := []string{}
	if strings.Contains(op.path, "{id}") {
		properties["id"] = map[string]any{"type": "string", "format": "uuid"}
		required = append(required, "id")
	}
	if strings.Contains(op.path, "{evidence_id}") {
		properties["secondary_id"] = map[string]any{"type": "string", "format": "uuid"}
		required = append(required, "secondary_id")
	}
	// Canonical paths name parameters by their resource; resolve exact shapes below.
	if body, ok := operation["requestBody"].(map[string]any); ok {
		media := body["content"].(map[string]any)["application/json"].(map[string]any)
		properties["body"] = media["schema"]
		if body["required"] == true {
			required = append(required, "body")
		}
	}
	query := map[string]any{}
	queryRequired := []string{}
	parameters, _ := operation["parameters"].([]any)
	for _, raw := range parameters {
		parameter := raw.(map[string]any)
		if ref, ok := parameter["$ref"].(string); ok {
			parameter = components["parameters"].(map[string]any)[strings.TrimPrefix(ref, "#/components/parameters/")].(map[string]any)
		}
		name, _ := parameter["name"].(string)
		location, _ := parameter["in"].(string)
		arg := ""
		switch {
		case location == "header" && name == "Idempotency-Key":
			arg = "idempotency_key"
		case location == "header" && name == "If-Match":
			arg = "if_match"
		case location == "query":
			query[name] = parameter["schema"]
			if parameter["required"] == true {
				queryRequired = append(queryRequired, name)
			}
		}
		if arg != "" {
			properties[arg] = parameter["schema"]
			if parameter["required"] == true {
				required = append(required, arg)
			}
		}
	}
	if len(query) > 0 {
		properties["query"] = map[string]any{"type": "object", "properties": query, "additionalProperties": false, "required": queryRequired}
		if len(queryRequired) > 0 {
			required = append(required, "query")
		}
	}
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false, "required": required, "$defs": components["schemas"]}
	schema["$defs"] = reachableNetworkSchemas(properties, components["schemas"].(map[string]any))
	b, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	b = bytes.ReplaceAll(b, []byte("#/components/schemas/"), []byte("#/$defs/"))
	var result map[string]any
	err = json.Unmarshal(b, &result)
	return result, err
}

func reachableNetworkSchemas(value any, all map[string]any) map[string]any {
	used := map[string]any{}
	var scan func(any)
	scan = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for key, item := range x {
				if key == "$ref" {
					if ref, ok := item.(string); ok && strings.HasPrefix(ref, "#/components/schemas/") {
						name := strings.TrimPrefix(ref, "#/components/schemas/")
						if _, seen := used[name]; !seen {
							used[name] = all[name]
							scan(all[name])
						}
					}
				} else {
					scan(item)
				}
			}
		case []any:
			for _, item := range x {
				scan(item)
			}
		}
	}
	scan(value)
	return used
}
func networkOperationOutputSchema(op networkOperation) (any, error) {
	var doc map[string]any
	if err := json.Unmarshal(NetworkOpenAPI(), &doc); err != nil {
		return nil, err
	}
	operation := doc["paths"].(map[string]any)[op.path].(map[string]any)[strings.ToLower(op.method)].(map[string]any)
	statuses := []any{}
	dataSchemas := []any{}
	for status, raw := range operation["responses"].(map[string]any) {
		if !strings.HasPrefix(status, "2") {
			continue
		}
		var code int
		if _, err := fmt.Sscanf(status, "%d", &code); err != nil {
			return nil, err
		}
		statuses = append(statuses, code)
		response := raw.(map[string]any)
		if content, ok := response["content"].(map[string]any); ok {
			if media, ok := content["application/json"].(map[string]any); ok {
				dataSchemas = append(dataSchemas, media["schema"])
			}
		} else {
			dataSchemas = append(dataSchemas, map[string]any{"type": "null"})
		}
	}
	if len(statuses) == 0 || len(dataSchemas) == 0 {
		return nil, fmt.Errorf("missing network command success schema for %s", op.name)
	}
	props := map[string]any{"status": map[string]any{"type": "integer", "enum": statuses}, "data": map[string]any{"anyOf": dataSchemas}}
	defs := reachableNetworkSchemas(props, doc["components"].(map[string]any)["schemas"].(map[string]any))
	schema := map[string]any{"type": "object", "properties": props, "required": []string{"status", "data"}, "additionalProperties": false, "$defs": defs}
	b, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	b = bytes.ReplaceAll(b, []byte("#/components/schemas/"), []byte("#/$defs/"))
	var out map[string]any
	err = json.Unmarshal(b, &out)
	return out, err
}
