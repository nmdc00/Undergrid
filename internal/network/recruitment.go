package network

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

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
func (s *Service) ScanRecruitment(ctx context.Context, swarmID string) error {
	db, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if _, err = db.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		return err
	}
	rows, err := db.Query(ctx, "SELECT id,coordinator_agent_id,recruitment_roles FROM swarms WHERE state='active' AND visibility<>'private' AND ($1='' OR id::text=$1) ORDER BY id LIMIT 1000", swarmID)
	if err != nil {
		return err
	}
	type room struct {
		id, coordinator string
		roles           []RecruitmentRole
	}
	rooms := []room{}
	for rows.Next() {
		var v room
		var raw []byte
		if err = rows.Scan(&v.id, &v.coordinator, &raw); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(raw, &v.roles); err != nil {
			rows.Close()
			return err
		}
		rooms = append(rooms, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = db.Query(ctx, "SELECT a.id,a.principal_id,a.profile FROM agents a JOIN principals p ON p.id=a.principal_id WHERE a.status='active' AND p.status='active' AND a.profile->>'public'='true' AND a.profile->>'recruitment'='true' ORDER BY a.id LIMIT 1000")
	if err != nil {
		return err
	}
	type target struct {
		id, principal string
		profile       AgentProfile
	}
	targets := []target{}
	for rows.Next() {
		var v target
		var raw []byte
		if err = rows.Scan(&v.id, &v.principal, &raw); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(raw, &v.profile); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, room := range rooms {
		for _, role := range room.roles {
			for _, target := range targets {
				if target.id == room.coordinator || !slices.Contains(target.profile.Capabilities, role.SchemaFamily) {
					continue
				}
				if err = s.recruitOne(ctx, db, room.id, role, target.id, target.principal); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func (s *Service) recruitOne(ctx context.Context, db *pgx.Conn, swarm string, role RecruitmentRole, agent, principal string) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var state string
	if err = tx.QueryRow(ctx, "SELECT status FROM principals WHERE id=$1 FOR UPDATE", principal).Scan(&state); err != nil {
		return err
	}
	if state != "active" {
		return nil
	}
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT status,profile FROM agents WHERE id=$1 FOR UPDATE", agent).Scan(&state, &raw); err != nil {
		return err
	}
	if state != "active" {
		return nil
	}
	var profile AgentProfile
	if json.Unmarshal(raw, &profile) != nil || !profile.Public || !profile.Recruitment || !slices.Contains(profile.Capabilities, role.SchemaFamily) {
		return nil
	}
	id := NewID()
	hash := digest(canonical([]any{swarm, role, agent, profile.Capabilities}))
	evidence := map[string]any{"algorithm": "declared-capability-v1", "reason": "declared_schema_capability", "assurance": "self_declared"}
	tag, err := tx.Exec(ctx, "INSERT INTO opportunities(id,recipient_agent_id,source_swarm_id,role_key,candidate_agent_id,kind,input_hash,evidence) VALUES($1,$2,$3,$4,$2,'builder_recruitment',$5,$6) ON CONFLICT DO NOTHING", id, agent, swarm, role.Key, hash, canonical(evidence))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		if err = s.notify(ctx, tx, principal, agent, id, 1, "opportunity.available", "/v1/opportunities/"+id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func listRecruitment(r *http.Request, tx pgx.Tx, a actor, id string) (int, any, error) {
	rows, err := tx.Query(r.Context(), `SELECT o.id,o.source_swarm_id,o.role_key,s.objective,o.evidence FROM opportunities o JOIN swarms s ON s.id=o.source_swarm_id WHERE o.recipient_agent_id=$1 AND o.kind='builder_recruitment' AND o.state='suggested' AND s.state='active' AND s.visibility<>'private' AND ($2='' OR o.id::text=$2) ORDER BY o.created_at LIMIT 100`, a.AgentID, id)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var oid, swarm, role, objective string
		var evidence []byte
		if err = rows.Scan(&oid, &swarm, &role, &objective, &evidence); err != nil {
			return 0, nil, err
		}
		items = append(items, map[string]any{"id": oid, "kind": "builder_recruitment", "origin": "registered_capability", "swarm_id": swarm, "role_key": role, "objective": objective, "evidence": json.RawMessage(evidence)})
	}
	if err = rows.Err(); err != nil {
		return 0, nil, err
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
	var raw []byte
	var visibility, state string
	err := tx.QueryRow(r.Context(), "SELECT recruitment_roles,visibility,state FROM swarms WHERE id=$1", swarm).Scan(&raw, &visibility, &state)
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
	found := false
	for _, role := range roles {
		if role.Key == in.RoleKey {
			found = true
		}
	}
	if !found {
		return 0, nil, fail(422, "role_not_available")
	}
	id := NewID()
	tag, err := tx.Exec(r.Context(), "INSERT INTO memberships(id,swarm_id,agent_id,principal_id,state,role,origin,role_key) VALUES($1,$2,$3,$4,'applied','contributor','application',$5) ON CONFLICT(swarm_id,agent_id) DO NOTHING", id, swarm, a.AgentID, a.PrincipalID, in.RoleKey)
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
