package network

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSemanticEmbeddingCannotBeInsertedByApplicationRole(t *testing.T) {
	s, db, _ := semanticFixture(t)
	a := client(t, s)
	input := intentFixture()
	input.Visibility = "redacted_public"
	input.PublicSummary = "Application cannot forge embeddings " + NewID()
	code, created := a.call("POST", "/v1/intents", "embedding-insert-boundary", input)
	if code != 201 {
		t.Fatal(code, created)
	}
	id := created["id"].(string)
	t.Cleanup(func() {
		a.versioned("PATCH", "/v1/intents/"+id, "finish-privilege-fixture", created["etag"].(string), map[string]any{"state": "withdrawn"})
	})
	ctx := context.Background()
	var principal, agent string
	if err := db.QueryRow(ctx, "SELECT principal_id::text,agent_id::text FROM undergrid.intents WHERE id=$1", id).Scan(&principal, &agent); err != nil {
		t.Fatal(err)
	}
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
	if _, err = tx.Exec(ctx, "SELECT set_config('undergrid.principal_id',$1,true),set_config('undergrid.agent_id',$2,true)", principal, agent); err != nil {
		t.Fatal(err)
	}
	// All values are well-formed and the revision belongs to this principal.
	// The application must still lack permission to mint provider evidence.
	_, err = tx.Exec(ctx, `INSERT INTO undergrid.intent_revisions(intent_id,revision,document,public_projection,content_hash,
 embedding,embedding_profile_hash,embedding_input_hash,embedding_projection_hash,embedding_provider,embedding_model,embedding_model_revision,embedding_dimensions,embedded_at)
 SELECT intent_id,2,document,public_projection,content_hash,'[1,0,0]'::public.vector,repeat('a',64),
 encode(sha256(convert_to(public_projection->>'public_summary','UTF8')),'hex'),encode(sha256(convert_to(public_projection::text,'UTF8')),'hex'),
 'forged-provider','forged-model','forged-revision',3,clock_timestamp()
 FROM undergrid.intent_revisions WHERE intent_id=$1 AND revision=1`, id)
	var denied *pgconn.PgError
	if !errors.As(err, &denied) || denied.Code != "42501" {
		t.Fatalf("application could insert provider evidence: %v", err)
	}
}
