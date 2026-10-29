SET search_path=undergrid,pg_catalog;

-- Public permission provenance is deliberately distinct from private approval
-- evidence. No raw config, source endpoint or internal basis text is projected.
CREATE FUNCTION external_leads_for_intention(target uuid, selected_kind text, query_text text, after_id uuid, page_size integer)
RETURNS TABLE(id uuid,source_id uuid,source_version text,origin text,canonical_url text,publisher text,permission_notice text,metadata jsonb,observed_at timestamptz,checked_at timestamptz)
LANGUAGE sql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT o.id,o.source_id,o.source_version,o.kind,o.canonical_url,s.config->>'Publisher',s.config->>'PublicPermissionNotice',
 COALESCE((SELECT jsonb_object_agg(f.field,o.metadata->f.field)
 FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(s.config->'Fields')='array' THEN s.config->'Fields' ELSE '[]'::jsonb END) AS f(field)
 WHERE f.field=ANY(ARRAY['title','summary','tags','updated_at']::text[]) AND o.metadata ? f.field),'{}'::jsonb),o.first_seen,LEAST(o.checked_at,s.last_success)
 FROM external_observations o JOIN external_sources s ON s.id=o.source_id
 WHERE EXISTS(SELECT 1 FROM intents i JOIN principals p ON p.id=i.principal_id JOIN agents a ON a.id=i.agent_id
  WHERE i.id=target AND i.principal_id=actor_principal() AND i.state='active' AND i.expires_at>now() AND p.status='active' AND a.status='active')
 AND o.status='active' AND o.checked_at>now()-interval '24 hours' AND s.last_success>now()-interval '24 hours'
 AND jsonb_typeof(s.config->'PublicPermissionNotice')='string' AND length(btrim(s.config->>'PublicPermissionNotice')) BETWEEN 1 AND 2000
 AND jsonb_typeof(s.config->'Publisher')='string' AND length(s.config->>'Publisher') BETWEEN 1 AND 1000
 AND jsonb_typeof(s.config->'Fields')='array' AND COALESCE(s.config->'Fields','[]'::jsonb) ? 'canonical_url'
 AND (selected_kind='' OR o.kind=selected_kind) AND (after_id IS NULL OR o.id>after_id)
 AND (query_text='' OR EXISTS(SELECT 1 FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(s.config->'Fields')='array' THEN s.config->'Fields' ELSE '[]'::jsonb END) AS q(field)
  WHERE q.field IN ('title','summary') AND strpos(lower(COALESCE(o.metadata->>q.field,'')),lower(query_text))>0))
 AND length(query_text)<=200 AND selected_kind IN ('','external_request','external_capability')
 ORDER BY o.id LIMIT LEAST(GREATEST(page_size,1),51)
 $$;
REVOKE ALL ON FUNCTION external_leads_for_intention(uuid,text,text,uuid,integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION external_leads_for_intention(uuid,text,text,uuid,integer) TO undergrid_app;
