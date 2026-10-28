package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The interactive owner selects one of their active agents for attribution.
// Authority comes from the owner transaction, never a browser-supplied principal.
// Existing intention commands retain validation, quota, versions, event/inbox and
// transactional queue rules; no agent credential is minted for the browser.
func (s *Service) ownerIntents(r *http.Request, tx pgx.Tx, principal string) (int, any, error) {
	if id, ok := partialSatisfactionID(strings.Replace(r.URL.Path, "/v1/owner/intents/", "/v1/intents/", 1)); ok {
		return s.ownerPartialSatisfaction(r, tx, principal, id)
	}
	collection := r.URL.Path == "/v1/owner/intents"
	groups := strings.HasSuffix(r.URL.Path, "/compatible-groups")
	assessments := strings.HasSuffix(r.URL.Path, "/constraint-assessments")
	if (groups || assessments) && r.Method != "GET" {
		return 0, nil, fail(405, "method_not_allowed")
	}
	opportunities := strings.HasSuffix(r.URL.Path, "/opportunities")
	externalLeads := strings.HasSuffix(r.URL.Path, "/external-leads")
	externalFit := strings.HasSuffix(r.URL.Path, "/external-fit-report")
	if externalFit && r.Method != http.MethodGet {
		return 0, nil, fail(405, "method_not_allowed")
	}
	if externalLeads && r.Method != http.MethodGet {
		return 0, nil, fail(405, "method_not_allowed")
	}
	share, shareID, recipientID, shareRoute := ownerIntentGrantRoute(r.URL.Path)
	if share == "collection" {
		if r.Method == "GET" {
			share = "list"
		} else if r.Method == "POST" {
			share = "grant"
		}
	}
	allowedShareMethod := shareRoute && ((share == "list" && r.Method == "GET") || ((share == "grant" || share == "revoke") && r.Method == "POST"))
	if shareRoute && !allowedShareMethod {
		return 0, nil, fail(405, "method_not_allowed")
	}
	if !(r.Method == "GET" || (r.Method == "POST" && collection) || (r.Method == "PATCH" && !collection && !opportunities) || allowedShareMethod) {
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
	if shareRoute {
		if !validID(shareID) {
			return 0, nil, fail(404, "not_found")
		}
		var agent string
		err := tx.QueryRow(ctx, "SELECT agent_id FROM intents WHERE id=$1 AND principal_id=$2", shareID, principal).Scan(&agent)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil, fail(404, "not_found")
		}
		if err != nil {
			return 0, nil, err
		}
		a, err := lockOwnerAgent(ctx, tx, principal, agent)
		if err != nil {
			return 0, nil, err
		}
		if share == "list" {
			return listOwnerIntentGrants(ctx, tx, shareID, principal)
		}
		return idempotent(r, tx, a, func() (int, any, error) {
			return mutateOwnerIntentGrant(r, tx, a, shareID, recipientID, share, s.environment)
		})
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
		id = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(command.URL.Path, "/v1/intents/"), "/opportunities"), "/external-leads"), "/external-fit-report"), "/compatible-groups")
		id = strings.TrimSuffix(id, "/constraint-assessments")
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
	a, err := lockOwnerAgent(ctx, tx, principal, agent)
	if err != nil {
		return 0, nil, err
	}
	if assessments {
		a.Scopes = []string{"intents:read"}
		return s.constraintAssessments(command, tx, a, id)
	}
	if groups {
		return s.compatibleGroups(command, tx, a, id)
	}
	if externalFit {
		return s.externalFitReport(command, tx, a, id)
	}
	if externalLeads {
		return s.listExternalLeads(command, tx, a, id, "owner")
	}
	if r.Method == "GET" && strings.HasSuffix(command.URL.Path, "/opportunities") {
		// This session has already proved ownership of this intention and agent.
		// Grant only the read capability needed by the shared domain projection.
		a.Scopes = []string{"intents:read"}
		return s.listOpportunities(command, tx, a)
	}
	if r.Method == "GET" {
		return getIntent(ctx, tx, a, id)
	}
	if r.Method == "POST" || r.Method == "PATCH" {
		return mutateIntent(command, tx, a, s.environment)
	}
	return 0, nil, fail(405, "method_not_allowed")
}

