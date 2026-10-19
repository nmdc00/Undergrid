package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"undergrid/internal/network"
)

func TestExecutableRoutesAgentSkillWithoutStaticFallback(t *testing.T) {
	mux := http.NewServeMux()
	registerNetworkRoutes(mux, network.New("unavailable database", "development", nil))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "static fallback", http.StatusTeapot)
	})
	path := "/skills/undergrid-network/0.1.0/SKILL.md"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	if w.Code != 200 || !strings.HasPrefix(w.Body.String(), "---\nname: undergrid-network\n") || w.Header().Get("Content-Disposition") != `attachment; filename="SKILL.md"` {
		t.Fatalf("executable skill download fell through: %d %v %s", w.Code, w.Header(), w.Body.String())
	}
	head := httptest.NewRecorder()
	mux.ServeHTTP(head, httptest.NewRequest("HEAD", path, nil))
	if head.Code != 200 || head.Body.Len() != 0 || head.Header().Get("Content-Length") != w.Header().Get("Content-Length") {
		t.Fatalf("executable skill HEAD: %d %v %s", head.Code, head.Header(), head.Body.String())
	}
	denied := httptest.NewRecorder()
	mux.ServeHTTP(denied, httptest.NewRequest("POST", path, nil))
	if denied.Code != 405 || denied.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("executable skill mutation: %d %s", denied.Code, denied.Body.String())
	}
	unknown := httptest.NewRecorder()
	mux.ServeHTTP(unknown, httptest.NewRequest("GET", "/skills/undergrid-network/9.9.9/SKILL.md", nil))
	if unknown.Code != 404 || !strings.Contains(unknown.Body.String(), "not_found") {
		t.Fatalf("unknown version reached static fallback: %d %s", unknown.Code, unknown.Body.String())
	}
}
