//go:build hp1_live && hp2_live

package network

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestHP3LiveOwnerContributionsPreserveAuthorityAndRecover(t *testing.T) {
	hp2LiveOwnerSetup(t, hp3OwnerContributions)
}

// Only the pinned repository verifier is executed. Uploaded owner bytes remain
// inert input. Development process evidence is explicitly not host qualification.
func configureOwnerContributionBrowserRuntime(t *testing.T, service *Service, origin string) {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "undergrid-hp3-contribution-runtime-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Retained owner contribution runtime: %s", root)
	bin := filepath.Join(root, "fixed-verifier")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", bin, "../../cmd/undergrid-verifier")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build pinned verifier: %v %s", err, out)
	}
	binary, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	blobs := filepath.Join(root, "blobs")
	if err = os.Mkdir(blobs, 0700); err != nil {
		t.Fatal(err)
	}
	config := LocalArtifactConfig{Root: blobs, Origin: origin, VerifierPath: bin, VerifierSHA256: digest(binary), ImageSHA256: strings.Repeat("b", 64)}
	if err = service.SetLocalArtifactRuntime(config); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "local-fixture-runtime.json")
	if err = os.WriteFile(path, canonical(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERGRID_LOCAL_ARTIFACT_CONFIG_FILE", path)
	t.Setenv("UNDERGRID_ISOLATED_VERIFIER_CONFIG_FILE", "")
}

func assertOwnerContributionBrowserState(t *testing.T, ctx context.Context, db *pgx.Conn) {
	t.Helper()
	var verified, ownerModes, progress, output, evaluations, consent, reports, submitted, activity int
	var parity bool
	if err := db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM undergrid.artifacts WHERE state='verified'),
 (SELECT count(*) FROM undergrid.artifacts WHERE upload_authentication='owner_session' AND upload_credential_id IS NULL),
 (SELECT count(*) FROM undergrid.artifacts WHERE purpose='progress' AND cardinality(audience_principal_ids)=2 AND progress_reviewer_principal_id=ANY(audience_principal_ids)),
 (SELECT count(*) FROM undergrid.artifacts WHERE purpose='output'),
 (SELECT count(*) FROM undergrid.evaluations WHERE result='passed' AND runtime_isolation='development_process_not_qualified'),
 (SELECT count(*) FROM undergrid.approvals WHERE purpose IN ('acceptance','publication')),
 (SELECT count(*) FROM undergrid.task_progress_evidence),
 (SELECT count(*) FROM undergrid.tasks t JOIN undergrid.plan_versions p ON p.id=t.plan_id WHERE p.version=2 AND t.task_key='collect' AND t.state='submitted' AND t.version=4 AND t.claim_epoch=1 AND t.credited_progress_evidence_id IS NULL AND EXISTS(SELECT 1 FROM undergrid.artifacts a WHERE a.id=t.submitted_artifact_id AND a.purpose='output')),
 (SELECT count(*) FROM undergrid.events WHERE type='task.progress_recorded'),
 NOT EXISTS(SELECT 1 FROM undergrid.evaluations e JOIN undergrid.artifacts a ON a.id=e.artifact_id WHERE e.input_sha256<>a.sha256 OR e.manifest_sha256<>a.manifest_hash OR e.evidence->>'input_sha256'<>e.input_sha256 OR e.evidence->>'manifest_sha256'<>e.manifest_sha256 OR e.evidence->>'runtime_isolation'<>e.runtime_isolation)`).Scan(&verified, &ownerModes, &progress, &output, &evaluations, &consent, &reports, &submitted, &activity, &parity); err != nil {
		t.Fatal(err)
	}
	if verified != 2 || ownerModes != 2 || progress != 1 || output != 1 || evaluations != 2 || consent != 0 || reports != 0 || submitted != 1 || activity != 1 || !parity {
		t.Fatal("owner contribution normalized effects", verified, ownerModes, progress, output, evaluations, consent, reports, submitted, activity, parity)
	}
}