func lockOwnerAgent(ctx context.Context, tx pgx.Tx, principal, agent string) (actor, error) {
	var state string
	err := tx.QueryRow(ctx, "SELECT status FROM agents WHERE id=$1 AND principal_id=$2 FOR UPDATE", agent, principal).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return actor{}, fail(404, "not_found")
	}
	if err != nil {
		return actor{}, err
	}
	if state != "active" {
		return actor{}, fail(403, "inactive_agent")
	}
	if _, err = tx.Exec(ctx, "SELECT set_config('undergrid.agent_id',$1,true)", agent); err != nil {
		return actor{}, err
	}
	return actor{PrincipalID: principal, AgentID: agent}, nil
}

func ownerIntentGrantRoute(path string) (action, intentID, recipientID string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 5 && parts[0] == "v1" && parts[1] == "owner" && parts[2] == "intents" && parts[4] == "grants" {
		return "collection", parts[3], "", true
	}
	if len(parts) == 7 && parts[0] == "v1" && parts[1] == "owner" && parts[2] == "intents" && parts[4] == "grants" && parts[6] == "revoke" {
		return "revoke", parts[3], parts[5], true
	}
	return "", "", "", false
}

// The database projection has its own copy of this allowlist as a trust-boundary
// guard; HTTP validation and SQL filtering must both reject unapproved fields.
var intentionShareableFields = []string{"objective", "output", "hard_constraints", "preferences", "public_summary", "budget"}

func listOwnerIntentGrants(ctx context.Context, tx pgx.Tx, intentID, principal string) (int, any, error) {
	var version int64
	if err := tx.QueryRow(ctx, "SELECT version FROM intents WHERE id=$1 AND principal_id=$2", intentID, principal).Scan(&version); errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	} else if err != nil {
		return 0, nil, err
	}
	rows, err := tx.Query(ctx, `SELECT jsonb_build_object(
		'principal_id',g.principal_id,'fields',g.fields,'expires_at',g.expires_at,
		'revision',g.revision,'status',CASE
		 WHEN g.expires_at<=statement_timestamp() THEN 'expired_or_revoked'
		 WHEN g.revision IS NULL THEN 'legacy_unbound'
		 WHEN g.revision<>i.current_revision THEN 'stale_revision'
		 WHEN i.state<>'active' OR i.expires_at<=statement_timestamp() THEN 'inactive_source'
		 ELSE 'current' END)
		FROM intent_grants g JOIN intents i ON i.id=g.intent_id
		WHERE g.intent_id=$1 AND i.principal_id=$2 ORDER BY g.principal_id`, intentID, principal)
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
	return 200, map[string]any{"items": items, "version": version, "etag": etag(version)}, rows.Err()
}

