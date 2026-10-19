package network

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// A clock fixture changes only creation time. Every room and participant is
// created through the restricted API; the worker uses its own LOGIN.
func TestSwarmInactivityWarningAndArchiveSurviveRestart(t *testing.T) {
	s, db := coordinatorFixture(t)
	enableTestTerms(t, s)
	a := planningClient(t, s)
	code, room := a.call("POST", "/v1/swarms", "inactivity-room", map[string]any{"objective": "inactivity lifecycle", "visibility": "restricted", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	id := room["id"].(string)
	ctx := context.Background()
	created := time.Now().UTC().Truncate(time.Microsecond).Add(-46 * 24 * time.Hour)
	if _, err := db.Exec(ctx, "UPDATE undergrid.swarms SET created_at=$2 WHERE id=$1", id, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='undergrid_inactivity_worker_runner') THEN CREATE ROLE undergrid_inactivity_worker_runner LOGIN NOSUPERUSER NOBYPASSRLS; END IF; END $$; GRANT undergrid_worker TO undergrid_inactivity_worker_runner`); err != nil {
		t.Fatal(err)
	}
	worker := New(networkFixtureRoleDSN(t, db, "undergrid_inactivity_worker_runner"), "development", nil)
	// The boundary is inclusive. Repeated repair sweeps after restart must not
	// duplicate a warning or silently archive before the complete grace period.
	for _, days := range []int{29, 30, 30, 44, 45, 46} {
		if err := worker.SweepCoordinationDeadlines(ctx, created.Add(time.Duration(days)*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
		var state string
		var warnings, archives int
		if err := db.QueryRow(ctx, `SELECT state,
		 (SELECT count(*) FROM undergrid.events WHERE aggregate_id=s.id AND type='swarm.inactivity_warning'),
		 (SELECT count(*) FROM undergrid.events WHERE aggregate_id=s.id AND type='swarm.archived')
		 FROM undergrid.swarms s WHERE id=$1`, id).Scan(&state, &warnings, &archives); err != nil {
			t.Fatal(err)
		}
		wantWarnings, wantArchives, wantState := 0, 0, "active"
		if days >= 30 {
			wantWarnings = 1
		}
		if days >= 45 {
			wantArchives, wantState = 1, "archived"
		}
		if state != wantState || warnings != wantWarnings || archives != wantArchives {
			t.Fatalf("day %d: state=%s warnings=%d archives=%d; want %s/%d/%d", days, state, warnings, archives, wantState, wantWarnings, wantArchives)
		}
	}
	// Both policy events and their durable fanout jobs commit with state. Policy
	// attribution must not pretend the coordinator submitted a command.
	var invalid, jobs int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type IN ('swarm.inactivity_warning','swarm.archived') AND facts->>'initiated_by' IS DISTINCT FROM 'inactivity_policy'`, id).Scan(&invalid); err != nil || invalid != 0 {
		t.Fatalf("policy event attribution invalid=%d err=%v", invalid, err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM undergrid_queue.river_job j JOIN undergrid.events e ON j.args->>'event_id'=e.id::text WHERE e.aggregate_id=$1 AND e.type IN ('swarm.inactivity_warning','swarm.archived') AND j.kind='event_fanout_v1'`, id).Scan(&jobs); err != nil || jobs != 2 {
		t.Fatalf("policy event fanout jobs=%d err=%v", jobs, err)
	}
	// Current context remains readable; archiving never manufactures consent or
	// erases the coordinator's participation and historical export authority.
	code, snapshot := a.call("GET", "/v1/swarms/"+id+"/context", "", nil)
	if code != 200 || snapshot["swarm"].(map[string]any)["state"] != "archived" {
		t.Fatal(code, snapshot)
	}
}

func TestSwarmInactivityMessagesDoNotPostponeOverdueArchive(t *testing.T) {
	s, db := coordinatorFixture(t)
	enableTestTerms(t, s)
	a := planningClient(t, s)
	code, room := a.call("POST", "/v1/swarms", "no-message-progress", map[string]any{"objective": "messages are not accepted progress", "visibility": "restricted", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	id := room["id"].(string)
	ctx := context.Background()
	if _, err := db.Exec(ctx, "UPDATE undergrid.swarms SET created_at=clock_timestamp()-interval '46 days' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	code, out := a.call("POST", "/v1/swarms/"+id+"/messages", "recent-message", map[string]any{"kind": "text", "body": "Still here. This is not reviewed work."})
	if code != 201 {
		t.Fatal(code, out)
	}
	// First repair after a long outage must archive without an earlier warning
	// sweep. Fresh chatter and event timestamps cannot reset the progress clock.
	if _, err := db.Exec(ctx, `DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='undergrid_inactivity_worker_runner') THEN CREATE ROLE undergrid_inactivity_worker_runner LOGIN NOSUPERUSER NOBYPASSRLS; END IF; END $$; GRANT undergrid_worker TO undergrid_inactivity_worker_runner`); err != nil {
		t.Fatal(err)
	}
	worker := New(networkFixtureRoleDSN(t, db, "undergrid_inactivity_worker_runner"), "development", nil)
	if err := worker.SweepCoordinationDeadlines(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.QueryRow(ctx, "SELECT state FROM undergrid.swarms WHERE id=$1", id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "archived" {
		t.Fatalf("overdue room remained %s after fresh chatter or missing day-30 sweep", state)
	}
}

func TestSwarmInactivityOnlyReviewedAcceptedEvidenceRenewsClock(t *testing.T) {
	for _, decision := range []string{"pending", "reject", "accept"} {
		t.Run(decision, func(t *testing.T) {
			x := newArtifactScenario(t)
			ctx := context.Background()
			id := x.room["id"].(string)
			data := []byte(`{"sources":[{"source_id":"inactivity-fixture","source_url":"https://example.org/"}]}`)
			// Keep the report narrower than the complete membership. The warning
			// must never copy private evidence metadata into room-wide events.
			audience := []string{x.principals[0], x.principals[2]}
			upload := x.uploadForAudience(t, data, "clock-progress", "progress", audience)
			artifactPath := "/v1/artifacts/" + upload["id"].(string)
			code, out := x.actors[2].call("POST", artifactPath+"/finalize", "clock-seal", x.finalBody(data))
			if code != 200 {
				t.Fatal(code, out)
			}
			x.startWorker(t)
			evaluation := x.waitEvaluation(t, upload["id"].(string))
			if evaluation["result"] != "passed" {
				t.Fatal(evaluation)
			}
			taskPath := "/v1/tasks/" + x.task["id"].(string)
			_, current := x.actors[2].call("GET", taskPath, "", nil)
			code, evidence := x.actors[2].versioned("POST", taskPath+"/progress-evidence", "clock-evidence", current["etag"].(string), map[string]any{
				"claim_epoch": current["claim_epoch"], "artifact_id": upload["id"], "artifact_sha256": digest(data), "evaluation_id": evaluation["id"],
				"report": "Private reviewed fixture progress.", "audience_principal_ids": audience, "reviewer_principal_id": x.principals[0],
			})
			if code != 201 {
				t.Fatal(code, evidence)
			}
			if decision != "pending" {
				code, out = x.actors[0].call("POST", taskPath+"/progress-evidence-review", "clock-review", map[string]any{
					"evidence_id": evidence["id"], "decision": decision, "report_sha256": evidence["report_sha256"],
				})
				if code != 200 {
					t.Fatal(code, out)
				}
			}
			if _, err := x.db.Exec(ctx, "UPDATE undergrid.swarms SET created_at=clock_timestamp()-interval '46 days' WHERE id=$1", id); err != nil {
				t.Fatal(err)
			}
			if decision == "accept" {
				projection, err := pgx.Connect(ctx, x.workerDSN)
				if err != nil {
					t.Fatal(err)
				}
				defer projection.Close(ctx)
				var stamp time.Time
				if err = projection.QueryRow(ctx, "SELECT undergrid.swarm_accepted_progress_at($1)", id).Scan(&stamp); err != nil {
					t.Fatal(err)
				}
				if stamp.Before(time.Now().Add(-time.Hour)) {
					t.Fatal("accepted progress projection lost clock", stamp)
				}
				if _, err = projection.Exec(ctx, "SELECT report FROM undergrid.task_progress_evidence"); err == nil {
					t.Fatal("clock worker gained raw private evidence reads")
				}
				api, err := pgx.Connect(ctx, x.s.dsn)
				if err != nil {
					t.Fatal(err)
				}
				defer api.Close(ctx)
				if _, err = api.Exec(ctx, "SELECT undergrid.swarm_accepted_progress_at($1)", id); err == nil {
					t.Fatal("API gained privileged private progress projection")
				}
			}
			worker := New(x.workerDSN, "development", nil)
			if err := worker.SweepCoordinationDeadlines(ctx, time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			var state string
			if err := x.db.QueryRow(ctx, "SELECT state FROM undergrid.swarms WHERE id=$1", id).Scan(&state); err != nil {
				t.Fatal(err)
			}
			want := "archived"
			if decision == "accept" {
				want = "active"
			}
			if state != want {
				t.Fatalf("%s evidence: room=%s want=%s", decision, state, want)
			}
			if decision == "accept" {
				if err := worker.SweepCoordinationDeadlines(ctx, time.Now().UTC().Add(31*24*time.Hour)); err != nil {
					t.Fatal(err)
				}
				var warnings int
				if err := x.db.QueryRow(ctx, "SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type='swarm.inactivity_warning'", id).Scan(&warnings); err != nil || warnings != 1 {
					t.Fatalf("accepted progress did not start a new warning clock: warnings=%d err=%v", warnings, err)
				}
			}
			var leaked int
			if err := x.db.QueryRow(ctx, `SELECT count(*) FROM undergrid.events WHERE aggregate_id=$1 AND type IN('swarm.inactivity_warning','swarm.archived')
			 AND (strpos(facts::text,$2)>0 OR strpos(facts::text,$3)>0 OR strpos(facts::text,$4)>0 OR strpos(facts::text,$5)>0)`,
				id, evidence["id"], upload["id"], x.task["id"], "Private reviewed fixture progress.").Scan(&leaked); err != nil || leaked != 0 {
				t.Fatalf("room-wide inactivity event disclosed private progress metadata: count=%d err=%v", leaked, err)
			}
		})
	}
}

// A verifier result and one beneficiary's acceptance are not completed output.
// Only the final required acceptance can renew the room's accepted-work clock.
func TestSwarmInactivityOnlyCompleteOutputAcceptanceRenewsClock(t *testing.T) {
	for _, complete := range []bool{false, true} {
		name := "partial"
		if complete {
			name = "complete"
		}
		t.Run(name, func(t *testing.T) {
			x := newArtifactScenario(t)
			data := []byte(`{"sources":[{"source_id":"inactivity-output","source_url":"https://example.org/"}]}`)
			upload := x.upload(t, data, "output-clock")
			path := "/v1/artifacts/" + upload["id"].(string)
			if code, out := x.actors[2].call("POST", path+"/finalize", "seal-clock", x.finalBody(data)); code != 200 {
				t.Fatal(code, out)
			}
			x.startWorker(t)
			evaluation := x.waitEvaluation(t, upload["id"].(string))
			if evaluation["result"] != "passed" {
				t.Fatal(evaluation)
			}
			actors := x.actors[:1]
			if complete {
				actors = x.actors[:2]
			}
			for _, a := range actors {
				if code, out := a.call("POST", path+"/acceptances", "clock-accept", map[string]any{"evaluation_id": evaluation["id"], "artifact_sha256": digest(data), "plan_hash": x.plan["content_hash"]}); code != 200 {
					t.Fatal(code, out)
				}
			}
			ctx := context.Background()
			if _, err := x.db.Exec(ctx, "UPDATE undergrid.swarms SET created_at=clock_timestamp()-interval '46 days' WHERE id=$1", x.room["id"]); err != nil {
				t.Fatal(err)
			}
			worker := New(x.workerDSN, "development", nil)
			if err := worker.SweepCoordinationDeadlines(ctx, time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			var state string
			if err := x.db.QueryRow(ctx, "SELECT state FROM undergrid.swarms WHERE id=$1", x.room["id"]).Scan(&state); err != nil {
				t.Fatal(err)
			}
			want := "archived"
			if complete {
				want = "active"
			}
			if state != want {
				t.Fatalf("%s acceptance room=%s want=%s", name, state, want)
			}
		})
	}
}

func TestSwarmInactivityBoundedWarningDrainDoesNotStarveLaterRooms(t *testing.T) {
	s, db := coordinatorFixture(t)
	enableTestTerms(t, s)
	var a testClient
	ctx := context.Background()
	ids := []string{}
	for index := range 101 {
		// Respect the real ten-active-room per-principal admission limit.
		if index%10 == 0 {
			a = planningClient(t, s)
		}
		code, room := a.call("POST", "/v1/swarms", NewID(), map[string]any{"objective": "fair bounded inactivity warnings", "visibility": "restricted", "recruitment_roles": []any{}})
		if code != 201 {
			t.Fatal(code, room)
		}
		ids = append(ids, room["id"].(string))
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := db.Exec(ctx, "UPDATE undergrid.swarms SET created_at=$2 WHERE id=ANY($1::uuid[])", ids, now.Add(-31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='undergrid_inactivity_worker_runner') THEN CREATE ROLE undergrid_inactivity_worker_runner LOGIN NOSUPERUSER NOBYPASSRLS; END IF; END $$; GRANT undergrid_worker TO undergrid_inactivity_worker_runner`); err != nil {
		t.Fatal(err)
	}
	worker := New(networkFixtureRoleDSN(t, db, "undergrid_inactivity_worker_runner"), "development", nil)
	for range 2 {
		if err := worker.SweepCoordinationDeadlines(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.events WHERE aggregate_id=ANY($1::uuid[]) AND type='swarm.inactivity_warning'", ids).Scan(&count); err != nil || count != len(ids) {
		t.Fatalf("bounded discovery starved rooms: warned=%d want=%d err=%v", count, len(ids), err)
	}
	// A restarted sweep with a different timezone represents the same instant,
	// not a new warning episode.
	restarted := New(worker.dsn, "development", nil)
	if err := restarted.SweepCoordinationDeadlines(ctx, now.In(time.FixedZone("fixture", 7*60*60))); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM undergrid.events WHERE aggregate_id=ANY($1::uuid[]) AND type='swarm.inactivity_warning'", ids).Scan(&count); err != nil || count != len(ids) {
		t.Fatalf("timezone/restart duplicated warnings: count=%d err=%v", count, err)
	}
}

func TestSwarmInactivitySerializesWithFinalOutputAcceptance(t *testing.T) {
	x := newArtifactScenario(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	data := []byte(`{"sources":[{"source_id":"inactivity-race","source_url":"https://example.org/"}]}`)
	upload := x.upload(t, data, "race-output")
	path := "/v1/artifacts/" + upload["id"].(string)
	if code, out := x.actors[2].call("POST", path+"/finalize", "race-seal", x.finalBody(data)); code != 200 {
		t.Fatal(code, out)
	}
	var args artifactEvaluationArgs
	if err := x.db.QueryRow(ctx, "SELECT args FROM undergrid_queue.river_job WHERE kind='artifact_evaluation_v1' AND args->>'artifact_id'=$1 ORDER BY id LIMIT 1", upload["id"]).Scan(&args); err != nil {
		t.Fatal(err)
	}
	worker := New(x.workerDSN, "development", nil)
	worker.artifacts = x.s.artifacts
	if err := worker.evaluateArtifact(ctx, args); err != nil {
		t.Fatal(err)
	}
	evaluation := x.waitEvaluation(t, upload["id"].(string))
	acceptance := map[string]any{"evaluation_id": evaluation["id"], "artifact_sha256": digest(data), "plan_hash": x.plan["content_hash"]}
	if code, out := x.actors[1].call("POST", path+"/acceptances", "race-first", acceptance); code != 200 {
		t.Fatal(code, out)
	}
	if _, err := x.db.Exec(ctx, "UPDATE undergrid.swarms SET created_at=clock_timestamp()-interval '46 days' WHERE id=$1", x.room["id"]); err != nil {
		t.Fatal(err)
	}
	held, err := x.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Rollback(context.Background())
	if _, err = held.Exec(ctx, "SELECT id FROM undergrid.swarms WHERE id=$1 FOR UPDATE", x.room["id"]); err != nil {
		t.Fatal(err)
	}
	swept, accepted := make(chan error, 1), make(chan int, 1)
	go func() { swept <- worker.SweepCoordinationDeadlines(ctx, time.Now().UTC()) }()
	// Queue the worker at the room before the coordinator's command. This also
	// exercises principal→room order against event foreign-key attribution.
	for {
		var waiting bool
		if _, err = held.Exec(ctx, "SELECT pg_stat_clear_snapshot()"); err != nil {
			t.Fatal(err)
		}
		if err = held.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE usename='undergrid_artifact_worker_runner' AND wait_event_type='Lock')").Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(5 * time.Millisecond):
		}
	}
	go func() {
		code, _ := x.actors[0].call("POST", path+"/acceptances", "race-final", acceptance)
		accepted <- code
	}()
	for {
		var waiting int
		if _, err = held.Exec(ctx, "SELECT pg_stat_clear_snapshot()"); err != nil {
			t.Fatal(err)
		}
		if err = held.QueryRow(ctx, "SELECT count(DISTINCT usename) FROM pg_stat_activity WHERE usename IN ('undergrid_artifact_worker_runner','undergrid_artifact_api_runner') AND wait_event_type='Lock'").Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting == 2 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(5 * time.Millisecond):
		}
	}
	if err = held.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-swept; err != nil {
		t.Fatal(err)
	}
	code := <-accepted
	var state, taskState string
	var acceptedEvents int
	if err = x.db.QueryRow(ctx, "SELECT s.state,t.state,(SELECT count(*) FROM undergrid.events WHERE aggregate_id=t.id AND type='task.output_accepted') FROM undergrid.swarms s JOIN undergrid.tasks t ON t.id=$2 WHERE s.id=$1", x.room["id"], x.task["id"]).Scan(&state, &taskState, &acceptedEvents); err != nil {
		t.Fatal(err)
	}
	if code == 200 {
		if state != "active" || taskState != "accepted_output" || acceptedEvents != 1 {
			t.Fatal("accepted progress lost archive race", state, taskState, acceptedEvents)
		}
	} else if code == 409 {
		if state != "archived" || taskState == "accepted_output" || acceptedEvents != 0 {
			t.Fatal("archive manufactured late acceptance", state, taskState, acceptedEvents)
		}
	} else {
		t.Fatal("unexpected racing acceptance", code, state)
	}
}

