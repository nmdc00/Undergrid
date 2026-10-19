package network

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Capture only a public, approved projection in an autocommitted read. Provider
// I/O finishes before the principal→agent→intention authority transaction starts.
func (s *Service) ensureSemanticEmbedding(ctx context.Context, db *pgx.Conn, id string) error {
	if s.semantic == nil || !s.semantic.config.AllowPublicProjections {
		return nil
	}
	var principal, agent, projection string
	var revision int
	err := db.QueryRow(ctx, `SELECT p.principal_id::text,p.agent_id::text,i.current_revision,r.public_projection::text
 FROM public_intents() p JOIN intents i ON i.id=p.id JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision
 WHERE i.id=$1 AND i.expires_at>clock_timestamp() AND r.embedding IS NULL`, id).Scan(&principal, &agent, &revision, &projection)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var approved struct {
		Summary string `json:"public_summary"`
	}
	if err = json.Unmarshal([]byte(projection), &approved); err != nil {
		return err
	}
	vector, err := s.semantic.embed(ctx, approved.Summary)
	if err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var state string
	if err = tx.QueryRow(ctx, "SELECT status FROM principals WHERE id=$1 FOR UPDATE", principal).Scan(&state); err != nil {
		return err
	}
	if state != "active" {
		return nil
	}
	if err = tx.QueryRow(ctx, "SELECT status FROM agents WHERE id=$1 FOR UPDATE", agent).Scan(&state); err != nil {
		return err
	}
	if state != "active" {
		return nil
	}
	var currentRevision int
	var currentProjection, visibility string
	var expires time.Time
	if err = tx.QueryRow(ctx, `SELECT i.state,i.current_revision,i.visibility,i.expires_at,r.public_projection::text FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1 FOR UPDATE OF i`, id).Scan(&state, &currentRevision, &visibility, &expires, &currentProjection); err != nil {
		return err
	}
	if state != "active" || visibility != "redacted_public" || !expires.After(time.Now()) || revision != currentRevision || projection != currentProjection {
		return nil
	}
	p := s.semantic
	tag, err := tx.Exec(ctx, `UPDATE intent_revisions SET embedding=$3::public.vector,embedding_profile_hash=$4,embedding_input_hash=$5,embedding_projection_hash=$6,embedding_provider=$7,embedding_model=$8,embedding_model_revision=$9,embedding_dimensions=$10,embedded_at=clock_timestamp()
 WHERE intent_id=$1 AND revision=$2 AND embedding IS NULL`, id, revision, string(canonical(vector)), p.hash, digest([]byte(approved.Summary)), digest([]byte(projection)), p.config.ProviderID, p.config.Model, p.config.ModelRevision, p.config.Dimensions)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		if err = enqueueScan(ctx, tx, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
