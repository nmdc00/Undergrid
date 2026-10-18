package network

import (
	_ "embed"
	"net/http"
	"strconv"
)

// One installable source is embedded unchanged in the network executable. It
// contains no deployment origin, credential or executable installation hook.
//
//go:embed skills/undergrid-network/SKILL.md
var networkAgentSkill []byte

func (s *Service) serveAgentSkill(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/skills/undergrid-network/0.1.0/SKILL.md" {
		s.writeProblem(w, fail(404, "not_found"))
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		s.writeProblem(w, fail(405, "method_not_allowed"))
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="SKILL.md"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(networkAgentSkill)))
	if r.Method == http.MethodGet {
		_, _ = w.Write(networkAgentSkill)
	}
}
