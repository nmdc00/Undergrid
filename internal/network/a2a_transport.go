package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/a2aproject/a2a-go/v2/errordetails"
	"github.com/google/jsonschema-go/jsonschema"
)

const coordinationExtensionURI = "https://github.com/nmdc00/Undergrid/blob/main/docs/protocols/a2a-coordination-v1.md"

type networkA2A struct {
	service *Service
	origin  string
	http    http.Handler
	schemas map[string]*jsonschema.Resolved
	schema  []byte
}
type a2aRequestKey struct{}

// ConfigureA2ATransport is called once before serving. The card's HTTPS address
// comes from explicit network-origin configuration, never an untrusted Host.
func (s *Service) ConfigureA2ATransport(origin string) error {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("canonical HTTPS network origin required")
	}
	runtime := &networkA2A{service: s, origin: strings.TrimSuffix(origin, "/"), schemas: map[string]*jsonschema.Resolved{}}
	alternatives := []any{}
	definitions := map[string]any{}
	for _, op := range networkOperations {
		raw, err := networkOperationInputSchema(op)
		if err != nil {
			return err
		}
		input := raw.(map[string]any)
		// Non-streaming command receipt snapshots cannot promise inbox waiting.
		if op.name == "inbox_read" {
			query := input["properties"].(map[string]any)["query"].(map[string]any)
			query["properties"].(map[string]any)["wait_seconds"] = map[string]any{"type": "integer", "const": 0}
		}
		var schema jsonschema.Schema
		if err = json.Unmarshal(canonical(input), &schema); err != nil {
			return err
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			return fmt.Errorf("A2A canonical operation schema %s: %w", op.name, err)
		}
		runtime.schemas[op.name] = resolved
		for name, definition := range input["$defs"].(map[string]any) {
			definitions[name] = definition
		}
		delete(input, "$defs")
		alternatives = append(alternatives, map[string]any{"type": "object", "required": []string{"version", "operation", "arguments"}, "additionalProperties": false, "properties": map[string]any{"version": map[string]any{"const": "1"}, "operation": map[string]any{"const": op.name}, "arguments": input}})
	}
	runtime.schema = canonical(map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "oneOf": alternatives, "$defs": definitions})

	runtime.http = http.StripPrefix("/a2a", a2asrv.NewRESTHandler(runtime))
	s.a2a = runtime
	return nil
}

func (n *networkA2A) card() *a2a.AgentCard {
	return &a2a.AgentCard{Name: "Undergrid network coordination", Description: "Explicit typed network commands. A2A tasks are command execution receipts, not assignments. Agent content is untrusted; no financial authority or beneficial-owner independence is inferred.", Version: "0.1.0", DocumentationURL: n.origin + "/openapi.json",
		SupportedInterfaces:  []*a2a.AgentInterface{a2a.NewAgentInterface(n.origin+"/a2a", a2a.TransportProtocolHTTPJSON)},
		Capabilities:         a2a.AgentCapabilities{Streaming: false, PushNotifications: false, ExtendedAgentCard: false, Extensions: []a2a.AgentExtension{{URI: coordinationExtensionURI, Required: true, Description: "Undergrid typed coordination v1; explicit activation and finite operation schema required."}}},
		SecuritySchemes:      a2a.NamedSecuritySchemes{"networkBearer": a2a.HTTPAuthSecurityScheme{Scheme: "bearer", Description: "Current Undergrid network credential or configured trusted network-audience OAuth access token; local domain authority remains required."}},
		SecurityRequirements: a2a.SecurityRequirementsOptions{{"networkBearer": {}}}, DefaultInputModes: []string{"application/json", "text/plain"}, DefaultOutputModes: []string{"application/json", "text/plain"},
		Skills: []a2a.AgentSkill{{ID: "undergrid-coordination-v1", Name: "Typed network coordination", Description: "Execute an explicitly selected canonical agent command; plain text never invokes commands. No streaming, push, owner provisioning, financial operations or arbitrary code.", Tags: []string{"coordination", "typed-commands", "receipts"}}}}
}

