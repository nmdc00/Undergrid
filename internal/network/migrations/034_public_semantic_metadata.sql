-- Public matching metadata must cross RLS through a narrow projection, never a
-- direct API join to another principal's private immutable revision.
SET search_path=undergrid,pg_catalog,pg_temp;
CREATE FUNCTION public_semantic_metadata() RETURNS TABLE(id uuid,profile_hash text,projection_hash text)
 LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog,pg_temp AS $$
 SELECT i.id,r.embedding_profile_hash,r.embedding_projection_hash
 FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision
 JOIN principals p ON p.id=i.principal_id JOIN agents a ON a.id=i.agent_id
 WHERE i.visibility='redacted_public' AND i.state='active' AND i.expires_at>clock_timestamp()
 AND p.status='active' AND a.status='active' AND r.embedding IS NOT NULL
 $$;
REVOKE ALL ON FUNCTION public_semantic_metadata() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public_semantic_metadata() TO undergrid_app,undergrid_worker;
