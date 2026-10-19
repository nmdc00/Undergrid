package network

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Capture only an approved public or explicitly consented private query. Provider
// I/O finishes before the principal→agent→intention authority transaction starts.
func (s *Service) ensureSemanticEmbedding(ctx context.Context, db *pgx.Conn, id string) error {
	if s.semantic == nil {
		return nil
	}
	var principal, agent, projection string
	var raw []byte
	var revision int
	err := db.QueryRow(ctx, `SELECT i.principal_id::text,i.agent_id::text,i.current_revision,r.public_projection::text,r.document
 FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision
 JOIN principals p ON p.id=i.principal_id JOIN agents a ON a.id=i.agent_id
 WHERE i.id=$1 AND i.state='active' AND p.status='active' AND a.status='active'
 AND i.expires_at>clock_timestamp() AND r.embedding IS NULL`, id).Scan(&principal, &agent, &revision, &projection, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var input IntentInput
	if err = json.Unmarshal(raw, &input); err != nil {
		return err
	}
	text, projectionHash, eligible := s.semantic.selectedProjection(input, projection)
	if !eligible {
		return nil
	}
	current, err := s.privateConsentCurrent(input)
	if err != nil || !current {
		return err
	}
	vector, err := s.semantic.embed(ctx, text)
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
	var currentProjection string
	var currentRaw []byte
	var expires time.Time
	if err = tx.QueryRow(ctx, `SELECT i.state,i.current_revision,i.expires_at,r.public_projection::text,r.document FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1 FOR UPDATE OF i`, id).Scan(&state, &currentRevision, &expires, &currentProjection, &currentRaw); err != nil {
		return err
	}
	if state != "active" || !expires.After(time.Now()) || revision != currentRevision || projection != currentProjection {
		return nil
	}
	var currentInput IntentInput
	if err = json.Unmarshal(currentRaw, &currentInput); err != nil {
		return err
	}
	currentText, currentHash, currentEligible := s.semantic.selectedProjection(currentInput, currentProjection)
	if !currentEligible || currentText != text || currentHash != projectionHash {
		return nil
	}
	current, err = s.privateConsentCurrent(currentInput)
	if err != nil || !current {
		return err
	}
	p := s.semantic
	tag, err := tx.Exec(ctx, `UPDATE intent_revisions SET embedding=$3::public.vector,embedding_profile_hash=$4,embedding_input_hash=$5,embedding_projection_hash=$6,embedding_provider=$7,embedding_model=$8,embedding_model_revision=$9,embedding_dimensions=$10,embedded_at=clock_timestamp()
 WHERE intent_id=$1 AND revision=$2 AND embedding IS NULL`, id, revision, string(canonical(vector)), p.hash, digest([]byte(text)), projectionHash, p.config.ProviderID, p.config.Model, p.config.ModelRevision, p.config.Dimensions)
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
