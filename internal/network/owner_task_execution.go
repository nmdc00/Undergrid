package network

import (
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
)

func ownerTaskExecutionRoute(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	return len(parts) == 7 && parts[0] == "v1" && parts[1] == "owner" && parts[2] == "agents" && validID(parts[3]) && parts[4] == "tasks" && validID(parts[5]) && parts[6] == "progress"
}

func (s *Service) ownerTaskExecution(r *http.Request, tx pgx.Tx, principal string) (int, any, error) {
	if r.Method != http.MethodPost {
		return 0, nil, fail(405, "method_not_allowed")
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	a, err := lockOwnerAgent(r.Context(), tx, principal, parts[3])
	if err != nil {
		return 0, nil, err
	}
	a.Scopes = []string{"tasks:progress", "rights:accept"}
	command := r.Clone(r.Context())
	command.URL.Path = "/v1/tasks/" + parts[5] + "/progress"
	// The domain owns room/policy/dependency/version/epoch checks, event effects,
	// idempotency and unchanged heartbeat clocks, exactly as for agent commands.
	return s.planCommand(command, tx, a)
}
