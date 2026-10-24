SET search_path=undergrid,pg_catalog;

ALTER TABLE opportunities ADD COLUMN external_observation_id uuid REFERENCES external_observations(id);
ALTER TABLE opportunities ADD COLUMN external_intent_revision integer;
ALTER TABLE opportunities ADD COLUMN external_observation_hash text;
ALTER TABLE opportunities ADD COLUMN external_source_fingerprint text;
ALTER TABLE opportunities DROP CONSTRAINT opportunities_kind_check;
ALTER TABLE opportunities ADD CONSTRAINT opportunities_kind_check CHECK(kind IN ('shared_demand','builder_recruitment','external_request','external_capability'));
ALTER TABLE opportunities DROP CONSTRAINT opportunities_typed_candidate;
ALTER TABLE opportunities ADD CONSTRAINT opportunities_typed_candidate CHECK(num_nonnulls(candidate_intent_id,candidate_agent_id,external_observation_id)=1);
ALTER TABLE opportunities DROP CONSTRAINT opportunities_source;
ALTER TABLE opportunities ADD CONSTRAINT opportunities_source CHECK(
 (kind='shared_demand' AND source_intent_id IS NOT NULL AND candidate_intent_id IS NOT NULL AND source_swarm_id IS NULL AND role_key IS NULL AND external_observation_id IS NULL)
 OR (kind='builder_recruitment' AND source_intent_id IS NULL AND source_swarm_id IS NOT NULL AND role_key IS NOT NULL AND candidate_agent_id=recipient_agent_id AND external_observation_id IS NULL)
 OR (kind IN ('external_request','external_capability') AND source_intent_id IS NOT NULL AND source_swarm_id IS NULL AND role_key IS NULL AND external_observation_id IS NOT NULL AND external_intent_revision>0 AND external_observation_hash IS NOT NULL AND external_source_fingerprint IS NOT NULL));

-- A current source projection is the common permission/freshness boundary for
-- scanners and read authorization. The worker can inspect only approved fields
-- in this view; neither source configuration nor private approval bases escape.
CREATE VIEW external_notification_candidates AS
SELECT o.id,o.source_id,o.source_version,o.kind,o.content_hash,
 md5(s.config::text) AS source_fingerprint,
 concat_ws(' ',CASE WHEN s.config->'Fields' ? 'title' THEN o.metadata->>'title' END,
 CASE WHEN s.config->'Fields' ? 'summary' THEN o.metadata->>'summary' END) AS search_text
FROM external_observations o JOIN external_sources s ON s.id=o.source_id
WHERE o.status='active' AND o.checked_at>now()-interval '24 hours' AND s.last_success>now()-interval '24 hours'
 AND jsonb_typeof(s.config->'PublicPermissionNotice')='string' AND length(btrim(s.config->>'PublicPermissionNotice')) BETWEEN 1 AND 2000
 AND jsonb_typeof(s.config->'Publisher')='string' AND length(s.config->>'Publisher') BETWEEN 1 AND 1000
 AND jsonb_typeof(s.config->'Fields')='array' AND s.config->'Fields' ? 'canonical_url';
REVOKE ALL ON external_notification_candidates FROM PUBLIC,undergrid_app;
GRANT SELECT ON external_notification_candidates TO undergrid_worker;

CREATE FUNCTION external_opportunity_current(target uuid) RETURNS boolean
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,undergrid,pg_temp AS $$
 SELECT EXISTS(SELECT 1 FROM undergrid.opportunities o
 JOIN undergrid.intents i ON i.id=o.source_intent_id
 JOIN undergrid.principals p ON p.id=i.principal_id
 JOIN undergrid.agents a ON a.id=i.agent_id
 JOIN undergrid.external_notification_candidates c ON c.id=o.external_observation_id
 WHERE o.id=target AND o.recipient_agent_id=undergrid.actor_agent()
 AND i.principal_id=undergrid.actor_principal() AND i.agent_id=o.recipient_agent_id
 AND o.state='suggested' AND o.kind IN ('external_request','external_capability') AND c.kind=o.kind
 AND i.state='active' AND i.watch_enabled AND i.expires_at>now() AND i.current_revision=o.external_intent_revision
 AND p.status='active' AND a.status='active' AND c.content_hash=o.external_observation_hash
 AND c.source_fingerprint=o.external_source_fingerprint)
$$;
REVOKE ALL ON FUNCTION external_opportunity_current(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION external_opportunity_current(uuid) TO undergrid_app;
