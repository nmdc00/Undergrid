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

// Discovery, application, offer and admission are separate owner operations.
// Publishing these contracts must not silently substitute an invitation input
// containing a caller-selected target for the reviewed application offer.
func TestOpenAPIOwnerApplicationOperations(t *testing.T) {
	var document struct {
		Paths map[string]map[string]struct {
			Security   []map[string][]string `json:"security"`
			Parameters []struct {
				Name     string `json:"name"`
				Required bool   `json:"required"`
			} `json:"parameters"`
			RequestBody struct {
				Content map[string]struct {
					Schema struct {
						Ref string `json:"$ref"`
					} `json:"schema"`
				} `json:"content"`
			} `json:"requestBody"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(NetworkOpenAPI(), &document); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct{ path, method, input string }{
		{"/v1/owner/agents/{agent_id}/recruitment", "get", ""},
		{"/v1/owner/agents/{agent_id}/recruitment/{id}", "get", ""},
		{"/v1/owner/agents/{agent_id}/applications", "get", ""},
		{"/v1/owner/agents/{agent_id}/swarms/{id}/applications", "get", ""},
		{"/v1/owner/agents/{agent_id}/swarms/{id}/applications", "post", "OwnerApplicationInput"},
		{"/v1/owner/agents/{agent_id}/swarms/{id}/applications/{application_id}/offer", "post", "OwnerApplicationOfferInput"},
	} {
		t.Run(scenario.method+scenario.path, func(t *testing.T) {
			operation, ok := document.Paths[scenario.path][scenario.method]
			if !ok {
				t.Fatal("owner operation missing from public contract")
			}
			if len(operation.Security) != 1 {
				t.Fatal("owner session must be explicit")
			}
			if _, ok := operation.Security[0]["OwnerSession"]; !ok {
				t.Fatal("owner session absent")
			}
			if scenario.method == "post" {
				for _, name := range []string{"Idempotency-Key", "X-CSRF-Token", "If-Match"} {
					found := false
					for _, p := range operation.Parameters {
						if p.Name == name && p.Required {
							found = true
						}
					}
					if !found {
						t.Error("missing required reviewed-write header", name)
					}
				}
				if got := operation.RequestBody.Content["application/json"].Schema.Ref; got != "#/components/schemas/"+scenario.input {
					t.Fatal("incorrect command input", got)
				}
			}
		})
	}
}

// Independently validate actual owner response bytes, not domain helper structs.
func observeOwnerApplicationContract(t *testing.T, clients ...*testClient) {
	t.Helper()
	var cases []map[string]any
	observe := func(r *http.Request, w *httptest.ResponseRecorder, body any) {
		if !strings.HasPrefix(r.URL.Path, "/v1/owner/") {
			return
		}
		var response any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, map[string]any{"method": r.Method, "path": r.URL.String(), "request": body, "response": response, "status": w.Code, "request_id": w.Header().Get("X-Request-ID"), "etag": w.Header().Get("ETag")})
	}
	for _, c := range clients {
		c.observe = observe
	}
	t.Cleanup(func() {
		if t.Failed() {
			return
		}
		raw, err := json.Marshal(cases)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("node", "../../web/scripts/validate-network-contract.mjs", "--owner-applications")
		cmd.Stdin = bytes.NewReader(raw)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("actual owner application contract: %v\n%s", err, out)
		}
	})
}