func TestSwarmInactivityArchiveStopsLaterDeadlineMutation(t *testing.T) {
	x := newArtifactScenario(t)
	ctx := context.Background()
	if _, err := x.db.Exec(ctx, "UPDATE undergrid.swarms SET created_at=clock_timestamp()-interval '46 days' WHERE id=$1", x.room["id"]); err != nil {
		t.Fatal(err)
	}
	worker := New(x.workerDSN, "development", nil)
	now := time.Now().UTC()
	if err := worker.SweepCoordinationDeadlines(ctx, now); err != nil {
		t.Fatal(err)
	}
	var state, taskBefore, taskAfter string
	if err := x.db.QueryRow(ctx, "SELECT s.state,t.state FROM undergrid.swarms s JOIN undergrid.tasks t ON t.id=$2 WHERE s.id=$1", x.room["id"], x.task["id"]).Scan(&state, &taskBefore); err != nil {
		t.Fatal(err)
	}
	if state != "archived" {
		t.Fatal("room not archived", state)
	}
	if err := worker.SweepCoordinationDeadlines(ctx, now.Add(60*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := x.db.QueryRow(ctx, "SELECT state FROM undergrid.tasks WHERE id=$1", x.task["id"]).Scan(&taskAfter); err != nil {
		t.Fatal(err)
	}
	if taskBefore != taskAfter {
		t.Fatalf("terminal room task mutated by later timer: %s -> %s", taskBefore, taskAfter)
	}
}
