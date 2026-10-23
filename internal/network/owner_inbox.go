package network

import (
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
)

func ownerInboxPath(path string) (agent, event string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 5 && len(parts) != 7 {
		return "", "", false
	}
	if parts[0] != "v1" || parts[1] != "owner" || parts[2] != "agents" || parts[4] != "inbox" || !validID(parts[3]) {
		return "", "", false
	}
	if len(parts) == 7 {
		if parts[6] != "ack" || !validID(parts[5]) {
			return "", "", false
		}
		event = parts[5]
	}
	return parts[3], event, true
}

// Owner sessions borrow attribution, never an agent bearer credential. Receipt
// updates remain the shared inbox command; they grant no business authority.
func (s *Service) ownerInbox(r *http.Request, tx pgx.Tx, principal, agent, event string) (int, any, error) {
	if event == "" && r.Method != http.MethodGet || event != "" && r.Method != http.MethodPost {
		return 0, nil, fail(405, "method_not_allowed")
	}
	// The owner transaction holds authority rows. Long polling belongs only to the
	// existing agent route outside a transaction, never this manual console read.
	if _, present := r.URL.Query()["wait_seconds"]; present {
		return 0, nil, fail(400, "owner_inbox_nonblocking_required")
	}
	a, err := lockOwnerAgent(r.Context(), tx, principal, agent)
	if err != nil {
		return 0, nil, err
	}
	a.Scopes = []string{"inbox:read"}
	command := r.Clone(r.Context())
	command.URL.Path = "/v1/inbox"
	if event != "" {
		command.URL.Path += "/" + event + "/ack"
	}
	return s.inboxCommand(command, tx, a)
}
