package network

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type RecruitmentRole struct {
	Key          string `json:"key"`
	SchemaFamily string `json:"schema_family"`
	Description  string `json:"description"`
}
type SwarmInput struct {
	Objective               string            `json:"objective"`
	Visibility              string            `json:"visibility"`
	OriginOpportunityID     string            `json:"origin_opportunity_id,omitempty"`
	RecruitmentRoles        []RecruitmentRole `json:"recruitment_roles"`
	PublicTranscriptConsent bool              `json:"public_transcript_consent,omitempty"`
}

func (s *Service) swarmCommand(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	if r.Method == "GET" {
		if !slices.Contains(a.Scopes, "swarms:read") {
			return 0, nil, insufficientScope(a, "swarms:read")
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/swarms/"), "/")
		if len(parts) == 2 && validID(parts[0]) && parts[1] == "context" {
			return swarmContext(r, tx, a, parts[0])
		}
		return 0, nil, fail(404, "not_found")
	}
	if r.Method != "POST" {
		return 0, nil, fail(405, "method_not_allowed")
	}
	scope := "swarms:create"
	if r.URL.Path != "/v1/swarms" {
		parts := strings.Split(r.URL.Path, "/")
		scope = map[string]string{"archive": "swarms:archive", "plans": "plans:propose", "applications": "swarms:apply", "invitations": "members:invite", "membership-response": "members:respond", "withdraw": "members:withdraw", "messages": "messages:write"}[parts[len(parts)-1]]
		if scope == "" {
			return 0, nil, fail(404, "not_found")
		}
	}
	if !slices.Contains(a.Scopes, scope) {
		return 0, nil, insufficientScope(a, scope)
	}
	if r.URL.Path != "/v1/swarms" {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/swarms/"), "/")
		if len(parts) != 2 || !validID(parts[0]) {
			return 0, nil, fail(404, "not_found")
		}
		if parts[1] == "applications" {
			return s.applyToSwarm(r, tx, a, parts[0])
		}
		if parts[1] == "archive" {
			if _, err := roomMembershipAuthority(r.Context(), tx, parts[0], true); err != nil {
				return 0, nil, err
			}
		}
		if slices.Contains([]string{"plans", "messages", "invitations"}, parts[1]) {
			if err := roomAuthority(r.Context(), tx, a, parts[0], parts[1] == "invitations"); err != nil {
				return 0, nil, err
			}
		}
	}
	if r.URL.Path == "/v1/swarms" {
		request, err := readIdempotencyRequest(r, true)
		if err != nil {
			return 0, nil, err
		}
		restoreIdempotencyBody(r, request)
		var input SwarmInput
		if err = decode(r, &input); err != nil {
			return 0, nil, err
		}
		restoreIdempotencyBody(r, request)
		if err = s.requireCurrentRoomOrigin(r, tx, a, input.OriginOpportunityID); err != nil {
			return 0, nil, err
		}
	}
	if strings.HasSuffix(r.URL.Path, "/plans") {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 4 || !validID(parts[2]) {
			return 0, nil, fail(404, "not_found")
		}
		if err := requirePlanProposer(r.Context(), tx, a, parts[2]); err != nil {
			return 0, nil, err
		}
	}
	return idempotent(r, tx, a, func() (int, any, error) {
		if r.URL.Path == "/v1/swarms" {
			return s.createSwarm(r, tx, a)
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/swarms/"), "/")
		if len(parts) != 2 || !validID(parts[0]) {
			return 0, nil, fail(404, "not_found")
		}
		id, action := parts[0], parts[1]
		switch action {
		case "archive":
			return s.archiveSwarm(r, tx, a, id)
		case "plans":
			return s.proposePlan(r, tx, a, id)
		case "invitations":
			return s.invite(r, tx, a, id)
		case "membership-response":
			return s.respondMembership(r, tx, a, id)
		case "messages":
			return s.postMessage(r, tx, a, id)
		case "withdraw":
			return s.withdrawMembership(r, tx, a, id)
		default:
			return 0, nil, fail(404, "not_found")
		}
	})
}
func (s *Service) createSwarm(r *http.Request, tx pgx.Tx, a actor) (int, any, error) {
	var in SwarmInput
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if len(in.Objective) < 3 || len(in.Objective) > 4000 || !slices.Contains([]string{"public", "private", "restricted"}, in.Visibility) || len(in.RecruitmentRoles) > 10 {
		return 0, nil, fail(422, "invalid_swarm")
	}
	if in.Visibility == "public" && !in.PublicTranscriptConsent {
		return 0, nil, fail(422, "public_transcript_consent_required")
	}
	for _, role := range in.RecruitmentRoles {
		if role.Key == "" || role.SchemaFamily != "source_catalog_v1" || len(role.Description) > 1000 {
			return 0, nil, fail(422, "invalid_recruitment_role")
		}
	}
	if err := admitPrincipalResource(r.Context(), tx, a.PrincipalID, "active_swarms"); err != nil {
		return 0, nil, err
	}
	id := NewID()
	_, err := tx.Exec(r.Context(), "INSERT INTO swarms(id,creator_principal_id,coordinator_agent_id,objective,visibility,recruitment_roles,origin_opportunity_id) VALUES($1,$2,$3,$4,$5,$6,nullif($7,'')::uuid)", id, a.PrincipalID, a.AgentID, in.Objective, in.Visibility, canonical(in.RecruitmentRoles), in.OriginOpportunityID)
	if err != nil {
		return 0, nil, err
	}
	_, err = tx.Exec(r.Context(), "INSERT INTO memberships(id,swarm_id,agent_id,principal_id,state,role,origin,joined_at) VALUES($1,$2,$3,$4,'accepted','coordinator','creation',now())", NewID(), id, a.AgentID, a.PrincipalID)
	if err != nil {
		return 0, nil, err
	}
	if err = s.roomEvent(r.Context(), tx, a, id, 1, "swarm.created", id, "/v1/swarms/"+id+"/context"); err != nil {
		return 0, nil, err
	}
	if err = enqueueRecruitment(r.Context(), tx, id); err != nil {
		return 0, nil, err
	}
	return 201, map[string]any{"id": id, "version": 1, "etag": etag(1), "visibility": in.Visibility, "history_policy": "full"}, nil
}
func roomMembershipAuthority(ctx context.Context, tx pgx.Tx, id string, coordinator bool) (string, error) {
	var member, manager bool
	var state string
	err := tx.QueryRow(ctx, "SELECT swarm_member(id),swarm_coordinator(id),state FROM swarms WHERE id=$1 FOR UPDATE", id).Scan(&member, &manager, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fail(404, "not_found")
	}
	if err != nil {
		return "", err
	}
	if !member {
		return "", fail(404, "not_found")
	}
	if coordinator && !manager {
		return "", fail(403, "coordinator_required")
	}
	return state, nil
}

func roomAuthority(ctx context.Context, tx pgx.Tx, a actor, id string, coordinator bool) error {
	state, err := roomMembershipAuthority(ctx, tx, id, coordinator)
	if err != nil {
		return err
	}
	if state != "active" {
		return fail(409, "swarm_archived")
	}
	return nil
}

func (s *Service) archiveSwarm(r *http.Request, tx pgx.Tx, a actor, swarm string) (int, any, error) {
	var in struct{}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	var state string
	var version int64
	if err := tx.QueryRow(r.Context(), "SELECT state,version FROM swarms WHERE id=$1 FOR UPDATE", swarm).Scan(&state, &version); err != nil {
		return 0, nil, err
	}
	if r.Header.Get("If-Match") == "" {
		return 0, nil, fail(428, "if_match_required")
	}
	if r.Header.Get("If-Match") != etag(version) {
		return 0, nil, fail(409, "version_conflict")
	}
	if state != "active" {
		return 0, nil, fail(409, "swarm_archived")
	}
	version++
	if err := s.transitionSwarmArchive(r.Context(), tx, a, swarm, version, nil); err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"id": swarm, "state": "archived", "version": version, "etag": etag(version)}, nil
}

// The caller holds the room lock and owns the command or worker preconditions.
// This shared terminal transition never grants consent or creates completion.
func (s *Service) transitionSwarmArchive(ctx context.Context, tx pgx.Tx, a actor, swarm string, version int64, policy map[string]any) error {
	tag, err := tx.Exec(ctx, "UPDATE swarms SET state='archived',version=$2 WHERE id=$1 AND state='active' AND version=$2-1", swarm, version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fail(409, "swarm_archived_or_changed")
	}
	return s.roomEventFacts(ctx, tx, a, swarm, version, "swarm.archived", swarm, "/v1/swarms/"+swarm+"/context", nil, policy)
}

func (s *Service) invite(r *http.Request, tx pgx.Tx, a actor, swarm string) (int, any, error) {
	if err := roomAuthority(r.Context(), tx, a, swarm, true); err != nil {
		return 0, nil, err
	}
	var in struct {
		AgentID       string    `json:"agent_id"`
		Role          string    `json:"role"`
		RespondBy     time.Time `json:"respond_by"`
		ApplicationID string    `json:"application_id,omitempty"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if !validID(in.AgentID) || !slices.Contains([]string{"contributor", "observer"}, in.Role) || !in.RespondBy.After(time.Now()) || in.RespondBy.After(time.Now().Add(7*24*time.Hour)) {
		return 0, nil, fail(422, "invalid_invitation")
	}
	var principal string
	id := NewID()
	version := int64(1)
	var err error
	if in.ApplicationID != "" {
		if !validID(in.ApplicationID) {
			return 0, nil, fail(422, "invalid_application")
		}
		id = in.ApplicationID
		err = tx.QueryRow(r.Context(), "UPDATE memberships SET state='offered',role=$4,respond_by=$5,version=version+1 WHERE id=$1 AND swarm_id=$2 AND agent_id=$3 AND state='applied' AND respond_by>now() RETURNING principal_id,version", id, swarm, in.AgentID, in.Role, in.RespondBy).Scan(&principal, &version)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil, fail(409, "application_not_current")
		}
		if err != nil {
			return 0, nil, err
		}
	} else {
		err = tx.QueryRow(r.Context(), "SELECT principal_id FROM invitation_target($1)", in.AgentID).Scan(&principal)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil, fail(404, "invitation_target_unavailable")
		}
		if err != nil {
			return 0, nil, err
		}
		tag, err := tx.Exec(r.Context(), "INSERT INTO memberships(id,swarm_id,agent_id,principal_id,state,role,respond_by,origin) VALUES($1,$2,$3,$4,'offered',$5,$6,'invitation') ON CONFLICT(swarm_id,agent_id) DO NOTHING", id, swarm, in.AgentID, principal, in.Role, in.RespondBy)
		if err != nil {
			return 0, nil, err
		}
		if tag.RowsAffected() == 0 {
			return 0, nil, fail(409, "membership_exists")
		}
	}
	if err = s.roomEvent(r.Context(), tx, a, id, version, "membership.offered", swarm, "/v1/invitations/"+id); err != nil {
		return 0, nil, err
	}
	return 201, map[string]any{"id": id, "version": version, "etag": etag(version), "state": "offered", "history_policy": "full", "respond_by": in.RespondBy}, nil
}
func (s *Service) respondMembership(r *http.Request, tx pgx.Tx, a actor, swarm string) (int, any, error) {
	var in struct {
		InvitationID string `json:"invitation_id"`
		Decision     string `json:"decision"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if !validID(in.InvitationID) || !slices.Contains([]string{"accept", "decline"}, in.Decision) {
		return 0, nil, fail(422, "invalid_membership_response")
	}
	var roomState string
	err := tx.QueryRow(r.Context(), "SELECT state FROM swarms WHERE id=$1 FOR UPDATE", swarm).Scan(&roomState)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	if roomState != "active" {
		return 0, nil, fail(409, "swarm_archived")
	}
	var state string
	var version int64
	var deadline time.Time
	err = tx.QueryRow(r.Context(), "SELECT state,version,respond_by FROM memberships WHERE id=$1 AND swarm_id=$2 AND agent_id=$3 FOR UPDATE", in.InvitationID, swarm, a.AgentID).Scan(&state, &version, &deadline)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	if r.Header.Get("If-Match") == "" {
		return 0, nil, fail(428, "if_match_required")
	}
	if r.Header.Get("If-Match") != etag(version) || state != "offered" || !deadline.After(time.Now()) {
		return 0, nil, fail(409, "invitation_not_current")
	}
	next := "declined"
	if in.Decision == "accept" {
		next = "accepted"
	}
	version++
	_, err = tx.Exec(r.Context(), "UPDATE memberships SET state=$2,version=$3,joined_at=CASE WHEN $2='accepted' THEN now() ELSE NULL END WHERE id=$1", in.InvitationID, next, version)
	if err != nil {
		return 0, nil, err
	}
	if err = s.roomEvent(r.Context(), tx, a, in.InvitationID, version, "membership."+next, swarm, "/v1/swarms/"+swarm+"/context"); err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"id": in.InvitationID, "state": next, "version": version, "etag": etag(version)}, nil
}
func (s *Service) postMessage(r *http.Request, tx pgx.Tx, a actor, swarm string) (int, any, error) {
	if err := roomAuthority(r.Context(), tx, a, swarm, false); err != nil {
		return 0, nil, err
	}
	var canContribute bool
	if err := tx.QueryRow(r.Context(), "SELECT swarm_contributor($1)", swarm).Scan(&canContribute); err != nil {
		return 0, nil, err
	}
	if !canContribute {
		return 0, nil, fail(403, "contributor_required")
	}
	var in struct {
		Body       string `json:"body"`
		Kind       string `json:"kind"`
		ParentID   string `json:"parent_id,omitempty"`
		ReplacesID string `json:"replaces_id,omitempty"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if len(in.Body) < 1 || len(in.Body) > 16000 || in.Kind != "text" {
		return 0, nil, fail(422, "invalid_message")
	}
	for _, reference := range []string{in.ParentID, in.ReplacesID} {
		if reference != "" {
			if !validID(reference) {
				return 0, nil, fail(422, "invalid_message_reference")
			}
			var valid bool
			if err := tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM messages WHERE id=$1 AND swarm_id=$2 AND (NOT $3 OR agent_id=$4))", reference, swarm, reference == in.ReplacesID, a.AgentID).Scan(&valid); err != nil {
				return 0, nil, err
			}
			if !valid {
				return 0, nil, fail(404, "message_reference_not_found")
			}
		}
	}
	var sequence, version int64
	if err := tx.QueryRow(r.Context(), "UPDATE swarms SET sequence=sequence+1,version=version+1 WHERE id=$1 RETURNING sequence,version", swarm).Scan(&sequence, &version); err != nil {
		return 0, nil, err
	}
	id := NewID()
	_, err := tx.Exec(r.Context(), "INSERT INTO messages(id,swarm_id,sequence,principal_id,agent_id,body,content_hash,kind,parent_id,replaces_id) VALUES($1,$2,$3,$4,$5,$6,$7,'text',nullif($8,'')::uuid,nullif($9,'')::uuid)", id, swarm, sequence, a.PrincipalID, a.AgentID, in.Body, digest([]byte(in.Body)), in.ParentID, in.ReplacesID)
	if err != nil {
		return 0, nil, err
	}
	if err = s.roomEvent(r.Context(), tx, a, swarm, version, "swarm.message", swarm, "/v1/swarms/"+swarm+"/context"); err != nil {
		return 0, nil, err
	}
	return 201, map[string]any{"id": id, "sequence": sequence, "content_trust": "untrusted"}, nil
}
func (s *Service) withdrawMembership(r *http.Request, tx pgx.Tx, a actor, swarm string) (int, any, error) {
	roomState, err := roomMembershipAuthority(r.Context(), tx, swarm, false)
	if err != nil {
		return 0, nil, err
	}
	var in struct{}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	var id, role string
	var version int64
	if err := tx.QueryRow(r.Context(), "SELECT id,role,version FROM memberships WHERE swarm_id=$1 AND agent_id=$2 FOR UPDATE", swarm, a.AgentID).Scan(&id, &role, &version); err != nil {
		return 0, nil, err
	}
	if role == "coordinator" && roomState == "active" {
		return 0, nil, fail(409, "archive_or_transfer_before_withdrawal")
	}
	// Keep submitted evidence while invalidating live claims and their DAG.
	if err := s.withdrawAssignments(r.Context(), tx, a, swarm); err != nil {
		return 0, nil, err
	}
	version++
	if _, err := tx.Exec(r.Context(), "UPDATE memberships SET state='withdrawn',version=$2 WHERE id=$1", id, version); err != nil {
		return 0, nil, err
	}
	if err := s.roomEvent(r.Context(), tx, a, id, version, "membership.withdrawn", swarm, "/v1/swarms/"+swarm+"/context"); err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"state": "withdrawn"}, nil
}
func swarmContext(r *http.Request, tx pgx.Tx, a actor, id string) (int, any, error) {
	var objective, visibility, state string
	var version, sequence int64
	var member bool
	err := tx.QueryRow(r.Context(), "SELECT objective,visibility,state,version,sequence,swarm_member(id) FROM swarms WHERE id=$1", id).Scan(&objective, &visibility, &state, &version, &sequence, &member)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	if !member && visibility != "public" {
		return 0, nil, fail(404, "not_found")
	}
	var after int64
	if v := r.URL.Query().Get("after_seq"); v != "" {
		after, err = strconv.ParseInt(v, 10, 64)
		if err != nil || after < 0 {
			return 0, nil, fail(400, "invalid_sequence")
		}
	}
	rows, err := tx.Query(r.Context(), "SELECT id,sequence,agent_id,body FROM messages WHERE swarm_id=$1 AND sequence>$2 ORDER BY sequence LIMIT 100", id, after)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	messages := []any{}
	for rows.Next() {
		var mid, agent, body string
		var seq int64
		if err = rows.Scan(&mid, &seq, &agent, &body); err != nil {
			return 0, nil, err
		}
		messages = append(messages, map[string]any{"id": mid, "sequence": seq, "agent_id": agent, "body": body, "content_trust": "untrusted"})
		after = seq
	}
	if err = rows.Err(); err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"swarm": map[string]any{"id": id, "objective": objective, "visibility": visibility, "state": state, "version": version, "etag": etag(version), "history_policy": "full"}, "messages": messages, "next_sequence": after, "head_sequence": sequence, "has_more": after < sequence}, nil
}
func (s *Service) roomEvent(ctx context.Context, tx pgx.Tx, a actor, aggregate string, version int64, kind, swarm, path string) error {
	return s.roomEventAudience(ctx, tx, a, aggregate, version, kind, swarm, path, nil)
}

func (s *Service) roomEventAudience(ctx context.Context, tx pgx.Tx, a actor, aggregate string, version int64, kind, swarm, path string, audience []string) error {
	return s.roomEventFacts(ctx, tx, a, aggregate, version, kind, swarm, path, audience, nil)
}

func (s *Service) roomEventFacts(ctx context.Context, tx pgx.Tx, a actor, aggregate string, version int64, kind, swarm, path string, audience []string, extra map[string]any) error {
	id := NewID()
	origin := "registered"
	if s.environment != "production" {
		origin = "release_test"
	}
	facts := map[string]any{}
	for key, value := range extra {
		facts[key] = value
	}
	facts["swarm_id"], facts["resource_path"] = swarm, path
	if len(audience) > 0 {
		facts["audience_principal_ids"] = audience
	}
	_, err := tx.Exec(ctx, "INSERT INTO events(id,aggregate_kind,aggregate_id,version,principal_id,agent_id,type,facts,origin) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)", id, strings.Split(kind, ".")[0], aggregate, version, a.PrincipalID, a.AgentID, kind, canonical(facts), origin)
	if err != nil {
		return err
	}
	switch kind {
	case "coordinator.expired", "membership.expired", "plan.expired", "task.assignment_expired", "task.at_risk", "task.abandoned", "task.cancelled":
		return enqueueFanoutInQueue(ctx, tx, id, "coordination_deadlines")
	default:
		return enqueueFanout(ctx, tx, id)
	}
}
