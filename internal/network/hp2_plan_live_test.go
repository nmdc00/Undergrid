//go:build hp1_live && hp2_live

package network

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type hp2SetupMode int

const (
	hp2Foundation hp2SetupMode = iota
	hp2Publication
	hp2Builder
	hp2PlanProposal
	hp2RoomOrigin
	hp2CompatibleGroups
	hp2Clarifications
	hp2CapabilityDiscovery
	hp2TypedRequirements
	hp2Applications
	hp2Continuous
	hp3TaskExecution
)

func TestHP2LiveOwnerPlanProposalRevisionAndRecovery(t *testing.T) {
	hp2LiveOwnerSetup(t, hp2PlanProposal)
}

// Checkpoints are read-only independent SQL oracles in the controlled test host.
// The restart endpoint controls the actual test worker, never product authority.
func hp2PlanOracle(t *testing.T, ctx context.Context, mux *http.ServeMux, admin *pgx.Conn) func() {
	t.Helper()
	role := "hp2_plan_worker_" + strings.ReplaceAll(NewID(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" LOGIN NOSUPERUSER NOBYPASSRLS; GRANT undergrid_worker TO "+pgx.Identifier{role}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	dsn := networkFixtureRoleDSN(t, admin, role)
	var mu sync.Mutex
	var cancel context.CancelFunc
	var stopped chan error
	start := func() {
		var run context.Context
		run, cancel = context.WithCancel(ctx)
		done := make(chan error, 1)
		stopped = done
		go func() { done <- RunWorker(run, dsn, "development") }()
	}
	stop := func() error {
		if cancel == nil {
			return nil
		}
		cancel()
		select {
		case err := <-stopped:
			cancel = nil
			return err
		case <-time.After(12 * time.Second):
			return fmt.Errorf("plan fixture worker did not stop within twelve seconds")
		}
	}
	t.Setenv("UNDERGRID_SOURCES_FILE", "")
	start()
	mux.HandleFunc("/__fixture/plan-checkpoint", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		var raw []byte
		err := admin.QueryRow(r.Context(), `SELECT jsonb_build_object('id',s.id,'active_plan_id',s.active_plan_id,'version',s.version,
   'plans',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',p.id,'version',p.version,'state',p.state,'parent_id',p.parent_id,'content_hash',p.content_hash,'sealed',p.specification,
    'plan_votes',(SELECT count(*) FROM undergrid.approvals WHERE plan_id=p.id AND purpose='plan' AND decision='approve' AND content_hash=p.content_hash),
    'rights_votes',(SELECT count(*) FROM undergrid.approvals WHERE plan_id=p.id AND purpose='rights' AND decision='approve' AND content_hash=p.content_hash),
    'tasks',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',t.id,'state',t.state,'version',t.version,'claim_epoch',t.claim_epoch,'dependency_blocked',t.dependency_blocked,'specification',t.specification,'assigned_agent_id',t.assigned_agent_id,'assigned_principal_id',t.assigned_principal_id,'response_deadline',t.response_deadline,'progress_deadline',t.progress_deadline,'final_deadline',t.final_deadline) ORDER BY t.task_key) FROM undergrid.tasks t WHERE t.plan_id=p.id),'[]'::jsonb)) ORDER BY p.version) FROM undergrid.plan_versions p WHERE p.swarm_id=s.id),'[]'::jsonb))
   FROM undergrid.swarms s ORDER BY s.created_at LIMIT 1`).Scan(&raw)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(raw)
	})
	mux.HandleFunc("/__fixture/restart-plan-worker", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if err := stop(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		restarted := time.Now()
		start()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			var completed bool
			err := admin.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM undergrid_queue.river_job WHERE kind='coordination_deadlines_v1' AND state='completed' AND attempted_at>=$1)", restarted).Scan(&completed)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			if completed {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"restarted":true,"completed_startup_job":true}`))
				return
			}
			select {
			case <-r.Context().Done():
				http.Error(w, "restart request cancelled", 500)
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
		http.Error(w, "restarted worker did not complete its normal startup deadline job", 500)
	})
	return func() {
		mu.Lock()
		defer mu.Unlock()
		if err := stop(); err != nil {
			t.Error(err)
		}
	}
}

func assertHP2PlanFinalState(t *testing.T, ctx context.Context, admin *pgx.Conn, executing bool) {
	t.Helper()
	var oldCancelled, newAccepted, votes, planEvents int
	var valid bool
	if err := admin.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM undergrid.tasks t JOIN undergrid.plan_versions p ON p.id=t.plan_id WHERE p.version=1 AND t.state='cancelled'),
  (SELECT count(*) FROM undergrid.tasks t JOIN undergrid.plan_versions p ON p.id=t.plan_id WHERE p.version=2 AND t.state='accepted'),
  (SELECT count(*) FROM undergrid.approvals a JOIN undergrid.plan_versions p ON p.id=a.plan_id WHERE a.content_hash=p.content_hash AND a.decision='approve'),
  (SELECT count(*) FROM undergrid.events WHERE type='plan.proposed'),
  EXISTS(SELECT 1 FROM undergrid.swarms s JOIN undergrid.plan_versions p2 ON p2.id=s.active_plan_id JOIN undergrid.plan_versions p1 ON p1.id=p2.parent_id WHERE p1.version=1 AND p1.state='superseded' AND p2.version=2 AND p2.state='accepted' AND p1.content_hash<>p2.content_hash AND (p1.specification->'plan')-'parent_version'-'objective'=(p2.specification->'plan')-'parent_version'-'objective')`).Scan(&oldCancelled, &newAccepted, &votes, &planEvents, &valid); err != nil {
		t.Fatal(err)
	}
	expectedAccepted := 2
	if executing {
		expectedAccepted = 1
	}
	if oldCancelled != 2 || newAccepted != expectedAccepted || votes != 12 || planEvents != 2 || !valid {
		t.Fatalf("final immutable revision effects: oldcancelled=%d newaccepted=%d exactvotes=%d proposals=%d validheadlineage=%v", oldCancelled, newAccepted, votes, planEvents, valid)
	}
}
