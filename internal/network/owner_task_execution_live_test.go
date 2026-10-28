//go:build hp1_live && hp2_live

package network

import (
	"context"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestHP3LiveOwnerTaskExecutionPreservesClocksAndRecovers(t *testing.T) {
	hp2LiveOwnerSetup(t, hp3TaskExecution)
}
func assertOwnerExecutionBrowserState(t *testing.T, ctx context.Context, db *pgx.Conn) {
	t.Helper()
	var executing, events, artifacts, evidence int
	if err := db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM undergrid.tasks t JOIN undergrid.plan_versions p ON p.id=t.plan_id JOIN undergrid.agents a ON a.id=t.assigned_agent_id JOIN undergrid.principals owner ON owner.id=a.principal_id WHERE t.task_key='collect' AND p.version=2 AND p.state='accepted' AND t.state='in_progress' AND t.version=3 AND t.claim_epoch=1 AND owner.subject='hp2-controlled-builder-c'),
 (SELECT count(*) FROM undergrid.events WHERE type='task.progress_recorded'),
 (SELECT count(*) FROM undergrid.artifacts),(SELECT count(*) FROM undergrid.task_progress_evidence)`).Scan(&executing, &events, &artifacts, &evidence); err != nil {
		t.Fatal(err)
	}
	if executing != 1 || events != 1 || artifacts != 0 || evidence != 0 {
		t.Fatal("owner execution effects", executing, events, artifacts, evidence)
	}
}
