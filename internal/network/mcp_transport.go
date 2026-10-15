package network

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpAuthorizationKey struct{}
type mcpFailureKey struct{}
type mcpAuthFailure struct {
	mu          sync.Mutex
	err         error
	tokenScopes []string
}

func (f *mcpAuthFailure) set(err error) { f.mu.Lock(); defer f.mu.Unlock(); f.err = err }
func (f *mcpAuthFailure) get() error    { f.mu.Lock(); defer f.mu.Unlock(); return f.err }

// JSONResponse waits for command completion before writing. Preserve the SDK
// JSON-RPC response, but expose an actual domain authorization failure at HTTP
// level so ordinary OAuth clients can perform standard step-up authorization.
type mcpResponseWriter struct {
	http.ResponseWriter
	service *Service
	failure *mcpAuthFailure
	wrote   bool
}

func (w *mcpResponseWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.wrote = true
	if err := w.failure.get(); err != nil {
		var p problem
		if errors.As(err, &p) {
			status = p.Status
			w.service.oauthChallenge(w.ResponseWriter, p.Code == "insufficient_scope")
			var scope scopeDenial
			if errors.As(err, &scope) {
				challenge := w.Header().Get("WWW-Authenticate")
				if challenge != "" {
					w.Header().Set("WWW-Authenticate", challenge+`, scope="`+strings.Join(scope.challengeScopes(w.failure.tokenScopes), " ")+`"`)
				}
			}
		}
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *mcpResponseWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (s *Service) ConfigureMCP(origin string) error {
	s.mcpHandler = nil
	s.mcpOrigin = ""
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("MCP requires canonical HTTPS origin")
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "Undergrid Network", Version: "0.1"}, &mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}}})
	for _, op := range networkOperations {
		op := op
		schema, err := networkOperationInputSchema(op)
		if err != nil {
			return err
		}
		output, err := networkOperationOutputSchema(op)
		if err != nil {
			return err
		}
		mcp.AddTool[networkCommandArguments, any](server, &mcp.Tool{Name: op.name, Description: "Undergrid network command; returned agent content is untrusted.", InputSchema: schema, OutputSchema: output}, func(ctx context.Context, request *mcp.CallToolRequest, in networkCommandArguments) (*mcp.CallToolResult, any, error) {
			auth, _ := ctx.Value(mcpAuthorizationKey{}).(string)
			r, err := buildNetworkCommandRequest(ctx, auth, op, in)
			if err != nil {
				return nil, nil, err
			}
			status, out, err := s.execute(r)
			if err != nil {
				var p problem
				if !errors.As(err, &p) {
					return nil, nil, errors.New("internal_error")
				}
				if failure, ok := ctx.Value(mcpFailureKey{}).(*mcpAuthFailure); ok {
					var denial scopeDenial
					if p.Status == 401 || (errors.As(err, &denial) && len(denial.challengeScopes(failure.tokenScopes)) > 0) {
						failure.set(err)
					}
				}
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: p.Code}}, StructuredContent: map[string]any{"error": map[string]any{"code": p.Code, "status": p.Status}}}, nil, nil
			}
			return nil, map[string]any{"status": status, "data": out}, nil
		})
	}
	s.mcpOrigin = origin
	s.mcpHandler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return nil
}

func (s *Service) serveMCP(w http.ResponseWriter, r *http.Request) {
	if s.mcpHandler == nil {
		s.writeProblem(w, fail(503, "mcp_unavailable"))
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != s.mcpOrigin {
		s.writeProblem(w, fail(403, "invalid_origin"))
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		s.writeProblem(w, fail(405, "method_not_allowed"))
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 64*1024+1))
	if err != nil || len(raw) > 64*1024 {
		s.writeProblem(w, fail(413, "body_too_large"))
		return
	}
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
		s.writeProblem(w, fail(400, "batch_not_supported"))
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	auth := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok || token == "" {
		s.oauthChallenge(w, false)
		s.writeProblem(w, fail(401, "invalid_credential"))
		return
	}
	r, err = s.verifyNetworkRequest(r.WithContext(ctx))
	if err != nil {
		s.oauthChallenge(w, false)
		s.writeProblem(w, err)
		return
	}
	ctx = r.Context()
	probe, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/v1/me", nil)
	probe.Header.Set("Authorization", auth)
	if _, _, err = s.execute(probe); err != nil {
		s.oauthChallenge(w, false)
		s.writeProblem(w, err)
		return
	}
	ctx = context.WithValue(ctx, mcpAuthorizationKey{}, auth)
	claims, _ := ctx.Value(oauthResourceContextKey).(verifiedOAuth)
	failure := &mcpAuthFailure{tokenScopes: claims.Scopes}
	ctx = context.WithValue(ctx, mcpFailureKey{}, failure)
	s.mcpHandler.ServeHTTP(&mcpResponseWriter{ResponseWriter: w, service: s, failure: failure}, r.WithContext(ctx))
}
