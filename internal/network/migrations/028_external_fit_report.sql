SET search_path=undergrid,pg_catalog;

-- Reuse026's current public permission/freshness projection, including its
-- actor checks, without widening that published closed response. The indexed
-- predecessor positions its bounded keyset query immediately before the chosen
-- UUID; selecting an item beyond the first51 remains correct and bounded.
CREATE FUNCTION external_fit_source(target uuid, selected_lead uuid)
RETURNS TABLE(id uuid,source_id uuid,source_version text,origin text,canonical_url text,publisher text,permission_notice text,metadata jsonb,observed_at timestamptz,checked_at timestamptz,capability_claim jsonb)
LANGUAGE sql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT l.*,CASE WHEN l.origin='external_capability' AND s.config->>'Kind'='capability_manifest'
 AND s.config->'Fields' ? 'capability_claim' THEN o.metadata->'capability_claim' ELSE NULL END
 FROM external_leads_for_intention(target,'','',
 (SELECT p.id FROM external_observations p WHERE p.id<selected_lead ORDER BY p.id DESC LIMIT 1),1) l
 JOIN external_observations o ON o.id=l.id JOIN external_sources s ON s.id=l.source_id
 WHERE l.id=selected_lead
 $$;
REVOKE ALL ON FUNCTION external_fit_source(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION external_fit_source(uuid,uuid) TO undergrid_app;
