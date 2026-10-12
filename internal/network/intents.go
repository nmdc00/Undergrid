package network

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

func getIntent(ctx context.Context, tx pgx.Tx, a actor, id string) (int, any, error) {
	if !validID(id) {
		return 0, nil, fail(404, "not_found")
	}
	var document []byte
	var version int64
	var state string
	var watch bool
	err := tx.QueryRow(ctx, `SELECT r.document,i.version,i.state,i.watch_enabled FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1 AND i.principal_id=$2`, id, a.PrincipalID).Scan(&document, &version, &state, &watch)
	if errors.Is(err, pgx.ErrNoRows) {
		return publicIntent(ctx, tx, id)
	}
	if err != nil {
		return 0, nil, err
	}
	var result map[string]any
	if err = json.Unmarshal(document, &result); err != nil {
		return 0, nil, err
	}
	result["id"] = id
	result["version"] = version
	result["etag"] = etag(version)
	result["state"] = state
	result["watch"] = Watch{watch, []string{"inbox"}}
	return 200, result, nil
}

func mutateIntent(r *http.Request, tx pgx.Tx, a actor, environment string) (int, any, error) {
	return idempotent(r, tx, a, func() (int, any, error) { return mutateIntentOnce(r, tx, a, environment) })
}

func mutateIntentOnce(r *http.Request, tx pgx.Tx, a actor, environment string) (int, any, error) {
	ctx := r.Context()
	var err error
	var id string
	var version int64
	var eventType string
	if r.Method == "POST" {
		var input IntentInput
		if err = decode(r, &input); err != nil {
			return 0, nil, err
		}
		if err = input.validate(); err != nil {
			return 0, nil, err
		}
		id = NewID()
		version = 1
		eventType = "intent.created"
		if _, err = tx.Exec(ctx, `INSERT INTO intents(id,principal_id,agent_id,state,visibility,expires_at,watch_enabled) VALUES($1,$2,$3,'active',$4,$5,$6)`, id, a.PrincipalID, a.AgentID, input.Visibility, input.ExpiresAt, input.Watch.Enabled); err != nil {
			return 0, nil, err
		}
		projection := map[string]any{}
		if input.Visibility == "redacted_public" {
			projection = map[string]any{"public_summary": input.PublicSummary, "output": input.Output, "hard_constraints": input.HardConstraints}
		}
		if _, err = tx.Exec(ctx, "INSERT INTO intent_revisions(intent_id,revision,document,public_projection,content_hash) VALUES($1,1,$2,$3,$4)", id, canonical(input), canonical(projection), digest(canonical(input))); err != nil {
			return 0, nil, err
		}
	} else {
		id = strings.TrimPrefix(r.URL.Path, "/v1/intents/")
		if !validID(id) {
			return 0, nil, fail(404, "not_found")
		}
		var input struct {
			State string `json:"state"`
		}
		if err = decode(r, &input); err != nil {
			return 0, nil, err
		}
		if input.State != "paused" && input.State != "active" && input.State != "withdrawn" {
			return 0, nil, fail(422, "invalid_state")
		}
		var previous string
		err = tx.QueryRow(ctx, "SELECT version,state FROM intents WHERE id=$1 AND principal_id=$2 FOR UPDATE", id, a.PrincipalID).Scan(&version, &previous)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil, fail(404, "not_found")
		}
		if err != nil {
			return 0, nil, err
		}
		if r.Header.Get("If-Match") == "" {
			return 0, nil, fail(428, "if_match_required")
		}
		if r.Header.Get("If-Match") != etag(version) {
			return 0, nil, fail(409, "version_conflict")
		}
		if previous == "withdrawn" || previous == "expired" {
			return 0, nil, fail(409, "terminal_intent")
		}
		version++
		eventType = "intent." + input.State
		if _, err = tx.Exec(ctx, "UPDATE intents SET state=$2,version=$3,watch_enabled=CASE WHEN $2='active' THEN watch_enabled ELSE false END WHERE id=$1", id, input.State, version); err != nil {
			return 0, nil, err
		}
	}
	origin := "registered"
	if environment != "production" {
		origin = "release_test"
	}
	eventID := NewID()
	if _, err = tx.Exec(ctx, "INSERT INTO events(id,aggregate_kind,aggregate_id,version,principal_id,agent_id,type,facts,origin) VALUES($1,'intent',$2,$3,$4,$5,$6,'{}',$7)", eventID, id, version, a.PrincipalID, a.AgentID, eventType, origin); err != nil {
		return 0, nil, err
	}
	var sequence int64
	if err = tx.QueryRow(ctx, "UPDATE agents SET inbox_sequence=inbox_sequence+1 WHERE id=$1 RETURNING inbox_sequence", a.AgentID).Scan(&sequence); err != nil {
		return 0, nil, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO inbox(agent_id,sequence,event_id,resource_path) VALUES($1,$2,$3,$4)", a.AgentID, sequence, eventID, "/v1/intents/"+id); err != nil {
		return 0, nil, err
	}
	_, result, err := getIntent(ctx, tx, a, id)
	if err != nil {
		return 0, nil, err
	}
	if err = enqueueScan(ctx, tx, id); err != nil {
		return 0, nil, err
	}
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	return status, result, nil
}
