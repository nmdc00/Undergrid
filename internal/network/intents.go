package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

func intentPublicProjection(input IntentInput) map[string]any {
	if input.Visibility != "redacted_public" {
		return map[string]any{}
	}
	return map[string]any{"public_summary": input.PublicSummary, "output": input.Output, "hard_constraints": input.HardConstraints}
}

func getIntent(ctx context.Context, tx pgx.Tx, a actor, id string) (int, any, error) {
	if !validID(id) {
		return 0, nil, fail(404, "not_found")
	}
	var document []byte
	var version int64
	var revision int
	var state string
	var watch bool
	err := tx.QueryRow(ctx, `SELECT r.document,i.version,i.current_revision,i.state,i.watch_enabled FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1 AND i.principal_id=$2`, id, a.PrincipalID).Scan(&document, &version, &revision, &state, &watch)
	if errors.Is(err, pgx.ErrNoRows) {
		var granted []byte
		var grantedVersion int64
		err = tx.QueryRow(ctx, `SELECT document,version FROM granted_intent($1)`, id).Scan(&granted, &grantedVersion)
		if errors.Is(err, pgx.ErrNoRows) {
			return publicIntent(ctx, tx, id)
		}
		if err != nil {
			return 0, nil, err
		}
		var shared map[string]any
		if err = json.Unmarshal(granted, &shared); err != nil {
			return 0, nil, err
		}
		shared["etag"] = etag(grantedVersion)
		return 200, shared, nil
	}
	if err != nil {
		return 0, nil, err
	}
	var result map[string]any
	if err = json.Unmarshal(document, &result); err != nil {
		return 0, nil, err
	}
	delete(result, "semantic_processing_grant")
	stored, err := decodeStoredIntent(document, revision)
	if err != nil {
		return 0, nil, err
	}
	if stored.PrivateSemanticQuery != nil {
		status := "review_required"
		if stored.processingGrant.matches(stored, revision) {
			status = "revision_bound"
		}
		result["private_semantic_query_status"] = status
	}
	result["id"] = id
	result["version"] = version
	result["current_revision"] = revision
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
		if err = validateSemanticConsent(input, environment); err != nil {
			return 0, nil, err
		}
		if err = admitPrincipalResource(ctx, tx, a.PrincipalID, "active_intents"); err != nil {
			return 0, nil, err
		}
		id = NewID()
		version = 1
		eventType = "intent.created"
		if _, err = tx.Exec(ctx, `INSERT INTO intents(id,principal_id,agent_id,state,visibility,expires_at,watch_enabled) VALUES($1,$2,$3,'active',$4,$5,$6)`, id, a.PrincipalID, a.AgentID, input.Visibility, input.ExpiresAt, input.Watch.Enabled); err != nil {
			return 0, nil, err
		}
		projection := intentPublicProjection(input)
		encoded := canonical(intentRevisionDocument{IntentInput: input, SemanticProcessingGrant: newSemanticProcessingGrant(input, 1)})
		if _, err = tx.Exec(ctx, "INSERT INTO intent_revisions(intent_id,revision,document,public_projection,content_hash) VALUES($1,1,$2,$3,$4)", id, encoded, canonical(projection), digest(encoded)); err != nil {
			return 0, nil, err
		}
	} else {
		id = strings.TrimPrefix(r.URL.Path, "/v1/intents/")
		if !validID(id) {
			return 0, nil, fail(404, "not_found")
		}
		var mutation struct {
			State    json.RawMessage `json:"state"`
			Revision json.RawMessage `json:"revision"`
		}
		if err = decode(r, &mutation); err != nil {
			return 0, nil, err
		}
		hasState, hasRevision := len(mutation.State) != 0, len(mutation.Revision) != 0
		if hasState == hasRevision {
			return 0, nil, fail(422, "invalid_intent_mutation")
		}
		var state string
		if hasState {
			if err = decodeIntentJSON(mutation.State, &state); err != nil {
				return 0, nil, err
			}
			if state != "paused" && state != "active" && state != "withdrawn" && state != "satisfied" {
				return 0, nil, fail(422, "invalid_state")
			}
		}
		var previous, visibility string
		var currentRevision int
		err = tx.QueryRow(ctx, "SELECT version,state,visibility,current_revision FROM intents WHERE id=$1 AND principal_id=$2 FOR UPDATE", id, a.PrincipalID).Scan(&version, &previous, &visibility, &currentRevision)
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
		if previous == "withdrawn" || previous == "satisfied" {
			return 0, nil, fail(409, "terminal_intent")
		}
		if hasState {
			inputState := state
			if previous == "expired" {
				return 0, nil, fail(409, "terminal_intent")
			}
			if inputState == "active" && previous != "active" {
				if err = admitPrincipalResource(ctx, tx, a.PrincipalID, "active_intents"); err != nil {
					return 0, nil, err
				}
			}
			version++
			eventType = "intent." + inputState
			if _, err = tx.Exec(ctx, "UPDATE intents SET state=$2,version=$3,watch_enabled=CASE WHEN $2='active' THEN (SELECT (document->'watch'->>'enabled')::boolean FROM intent_revisions WHERE intent_id=$1 AND revision=intents.current_revision) ELSE false END WHERE id=$1", id, inputState, version); err != nil {
				return 0, nil, err
			}
		} else {
			var input IntentInput
			if err = decodeIntentJSON(mutation.Revision, &input); err != nil {
				return 0, nil, err
			}

			if err = input.validate(); err != nil {
				return 0, nil, err
			}
			if err = validateSemanticConsent(input, environment); err != nil {
				return 0, nil, err
			}
			if input.Visibility != visibility {
				return 0, nil, fail(409, "visibility_change_requires_grant")
			}
			if previous == "expired" {
				if err = admitPrincipalResource(ctx, tx, a.PrincipalID, "active_intents"); err != nil {
					return 0, nil, err
				}
				eventType = "intent.renewed"
			} else {
				eventType = "intent.revised"
			}
			version++
			state := previous
			if previous == "expired" {
				state = "active"
			}
			var previousDocument intentRevisionDocument
			var previousRaw []byte
			if err = tx.QueryRow(ctx, "SELECT document FROM intent_revisions WHERE intent_id=$1 AND revision=$2", id, currentRevision).Scan(&previousRaw); err != nil {
				return 0, nil, err
			}
			if err = json.Unmarshal(previousRaw, &previousDocument); err != nil {
				return 0, nil, err
			}
			if err = writeIntentRevision(ctx, tx, id, currentRevision+1, version, state, input, previousDocument.PartialSatisfaction, newSemanticProcessingGrant(input, currentRevision+1)); err != nil {
				return 0, nil, err
			}
		}
	}
	result, err := finishIntentMutation(ctx, tx, a, id, version, eventType, environment)
	if err != nil {
		return 0, nil, err
	}
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	return status, result, nil
}

