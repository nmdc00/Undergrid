package network

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestPublicOpenAPIDoesNotRequireDatabaseOrCredentials(t *testing.T) {
	s := New("invalid database connection", "development", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/openapi.json", nil))
	if w.Code != 200 {
		t.Fatalf("public contract: %d %s", w.Code, w.Body.String())
	}
	var document map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document["openapi"] != "3.1.1" {
		t.Fatal(document["openapi"])
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatal(w.Header())
	}
	paths := document["paths"].(map[string]any)
	if paths["/v1/intents"] == nil || paths["/v1/owner/intents"] == nil {
		t.Fatal("network contracts missing")
	}
	if paths["/mcp"] != nil || paths["/v1/payments"] != nil {
		t.Fatal("unimplemented or financial route advertised")
	}
}
