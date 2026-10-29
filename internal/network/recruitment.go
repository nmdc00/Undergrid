package network

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

type recruitArgs struct {
	SwarmID string `json:"swarm_id"`
}

func (recruitArgs) Kind() string { return "builder_recruitment_v1" }

type recruitWorker struct {
	river.WorkerDefaults[recruitArgs]
	service *Service
}

func (w *recruitWorker) Work(ctx context.Context, job *river.Job[recruitArgs]) error {
	return w.service.ScanRecruitment(ctx, job.Args.SwarmID)
}
func enqueueRecruitment(ctx context.Context, tx pgx.Tx, swarm string) error {
	client, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		return err
	}
	_, err = client.InsertTx(ctx, tx, recruitArgs{swarm}, &river.InsertOpts{MaxAttempts: 10})
	return err
}

const recruitmentDailyLimit = 5
const recruitmentScanPageSize = 256

// The day boundary is database UTC, independent of a worker's local timezone.
const recruitmentDayStartSQL = "date_trunc('day',emission.at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'"

type recruitmentRoom struct {
	id, coordinator string
	roles           []RecruitmentRole
}
type recruitmentTarget struct {
	id, principal string
	profile       AgentProfile
}

func (s *Service) ScanRecruitment(ctx context.Context, swarmID string) error {
	db, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if _, err = db.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		return err
	}
	// Keyset pages have no global LIMIT cutoff: later rooms and agents remain
	// reachable even after the network exceeds a single page.
	after := ""
	for {
		rows, err := db.Query(ctx, "SELECT id::text,coordinator_agent_id::text,recruitment_roles FROM swarms WHERE state='active' AND visibility<>'private' AND ($1='' OR id::text=$1) AND id::text>$2 ORDER BY id::text LIMIT $3", swarmID, after, recruitmentScanPageSize)
		if err != nil {
			return err
		}
		rooms := []recruitmentRoom{}
		for rows.Next() {
			var room recruitmentRoom
			var raw []byte
			if err = rows.Scan(&room.id, &room.coordinator, &raw); err != nil {
				rows.Close()
				return err
			}
			if err = json.Unmarshal(raw, &room.roles); err != nil {
				rows.Close()
				return err
			}
			rooms = append(rooms, room)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, room := range rooms {
			for _, role := range room.roles {
				if err = s.scanRecruitmentRole(ctx, db, room, role); err != nil {
					return err
				}
			}
		}
		if len(rooms) < recruitmentScanPageSize {
			return nil
		}
		after = rooms[len(rooms)-1].id
	}
}
func recruitmentRemaining(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, swarm, role string) (int, time.Time, error) {
	var count int
	var at time.Time
	err := db.QueryRow(ctx, "WITH emission AS MATERIALIZED (SELECT clock_timestamp() AS at) SELECT (SELECT count(*) FROM opportunities WHERE kind='builder_recruitment' AND source_swarm_id=$1 AND role_key=$2 AND created_at>="+recruitmentDayStartSQL+"),emission.at FROM emission", swarm, role).Scan(&count, &at)
	return max(0, recruitmentDailyLimit-count), at, err
}
func (s *Service) scanRecruitmentRole(ctx context.Context, db *pgx.Conn, room recruitmentRoom, role RecruitmentRole) error {
	remaining, _, err := recruitmentRemaining(ctx, db, room.id, role.Key)
	if err != nil || remaining == 0 {
		return err
	}
	after := ""
	for {
		rows, err := db.Query(ctx, `SELECT a.id::text,a.principal_id::text,a.profile FROM agents a JOIN principals p ON p.id=a.principal_id WHERE a.status='active' AND p.status='active' AND a.profile->>'public'='true' AND a.profile->>'recruitment'='true' AND a.profile->'capabilities' @> $1::jsonb AND a.id<>$2 AND a.id::text>$3 ORDER BY a.id::text LIMIT $4`, canonical([]string{role.SchemaFamily}), room.coordinator, after, recruitmentScanPageSize)
		if err != nil {
			return err
		}
		targets := []recruitmentTarget{}
		ids := []string{}
		for rows.Next() {
			var target recruitmentTarget
			var raw []byte
			if err = rows.Scan(&target.id, &target.principal, &raw); err != nil {
				rows.Close()
				return err
			}
			if err = json.Unmarshal(raw, &target.profile); err != nil {
				rows.Close()
				return err
			}
			targets = append(targets, target)
			ids = append(ids, target.id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			return nil
		}
		// Read dedup knowledge once per bounded page, not one transaction per
		// already-offered recipient. Hash construction stays in one Go rule.
		rows, err = db.Query(ctx, "SELECT input_hash FROM opportunities WHERE kind='builder_recruitment' AND source_swarm_id=$1 AND role_key=$2 AND recipient_agent_id=ANY($3::uuid[])", room.id, role.Key, ids)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for rows.Next() {
			var hash string
			if err = rows.Scan(&hash); err != nil {
				rows.Close()
				return err
			}
			seen[hash] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, target := range targets {
			if seen[recruitmentInputHash(room.id, role, target.id, target.profile)] {
				continue
			}
			exhausted, err := s.recruitOne(ctx, db, room.id, role, target.id, target.principal)
			if err != nil {
				return err
			}
			if exhausted {
				return nil
			}
		}
		if len(targets) < recruitmentScanPageSize {
			return nil
		}
		after = targets[len(targets)-1].id
	}
}
func recruitmentInputHash(swarm string, role RecruitmentRole, agent string, profile AgentProfile) string {
	return digest(canonical([]any{swarm, role, agent, profile.Capabilities}))
}
func (s *Service) recruitOne(ctx context.Context, db *pgx.Conn, swarm string, role RecruitmentRole, agent, principal string) (bool, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	// Serialize only workers emitting this role's offers. This is a budget
	// guard, never a substitute for current domain authority below.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 702836))", swarm+"/"+role.Key); err != nil {
		return false, err
	}
	var coordinator, owner string
	err = tx.QueryRow(ctx, "SELECT s.coordinator_agent_id::text,a.principal_id::text FROM swarms s JOIN agents a ON a.id=s.coordinator_agent_id WHERE s.id=$1", swarm).Scan(&coordinator, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Match the domain's principal -> agent guard order. Lock both owners in
	// sorted order; never acquire the source swarm after recipient authority.
	rows, err := tx.Query(ctx, "SELECT id::text,status FROM principals WHERE id=ANY($1::uuid[]) ORDER BY id FOR UPDATE", []string{owner, principal})
	if err != nil {
		return false, err
	}
	active := map[string]bool{}
	for rows.Next() {
		var id, state string
		if err = rows.Scan(&id, &state); err != nil {
			rows.Close()
			return false, err
		}
		active[id] = state == "active"
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	if !active[owner] {
		return true, nil
	}
	if !active[principal] {
		return false, nil
	}
	rows, err = tx.Query(ctx, "SELECT id::text,principal_id::text,status,profile FROM agents WHERE id=ANY($1::uuid[]) ORDER BY id FOR UPDATE", []string{coordinator, agent})
	if err != nil {
		return false, err
	}
	active = map[string]bool{}
	var profile AgentProfile
	for rows.Next() {
		var id, p, state string
		var raw []byte
		if err = rows.Scan(&id, &p, &state, &raw); err != nil {
			rows.Close()
			return false, err
		}
		active[id] = state == "active" && ((id == coordinator && p == owner) || (id == agent && p == principal))
		if id == agent {
			if err = json.Unmarshal(raw, &profile); err != nil {
				rows.Close()
				return false, err
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	if !active[coordinator] {
		return true, nil
	}
	if !active[agent] || agent == coordinator || !profile.Public || !profile.Recruitment || !slices.Contains(profile.Capabilities, role.SchemaFamily) {
		return false, nil
	}
	var currentCoordinator, state, visibility string
	var raw []byte
	err = tx.QueryRow(ctx, "SELECT coordinator_agent_id::text,state,visibility,recruitment_roles FROM swarms WHERE id=$1", swarm).Scan(&currentCoordinator, &state, &visibility, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if currentCoordinator != coordinator {
		return false, errors.New("recruitment coordinator changed; retry")
	}
	if state != "active" || visibility == "private" {
		return true, nil
	}
	var roles []RecruitmentRole
	if err = json.Unmarshal(raw, &roles); err != nil {
		return false, err
	}
	if !slices.Contains(roles, role) {
		return true, nil
	}
	// Capture the database clock after all authority waits. Explicitly store
	// this instant so a transaction crossing midnight consumes its emission
	// day's budget, rather than its earlier transaction-start day.
	remaining, emittedAt, err := recruitmentRemaining(ctx, tx, swarm, role.Key)
	if err != nil || remaining == 0 {
		return remaining == 0, err
	}
	id := NewID()
	hash := recruitmentInputHash(swarm, role, agent, profile)
	evidence := map[string]any{"algorithm": "declared-capability-v1", "reason": "declared_schema_capability", "assurance": "self_declared"}
	tag, err := tx.Exec(ctx, "INSERT INTO opportunities(id,recipient_agent_id,source_swarm_id,role_key,candidate_agent_id,kind,input_hash,evidence,created_at) VALUES($1,$2,$3,$4,$2,'builder_recruitment',$5,$6,$7) ON CONFLICT DO NOTHING", id, agent, swarm, role.Key, hash, canonical(evidence), emittedAt)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 1 {
		if err = s.notify(ctx, tx, principal, agent, id, 1, "opportunity.available", "/v1/opportunities/"+id); err != nil {
			return false, err
		}
		remaining--
	}
	return remaining == 0, tx.Commit(ctx)
}
func listRecruitment(r *http.Request, tx pgx.Tx, a actor, id string) (int, any, error) {
	rows, err := tx.Query(r.Context(), `SELECT o.id,o.source_swarm_id,o.role_key,o.input_hash,o.evidence FROM opportunities o JOIN swarms s ON s.id=o.source_swarm_id WHERE o.recipient_agent_id=$1 AND o.kind='builder_recruitment' AND o.state='suggested' AND s.state='active' AND s.visibility<>'private' AND ($2='' OR o.id::text=$2) ORDER BY o.created_at LIMIT 100`, a.AgentID, id)
	if err != nil {
		return 0, nil, err
	}
	type candidate struct {
		id, swarm, role, hash string
		evidence              []byte
	}
	candidates := []candidate{}
	for rows.Next() {
		var item candidate
		if err = rows.Scan(&item.id, &item.swarm, &item.role, &item.hash, &item.evidence); err != nil {
			rows.Close()
			return 0, nil, err
		}
		candidates = append(candidates, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, nil, err
	}
	items := []any{}
	for _, item := range candidates {
		if err = requireCurrentRecruitmentOpportunityEvidence(r, tx, a, item.id, item.swarm, item.role, item.hash); err != nil {
			var denial problem
			if errors.As(err, &denial) && denial.Status == 404 {
				continue
			}
			return 0, nil, err
		}
		var objective string
		if err = tx.QueryRow(r.Context(), `SELECT objective FROM swarms WHERE id=$1`, item.swarm).Scan(&objective); err != nil {
			return 0, nil, err
		}
		items = append(items, map[string]any{"id": item.id, "kind": "builder_recruitment", "origin": "registered_capability", "swarm_id": item.swarm, "role_key": item.role, "objective": objective, "evidence": json.RawMessage(item.evidence)})
	}
	if id != "" {
		if len(items) == 0 {
			return 0, nil, fail(404, "not_found")
		}
		return 200, items[0], nil
	}
	return 200, map[string]any{"items": items}, nil
}

func (s *Service) applyToSwarm(r *http.Request, tx pgx.Tx, a actor, swarm string) (int, any, error) {
	request, err := readIdempotencyRequest(r, true)
	if err != nil {
		return 0, nil, err
	}
	restoreIdempotencyBody(r, request)
	var in struct {
		RoleKey        string   `json:"role_key"`
		CapabilityRefs []string `json:"capability_refs"`
	}
	if err := decode(r, &in); err != nil {
		return 0, nil, err
	}
	if in.RoleKey == "" || len(in.CapabilityRefs) > 20 {
		return 0, nil, fail(422, "invalid_application")
	}
	restoreIdempotencyBody(r, request)
	var roomVersion int64
	var raw []byte
	var visibility, state string
	err = tx.QueryRow(r.Context(), "SELECT recruitment_roles,visibility,state,version FROM application_room_context($1)", swarm).Scan(&raw, &visibility, &state, &roomVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fail(404, "not_found")
	}
	if err != nil {
		return 0, nil, err
	}
	if visibility == "private" || state != "active" {
		return 0, nil, fail(404, "not_found")
	}
	var roles []RecruitmentRole
	if err = json.Unmarshal(raw, &roles); err != nil {
		return 0, nil, err
	}
	var selectedRole RecruitmentRole
	found := false
	for _, role := range roles {
		if role.Key == in.RoleKey {
			found = true
			selectedRole = role
		}
	}
	if !found {
		return 0, nil, fail(422, "role_not_available")
	}
	return idempotent(r, tx, a, func() (int, any, error) {
		if expected := r.Header.Get("If-Match"); expected != "" && expected != etag(roomVersion) {
			return 0, nil, fail(409, "version_conflict")
		}
		claims := map[string]any{"schema": "application-claims-v1", "role_key": in.RoleKey, "capability_refs": in.CapabilityRefs, "role": selectedRole, "swarm_version": roomVersion}
		id := NewID()
		tag, err := tx.Exec(r.Context(), "WITH application_clock AS MATERIALIZED (SELECT clock_timestamp() AS at) INSERT INTO memberships(id,swarm_id,agent_id,principal_id,state,role,origin,role_key,application_claims,respond_by,created_at) SELECT $1,$2,$3,$4,'applied','contributor','application',$5,$6,at+interval '72 hours',at FROM application_clock ON CONFLICT(swarm_id,agent_id) DO NOTHING", id, swarm, a.AgentID, a.PrincipalID, in.RoleKey, canonical(claims))
		if err != nil {
			return 0, nil, err
		}
		if tag.RowsAffected() == 0 {
			return 0, nil, fail(409, "membership_exists")
		}
		if err = s.roomEvent(r.Context(), tx, a, id, 1, "membership.applied", swarm, "/v1/swarms/"+swarm+"/context"); err != nil {
			return 0, nil, err
		}
		return 201, map[string]any{"id": id, "state": "applied", "version": 1, "etag": etag(1)}, nil
	})
}

func recruitmentRoute(r *http.Request, tx pgx.Tx, a actor) (bool, int, any, error) {
	if r.URL.Query().Get("kind") == "builder_recruitment" {
		status, result, err := listRecruitment(r, tx, a, "")
		return true, status, result, err
	}
	if strings.HasPrefix(r.URL.Path, "/v1/opportunities/") {
		id := strings.TrimPrefix(r.URL.Path, "/v1/opportunities/")
		if validID(id) {
			var kind string
			err := tx.QueryRow(r.Context(), "SELECT kind FROM opportunities WHERE id=$1", id).Scan(&kind)
			if err == nil && kind == "builder_recruitment" {
				status, result, err := listRecruitment(r, tx, a, id)
				return true, status, result, err
			}
		}
	}
	return false, 0, nil, nil
}
