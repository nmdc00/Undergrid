package network

import (
	"context"
	"testing"
	"time"
)

func TestCoordinationDeadlineSilenceAndRestart(t *testing.T) {
	s, db := fixture(t)
	enableTestTerms(t, s)
	a, b := planningClient(t, s), planningClient(t, s)
	_, bid := b.call("GET", "/v1/me", "", nil)
	code, room := a.call("POST", "/v1/swarms", "room", map[string]any{"objective": "deadline catalog", "visibility": "restricted", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	path := "/v1/swarms/" + room["id"].(string)
	code, invite := a.call("POST", path+"/invitations", "invite", map[string]any{"agent_id": bid["agent_id"], "role": "contributor", "respond_by": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, invite)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := db.Exec(ctx, "UPDATE undergrid.memberships SET respond_by=$2 WHERE id=$1", invite["id"], now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	restarted := New(s.dsn, "development", nil)
	// Run the actual River worker: durable timer events reach both expired
	// target and current coordinator exactly once, even after process restart.
	workerCtx, cancel := context.WithCancel(ctx)
	workerDone := make(chan error, 1)
	go func() { workerDone <- RunWorker(workerCtx, s.dsn, "development") }()
	deadline := time.NewTimer(15 * time.Second)
	tick := time.NewTicker(20 * time.Millisecond)
	delivered := false
	for !delivered {
		select {
		case <-deadline.C:
			cancel()
			<-workerDone
			t.Fatal("expiry fanout not delivered")
		case <-tick.C:
			var count int
			if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.inbox i JOIN undergrid.events e ON e.id=i.event_id WHERE e.aggregate_id=$1 AND e.type='membership.expired'", invite["id"]).Scan(&count); err != nil {
				cancel()
				<-workerDone
				t.Fatal(err)
			}
			delivered = count == 2
		}
	}
	deadline.Stop()
	tick.Stop()
	cancel()
	if err := <-workerDone; err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if err := restarted.SweepCoordinationDeadlines(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	var state string
	var version, events int
	if err := db.QueryRow(ctx, "SELECT state,version,(SELECT count(*) FROM undergrid.events WHERE aggregate_id=m.id AND type='membership.expired') FROM undergrid.memberships m WHERE id=$1", invite["id"]).Scan(&state, &version, &events); err != nil {
		t.Fatal(err)
	}
	if state != "expired" || version != 2 || events != 1 {
		t.Fatalf("silence granted or duplicate transition: %s %d %d", state, version, events)
	}
	code, out := b.versioned("POST", path+"/membership-response", "late", invite["etag"].(string), map[string]any{"invitation_id": invite["id"], "decision": "accept"})
	if code != 409 && code != 404 {
		t.Fatal(code, out)
	}

	// Timely acceptance and the same stale repair job serialize on the room.
	c := planningClient(t, s)
	_, cid := c.call("GET", "/v1/me", "", nil)
	code, timely := a.call("POST", path+"/invitations", "timely", map[string]any{"agent_id": cid["agent_id"], "role": "contributor", "respond_by": now.Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, timely)
	}
	done := make(chan error, 1)
	go func() { done <- restarted.SweepCoordinationDeadlines(ctx, now) }()
	code, out = c.versioned("POST", path+"/membership-response", "timely-accept", timely["etag"].(string), map[string]any{"invitation_id": timely["id"], "decision": "accept"})
	if code != 200 {
		t.Fatal(code, out)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "UPDATE undergrid.memberships SET respond_by=$2 WHERE id=$1", timely["id"], now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := restarted.SweepCoordinationDeadlines(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT state,version FROM undergrid.memberships WHERE id=$1", timely["id"]).Scan(&state, &version); err != nil {
		t.Fatal(err)
	}
	if state != "accepted" || version != 2 {
		t.Fatalf("stale timer undid admission: %s %d", state, version)
	}

}

func TestProgressDeadlineCureAndExpiredPlan(t *testing.T) {
	s, db := fixture(t)
	enableTestTerms(t, s)
	a := planningClient(t, s)
	_, who := a.call("GET", "/v1/me", "", nil)
	code, room := a.call("POST", "/v1/swarms", "room", map[string]any{"objective": "deadline catalog", "visibility": "restricted", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	path := "/v1/swarms/" + room["id"].(string)
	_, snapshot := a.call("GET", path+"/context", "", nil)
	principals := []any{who["principal_id"]}
	code, plan := a.versioned("POST", path+"/plans", "plan", snapshot["swarm"].(map[string]any)["etag"].(string), map[string]any{"objective": "deadline catalog", "required_principals": principals, "acceptance_principals": principals, "publication_principals": principals, "rights": map[string]any{"template_id": "test-contribution-v1", "license_id": "CC0-1.0", "publication": "exact_manifest_approval"}, "tasks": []any{map[string]any{"key": "catalog", "assignee_agent_id": who["agent_id"], "depends_on": []any{}, "output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 60}}, "respond_by": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, plan)
	}
	task := plan["tasks"].([]any)[0].(map[string]any)
	taskPath := "/v1/tasks/" + task["id"].(string)
	code, out := a.versioned("POST", taskPath+"/assignment-response", "accept", task["etag"].(string), map[string]any{"decision": "accept", "plan_hash": plan["content_hash"]})
	if code != 200 || out["progress_deadline"] != nil {
		t.Fatal(code, out)
	}
	code, out = a.call("POST", "/v1/plans/"+plan["id"].(string)+"/approvals", "approve", map[string]any{"decision": "approve", "content_hash": plan["content_hash"]})
	if code != 200 {
		t.Fatal(code, out)
	}
	_, executing := a.call("GET", taskPath, "", nil)
	if executing["progress_deadline"] == nil {
		t.Fatal("accepted plan has no progress deadline", executing)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	due := now.Add(-time.Second)
	if _, err := db.Exec(ctx, "UPDATE undergrid.tasks SET progress_deadline=$2 WHERE id=$1", task["id"], due); err != nil {
		t.Fatal(err)
	}
	if err := s.SweepCoordinationDeadlines(ctx, now); err != nil {
		t.Fatal(err)
	}
	_, risk := a.call("GET", taskPath, "", nil)
	var cure time.Time
	if err := db.QueryRow(ctx, "SELECT cure_deadline FROM undergrid.tasks WHERE id=$1", task["id"]).Scan(&cure); err != nil {
		t.Fatal(err)
	}
	if !cure.Equal(due.Add(48 * time.Hour)) {
		t.Fatalf("cure is not 48 hours from missed progress: %s", cure)
	}
	if risk["state"] != "at_risk" || risk["claim_epoch"] != float64(1) || risk["cure_deadline"] == nil {
		t.Fatal(risk)
	}
	reconnected := a
	reconnected.s = New(s.dsn, "development", nil)
	reconnected.s.cursorKey = s.cursorKey
	_, obligations := reconnected.call("GET", "/v1/agent-context", "", nil)
	assignment := obligations["assignments"].([]any)[0].(map[string]any)
	if assignment["cure_deadline"] != risk["cure_deadline"] || assignment["claim_epoch"] != risk["claim_epoch"] {
		t.Fatal("restart lost obligation", obligations)
	}
	code, out = a.versioned("POST", taskPath+"/progress", "cure", risk["etag"].(string), map[string]any{"state": "in_progress", "claim_epoch": risk["claim_epoch"]})
	if code != 200 || out["cure_deadline"] != nil {
		t.Fatal(code, out)
	}
	// The refreshed deadline excludes the old due job.
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- s.SweepCoordinationDeadlines(ctx, now) }()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	_, fresh := a.call("GET", taskPath, "", nil)
	if fresh["state"] != "in_progress" {
		t.Fatal(fresh)
	}
	if _, err := db.Exec(ctx, "UPDATE undergrid.tasks SET progress_deadline=$2 WHERE id=$1", task["id"], now.Add(-49*time.Hour)); err != nil {
		t.Fatal(err)
	}
	code, out = a.versioned("POST", taskPath+"/progress", "overdue-before-sweep", fresh["etag"].(string), map[string]any{"state": "in_progress", "claim_epoch": fresh["claim_epoch"]})
	if code != 409 {
		t.Fatal("elapsed cure bypassed before worker repair", code, out)
	}
	for range 3 {
		if err := s.SweepCoordinationDeadlines(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	_, abandoned := a.call("GET", taskPath, "", nil)
	if abandoned["state"] != "abandoned" || abandoned["claim_epoch"] != float64(2) {
		t.Fatal(abandoned)
	}
	code, out = a.versioned("POST", taskPath+"/progress", "old-claim", abandoned["etag"].(string), map[string]any{"state": "in_progress", "claim_epoch": 1})
	if code != 409 {
		t.Fatal(code, out)
	}
	var events int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='task.abandoned'", task["id"]).Scan(&events); err != nil || events != 1 {
		t.Fatal(events, err)
	}
}

func TestProposalAndAssignmentExpiryPersistence(t *testing.T) {
	s, db := fixture(t)
	enableTestTerms(t, s)
	a := planningClient(t, s)
	_, who := a.call("GET", "/v1/me", "", nil)
	code, room := a.call("POST", "/v1/swarms", "room", map[string]any{"objective": "expiry catalog", "visibility": "restricted", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	path := "/v1/swarms/" + room["id"].(string)
	_, snapshot := a.call("GET", path+"/context", "", nil)
	principals := []any{who["principal_id"]}
	code, plan := a.versioned("POST", path+"/plans", "plan", snapshot["swarm"].(map[string]any)["etag"].(string), map[string]any{"objective": "expiry catalog", "required_principals": principals, "acceptance_principals": principals, "publication_principals": principals, "rights": map[string]any{"template_id": "test-contribution-v1", "license_id": "CC0-1.0", "publication": "exact_manifest_approval"}, "tasks": []any{map[string]any{"key": "catalog", "assignee_agent_id": who["agent_id"], "depends_on": []any{}, "output_schema": "source_catalog_v1", "checks": []string{"json_schema_v1"}, "progress_interval_seconds": 60}}, "respond_by": time.Now().Add(time.Hour)})
	if code != 201 {
		t.Fatal(code, plan)
	}
	task := plan["tasks"].([]any)[0].(map[string]any)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := db.Exec(ctx, "UPDATE undergrid.tasks SET response_deadline=$2 WHERE id=$1", task["id"], now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.SweepCoordinationDeadlines(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	var state string
	var version, epoch, events int
	if err := db.QueryRow(ctx, "SELECT state,version,claim_epoch,(SELECT count(*) FROM undergrid.events WHERE aggregate_id=t.id AND type='task.assignment_expired') FROM undergrid.tasks t WHERE id=$1", task["id"]).Scan(&state, &version, &epoch, &events); err != nil {
		t.Fatal(err)
	}
	if state != "open" || version != 2 || epoch != 2 || events != 1 {
		t.Fatal(state, version, epoch, events)
	}
	if _, err := db.Exec(ctx, "UPDATE undergrid.plan_versions SET respond_by=$2 WHERE id=$1", plan["id"], now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.SweepCoordinationDeadlines(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow(ctx, "SELECT state,(SELECT count(*) FROM undergrid.events WHERE aggregate_id=p.id AND type='plan.expired') FROM undergrid.plan_versions p WHERE id=$1", plan["id"]).Scan(&state, &events); err != nil {
		t.Fatal(err)
	}
	if state != "expired" || events != 1 {
		t.Fatal(state, events)
	}
	code, out := a.call("POST", "/v1/plans/"+plan["id"].(string)+"/approvals", "late", map[string]any{"decision": "approve", "content_hash": plan["content_hash"]})
	if code != 409 {
		t.Fatal(code, out)
	}
}

func TestApplicationDeadlineCannotAdmitAfterExpiry(t *testing.T) {
	s, db := fixture(t)
	a, b := collaborationClient(t, s), collaborationClient(t, s)
	code, room := a.call("POST", "/v1/swarms", "room", map[string]any{"objective": "source catalog", "visibility": "restricted", "recruitment_roles": []any{map[string]any{"key": "catalog-builder", "schema_family": "source_catalog_v1", "description": "Build the dataset"}}})
	if code != 201 {
		t.Fatal(code, room)
	}
	path := "/v1/swarms/" + room["id"].(string)
	code, application := b.call("POST", path+"/applications", "apply", map[string]any{"role_key": "catalog-builder", "capability_refs": []string{"source_catalog_v1"}})
	if code != 201 {
		t.Fatal(code, application)
	}
	ctx := context.Background()
	var deadline, created time.Time
	if err := db.QueryRow(ctx, "SELECT respond_by,created_at FROM undergrid.memberships WHERE id=$1", application["id"]).Scan(&deadline, &created); err != nil {
		t.Fatal(err)
	}
	if deadline.Sub(created) != 72*time.Hour {
		t.Fatal("application default not 72 hours", deadline, created)
	}
	_, who := b.call("GET", "/v1/me", "", nil)
	if _, err := db.Exec(ctx, "UPDATE undergrid.memberships SET respond_by=now()-interval '1 second' WHERE id=$1", application["id"]); err != nil {
		t.Fatal(err)
	}
	// Even before repair, the command denies an elapsed application atomically.
	code, out := a.call("POST", path+"/invitations", "late", map[string]any{"agent_id": who["agent_id"], "role": "contributor", "application_id": application["id"], "respond_by": time.Now().Add(time.Hour)})
	if code != 409 {
		t.Fatal(code, out)
	}
	for range 2 {
		if err := s.SweepCoordinationDeadlines(ctx, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	var state string
	var version int
	if err := db.QueryRow(ctx, "SELECT state,version FROM undergrid.memberships WHERE id=$1", application["id"]).Scan(&state, &version); err != nil {
		t.Fatal(err)
	}
	if state != "expired" || version != 2 {
		t.Fatal(state, version)
	}
}
