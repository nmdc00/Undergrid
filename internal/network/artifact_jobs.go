package network

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"undergrid/internal/verification"
)

type artifactEvaluationArgs struct {
	ArtifactID   string `json:"artifact_id"`
	Generation   string `json:"generation"`
	ManifestHash string `json:"manifest_hash"`
}

func (artifactEvaluationArgs) Kind() string { return "artifact_evaluation_v1" }

type artifactEvaluationWorker struct {
	river.WorkerDefaults[artifactEvaluationArgs]
	service *Service
}

func (w *artifactEvaluationWorker) Work(ctx context.Context, job *river.Job[artifactEvaluationArgs]) error {
	return w.service.evaluateArtifact(ctx, job.Args)
}
func enqueueArtifactEvaluation(ctx context.Context, tx pgx.Tx, id, generation, manifest string) error {
	client, err := river.NewClient(riverpgxv5.New(nil), &river.Config{Schema: "undergrid_queue"})
	if err != nil {
		return err
	}
	_, err = client.InsertTx(ctx, tx, artifactEvaluationArgs{id, generation, manifest}, &river.InsertOpts{MaxAttempts: 10, Queue: "artifact_verification"})
	return err
}
func (s *Service) evaluateArtifact(ctx context.Context, args artifactEvaluationArgs) error {
	if s.artifacts == nil {
		return errors.New("trusted artifact runtime unavailable")
	}
	db, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	if _, err = db.Exec(ctx, "SET ROLE undergrid_worker; SET search_path=undergrid,pg_catalog"); err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	art, err := loadArtifact(ctx, tx, args.ArtifactID, false)
	if err != nil {
		return err
	}
	key := digest(canonical(args))
	var existing bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM evaluations WHERE artifact_id=$1 AND request_key=$2)", art.id, key).Scan(&existing); err != nil {
		return err
	}
	if existing {
		return nil
	}
	if art.generation != args.Generation || art.manifestHash != args.ManifestHash || art.state != "sealed" {
		return nil
	}
	// No DB locks or transaction spans execution of untrusted parsing.
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	data, readErr := s.artifactBytes(art)
	evidence := verification.Evidence{Result: "error", Code: "artifact_content_unavailable", InputSHA256: art.sha, ManifestSHA256: art.manifestHash, SpecSHA256: digest([]byte(verification.Spec)), VerifierSHA256: s.artifacts.config.VerifierSHA256, ImageSHA256: s.artifacts.config.ImageSHA256, RuntimeIsolation: "not_run_content_unavailable", ResourceLimitsEvidence: "not_applied", ImageIdentityEvidence: "operator_configured_digest_attestation_not_runtime_attestation", CheckedAt: time.Now().UTC()}
	if readErr == nil {
		evidence, err = s.artifacts.verify(ctx, verification.Job{Data: data, Manifest: art.manifest, InputSHA256: art.sha, ManifestSHA256: art.manifestHash})
		if err != nil {
			return err
		}
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	current, err := loadArtifact(ctx, tx, art.id, true)
	if err != nil {
		return err
	}
	if current.state != "sealed" || current.generation != args.Generation || current.manifestHash != args.ManifestHash {
		return nil
	}
	id := NewID()
	tag, err := tx.Exec(ctx, `INSERT INTO evaluations(id,artifact_id,request_key,generation,evidence,result,input_sha256,manifest_sha256,spec_sha256,verifier_sha256,image_sha256,runtime_isolation,bubblewrap_sha256,prlimit_sha256,resource_limits_evidence,image_identity_evidence) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NULLIF($13,''),NULLIF($14,''),$15,$16) ON CONFLICT DO NOTHING`, id, art.id, key, art.generation, canonical(evidence), evidence.Result, evidence.InputSHA256, evidence.ManifestSHA256, evidence.SpecSHA256, evidence.VerifierSHA256, evidence.ImageSHA256, evidence.RuntimeIsolation, evidence.BubblewrapSHA256, evidence.PrlimitSHA256, evidence.ResourceLimitsEvidence, evidence.ImageIdentityEvidence)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}
	state := "failed"
	if evidence.Result == "passed" {
		state = "verified"
	}
	if _, err = tx.Exec(ctx, "UPDATE artifacts SET state=$2,version=version+1 WHERE id=$1", art.id, state); err != nil {
		return err
	}
	origin := "registered"
	if s.environment != "production" {
		origin = "release_test"
	}
	event := NewID()
	facts := map[string]any{"swarm_id": art.swarm, "artifact_id": art.id, "evaluation_id": id, "result": evidence.Result, "resource_path": "/v1/artifacts/" + art.id}
	if current.purpose == "progress" {
		facts["audience_principal_ids"] = current.audience
	}
	if _, err = tx.Exec(ctx, "INSERT INTO events(id,aggregate_kind,aggregate_id,version,principal_id,agent_id,type,facts,origin) VALUES($1,'swarm',$2,$3,$4,$5,'artifact.evaluated',$6,$7)", event, art.id, current.version+1, art.principal, art.agent, canonical(facts), origin); err != nil {
		return err
	}
	if err = enqueueFanout(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
