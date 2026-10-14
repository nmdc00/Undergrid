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
