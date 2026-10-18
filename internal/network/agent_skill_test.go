package network

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestVersionedAgentSkillIsPublicDownloadWithoutDatabase(t *testing.T) {
	s := New("invalid database connection", "development", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/skills/undergrid-network/0.1.0/SKILL.md", nil))
	if w.Code != 200 {
		t.Fatalf("public installable skill: %d %s", w.Code, w.Body.String())
	}
	// The shipped document must be the exact installable source, not a rendered
	// copy that can lose frontmatter, change instructions or acquire credentials.
	source, err := os.ReadFile("skills/undergrid-network/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if w.Body.String() != string(source) || !strings.HasPrefix(w.Body.String(), "---\nname: undergrid-network\n") {
		t.Fatal("download differs from the installable skill")
	}
	if w.Header().Get("Content-Type") != "text/markdown; charset=utf-8" || w.Header().Get("Content-Disposition") != `attachment; filename="SKILL.md"` || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal(w.Header())
	}
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		denied := httptest.NewRecorder()
		s.ServeHTTP(denied, httptest.NewRequest(method, "/skills/undergrid-network/0.1.0/SKILL.md", nil))
		if denied.Code != 405 || denied.Header().Get("Allow") != "GET, HEAD" {
			t.Fatalf("skill mutation %s: %d %v", method, denied.Code, denied.Header())
		}
	}
	head := httptest.NewRecorder()
	s.ServeHTTP(head, httptest.NewRequest("HEAD", "/skills/undergrid-network/0.1.0/SKILL.md", nil))
	if head.Code != 200 || head.Body.Len() != 0 || head.Header().Get("Content-Type") != w.Header().Get("Content-Type") {
		t.Fatalf("HEAD: %d %v %s", head.Code, head.Header(), head.Body.String())
	}
}

func TestUnknownAgentSkillVersionFailsWithoutDatabase(t *testing.T) {
	s := New("invalid database connection", "development", nil)
	for _, path := range []string{"/skills/undergrid-network/9.9.9/SKILL.md", "/skills/undergrid-network/0.1.0/secrets", "/skills/undergrid-network/0.1.0/SKILL.md/extra"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatalf("unknown skill resource %s: %d %s", path, w.Code, w.Body.String())
		}
	}
}
