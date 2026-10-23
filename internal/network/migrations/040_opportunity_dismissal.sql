SET search_path=undergrid,pg_catalog;

-- Only the recipient's state can change. Immutable identity/evidence remains
-- worker-owned; application SQL cannot reopen or fabricate a suggestion.
GRANT UPDATE(state) ON opportunities TO undergrid_app;
CREATE POLICY opportunities_dismiss ON opportunities FOR UPDATE TO undergrid_app
 USING(recipient_agent_id=actor_agent() AND state IN ('suggested','dismissed'))
 WITH CHECK(recipient_agent_id=actor_agent() AND state='dismissed');

-- Lock current eligibility without granting access to private foreign rows.
-- NOWAIT avoids acquiring a foreign principal behind an already-held recipient
-- lock in the opposite order. Contention rolls the command back for retry.
CREATE FUNCTION opportunity_dismissal_current(target uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,undergrid,pg_temp AS $$
DECLARE o undergrid.opportunities%ROWTYPE;
BEGIN
 SELECT * INTO o FROM undergrid.opportunities WHERE id=target
  AND recipient_agent_id=undergrid.actor_agent() AND state IN ('suggested','dismissed');
 IF NOT FOUND THEN RETURN false; END IF;
 IF o.source_intent_id IS NOT NULL THEN
  PERFORM 1 FROM undergrid.intents i JOIN undergrid.principals p ON p.id=i.principal_id
   JOIN undergrid.agents a ON a.id=i.agent_id
   WHERE i.id=o.source_intent_id AND i.agent_id=o.recipient_agent_id
   AND i.principal_id=undergrid.actor_principal() AND i.state='active'
   AND i.watch_enabled AND i.expires_at>clock_timestamp()
   AND p.status='active' AND a.status='active' FOR SHARE OF p,a,i NOWAIT;
  IF NOT FOUND THEN RETURN false; END IF;
 END IF;
 IF o.kind='shared_demand' THEN
  PERFORM 1 FROM undergrid.intents i JOIN undergrid.principals p ON p.id=i.principal_id
   JOIN undergrid.agents a ON a.id=i.agent_id
   WHERE i.id=o.candidate_intent_id AND i.visibility='redacted_public'
   AND i.state='active' AND i.expires_at>clock_timestamp()
   AND p.status='active' AND a.status='active'
   AND p.id<>undergrid.actor_principal() FOR SHARE OF p,a,i NOWAIT;
  RETURN FOUND;
 ELSIF o.kind IN ('external_request','external_capability') THEN
  PERFORM 1 FROM undergrid.external_observations e JOIN undergrid.external_sources s ON s.id=e.source_id
   JOIN undergrid.external_notification_candidates c ON c.id=e.id
   JOIN undergrid.intents i ON i.id=o.source_intent_id
   WHERE e.id=o.external_observation_id AND c.kind=o.kind
   AND i.current_revision=o.external_intent_revision
   AND c.content_hash=o.external_observation_hash
   AND c.source_version=o.evidence->>'source_version'
   AND c.source_fingerprint=o.external_source_fingerprint
   FOR SHARE OF e,s NOWAIT;
  RETURN FOUND;
 ELSIF o.kind='builder_recruitment' THEN
  PERFORM 1 FROM undergrid.swarms s JOIN undergrid.agents a ON a.id=s.coordinator_agent_id
   JOIN undergrid.principals p ON p.id=a.principal_id
   WHERE s.id=o.source_swarm_id AND s.state='active' AND s.visibility<>'private'
   AND a.status='active' AND p.status='active' FOR SHARE OF p,a,s NOWAIT;
  RETURN FOUND;
 END IF;
 RETURN false;
END;
$$;
REVOKE ALL ON FUNCTION opportunity_dismissal_current(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION opportunity_dismissal_current(uuid) TO undergrid_app;
