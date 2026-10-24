package network

import (
	"bytes"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// Owner text is attributed to an owned active agent, never interpreted as a
// command or consent. The shared domain command owns room state and storage.
func (s *Service) ownerRoomMessage(r *http.Request, tx pgx.Tx, a actor, swarm string) (int, any, error) {
	if err := roomAuthority(r.Context(), tx, a, swarm, false); err != nil {
		return 0, nil, err
	}
	var contributor bool
	if err := tx.QueryRow(r.Context(), "SELECT swarm_contributor($1)", swarm).Scan(&contributor); err != nil {
		return 0, nil, err
	}
	if !contributor {
		return 0, nil, fail(403, "contributor_required")
	}
	return idempotent(r, tx, a, func() (int, any, error) {
		var input struct {
			Body string `json:"body"`
		}
		if err := decode(r, &input); err != nil {
			return 0, nil, err
		}
		command := r.Clone(r.Context())
		command.Body = io.NopCloser(bytes.NewReader(canonical(map[string]any{"body": input.Body, "kind": "text"})))
		return s.postMessage(command, tx, a, swarm)
	})
}
