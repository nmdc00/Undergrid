package network

import (
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"slices"
)

type ownedDiscoverySource struct {
	Input    IntentInput
	Version  int64
	Revision int
}

// Pin a current owned source for an explicit read, including monitoring-off
// intentions. Candidate disclosure and watch-driven discovery remain separate.
func loadOwnedDiscoverySource(r *http.Request, tx pgx.Tx, a actor, id string) (ownedDiscoverySource, error) {
	var source ownedDiscoverySource
	if r.Method != http.MethodGet {
		return source, fail(405, "method_not_allowed")
	}
	if !slices.Contains(a.Scopes, "intents:read") {
		return source, insufficientScope(a, "intents:read")
	}
	if !validID(id) {
		return source, fail(404, "not_found")
	}
	var raw []byte
	err := tx.QueryRow(r.Context(), `SELECT r.document,i.version,i.current_revision FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1 AND i.principal_id=$2 AND i.agent_id=$3 AND i.state='active' AND i.expires_at>clock_timestamp() FOR SHARE OF i NOWAIT`, id, a.PrincipalID, a.AgentID).Scan(&raw, &source.Version, &source.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return source, fail(404, "not_found")
	}
	if err != nil {
		return source, err
	}
	err = json.Unmarshal(raw, &source.Input)
	return source, err
}
