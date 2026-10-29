SET search_path=undergrid,pg_catalog;
-- A narrow public declaration projection; never disclose profile routing,
-- invitation preferences, owner records or credentials to foreign readers.
CREATE FUNCTION public_capability_declarations(excluded_principal uuid, sample_limit integer)
RETURNS TABLE(agent_id uuid,principal_id uuid,capabilities jsonb)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT DISTINCT ON (a.principal_id) a.id,a.principal_id,a.profile->'capabilities'
 FROM agents a JOIN principals p ON p.id=a.principal_id
 WHERE a.principal_id<>excluded_principal AND a.status='active' AND p.status='active'
 AND a.profile->>'public'='true' AND jsonb_array_length(a.profile->'capabilities')>0
 ORDER BY a.principal_id,a.id LIMIT GREATEST(0,LEAST(sample_limit,21))
$$;
REVOKE ALL ON FUNCTION public_capability_declarations(uuid,integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public_capability_declarations(uuid,integer) TO undergrid_app;
