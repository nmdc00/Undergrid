package network

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// The interactive owner selects one of their active agents for attribution.
// Authority comes from the owner transaction, never a browser-supplied principal.
// Existing intention commands retain validation, quota, versions, event/inbox and
// transactional queue rules; no agent credential is minted for the browser.
func (s *Service) ownerIntents(r *http.Request, tx pgx.Tx, principal string) (int, any, error) {
	collection := r.URL.Path == "/v1/owner/intents"
	opportunities := strings.HasSuffix(r.URL.Path, "/opportunities")
	if !(r.Method == "GET" || (r.Method == "POST" && collection) || (r.Method == "PATCH" && !collection && !opportunities)) {
		return 0, nil, fail(405, "method_not_allowed")
	}
	ctx := r.Context()
	if _, err := tx.Exec(ctx, "SELECT set_config('undergrid.principal_id',$1,true)", principal); err != nil {
		return 0, nil, err
	}
	if r.Method == "GET" && r.URL.Path == "/v1/owner/intents" {
		rows, err := tx.Query(ctx, `SELECT jsonb_build_object('id',i.id,'agent_id',i.agent_id,'state',i.state,'version',i.version,'watch_enabled',i.watch_enabled,'objective',r.document->>'objective') FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.principal_id=$1 ORDER BY i.created_at DESC LIMIT 100`, principal)
		if err != nil {
			return 0, nil, err
		}
		defer rows.Close()
		items := []json.RawMessage{}
		for rows.Next() {
			var raw []byte
			if err = rows.Scan(&raw); err != nil {
				return 0, nil, err
			}
			items = append(items, json.RawMessage(raw))
		}
		return 200, map[string]any{"items": items, "limit": 100}, rows.Err()
	}
	var agent, id string
	command := r.Clone(ctx)
	command.URL.Path = strings.Replace(r.URL.Path, "/v1/owner/intents", "/v1/intents", 1)
	if r.Method == "POST" && r.URL.Path == "/v1/owner/intents" {
		var in struct {
			AgentID string      `json:"agent_id"`
			Intent  IntentInput `json:"intent"`
		}
		if err := decode(r, &in); err != nil {
			return 0, nil, err
		}
		agent = in.AgentID
		if !validID(agent) {
			return 0, nil, fail(404, "not_found")
		}
		command.Body = io.NopCloser(bytes.NewReader(canonical(in.Intent)))
	} else {
		id = strings.TrimSuffix(strings.TrimPrefix(command.URL.Path, "/v1/intents/"), "/opportunities")
		if !validID(id) {
			return 0, nil, fail(404, "not_found")
		}
		err := tx.QueryRow(ctx, "SELECT agent_id FROM intents WHERE id=$1 AND principal_id=$2", id, principal).Scan(&agent)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil, fail(404, "not_found")
		}
		if err != nil {
			return 0, nil, err
		}
	}
	var state string
	err := tx.QueryRow(ctx, "SELECT status FROM agents WHERE id=$1 AND principal_id=$2 FOR UPDATE", agent, principal).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	if state != "active" {
		return 0, nil, fail(403, "inactive_agent")
	}
	if _, err = tx.Exec(ctx, "SELECT set_config('undergrid.agent_id',$1,true)", agent); err != nil {
		return 0, nil, err
	}
	a := actor{PrincipalID: principal, AgentID: agent}
	if r.Method == "GET" && strings.HasSuffix(command.URL.Path, "/opportunities") {
		return listOpportunities(command, tx, a)
	}
	if r.Method == "GET" {
		return getIntent(ctx, tx, a, id)
	}
	if r.Method == "POST" || r.Method == "PATCH" {
		return mutateIntent(command, tx, a, s.environment)
	}
	return 0, nil, fail(405, "method_not_allowed")
}
