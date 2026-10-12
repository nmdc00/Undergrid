SET search_path=undergrid,pg_catalog;
CREATE FUNCTION swarm_contributor(s uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM memberships WHERE swarm_id=s AND agent_id=actor_agent() AND state='accepted' AND role IN ('coordinator','contributor'))
 $$;
REVOKE ALL ON FUNCTION swarm_contributor(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION swarm_contributor(uuid) TO undergrid_app;
ALTER POLICY message_insert ON messages WITH CHECK(agent_id=actor_agent() AND principal_id=actor_principal() AND swarm_contributor(swarm_id));
ALTER POLICY swarm_update ON swarms USING(swarm_member(id) OR swarm_coordinator(id) OR EXISTS(SELECT 1 FROM memberships m WHERE m.swarm_id=id AND m.agent_id=actor_agent() AND m.state='offered'))
 WITH CHECK(swarm_member(id) OR swarm_coordinator(id) OR EXISTS(SELECT 1 FROM memberships m WHERE m.swarm_id=id AND m.agent_id=actor_agent() AND m.state='offered'));
