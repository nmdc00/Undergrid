package network

import (
	"context"
	"testing"
	"time"
)

// A clock fixture changes only creation time. Every room and participant is
// created through the restricted API; the worker uses its own LOGIN.
func TestSwarmInactivityWarningAndArchiveSurviveRestart(t *testing.T) {
	s, db := coordinatorFixture(t)
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
	// Current context remains readable; archiving never manufactures consent or
	// erases the coordinator's participation and historical export authority.
	code, snapshot := a.call("GET", "/v1/swarms/"+id+"/context", "", nil)
	if code != 200 || snapshot["swarm"].(map[string]any)["state"] != "archived" {
		t.Fatal(code, snapshot)
	}
}

func TestSwarmInactivityMessagesDoNotPostponeOverdueArchive(t *testing.T) {
	s, db := coordinatorFixture(t)
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
	code, out := a.call("POST", "/v1/swarms/"+id+"/messages", "recent-message", map[string]any{"body": "Still here. This is not reviewed work."})
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
