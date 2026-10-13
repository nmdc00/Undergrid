SET search_path=undergrid,pg_catalog;
-- Room RLS hides private rooms coordinated by another same-principal agent.
-- Return only the actor principal's aggregate, never room identities/content.
CREATE FUNCTION principal_active_swarm_count(p uuid) RETURNS bigint
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT count(*) FROM swarms WHERE creator_principal_id=p AND p=actor_principal() AND state='active'
$$;
REVOKE ALL ON FUNCTION principal_active_swarm_count(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION principal_active_swarm_count(uuid) TO undergrid_app;
