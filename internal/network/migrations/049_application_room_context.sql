SET search_path=undergrid,pg_catalog;
-- A nonmember may review/apply to a visible role, but must not receive general
-- UPDATE eligibility merely to lock that reviewed public context.
CREATE FUNCTION application_room_context(target uuid)
RETURNS TABLE(recruitment_roles jsonb,visibility text,state text,version bigint)
LANGUAGE sql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT s.recruitment_roles,s.visibility,s.state,s.version FROM swarms s
 WHERE s.id=target AND s.visibility<>'private' AND s.state='active'
 AND EXISTS(SELECT 1 FROM agents a JOIN principals p ON p.id=a.principal_id
 WHERE a.id=actor_agent() AND a.principal_id=actor_principal()
 AND a.status='active' AND p.status='active')
 FOR SHARE OF s NOWAIT
$$;
REVOKE ALL ON FUNCTION application_room_context(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION application_room_context(uuid) TO undergrid_app;
