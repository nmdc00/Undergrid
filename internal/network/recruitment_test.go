package network

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestBuilderDiscoveryApplicationAndAdmission(t *testing.T) {
	s, db := fixture(t)
	organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
	code, room := organizer.call("POST", "/v1/swarms", "recruit-room", map[string]any{"objective": "Create a reusable source catalog", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "catalog-builder", "schema_family": "source_catalog_v1", "description": "Build and validate the dataset"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	// A bounded scheduler does not promise that this particular randomly
	// allocated builder sorts among the first five on a retained network.
	// Seed its checked offer explicitly, then prove ordinary scanning preserves it.
	_, builderIdentity := builder.call("GET", "/v1/me", "", nil)
	if _, err := db.Exec(context.Background(), "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.recruitOne(context.Background(), db, room["id"].(string), RecruitmentRole{Key: "catalog-builder", SchemaFamily: "source_catalog_v1", Description: "Build and validate the dataset"}, builderIdentity["agent_id"].(string), builderIdentity["principal_id"].(string)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(context.Background(), "RESET ROLE"); err != nil {
		t.Fatal(err)
	}
	if err := s.ScanRecruitment(context.Background(), room["id"].(string)); err != nil {
		t.Fatal(err)
	}
	code, page := builder.call("GET", "/v1/opportunities?kind=builder_recruitment", "", nil)
	if code != 200 {
		t.Fatal(code, page)
	}
	found := false
	for _, raw := range page["items"].([]any) {
		item := raw.(map[string]any)
		if item["swarm_id"] == room["id"] && item["role_key"] == "catalog-builder" {
			found = true
		}
	}
	if !found {
		t.Fatal("builder was not offered the published role", page)
	}
	path := "/v1/swarms/" + room["id"].(string)
	code, application := builder.call("POST", path+"/applications", "apply", map[string]any{"role_key": "catalog-builder", "capability_refs": []string{"source_catalog_v1"}})
	if code != 201 {
		t.Fatal(code, application)
	}
	code, _ = builder.call("GET", path+"/context", "", nil)
	if code != 404 {
		t.Fatalf("application granted access: %d", code)
	}
	_, me := builder.call("GET", "/v1/me", "", nil)
	code, invitation := organizer.call("POST", path+"/invitations", "offer", map[string]any{"agent_id": me["agent_id"], "role": "contributor", "application_id": application["id"], "respond_by": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, invitation)
	}
	code, accepted := builder.versioned("POST", path+"/membership-response", "accept", invitation["etag"].(string), map[string]any{"invitation_id": invitation["id"], "decision": "accept"})
	if code != 200 {
		t.Fatal(code, accepted)
	}
	code, _ = builder.call("GET", path+"/context", "", nil)
	if code != 200 {
		t.Fatalf("accepted builder lacks access: %d", code)
	}
}

func TestRecruitmentLimitsEachRoleToFiveOffersPerUTCDay(t *testing.T) {
	s, db := fixture(t)
	organizer := collaborationClient(t, s)
	for range 8 {
		collaborationClient(t, s)
	}
	code, room := organizer.call("POST", "/v1/swarms", "bounded-recruitment", map[string]any{"objective": "Bounded declared capability recruitment", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "builder", "schema_family": "source_catalog_v1", "description": "Build catalog"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	ctx := context.Background()
	for range 2 {
		if err := s.ScanRecruitment(ctx, room["id"].(string)); err != nil {
			t.Fatal(err)
		}
	}
	var offers, events, inbox int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM undergrid.opportunities WHERE source_swarm_id=$1 AND role_key='builder' AND kind='builder_recruitment'`, room["id"]).Scan(&offers); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM undergrid.events e JOIN undergrid.opportunities o ON o.id=e.aggregate_id WHERE o.source_swarm_id=$1 AND e.type='opportunity.available'`, room["id"]).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM undergrid.inbox i JOIN undergrid.events e ON e.id=i.event_id JOIN undergrid.opportunities o ON o.id=e.aggregate_id WHERE o.source_swarm_id=$1 AND e.type='opportunity.available'`, room["id"]).Scan(&inbox); err != nil {
		t.Fatal(err)
	}
	if offers != 5 || events != 5 || inbox != 5 {
		t.Fatalf("daily recruitment budget or retry effects violated: offers=%d events=%d inbox=%d", offers, events, inbox)
	}
}

func TestRecruitmentConcurrentScansShareOneRoleBudget(t *testing.T) {
	s, db := fixture(t)
	organizer := collaborationClient(t, s)
	for range 8 {
		collaborationClient(t, s)
	}
	code, room := organizer.call("POST", "/v1/swarms", "concurrent-recruitment", map[string]any{"objective": "Concurrent bounded recruitment", "visibility": "restricted", "recruitment_roles": []RecruitmentRole{{Key: "builder", SchemaFamily: "source_catalog_v1"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	errors := make(chan error, 4)
	for range 4 {
		go func() { errors <- s.ScanRecruitment(context.Background(), room["id"].(string)) }()
	}
	for range 4 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	var count, events int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.opportunities WHERE source_swarm_id=$1", room["id"]).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.events e JOIN undergrid.opportunities o ON o.id=e.aggregate_id WHERE o.source_swarm_id=$1 AND e.type='opportunity.available'", room["id"]).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if count != 5 || events != 5 {
		t.Fatalf("concurrent quota race: offers=%d events=%d", count, events)
	}
	// A new role revision and dismissed records must not replenish today's
	// emission budget. These are explicit test-owned state transitions.
	if _, err := db.Exec(context.Background(), "UPDATE undergrid.opportunities SET state='dismissed' WHERE source_swarm_id=$1", room["id"]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(context.Background(), `UPDATE undergrid.swarms SET recruitment_roles='[{"key":"builder","schema_family":"source_catalog_v1","description":"revised"}]' WHERE id=$1`, room["id"]); err != nil {
		t.Fatal(err)
	}
	if err := s.ScanRecruitment(context.Background(), room["id"].(string)); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM undergrid.opportunities WHERE source_swarm_id=$1", room["id"]).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatalf("role revision/state change reset daily cap: %d", count)
	}
}

func TestRecruitmentRechecksCurrentSourceAndRecipient(t *testing.T) {
	for _, scenario := range []string{"archived", "private", "role_changed", "coordinator_revoked", "owner_suspended", "recipient_revoked", "recipient_opted_out"} {
		t.Run(scenario, func(t *testing.T) {
			s, db := fixture(t)
			organizer, builder := collaborationClient(t, s), collaborationClient(t, s)
			role := RecruitmentRole{Key: "builder", SchemaFamily: "source_catalog_v1"}
			code, room := organizer.call("POST", "/v1/swarms", "current-authority", map[string]any{"objective": "Check authority before offering", "visibility": "restricted", "recruitment_roles": []RecruitmentRole{role}})
			if code != 201 {
				t.Fatal(code, room)
			}
			_, owner := organizer.call("GET", "/v1/me", "", nil)
			_, recipient := builder.call("GET", "/v1/me", "", nil)
			ctx := context.Background()
			var err error
			switch scenario {
			case "archived":
				_, err = db.Exec(ctx, "UPDATE undergrid.swarms SET state='archived' WHERE id=$1", room["id"])
			case "private":
				_, err = db.Exec(ctx, "UPDATE undergrid.swarms SET visibility='private' WHERE id=$1", room["id"])
			case "role_changed":
				_, err = db.Exec(ctx, "UPDATE undergrid.swarms SET recruitment_roles='[]' WHERE id=$1", room["id"])
			case "coordinator_revoked":
				_, err = db.Exec(ctx, "UPDATE undergrid.agents SET status='revoked' WHERE id=$1", owner["agent_id"])
			case "owner_suspended":
				_, err = db.Exec(ctx, "UPDATE undergrid.principals SET status='suspended' WHERE id=$1", owner["principal_id"])
			case "recipient_revoked":
				_, err = db.Exec(ctx, "UPDATE undergrid.agents SET status='revoked' WHERE id=$1", recipient["agent_id"])
			case "recipient_opted_out":
				_, err = db.Exec(ctx, `UPDATE undergrid.agents SET profile=jsonb_set(profile,'{recruitment}','false') WHERE id=$1`, recipient["agent_id"])
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, "SET ROLE undergrid_worker;SET search_path=undergrid,pg_catalog"); err != nil {
				t.Fatal(err)
			}
			if _, err = s.recruitOne(ctx, db, room["id"].(string), role, recipient["agent_id"].(string), recipient["principal_id"].(string)); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, "RESET ROLE"); err != nil {
				t.Fatal(err)
			}
			var count int
			if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid.opportunities WHERE source_swarm_id=$1", room["id"]).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("stale authority emitted %d offers", count)
			}
		})
	}
}

func TestRecruitmentUTCDayRecoveryReachesPreviouslyUnofferedAgents(t *testing.T) {
	s, db := fixture(t)
	organizer := collaborationClient(t, s)
	for range 12 {
		collaborationClient(t, s)
	}
	role := RecruitmentRole{Key: "builder", SchemaFamily: "source_catalog_v1"}
	code, room := organizer.call("POST", "/v1/swarms", "utc-recovery", map[string]any{"objective": "Resume bounded offers on the next day", "visibility": "restricted", "recruitment_roles": []RecruitmentRole{role}})
	if code != 201 {
		t.Fatal(code, room)
	}
	_, owner := organizer.call("GET", "/v1/me", "", nil)
	ctx := context.Background()
	rows, err := db.Query(ctx, `SELECT a.id::text,a.profile FROM undergrid.agents a JOIN undergrid.principals p ON p.id=a.principal_id WHERE a.status='active' AND p.status='active' AND a.profile->>'public'='true' AND a.profile->>'recruitment'='true' AND a.profile->'capabilities' @> '["source_catalog_v1"]' AND a.id<>$1 ORDER BY a.id LIMIT 6`, owner["agent_id"])
	if err != nil {
		t.Fatal(err)
	}
	type prior struct {
		id      string
		profile AgentProfile
	}
	targets := []prior{}
	for rows.Next() {
		var target prior
		var raw []byte
		if err = rows.Scan(&target.id, &raw); err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &target.profile); err != nil {
			t.Fatal(err)
		}
		targets = append(targets, target)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 6 {
		t.Fatalf("need six historical recipients: %d", len(targets))
	}
	// Insert explicit historical fixtures; never rewrite acknowledged offers,
	// reset a ledger or move retained timestamps to make a gate pass.
	dayStart := time.Now().UTC().Truncate(24 * time.Hour)
	for i, target := range targets {
		created := dayStart.Add(-time.Microsecond)
		if i == 5 {
			created = dayStart
		}
		if _, err = db.Exec(ctx, `INSERT INTO undergrid.opportunities(id,recipient_agent_id,source_swarm_id,role_key,candidate_agent_id,kind,input_hash,evidence,created_at) VALUES($1,$2,$3,'builder',$2,'builder_recruitment',$4,'{"fixture":"historical-emission"}',$5)`, NewID(), target.id, room["id"], recruitmentInputHash(room["id"].(string), role, target.id, target.profile), created); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.ScanRecruitment(ctx, room["id"].(string)); err != nil {
		t.Fatal(err)
	}
	if err = s.ScanRecruitment(ctx, room["id"].(string)); err != nil {
		t.Fatal(err)
	}
	var today, all, events int
	if err = db.QueryRow(ctx, "SELECT count(*) FILTER(WHERE created_at >= $2),count(*) FROM undergrid.opportunities WHERE source_swarm_id=$1", room["id"], dayStart).Scan(&today, &all); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid.events e JOIN undergrid.opportunities o ON o.id=e.aggregate_id WHERE o.source_swarm_id=$1 AND e.type='opportunity.available'", room["id"]).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if today != 5 || all != 10 || events != 4 {
		t.Fatalf("UTC recovery/quota mismatch today=%d all=%d newly-notified=%d", today, all, events)
	}
	for _, target := range targets {
		var count int
		if err = db.QueryRow(ctx, "SELECT count(*) FROM undergrid.opportunities WHERE source_swarm_id=$1 AND recipient_agent_id=$2", room["id"], target.id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("prior recipient %s offered again: %d", target.id, count)
		}
	}
}
