package network

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestSwarmInactivityDefinerFunctionsIgnoreTemporaryRelations(t *testing.T) {
	s, db := coordinatorFixture(t)
	enableTestTerms(t, s)
	a := planningClient(t, s)
	code, room := a.call("POST", "/v1/swarms", "definer-shadow-room", map[string]any{"objective": "Protected inactivity projections", "visibility": "restricted", "recruitment_roles": []any{}})
	if code != 201 {
		t.Fatal(code, room)
	}
	ctx := context.Background()
	id := room["id"].(string)
	if _, err := db.Exec(ctx, `DO $$ BEGIN IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='undergrid_inactivity_worker_runner') THEN CREATE ROLE undergrid_inactivity_worker_runner LOGIN NOSUPERUSER NOBYPASSRLS; END IF; END $$; GRANT undergrid_worker TO undergrid_inactivity_worker_runner`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"swarm_accepted_progress_at", "lock_swarm_deadline_room"} {
		t.Run(name, func(t *testing.T) {
			worker, err := pgx.Connect(ctx, networkFixtureRoleDSN(t, db, "undergrid_inactivity_worker_runner"))
			if err != nil {
				t.Fatal(err)
			}
			defer worker.Close(ctx)
			if _, err = worker.Exec(ctx, `CREATE FUNCTION pg_temp.inactivity_shadow_id(target uuid) RETURNS uuid LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'temporary content executed as %',current_user; END $$;
  CREATE TEMP VIEW swarms AS SELECT pg_temp.inactivity_shadow_id(id) AS id,coordinator_agent_id,created_at,state FROM undergrid.swarms;`); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "swarm_accepted_progress_at":
				var expected, actual time.Time
				if err = db.QueryRow(ctx, "SELECT created_at FROM undergrid.swarms WHERE id=$1", id).Scan(&expected); err != nil {
					t.Fatal(err)
				}
				if err = worker.QueryRow(ctx, "SELECT undergrid.swarm_accepted_progress_at($1)", id).Scan(&actual); err != nil {
					t.Fatalf("definer evaluated temporary relation: %v", err)
				}
				if !actual.Equal(expected) {
					t.Fatalf("unexpected accepted progress %v/%v", actual, expected)
				}
			case "lock_swarm_deadline_room":
				var expected, actual string
				if err = db.QueryRow(ctx, "SELECT coordinator_agent_id::text FROM undergrid.swarms WHERE id=$1", id).Scan(&expected); err != nil {
					t.Fatal(err)
				}
				if err = worker.QueryRow(ctx, "SELECT coordinator_agent::text FROM undergrid.lock_swarm_deadline_room($1)", id).Scan(&actual); err != nil {
					t.Fatalf("definer evaluated temporary relation: %v", err)
				}
				if actual != expected {
					t.Fatal(actual, expected)
				}
			}
		})
	}
}
