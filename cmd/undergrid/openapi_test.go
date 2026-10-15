package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"undergrid/internal/network"
)

func TestExecutableRoutesPublicDocumentWithoutDatabase(t *testing.T) {
	mux := http.NewServeMux()
	registerNetworkRoutes(mux, network.New("unavailable database", "development", nil))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "static fallback", 404) })
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/openapi.json", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"openapi": "3.1.1"`) {
		t.Fatalf("executable contract route fell through: %d %s", w.Code, w.Body.String())
	}
	denied := httptest.NewRecorder()
	mux.ServeHTTP(denied, httptest.NewRequest("POST", "/openapi.json", nil))
	if denied.Code != 405 {
		t.Fatal(denied.Code)
	}
}

func TestExecutableRoutesMCPWithoutStaticFallback(t *testing.T) {
	service := network.New("unavailable database", "development", nil)
	if err := service.ConfigureMCP("https://network.fixture.test"); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerNetworkRoutes(mux, service)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "static fallback", 404) })
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`)))
	if w.Code != 401 || !strings.Contains(w.Body.String(), "invalid_credential") {
		t.Fatalf("MCP command mux fell through: %d %s", w.Code, w.Body.String())
	}
}