// Shared by ordinary revisions and owner-declared partial fulfillment. Input
// still owns the public projection; server-only audit never enters it.
func writeIntentRevision(ctx context.Context, tx pgx.Tx, id string, revision int, version int64, state string, input IntentInput, audit *partialSatisfactionAudit, grant *semanticProcessingGrant) error {
	encoded := canonical(intentRevisionDocument{IntentInput: input, PartialSatisfaction: audit, SemanticProcessingGrant: grant})
	if _, err := tx.Exec(ctx, "INSERT INTO intent_revisions(intent_id,revision,document,public_projection,content_hash) VALUES($1,$2,$3,$4,$5)", id, revision, encoded, canonical(intentPublicProjection(input)), digest(encoded)); err != nil {
		return err
	}
	watch := input.Watch.Enabled && state == "active"
	_, err := tx.Exec(ctx, "UPDATE intents SET current_revision=$2,visibility=$3,expires_at=$4,state=$5,watch_enabled=$6,version=$7 WHERE id=$1", id, revision, input.Visibility, input.ExpiresAt, state, watch, version)
	return err
}

func finishIntentMutation(ctx context.Context, tx pgx.Tx, a actor, id string, version int64, eventType, environment string) (any, error) {
	var err error
	origin := "registered"
	if environment != "production" {
		origin = "release_test"
	}
	var document []byte
	if err = tx.QueryRow(ctx, "SELECT r.document FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1", id).Scan(&document); err != nil {
		return nil, err
	}
	var input IntentInput
	if err = json.Unmarshal(document, &input); err != nil {
		return nil, err
	}
	facts := intentMatchingFacts(input)
	eventID := NewID()
	if _, err = tx.Exec(ctx, "INSERT INTO events(id,aggregate_kind,aggregate_id,version,principal_id,agent_id,type,facts,origin) VALUES($1,'intent',$2,$3,$4,$5,$6,$8,$7)", eventID, id, version, a.PrincipalID, a.AgentID, eventType, origin, canonical(facts)); err != nil {
		return nil, err
	}
	var sequence int64
	if err = tx.QueryRow(ctx, "UPDATE agents SET inbox_sequence=inbox_sequence+1 WHERE id=$1 RETURNING inbox_sequence", a.AgentID).Scan(&sequence); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO inbox(agent_id,sequence,event_id,resource_path) VALUES($1,$2,$3,$4)", a.AgentID, sequence, eventID, "/v1/intents/"+id); err != nil {
		return nil, err
	}
	_, result, err := getIntent(ctx, tx, a, id)
	if err != nil {
		return nil, err
	}
	if err = enqueueScan(ctx, tx, id); err != nil {
		return nil, err
	}
	return result, nil
}

func decodeIntentJSON(raw []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return fail(400, "invalid_json")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return fail(400, "invalid_json")
	}
	return nil
}
