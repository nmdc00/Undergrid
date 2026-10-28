package network

import (
	"encoding/json"
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
