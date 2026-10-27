SET search_path=undergrid,pg_catalog;

-- Creation and retained receipt recovery need the same current provenance.
-- All locks are NOWAIT because the command already holds recipient authority;
-- foreign contention aborts the entire transaction rather than forming a cycle.
-- Only eligibility leaves this definer boundary, never a foreign private row.
CREATE FUNCTION room_origin_authority(target uuid,current_profile text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,undergrid,pg_temp AS $$
DECLARE o undergrid.opportunities%ROWTYPE; linked_agents uuid[]; linked_intents uuid[];
BEGIN
 SELECT * INTO o FROM undergrid.opportunities WHERE id=target
  AND recipient_agent_id=undergrid.actor_agent() AND state='suggested' FOR SHARE NOWAIT;
 IF NOT FOUND THEN RETURN false; END IF;
 linked_intents:=array_remove(ARRAY[o.source_intent_id,o.candidate_intent_id],NULL);
 SELECT array_agg(DISTINCT id ORDER BY id) INTO linked_agents FROM (
  SELECT o.recipient_agent_id AS id
  UNION SELECT i.agent_id FROM undergrid.intents i WHERE i.id=ANY(linked_intents)
  UNION SELECT s.coordinator_agent_id FROM undergrid.swarms s WHERE s.id=o.source_swarm_id
 ) a;
 -- Consistent order within each authority class; no lock error is swallowed.
 PERFORM 1 FROM undergrid.principals p WHERE p.id IN
  (SELECT a.principal_id FROM undergrid.agents a WHERE a.id=ANY(linked_agents))
  ORDER BY p.id FOR SHARE NOWAIT;
 PERFORM 1 FROM undergrid.agents a WHERE a.id=ANY(linked_agents) ORDER BY a.id FOR SHARE NOWAIT;
 PERFORM 1 FROM undergrid.intents i WHERE i.id=ANY(linked_intents) ORDER BY i.id FOR SHARE NOWAIT;
 PERFORM 1 FROM undergrid.intent_revisions r JOIN undergrid.intents i
  ON i.id=r.intent_id AND i.current_revision=r.revision
  WHERE i.id=ANY(linked_intents) ORDER BY r.intent_id,r.revision FOR SHARE OF r NOWAIT;
 IF EXISTS(SELECT 1 FROM undergrid.agents a JOIN undergrid.principals p ON p.id=a.principal_id
  WHERE a.id=ANY(linked_agents) AND (a.status<>'active' OR p.status<>'active')) THEN RETURN false; END IF;
 IF NOT EXISTS(SELECT 1 FROM undergrid.agents a WHERE a.id=o.recipient_agent_id
  AND a.principal_id=undergrid.actor_principal() AND a.status='active') THEN RETURN false; END IF;
 IF o.source_intent_id IS NOT NULL THEN
  IF NOT EXISTS(SELECT 1 FROM undergrid.intents i WHERE i.id=o.source_intent_id
   AND i.agent_id=o.recipient_agent_id AND i.principal_id=undergrid.actor_principal()
   AND i.state='active' AND i.expires_at>clock_timestamp()
   AND (o.kind='shared_demand' OR i.watch_enabled)) THEN RETURN false; END IF;
 END IF;
 IF o.kind='shared_demand' THEN
  IF NOT EXISTS(SELECT 1 FROM undergrid.intents i WHERE i.id=o.candidate_intent_id
   AND i.visibility='redacted_public' AND i.state='active' AND i.expires_at>clock_timestamp()
   AND i.principal_id<>undergrid.actor_principal()) THEN RETURN false; END IF;
  IF o.evidence->>'algorithm'='hard-constraints-hybrid-cosine-v1' THEN
   RETURN EXISTS(SELECT 1 FROM undergrid.intents si
    JOIN undergrid.intent_revisions sr ON sr.intent_id=si.id AND sr.revision=si.current_revision
    JOIN undergrid.intents ci ON ci.id=o.candidate_intent_id
    JOIN undergrid.intent_revisions cr ON cr.intent_id=ci.id AND cr.revision=ci.current_revision
    WHERE si.id=o.source_intent_id AND current_profile<>''
    AND o.evidence->>'semantic_profile_hash'=current_profile
    AND sr.embedding_profile_hash=current_profile AND cr.embedding_profile_hash=current_profile
    AND sr.embedding IS NOT NULL AND cr.embedding IS NOT NULL
    AND o.evidence->>'source_projection_hash'=sr.embedding_projection_hash
    AND o.evidence->>'candidate_projection_hash'=cr.embedding_projection_hash
    AND (si.visibility='redacted_public' OR (
     si.visibility IN ('private','restricted')
     AND sr.document#>>'{semantic_processing_grant,revision}'=sr.revision::text
     AND sr.document#>>'{semantic_processing_grant,profile_hash}'=current_profile
     AND sr.document#>>'{semantic_processing_grant,projection_hash}'=sr.embedding_projection_hash
     AND sr.document#>>'{private_semantic_query,profile_hash}'=current_profile
     AND sr.document#>>'{private_semantic_query,acknowledge_external_processing}'='true'
     AND sr.embedding_projection_hash=encode(sha256(convert_to(sr.document#>>'{private_semantic_query,projection}','UTF8')),'hex')))
    AND 1-(sr.embedding OPERATOR(public.<=>) cr.embedding)>=(o.evidence->>'minimum_similarity')::double precision);
  END IF;
  RETURN o.evidence->>'algorithm'='hard-constraints-exact-v1';
 ELSIF o.kind IN ('external_request','external_capability') THEN
  PERFORM 1 FROM undergrid.external_sources s JOIN undergrid.external_observations e ON e.source_id=s.id
   WHERE e.id=o.external_observation_id FOR SHARE OF s NOWAIT;
  PERFORM 1 FROM undergrid.external_observations e WHERE e.id=o.external_observation_id FOR SHARE NOWAIT;
  RETURN undergrid.external_opportunity_current(target) AND EXISTS(
   SELECT 1 FROM undergrid.external_notification_candidates c JOIN undergrid.intents i ON i.id=o.source_intent_id
   WHERE c.id=o.external_observation_id AND c.kind=o.kind AND i.current_revision=o.external_intent_revision
   AND c.content_hash=o.external_observation_hash AND c.source_fingerprint=o.external_source_fingerprint
   AND c.source_version=o.evidence->>'source_version');
 ELSIF o.kind='builder_recruitment' THEN
  PERFORM 1 FROM undergrid.swarms s WHERE s.id=o.source_swarm_id AND s.state='active'
   AND s.visibility<>'private' FOR SHARE NOWAIT;
  RETURN FOUND;
 END IF;
 RETURN false;
END;
$$;
REVOKE ALL ON FUNCTION room_origin_authority(uuid,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION room_origin_authority(uuid,text) TO undergrid_app;