func mutateOwnerIntentGrant(r *http.Request, tx pgx.Tx, a actor, intentID, recipientID, action, environment string) (int, any, error) {
	ctx := r.Context()
	var version int64
	var currentRevision int
	var visibility, state string
	var intentExpiry, dbNow time.Time
	err := tx.QueryRow(ctx, `SELECT version,current_revision,visibility,state,expires_at,statement_timestamp()
		FROM intents WHERE id=$1 AND principal_id=$2 FOR UPDATE`, intentID, a.PrincipalID).Scan(&version, &currentRevision, &visibility, &state, &intentExpiry, &dbNow)
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
	var recipientPrincipal string
	var fields []string
	var expires time.Time
	var responseRevision any
	status := 200
	eventType := "intent.grant_revoked"
	if action == "grant" {
		var input struct {
			PrincipalID string    `json:"principal_id"`
			Fields      []string  `json:"fields"`
			ExpiresAt   time.Time `json:"expires_at"`
		}
		if err = decode(r, &input); err != nil {
			return 0, nil, err
		}
		if !validID(input.PrincipalID) || len(input.Fields) == 0 || len(input.Fields) > len(intentionShareableFields) {
			return 0, nil, fail(422, "invalid_grant")
		}
		seen := make(map[string]bool, len(input.Fields))
		for _, field := range input.Fields {
			if !slices.Contains(intentionShareableFields, field) || seen[field] {
				return 0, nil, fail(422, "invalid_grant_fields")
			}
			seen[field] = true
		}
		if !input.ExpiresAt.After(dbNow) || input.ExpiresAt.After(intentExpiry) {
			return 0, nil, fail(422, "invalid_grant_expiry")
		}
		if visibility != "restricted" {
			return 0, nil, fail(409, "restricted_intent_required")
		}
		if state != "active" || !intentExpiry.After(dbNow) {
			return 0, nil, fail(409, "active_intent_required")
		}
		var recipientActive bool
		if err = tx.QueryRow(ctx, "SELECT active_intent_grant_recipient($1)", input.PrincipalID).Scan(&recipientActive); err != nil {
			return 0, nil, err
		}
		if !recipientActive || input.PrincipalID == a.PrincipalID {
			return 0, nil, fail(422, "recipient_unavailable")
		}
		recipientPrincipal = input.PrincipalID
		fields = append([]string(nil), input.Fields...)
		slices.Sort(fields)
		expires = input.ExpiresAt
		responseRevision = currentRevision
		var existed bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM intent_grants WHERE intent_id=$1 AND principal_id=$2)", intentID, recipientPrincipal).Scan(&existed); err != nil {
			return 0, nil, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO intent_grants(intent_id,principal_id,fields,expires_at,revision)
			VALUES($1,$2,$3,$4,$5) ON CONFLICT(intent_id,principal_id) DO UPDATE
			SET fields=excluded.fields,expires_at=excluded.expires_at,revision=excluded.revision`, intentID, recipientPrincipal, fields, expires, currentRevision); err != nil {
			return 0, nil, err
		}
		if !existed {
			status = 201
			eventType = "intent.grant_created"
		} else {
			eventType = "intent.grant_updated"
		}
	} else {
		if !validID(recipientID) {
			return 0, nil, fail(404, "not_found")
		}
		if err = decode(r, &struct{}{}); err != nil {
			return 0, nil, err
		}
		var oldFields []string
		var revisionBound bool
		var savedRevision int
		err = tx.QueryRow(ctx, `UPDATE intent_grants SET expires_at=statement_timestamp()
			WHERE intent_id=$1 AND principal_id=$2 AND expires_at>statement_timestamp()
			RETURNING fields,expires_at,revision IS NOT NULL,COALESCE(revision,0)`, intentID, recipientID).Scan(&oldFields, &expires, &revisionBound, &savedRevision)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil, fail(404, "not_found")
		}
		if err != nil {
			return 0, nil, err
		}
		recipientPrincipal = recipientID
		fields = oldFields
		if revisionBound {
			responseRevision = savedRevision
		}
	}
	version++
	if _, err = tx.Exec(ctx, "UPDATE intents SET version=$2 WHERE id=$1", intentID, version); err != nil {
		return 0, nil, err
	}
	if err = writeIntentGrantEvent(ctx, tx, a, intentID, version, eventType, environment); err != nil {
		return 0, nil, err
	}
	return status, map[string]any{"intent_id": intentID, "principal_id": recipientPrincipal, "fields": fields, "expires_at": expires, "revision": responseRevision, "version": version, "etag": etag(version)}, nil
}

func writeIntentGrantEvent(ctx context.Context, tx pgx.Tx, a actor, intentID string, version int64, kind, environment string) error {
	origin := "registered"
	if environment != "production" {
		origin = "release_test"
	}
	eventID := NewID()
	if _, err := tx.Exec(ctx, "INSERT INTO events(id,aggregate_kind,aggregate_id,version,principal_id,agent_id,type,facts,origin) VALUES($1,'intent',$2,$3,$4,$5,$6,'{}',$7)", eventID, intentID, version, a.PrincipalID, a.AgentID, kind, origin); err != nil {
		return err
	}
	var sequence int64
	if err := tx.QueryRow(ctx, "UPDATE agents SET inbox_sequence=inbox_sequence+1 WHERE id=$1 RETURNING inbox_sequence", a.AgentID).Scan(&sequence); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "INSERT INTO inbox(agent_id,sequence,event_id,resource_path) VALUES($1,$2,$3,$4)", a.AgentID, sequence, eventID, "/v1/owner/intents/"+intentID+"/grants")
	return err
}
