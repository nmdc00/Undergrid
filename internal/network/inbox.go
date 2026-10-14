package network

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type cursor struct {
	Agent       string `json:"a"`
	Environment string `json:"e"`
	Sequence    int64  `json:"s"`
	Issued      int64  `json:"t"`
}

func (s *Service) encodeCursor(agent string, sequence int64, issued time.Time) string {
	raw := canonical(cursor{agent, s.environment, sequence, issued.Unix()})
	mac := hmac.New(sha256.New, s.cursorKey)
	mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Service) decodeCursor(value, agent string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	if len(value) > 1024 {
		return 0, fail(400, "invalid_cursor")
	}
	body, sig, ok := strings.Cut(value, ".")
	if !ok {
		return 0, fail(400, "invalid_cursor")
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return 0, fail(400, "invalid_cursor")
	}
	signature, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return 0, fail(400, "invalid_cursor")
	}
	mac := hmac.New(sha256.New, s.cursorKey)
	mac.Write(raw)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return 0, fail(400, "invalid_cursor")
	}
	var c cursor
	if json.Unmarshal(raw, &c) != nil || c.Sequence < 0 || c.Issued > time.Now().Unix()+60 {
		return 0, fail(400, "invalid_cursor")
	}
	if c.Agent != agent || c.Environment != s.environment {
		return 0, fail(403, "cursor_wrong_audience")
	}
	if time.Since(time.Unix(c.Issued, 0)) > 30*24*time.Hour {
		return 0, fail(410, "cursor_expired")
	}
	return c.Sequence, nil
}
func (s *Service) inboxCommand(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	if len(s.cursorKey) < 32 {
		return 0, nil, fail(503, "cursor_signing_unconfigured")
	}
	ctx := r.Context()
	if r.Method == "GET" && r.URL.Path == "/v1/agent-context" {
		var sequence int64
		if err := tx.QueryRow(ctx, "SELECT inbox_sequence FROM agents WHERE id=$1", a.AgentID).Scan(&sequence); err != nil {
			return 0, nil, err
		}
		rows, err := tx.Query(ctx, "SELECT id,state,version FROM intents WHERE agent_id=$1 AND watch_enabled AND state='active' AND expires_at>now() ORDER BY id", a.AgentID)
		if err != nil {
			return 0, nil, err
		}
		defer rows.Close()
		watches := []any{}
		for rows.Next() {
			var id, state string
			var version int64
			if err = rows.Scan(&id, &state, &version); err != nil {
				return 0, nil, err
			}
			watches = append(watches, map[string]any{"id": id, "state": state, "version": version})
		}
		if err = rows.Err(); err != nil {
			return 0, nil, err
		}
		rows.Close()
		pending, refs, err := membershipSnapshot(ctx, tx, a)
		if err != nil {
			return 0, nil, err
		}
		assignments, err := assignmentSnapshot(ctx, tx, a)
		if err != nil {
			return 0, nil, err
		}
		grants, err := s.inputGrantSnapshot(ctx, tx, a)
		if err != nil {
			return 0, nil, err
		}
		return 200, map[string]any{"input_grants": grants, "assignments": assignments, "snapshot_id": NewID(), "inbox_cursor": s.encodeCursor(a.AgentID, sequence, time.Now()), "pending": pending, "intent_watches": watches, "swarm_refs": refs}, nil
	}
	if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/ack") {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/inbox/"), "/ack")
		if !validID(id) {
			return 0, nil, fail(404, "not_found")
		}
		if k := r.Header.Get("Idempotency-Key"); len(k) < 1 || len(k) > 128 {
			return 0, nil, fail(400, "idempotency_key_required")
		}
		var input struct{}
		if err := decode(r, &input); err != nil {
			return 0, nil, err
		}
		tag, err := tx.Exec(ctx, "UPDATE inbox SET acked_at=COALESCE(acked_at,now()) WHERE agent_id=$1 AND event_id=$2", a.AgentID, id)
		if err != nil {
			return 0, nil, err
		}
		if tag.RowsAffected() == 0 {
			return 0, nil, fail(404, "not_found")
		}
		return 200, map[string]bool{"acknowledged": true}, nil
	}
	if r.Method != "GET" || r.URL.Path != "/v1/inbox" {
		return 0, nil, fail(404, "not_found")
	}
	sequence, err := s.decodeCursor(r.URL.Query().Get("cursor"), a.AgentID)
	if err != nil {
		return 0, nil, err
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil || limit < 1 || limit > 50 {
			return 0, nil, fail(400, "invalid_limit")
		}
	}
	rows, err := tx.Query(ctx, `SELECT i.sequence,i.event_id,e.type,i.resource_path,
CASE WHEN e.type='artifact.input_grant_created' THEN EXISTS(SELECT 1 FROM approvals g WHERE g.id=e.aggregate_id AND g.grant_recipient_agent_id=$1 AND g.grant_state='active' AND g.grant_expires_at>clock_timestamp() AND has_scoped_input_read(g.artifact_id))
WHEN e.facts->>'swarm_id' IS NULL THEN true
WHEN e.type='membership.offered' THEN EXISTS(SELECT 1 FROM memberships m WHERE m.id=e.aggregate_id AND m.agent_id=$1 AND m.state='offered' AND m.respond_by>now())
WHEN e.facts ? 'audience_principal_ids' THEN swarm_member((e.facts->>'swarm_id')::uuid)
 AND actor_principal()::text IN (SELECT jsonb_array_elements_text(e.facts->'audience_principal_ids'))
ELSE swarm_member((e.facts->>'swarm_id')::uuid) END
FROM inbox i JOIN events e ON e.id=i.event_id WHERE i.agent_id=$1 AND i.sequence>$2 ORDER BY i.sequence LIMIT $3`, a.AgentID, sequence, limit)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	events := []any{}
	for rows.Next() {
		var id, kind, path string
		var authorized bool
		if err = rows.Scan(&sequence, &id, &kind, &path, &authorized); err != nil {
			return 0, nil, err
		}
		if !authorized {
			continue
		}
		events = append(events, map[string]any{"event_id": id, "sequence": sequence, "type": kind, "resource_path": path, "content_trust": "untrusted"})
	}
	if err = rows.Err(); err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"events": events, "next_cursor": s.encodeCursor(a.AgentID, sequence, time.Now())}, nil
}

// Poll outside a transaction so a disconnected listener never holds authority
// locks. Each iteration reauthenticates; revocation terminates the next read.
func (s *Service) waitInbox(r *http.Request, status int, result any) (int, any, error) {
	raw := r.URL.Query().Get("wait_seconds")
	if raw == "" {
		return status, result, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 || seconds > 25 {
		return 0, nil, fail(400, "invalid_wait")
	}
	deadline := time.NewTimer(time.Duration(seconds) * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for len(result.(map[string]any)["events"].([]any)) == 0 && seconds > 0 {
		select {
		case <-r.Context().Done():
			return 0, nil, fail(408, "request_cancelled")
		case <-deadline.C:
			return status, result, nil
		case <-ticker.C:
			status, result, err = s.execute(r)
			if err != nil {
				return 0, nil, err
			}
		}
	}
	return status, result, nil
}
