-- Only current approved public revisions cross this privilege boundary. Never
-- expose vectors, private revisions or participant-selected thresholds.
SET search_path=undergrid,pg_catalog,pg_temp;
CREATE FUNCTION public_semantic_pair_scores(candidate_ids uuid[], profile text)
 RETURNS TABLE(left_id uuid,right_id uuid,similarity double precision,left_projection_hash text,right_projection_hash text)
 LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog,pg_temp AS $$
 WITH eligible AS MATERIALIZED (
 SELECT i.id,r.embedding,r.embedding_projection_hash FROM intents i
 JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision
 JOIN principals p ON p.id=i.principal_id JOIN agents a ON a.id=i.agent_id
 WHERE cardinality($1)<=20 AND $2 ~ '^[a-f0-9]{64}$'
 AND i.id=ANY($1) AND i.visibility='redacted_public'
 AND i.state='active' AND i.expires_at>clock_timestamp()
 AND p.status='active' AND a.status='active'
 AND r.embedding IS NOT NULL AND r.embedding_profile_hash=$2
 ) SELECT l.id,r.id,1-(l.embedding OPERATOR(public.<=>) r.embedding),l.embedding_projection_hash,r.embedding_projection_hash
 FROM eligible l JOIN eligible r ON l.id<r.id
 $$;
REVOKE ALL ON FUNCTION public_semantic_pair_scores(uuid[],text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public_semantic_pair_scores(uuid[],text) TO undergrid_app,undergrid_worker;
