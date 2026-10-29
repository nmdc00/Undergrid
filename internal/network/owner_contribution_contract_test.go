package network

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"testing"
)

func TestOwnerContributionActualHTTPMatchesPublishedContract(t *testing.T) {
	x := newArtifactScenario(t)
	owner := x.actors[2]
	_, who := owner.call("GET", "/v1/me", "", nil)
	base := "/v1/owner/agents/" + who["agent_id"].(string)
	var cases []map[string]any
	observe := func(r *http.Request, w *httptest.ResponseRecorder, body any) {
		var response any
		if w.Code != 204 {
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
		}
		cases = append(cases, map[string]any{"method": r.Method, "path": r.URL.String(), "request": body, "response": response, "status": w.Code, "request_id": w.Header().Get("X-Request-ID"), "etag": w.Header().Get("ETag")})
	}
	owner.observe = observe
	data := []byte(`{"sources":[{"source_id":"owner-contract","source_url":"https://example.org/"}]}`)
	create := base + "/tasks/" + x.task["id"].(string) + "/artifact-uploads"
	body := map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data)}
	if code, _ := ownerCallMatch(owner, "POST", create, NewID(), "", body); code != 428 {
		t.Fatal(code)
	}
	code, upload := ownerCallMatch(owner, "POST", create, NewID(), x.task["etag"].(string), body)
	if code != 201 {
		t.Fatal(code)
	}
	target, err := url.Parse(upload["upload_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("PUT", target.RequestURI(), bytes.NewReader(data))
	r.Header.Set("X-Test-Owner", owner.owner)
	r.Header.Set("Content-Type", "application/octet-stream")
	w := httptest.NewRecorder()
	owner.s.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal("owner bytes", w.Code)
	}
	observe(r, w, nil)
	path := base + "/artifacts/" + upload["id"].(string)
	key := NewID()
	for i := 0; i < 2; i++ {
		if code, _ := ownerCallMatch(owner, "POST", path+"/finalize", key, "", x.finalBody(data)); code != 200 {
			t.Fatal(code)
		}
	}
	for _, p := range []string{path, path + "/evaluations", base + "/tasks/" + x.task["id"].(string) + "/artifacts"} {
		if code, _ := ownerCallMatch(owner, "GET", p, "", "", nil); code != 200 {
			t.Fatal(code)
		}
	}
	// A finite owner contract also covers progress creation's explicit audience.
	progress := map[string]any{"claim_epoch": x.task["claim_epoch"], "media_type": "application/json", "size_bytes": len(data), "purpose": "progress", "audience_principal_ids": []string{x.principals[0], x.principals[2]}, "reviewer_principal_id": x.principals[0]}
	// Output submission intentionally makes a subsequent new generation ineligible.
	if code, _ := ownerCallMatch(owner, "POST", create, NewID(), x.task["etag"].(string), progress); code != 409 {
		t.Fatal(code)
	}
	raw, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--owner-contributions")
	cmd.Stdin = bytes.NewReader(raw)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Never expose capability URLs from a failing validator.
		message := strings.ReplaceAll(string(out), target.RawQuery, "[redacted capability]")
		t.Fatalf("owner contribution contract: %v\n%s", err, message)
	}
}