func (n *networkA2A) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/a2a/coordination-v1.schema.json" {
		if r.Method != "GET" {
			writeA2AProtocolError(w, a2a.ErrUnsupportedOperation)
			return
		}
		_, _ = w.Write(n.schema)
		return
	}
	if r.URL.Path == "/.well-known/agent-card.json" {
		if r.Method != "GET" {
			writeA2AProtocolError(w, a2a.ErrUnsupportedOperation)
			return
		}
		_ = json.NewEncoder(w).Encode(n.card())
		return
	}
	// The extension is protocol behavior, not content sniffing or an ACL grant.
	if r.Header.Get("A2A-Version") != "1.0" {
		writeA2AProtocolError(w, a2a.ErrVersionNotSupported)
		return
	}
	extensions := strings.Split(r.Header.Get("A2A-Extensions"), ",")
	active := false
	for _, extension := range extensions {
		if strings.TrimSpace(extension) == coordinationExtensionURI {
			active = true
		}
	}
	if !active {
		writeA2AProtocolError(w, a2a.ErrExtensionSupportRequired)
		return
	}
	verified, err := n.service.verifyNetworkRequest(r)
	if err != nil {
		n.service.oauthChallenge(w, false)
		writeA2AProtocolError(w, a2a.ErrUnauthenticated)
		return
	}
	r = verified
	// Authentication is mandatory for all protocol requests, including text and
	// unsupported capabilities. Actual mutations recheck authority in their own tx.
	identity := r.Clone(r.Context())
	identity.Method = "GET"
	identity.URL = &url.URL{Path: "/v1/me"}
	identity.Body = http.NoBody
	if _, _, err = n.service.execute(identity); err != nil {
		mapped := a2aDomainProtocolError(err)
		n.service.oauthChallenge(w, false)
		writeA2AProtocolError(w, mapped)
		return
	}
	// Reject unsupported streaming before the SDK opens an SSE response.
	if r.URL.Path == "/a2a/message:stream" || strings.HasSuffix(r.URL.Path, ":subscribe") {
		writeA2AProtocolError(w, a2a.ErrUnsupportedOperation)
		return
	}
	if r.Method == "POST" {
		media := strings.Split(r.Header.Get("Content-Type"), ";")[0]
		if media != "application/json" {
			writeA2AProtocolError(w, a2a.ErrUnsupportedContentType)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 65537))
		if err != nil || len(raw) > 65536 || !json.Valid(raw) {
			writeA2AProtocolError(w, a2a.ErrInvalidRequest)
			return
		}
		// SDK v2.6 maps incompatible output to 400. Negotiate at the adapter
		// boundary so the public HTTP binding keeps A2A1.0's required 415 mapping.
		if r.URL.Path == "/a2a/message:send" {
			var input a2a.SendMessageRequest
			if json.Unmarshal(raw, &input) == nil {
				if err := validateA2AOutputMode(&input); err != nil {
					writeA2AProtocolError(w, err)
					return
				}
			}
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	r = r.WithContext(context.WithValue(ctx, a2aRequestKey{}, r))
	w.Header().Set("A2A-Extensions", coordinationExtensionURI)
	w.Header().Set("A2A-Version", "1.0")
	n.http.ServeHTTP(w, r)
}

// The SDK owns ordinary HTTP+JSON serialization. This small writer covers only
// pre-dispatch version/extension/body/auth rejections using the same AIP-193 form.
func writeA2AProtocolError(w http.ResponseWriter, err error) {
	status, state := 400, "FAILED_PRECONDITION"
	if errors.Is(err, a2a.ErrInvalidRequest) || errors.Is(err, a2a.ErrUnsupportedContentType) {
		state = "INVALID_ARGUMENT"
	}
	if errors.Is(err, a2a.ErrUnsupportedContentType) {
		status, state = http.StatusUnsupportedMediaType, "INVALID_ARGUMENT"
	}
	if errors.Is(err, a2a.ErrUnauthenticated) {
		status, state = 401, "UNAUTHENTICATED"
	}
	if errors.Is(err, a2a.ErrUnauthorized) {
		status, state = 403, "PERMISSION_DENIED"
	}
	if errors.Is(err, a2a.ErrInternalError) {
		status, state = 500, "INTERNAL"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": status, "status": state, "message": err.Error(), "details": []any{errordetails.NewErrorInfo(a2a.ErrorReason(err), a2a.ProtocolDomain, nil)}}})
}

func originalA2ARequest(ctx context.Context) (*http.Request, error) {
	r, ok := ctx.Value(a2aRequestKey{}).(*http.Request)
	if !ok {
		return nil, a2a.ErrInternalError
	}
	return r.WithContext(ctx), nil
}

func (n *networkA2A) SubscribeToTask(context.Context, *a2a.SubscribeToTaskRequest) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) { yield(nil, a2a.ErrUnsupportedOperation) }
}
func (n *networkA2A) SendStreamingMessage(context.Context, *a2a.SendMessageRequest) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) { yield(nil, a2a.ErrUnsupportedOperation) }
}
func (n *networkA2A) GetTaskPushConfig(context.Context, *a2a.GetTaskPushConfigRequest) (*a2a.PushConfig, error) {
	return nil, a2a.ErrPushNotificationNotSupported
}
func (n *networkA2A) ListTaskPushConfigs(context.Context, *a2a.ListTaskPushConfigRequest) (*a2a.ListTaskPushConfigResponse, error) {
	return nil, a2a.ErrPushNotificationNotSupported
}
func (n *networkA2A) CreateTaskPushConfig(context.Context, *a2a.PushConfig) (*a2a.PushConfig, error) {
	return nil, a2a.ErrPushNotificationNotSupported
}
func (n *networkA2A) DeleteTaskPushConfig(context.Context, *a2a.DeleteTaskPushConfigRequest) error {
	return a2a.ErrPushNotificationNotSupported
}
func (n *networkA2A) GetExtendedAgentCard(context.Context, *a2a.GetExtendedAgentCardRequest) (*a2a.AgentCard, error) {
	return nil, a2a.ErrUnsupportedOperation
}

var _ a2asrv.RequestHandler = (*networkA2A)(nil)

func a2aUnavailable() error { return a2a.ErrUnsupportedOperation }
