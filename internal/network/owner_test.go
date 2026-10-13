package network

import "testing"

func TestOwnerCanReviewAndRevokeOnlyOwnCredentials(t *testing.T) {
	s, _ := fixture(t)
	a, b := client(t, s), client(t, s)
	code, list := a.call("GET", "/v1/agents", "", nil)
	if code != 200 {
		t.Fatal(code, list)
	}
	items := list["items"].([]any)
	if len(items) != 1 {
		t.Fatal(list)
	}
	agent := items[0].(map[string]any)
	credentials := agent["credentials"].([]any)
	if len(credentials) != 1 {
		t.Fatal(agent)
	}
	credential := credentials[0].(map[string]any)
	if credential["token"] != nil || credential["digest"] != nil {
		t.Fatal("credential secret exposed")
	}
	path := "/v1/owner/credentials/" + credential["id"].(string) + "/revoke"
	code, _ = b.call("POST", path, "revoke", map[string]any{})
	if code != 404 {
		t.Fatalf("foreign owner revoke %d", code)
	}
	code, out := a.call("POST", path, "revoke", map[string]any{})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, _ = a.call("GET", "/v1/me", "", nil)
	if code != 401 {
		t.Fatalf("revocation ineffective: %d", code)
	}
}
