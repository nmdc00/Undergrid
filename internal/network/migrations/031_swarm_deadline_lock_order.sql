SET search_path=undergrid,pg_catalog;
-- Event FKs acquire attribution key-share locks. Acquire them before the room,
-- matching principal→agent→room command order and preventing a wait cycle.
-- This helper grants no principal, agent, private report or timer write access.
CREATE FUNCTION lock_swarm_deadline_room(target uuid)
RETURNS TABLE(coordinator_agent uuid,coordinator_principal uuid)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
DECLARE source_agent uuid; source_principal uuid; current_state text;
BEGIN
 SELECT s.coordinator_agent_id,a.principal_id INTO source_agent,source_principal
 FROM swarms s JOIN agents a ON a.id=s.coordinator_agent_id WHERE s.id=target;
 IF NOT FOUND THEN RETURN; END IF;
 PERFORM 1 FROM principals p WHERE p.id=source_principal FOR KEY SHARE;
 PERFORM 1 FROM agents a WHERE a.id=source_agent FOR KEY SHARE;
 SELECT s.state INTO current_state FROM swarms s
 WHERE s.id=target AND s.coordinator_agent_id=source_agent FOR UPDATE;
 IF NOT FOUND OR current_state<>'active' THEN RETURN; END IF;
 RETURN QUERY SELECT source_agent,source_principal;
END $$;
REVOKE ALL ON FUNCTION lock_swarm_deadline_room(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION lock_swarm_deadline_room(uuid) TO undergrid_worker;
