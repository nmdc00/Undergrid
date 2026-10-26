package network

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// This migration fixture represents already accepted 036-era private evidence.
// Only identities/initial intentions use normal HTTP. Historical rows are then
// constructed explicitly under the old schema's restricted role permissions;
// no trigger or migration ledger is changed to fabricate legacy eligibility.
func TestLegacyUnboundPrivateSemanticEvidenceIsIneligibleAfterMigration(t *testing.T) {
	provider, providerCalls := controlledSemanticConsent(t)
	dsn := os.Getenv("UNDERGRID_SEMANTIC_LEGACY_DATABASE_URL")
	if dsn == "" {
		t.Fatal("set dedicated genuine 041-template copy for retained legacy migration proof")
	}
	ctx := context.Background()
	db, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)
	var upgraded bool
	if err = db.QueryRow(ctx, "SELECT EXISTS(SELECT FROM public.undergrid_migrations WHERE name='042_revision_bound_semantic_processing.sql')").Scan(&upgraded); err != nil || upgraded {
		t.Fatal("legacy fixture must start on genuine 041 schema", upgraded, err)
	}
	apiRole := "legacy_api_" + strings.ReplaceAll(NewID(), "-", "")
	workerRole := "legacy_worker_" + strings.ReplaceAll(NewID(), "-", "")
	for role, group := range map[string]string{apiRole: "undergrid_app", workerRole: "undergrid_worker"} {
		identifier := pgx.Identifier{role}.Sanitize()
		if _, err = db.Exec(ctx, "CREATE ROLE "+identifier+" LOGIN NOSUPERUSER NOBYPASSRLS; GRANT "+group+" TO "+identifier); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Exec(context.Background(), "DROP ROLE "+identifier) })
	}
	s := New(networkFixtureRoleDSN(t, db, apiRole), "development", func(r *http.Request) (Owner, error) {
		return Owner{"https://fixture.test", r.Header.Get("X-Test-Owner"), true}, nil
	})
	s.cursorKey = bytes.Repeat([]byte{42}, 32)
	s.termsVersion = "test-v1"
	if err = s.ConfigureCredentialIssuance("network-test-key", bytes.Repeat([]byte{41}, 32)); err != nil {
		t.Fatal(err)
	}
	a, b := client(t, s), client(t, s)
	input := intentFixture()
	code, source := a.call("POST", "/v1/intents", NewID(), input)
	if code != 201 {
		t.Fatal(code, source)
	}
	candidateInput := intentFixture()
	candidateInput.Visibility = "redacted_public"
	candidateInput.PublicSummary = "Different climate agency directory"
	code, candidate := b.call("POST", "/v1/intents", NewID(), candidateInput)
	if code != 201 {
		t.Fatal(code, candidate)
	}
	input.PrivateSemanticQuery = &PrivateSemanticQuery{Projection: "Old explicitly selected private climate query", ProfileHash: provider.hash, AcknowledgeExternalProcessing: true}
	app, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close(ctx)
	tx, err := app.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET LOCAL ROLE undergrid_app;SET LOCAL search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	actor, err := authenticate(ctx, tx, "Bearer "+a.token, "PATCH", "/v1/intents/"+source["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	// Append, never reset, the legacy revision accepted before grant metadata existed.
	document := canonical(input)
	if _, err = tx.Exec(ctx, "INSERT INTO intent_revisions(intent_id,revision,document,public_projection,content_hash) VALUES($1,2,$2,$3,$4)", source["id"], document, canonical(intentPublicProjection(input)), digest(document)); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "UPDATE intents SET current_revision=2,version=2 WHERE id=$1", source["id"]); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	worker, err := pgx.Connect(ctx, networkFixtureRoleDSN(t, db, workerRole))
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close(ctx)
	if _, err = worker.Exec(ctx, "SET ROLE undergrid_worker;SET search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		id       any
		revision int
		text     string
	}{{source["id"], 2, input.PrivateSemanticQuery.Projection}, {candidate["id"], 1, candidateInput.PublicSummary}} {
		_, err = worker.Exec(ctx, `UPDATE intent_revisions SET embedding='[1,0,0]'::public.vector,embedding_profile_hash=$3,embedding_input_hash=$4,embedding_projection_hash=CASE WHEN document->>'visibility'='redacted_public' THEN encode(sha256(convert_to(public_projection::text,'UTF8')),'hex') ELSE $4 END,embedding_provider=$5,embedding_model=$6,embedding_model_revision=$7,embedding_dimensions=3,embedded_at=clock_timestamp() WHERE intent_id=$1 AND revision=$2`, item.id, item.revision, provider.hash, digest([]byte(item.text)), provider.config.ProviderID, provider.config.Model, provider.config.ModelRevision)
		if err != nil {
			t.Fatal("old036 must accept historical query embedding before migration", err)
		}
	}
	var sourceHash, candidateHash, sourceProjection, candidateProjection string
	if err = worker.QueryRow(ctx, "SELECT content_hash,embedding_projection_hash FROM intent_revisions WHERE intent_id=$1 AND revision=2", source["id"]).Scan(&sourceHash, &sourceProjection); err != nil {
		t.Fatal(err)
	}
	if err = worker.QueryRow(ctx, "SELECT p.content_hash,r.embedding_projection_hash FROM public_intents() p JOIN intent_revisions r ON r.intent_id=p.id AND r.revision=1 WHERE p.id=$1", candidate["id"]).Scan(&candidateHash, &candidateProjection); err != nil {
		t.Fatal(err)
	}
	evidence := compareDemandConstraints(input, candidateInput)
	score, threshold := 1.0, 0.99
	evidence.Algorithm = "hard-constraints-hybrid-cosine-v1"
	evidence.Similarity = &score
	evidence.MinimumSimilarity = &threshold
	evidence.SemanticProfileHash = provider.hash
	evidence.SourceProjectionHash = sourceProjection
	evidence.CandidateProjectionHash = candidateProjection
	evidence.Model = provider.config.Model
	evidence.ModelRevision = provider.config.ModelRevision
	oid := NewID()
	hash := demandOpportunityHash(source["id"].(string), sourceHash, candidate["id"].(string), candidateHash, evidence)
	if _, err = worker.Exec(ctx, "INSERT INTO opportunities(id,recipient_agent_id,source_intent_id,candidate_intent_id,kind,input_hash,evidence) VALUES($1,$2,$3,$4,'shared_demand',$5,$6)", oid, actor.AgentID, source["id"], candidate["id"], hash, canonical(evidence)); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	// Completed legacy evidence remains retained; current APIs must decline it.
	code, page := a.call("GET", "/v1/intents/"+source["id"].(string)+"/opportunities", "", nil)
	if code != 200 || len(page["items"].([]any)) != 0 {
		t.Errorf("legacy unbound private evidence survived current read: %d %v", code, page)
	}
	code, out := a.call("GET", "/v1/opportunities/"+oid, "", nil)
	if code != 404 {
		t.Errorf("legacy unbound selected read: %d %v", code, out)
	}
	code, out = a.call("POST", "/v1/opportunities/"+oid+"/dismiss", "legacy-dismiss", map[string]any{})
	if code != 404 {
		t.Errorf("legacy unbound receipt recovered: %d %v", code, out)
	}
	code, out = a.call("GET", "/v1/intents/"+source["id"].(string)+"/compatible-groups", "", nil)
	if code != 200 || len(out["items"].([]any)) != 0 {
		t.Errorf("legacy unbound group survived: %d %v", code, out)
	}
	// Ineligibility for derived semantic processing must not stop valid exact work.
	exact := intentFixture()
	exact.Visibility = "redacted_public"
	exact.PublicSummary = input.Objective
	code, exactCandidate := b.call("POST", "/v1/intents", NewID(), exact)
	if code != 201 {
		t.Fatal(code, exactCandidate)
	}
	runtimeWorker := New(networkFixtureRoleDSN(t, db, workerRole), "development", nil)
	runtimeWorker.semantic = provider
	if err = runtimeWorker.scanIntentCandidate(ctx, worker, source["id"].(string), exactCandidate["id"].(string), true); err != nil {
		t.Fatal(err)
	}
	code, page = a.call("GET", "/v1/intents/"+source["id"].(string)+"/opportunities", "", nil)
	if code != 200 || len(page["items"].([]any)) != 1 {
		t.Fatalf("legacy grant denial disabled valid exact matching: %d %v", code, page)
	}
	exactItem := page["items"].([]any)[0].(map[string]any)
	if exactItem["candidate_intent_id"] != exactCandidate["id"] || exactItem["evidence"].(map[string]any)["algorithm"] != "hard-constraints-exact-v1" {
		t.Fatal("wrong current exact evidence", exactItem)
	}
	if providerCalls.Load() != 0 {
		t.Fatal("legacy unbound query was reprocessed", providerCalls.Load())
	}
	var embeddingRetained, watching bool
	if err = db.QueryRow(ctx, "SELECT r.embedding IS NOT NULL,i.watch_enabled FROM undergrid.intents i JOIN undergrid.intent_revisions r ON r.intent_id=i.id AND r.revision=2 WHERE i.id=$1", source["id"]).Scan(&embeddingRetained, &watching); err != nil {
		t.Fatal(err)
	}
	if !embeddingRetained || !watching {
		t.Fatal("migration must retain data and legitimate active watch", embeddingRetained, watching)
	}
}
