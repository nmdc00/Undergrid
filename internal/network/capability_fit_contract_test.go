package network

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestCapabilityFitActualHTTPMatchesPublishedContract(t *testing.T) {
	h := newClarificationFixture(t)
	samples := []any{}
	observe := func(r *http.Request, w *httptest.ResponseRecorder, _ any) {
		if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/capability-fit") {
			return
		}
		var body any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		samples = append(samples, map[string]any{"method": r.Method, "path": r.URL.Path, "status": w.Code, "response": body, "request": nil, "request_id": w.Header().Get("X-Request-ID"), "etag": w.Header().Get("ETag")})
	}
	h.owner.observe = observe
	collaborationClient(t, h.service)
	for _, owner := range []bool{true, false} {
		path := strings.ReplaceAll(h.path(owner), "/constraint-assessments", "/capability-fit")
		var code int
		var out map[string]any
		if owner {
			code, out = ownerCallMatch(h.owner, "GET", path, "", "", nil)
		} else {
			code, out = h.owner.call("GET", path, "", nil)
		}
		if code != 200 {
			t.Fatal(code, out)
		}
	}
	foreign := client(t, h.service)
	foreign.observe = observe
	for _, owner := range []bool{false, true} {
		if code, out := foreign.call("GET", strings.ReplaceAll(h.path(owner), "/constraint-assessments", "/capability-fit"), "", nil); code != 404 {
			t.Fatal("foreign contract sample disclosed", code, out)
		}
	}
	command := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--capability-fit")
	command.Stdin = bytes.NewReader(canonical(samples))
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("independent actual report contract: %v %s", err, out)
	}
}
