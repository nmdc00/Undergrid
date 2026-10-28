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

func TestConstraintAssessmentActualHTTPMatchesPublishedContract(t *testing.T) {
	h := newClarificationFixture(t)
	samples := []any{}
	observe := func(r *http.Request, w *httptest.ResponseRecorder, _ any) {
		if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/constraint-assessments") {
			return
		}
		var body any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		samples = append(samples, map[string]any{"method": r.Method, "path": r.URL.Path, "status": w.Code, "response": body, "request": nil, "request_id": w.Header().Get("X-Request-ID"), "etag": w.Header().Get("ETag")})
	}
	h.owner.observe = observe
	h.read(t, true)
	h.read(t, false)
	foreign := client(t, h.service)
	foreign.observe = observe
	for _, owner := range []bool{false, true} {
		if code, out := foreign.call("GET", h.path(owner), "", nil); code != 404 {
			t.Fatal("foreign contract sample disclosed", code, out)
		}
	}
	command := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--constraint-assessments")
	command.Stdin = bytes.NewReader(canonical(samples))
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("independent actual report contract: %v %s", err, out)
	}
}
