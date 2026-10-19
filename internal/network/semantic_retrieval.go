package network

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

type demandCandidate struct {
	id, hash       string
	projectionHash string
	input          IntentInput
	cosine         *float64
	lexical        float64
}

// Three bounded candidate sets retain exact demand while combining PostgreSQL
// full-text and actual pgvector cosine retrieval. Ranking is evidence, never
// authorization or an override of the independently evaluated constraints.
func (s *Service) semanticCandidates(ctx context.Context, tx pgx.Tx, id, principal, candidateID string, input IntentInput) ([]demandCandidate, error) {
	compatibleIDs, err := compatibleDemandIDs(ctx, tx, principal, candidateID, input)
	if err != nil {
		return nil, err
	}
	if len(compatibleIDs) == 0 {
		return []demandCandidate{}, nil
	}
	rows, err := tx.Query(ctx, `WITH source AS (
 SELECT CASE WHEN r.embedding_profile_hash=$2 THEN r.embedding END AS embedding,
 COALESCE(NULLIF(r.document->>'public_summary',''),r.document->>'objective','') AS query
 FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision WHERE i.id=$1
 ), eligible AS (
 SELECT p.id,p.projection,p.content_hash,r.embedding,r.embedding_profile_hash,r.embedding_projection_hash,r.search_vector FROM public_intents() p
 JOIN intents i ON i.id=p.id JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision
 WHERE p.principal_id<>$3 AND i.expires_at>clock_timestamp() AND ($4='' OR p.id::text=$4) AND p.id=ANY($5::uuid[])
 ), exact_set AS (
 SELECT e.id,0::real AS lexical,NULL::double precision AS cosine FROM eligible e CROSS JOIN source s
 WHERE lower(btrim(e.projection->>'public_summary'))=lower(btrim(s.query)) ORDER BY e.id LIMIT 20
 ), text_set AS (
 SELECT e.id,ts_rank_cd(e.search_vector,plainto_tsquery('simple',s.query)) AS lexical,
 CASE WHEN s.embedding IS NOT NULL AND e.embedding IS NOT NULL AND e.embedding_profile_hash=$2 THEN 1-(s.embedding OPERATOR(public.<=>) e.embedding) END AS cosine
 FROM eligible e CROSS JOIN source s WHERE e.search_vector @@ plainto_tsquery('simple',s.query)
 ORDER BY lexical DESC,e.id LIMIT 20
 ), vector_set AS (
 SELECT e.id,0::real AS lexical,1-(s.embedding OPERATOR(public.<=>) e.embedding) AS cosine
 FROM eligible e CROSS JOIN source s WHERE s.embedding IS NOT NULL AND e.embedding IS NOT NULL AND e.embedding_profile_hash=$2
 ORDER BY s.embedding OPERATOR(public.<=>) e.embedding,e.id LIMIT 20
 ), ranked AS (
 SELECT id,max(lexical) AS lexical,max(cosine) AS cosine FROM (SELECT * FROM exact_set UNION ALL SELECT * FROM text_set UNION ALL SELECT * FROM vector_set) all_candidates GROUP BY id
 ) SELECT e.id::text,e.projection,e.content_hash,r.cosine,r.lexical,COALESCE(e.embedding_projection_hash,'') FROM ranked r JOIN eligible e ON e.id=r.id
 ORDER BY (COALESCE(r.cosine,0)*0.9+r.lexical/(1+r.lexical)*0.1) DESC,e.id LIMIT 60`, id, s.semantic.hash, principal, candidateID, compatibleIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []demandCandidate{}
	for rows.Next() {
		var c demandCandidate
		var raw []byte
		if err = rows.Scan(&c.id, &raw, &c.hash, &c.cosine, &c.lexical, &c.projectionHash); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &c.input); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// Apply the shared domain policy before any retrieval limit. The scan streams
// public projections and retains IDs only; no second SQL implementation of hard
// constraint semantics can drift from exact matching. This baseline scans the
// eligible public corpus; large-cohort load qualification remains a release gate.
func compatibleDemandIDs(ctx context.Context, tx pgx.Tx, principal, candidateID string, input IntentInput) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT p.id::text,p.projection FROM public_intents() p
 JOIN intents i ON i.id=p.id WHERE p.principal_id<>$1 AND i.expires_at>clock_timestamp()
 AND ($2='' OR p.id::text=$2)`, principal, candidateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var candidate IntentInput
		if err := json.Unmarshal(raw, &candidate); err != nil {
			return nil, err
		}
		if compareDemandConstraints(input, candidate).Compatible {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}
