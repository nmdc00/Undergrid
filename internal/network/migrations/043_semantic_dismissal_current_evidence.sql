SET search_path=undergrid,pg_catalog;

-- Narrow current-evidence check under the same source/candidate authority locks
-- as ordinary dismissal. No foreign private document or vector is returned.
CREATE FUNCTION semantic_opportunity_dismissal_current(target uuid, current_profile text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,undergrid,pg_temp AS $$
BEGIN
 IF NOT undergrid.opportunity_dismissal_current(target) THEN RETURN false; END IF;
 RETURN EXISTS(
  SELECT 1 FROM undergrid.opportunities o
  JOIN undergrid.intents si ON si.id=o.source_intent_id
  JOIN undergrid.intent_revisions sr ON sr.intent_id=si.id AND sr.revision=si.current_revision
  JOIN undergrid.public_intents() p ON p.id=o.candidate_intent_id
  JOIN undergrid.intents ci ON ci.id=p.id
  JOIN undergrid.intent_revisions cr ON cr.intent_id=ci.id AND cr.revision=ci.current_revision
  WHERE o.id=target AND o.recipient_agent_id=undergrid.actor_agent()
  AND o.kind='shared_demand' AND o.state IN ('suggested','dismissed')
  AND o.evidence->>'algorithm'='hard-constraints-hybrid-cosine-v1'
  AND o.evidence->>'semantic_profile_hash'=current_profile
  AND sr.embedding_profile_hash=current_profile AND cr.embedding_profile_hash=current_profile
  AND sr.embedding IS NOT NULL AND cr.embedding IS NOT NULL
  AND o.evidence->>'source_projection_hash'=sr.embedding_projection_hash
  AND o.evidence->>'candidate_projection_hash'=cr.embedding_projection_hash
  AND (si.visibility='redacted_public' OR (
   sr.document#>>'{semantic_processing_grant,revision}'=sr.revision::text
   AND sr.document#>>'{semantic_processing_grant,profile_hash}'=current_profile
   AND sr.document#>>'{semantic_processing_grant,projection_hash}'=sr.embedding_projection_hash))
  AND 1-(sr.embedding OPERATOR(public.<=>) cr.embedding)>=(o.evidence->>'minimum_similarity')::double precision
  AND o.input_hash=encode(sha256(convert_to(array_to_json(ARRAY[
   si.id::text,sr.content_hash,ci.id::text,p.content_hash,
   'hard-constraints-hybrid-cosine-v1',current_profile])::text,'UTF8')),'hex')
 );
END;
$$;
REVOKE ALL ON FUNCTION semantic_opportunity_dismissal_current(uuid,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION semantic_opportunity_dismissal_current(uuid,text) TO undergrid_app;
