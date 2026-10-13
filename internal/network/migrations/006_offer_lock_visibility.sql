SET search_path=undergrid,pg_catalog;
-- Qualify the outer row: memberships also has an id column.
ALTER POLICY swarm_update ON swarms
 USING(swarm_member(swarms.id) OR swarm_coordinator(swarms.id) OR EXISTS(SELECT 1 FROM memberships m WHERE m.swarm_id=swarms.id AND m.agent_id=actor_agent() AND m.state='offered'))
 WITH CHECK(swarm_member(swarms.id) OR swarm_coordinator(swarms.id) OR EXISTS(SELECT 1 FROM memberships m WHERE m.swarm_id=swarms.id AND m.agent_id=actor_agent() AND m.state='offered'));
