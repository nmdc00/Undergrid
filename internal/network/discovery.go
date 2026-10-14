package network

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ScanDueIntentions is the durable queue worker's repair sweep. Each intention
// commits independently; a crash can retry without losing or duplicating output.
func (s *Service) ScanDueIntentions(ctx context.Context) error {
	return s.scanIntentions(ctx, "")
}

// A committed intention gets its own bounded job; background repair retains
// the existing sweep for other watches without occupying foreground slots.
func (s *Service) scanIntentions(ctx context.Context, cause string) error {
	db, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if _, err = db.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		return err
	}
	if cause != "" {
		return s.scanIntent(ctx, db, cause)
	}
	rows, err := db.Query(ctx, "SELECT id FROM intents WHERE state='active' AND watch_enabled AND next_scan<=now() ORDER BY next_scan,id LIMIT 1000")
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.scanIntent(ctx, db, id); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) scanIntent(ctx context.Context, db *pgx.Conn, id string) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var principal, agent string
	err = tx.QueryRow(ctx, "SELECT principal_id,agent_id FROM intents WHERE id=$1", id).Scan(&principal, &agent)
	if err != nil {
		return err
	}
	var active string
	if err = tx.QueryRow(ctx, "SELECT status FROM principals WHERE id=$1 FOR UPDATE", principal).Scan(&active); err != nil {
		return err
	}
	if active != "active" {
		return nil
	}
	if err = tx.QueryRow(ctx, "SELECT status FROM agents WHERE id=$1 FOR UPDATE", agent).Scan(&active); err != nil {
		return err
	}
	if active != "active" {
		return nil
	}
	var raw []byte
	var inputHash, state string
	var expires time.Time
	var watching bool
	var version int64
	err = tx.QueryRow(ctx, `SELECT r.document,r.content_hash,i.state,i.expires_at,i.watch_enabled,i.version FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1 FOR UPDATE OF i`, id).Scan(&raw, &inputHash, &state, &expires, &watching, &version)
	if err != nil {
		return err
	}
	if state != "active" || !watching {
		return nil
	}
	if !expires.After(time.Now()) {
		if _, err = tx.Exec(ctx, "UPDATE intents SET state='expired',version=version+1,watch_enabled=false WHERE id=$1", id); err != nil {
			return err
		}
		if err = s.notify(ctx, tx, principal, agent, id, version+1, "intent.expired", "/v1/intents/"+id); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	var input IntentInput
	if err = json.Unmarshal(raw, &input); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, "SELECT id,projection,content_hash FROM public_intents() WHERE principal_id<>$1 ORDER BY id", principal)
	if err != nil {
		return err
	}
	type candidate struct {
		id, hash string
		input    IntentInput
	}
	candidates := []candidate{}
	for rows.Next() {
		var c candidate
		var data []byte
		if err = rows.Scan(&c.id, &data, &c.hash); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(data, &c.input); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, c := range candidates {
		evidence := CompareDemand(input, c.input)
		if !evidence.Compatible {
			continue
		}
		opportunityID := NewID()
		hash := digest(canonical([]string{id, inputHash, c.id, c.hash, evidence.Algorithm}))
		tag, err := tx.Exec(ctx, "INSERT INTO opportunities(id,recipient_agent_id,source_intent_id,candidate_intent_id,kind,input_hash,evidence) VALUES($1,$2,$3,$4,'shared_demand',$5,$6) ON CONFLICT DO NOTHING", opportunityID, agent, id, c.id, hash, canonical(evidence))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 1 {
			if err = s.notify(ctx, tx, principal, agent, opportunityID, 1, "opportunity.available", "/v1/opportunities/"+opportunityID); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE intents SET last_scan=now(),next_scan=now()+interval '15 minutes' WHERE id=$1", id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) notify(ctx context.Context, tx pgx.Tx, principal, agent, aggregate string, version int64, kind, path string) error {
	eventID := NewID()
	origin := "registered"
	if s.environment != "production" {
		origin = "release_test"
	}
	aggregateKind := strings.Split(kind, ".")[0]
	if _, err := tx.Exec(ctx, "INSERT INTO events(id,aggregate_kind,aggregate_id,version,principal_id,type,facts,origin) VALUES($1,$2,$3,$4,$5,$6,'{}',$7)", eventID, aggregateKind, aggregate, version, principal, kind, origin); err != nil {
		return err
	}
	var sequence int64
	if err := tx.QueryRow(ctx, "UPDATE agents SET inbox_sequence=inbox_sequence+1 WHERE id=$1 RETURNING inbox_sequence", agent).Scan(&sequence); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "INSERT INTO inbox(agent_id,sequence,event_id,resource_path) VALUES($1,$2,$3,$4)", agent, sequence, eventID, path)
	return err
}
func listOpportunities(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	if handled, status, result, err := recruitmentRoute(r, tx, a); handled {
		return status, result, err
	}
	id := ""
	single := false
	if strings.HasPrefix(r.URL.Path, "/v1/intents/") {
		id = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/intents/"), "/opportunities")
		if !validID(id) {
			return 0, nil, fail(404, "not_found")
		}
		var exists bool
		if err := tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM intents WHERE id=$1 AND principal_id=$2)", id, a.PrincipalID).Scan(&exists); err != nil {
			return 0, nil, err
		}
		if !exists {
			return 0, nil, fail(404, "not_found")
		}
	} else if r.URL.Path != "/v1/opportunities" {
		id = strings.TrimPrefix(r.URL.Path, "/v1/opportunities/")
		single = true
		if !validID(id) {
			return 0, nil, fail(404, "not_found")
		}
	}
	rows, err := tx.Query(r.Context(), `SELECT o.id,o.source_intent_id,o.candidate_intent_id,o.evidence FROM opportunities o JOIN public_intents() p ON p.id=o.candidate_intent_id JOIN intents i ON i.id=o.source_intent_id WHERE o.recipient_agent_id=$1 AND o.state='suggested' AND i.state='active' AND i.expires_at>now() AND ($2='' OR CASE WHEN $3 THEN o.id::text=$2 ELSE o.source_intent_id::text=$2 END) ORDER BY o.created_at LIMIT 100`, a.AgentID, id, single)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var oid, source, candidate string
		var evidence []byte
		if err = rows.Scan(&oid, &source, &candidate, &evidence); err != nil {
			return 0, nil, err
		}
		items = append(items, map[string]any{"id": oid, "source_intent_id": source, "candidate_intent_id": candidate, "origin": "registered_intent", "kind": "shared_demand", "evidence": json.RawMessage(evidence)})
	}
	if err = rows.Err(); err != nil {
		return 0, nil, err
	}
	if single {
		if len(items) == 0 {
			return 0, nil, fail(404, "not_found")
		}
		return 200, items[0], nil
	}
	return 200, map[string]any{"items": items}, nil
}
func publicIntent(ctx context.Context, tx pgx.Tx, id string) (int, any, error) {
	var raw []byte
	var version int64
	err := tx.QueryRow(ctx, "SELECT projection,version FROM public_intents() WHERE id=$1", id).Scan(&raw, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	var result map[string]any
	if err = json.Unmarshal(raw, &result); err != nil {
		return 0, nil, err
	}
	result["id"] = id
	result["version"] = version
	result["etag"] = etag(version)
	result["state"] = "active"
	return 200, result, nil
}
