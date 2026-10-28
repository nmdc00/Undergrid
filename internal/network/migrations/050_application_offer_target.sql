SET search_path=undergrid,pg_catalog;
CREATE FUNCTION lock_application_offer_target(room uuid,application uuid,target_agent uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
DECLARE target_principal uuid;
BEGIN
 SELECT m.principal_id INTO target_principal
 FROM memberships m JOIN swarms s ON s.id=m.swarm_id
 JOIN agents c ON c.id=s.coordinator_agent_id JOIN principals p ON p.id=c.principal_id
 WHERE m.id=application AND m.swarm_id=room AND m.agent_id=target_agent AND m.origin='application'
 AND m.state IN ('applied','offered','accepted') AND (m.state='accepted' OR m.respond_by>clock_timestamp())
 AND s.state='active' AND c.id=actor_agent() AND c.principal_id=actor_principal()
 AND c.status='active' AND p.status='active'
 AND EXISTS(SELECT 1 FROM memberships cm WHERE cm.swarm_id=s.id AND cm.agent_id=c.id AND cm.state='accepted' AND cm.role='coordinator');
 IF NOT FOUND THEN RETURN false; END IF;
 PERFORM 1 FROM principals WHERE id=target_principal AND status='active' FOR SHARE NOWAIT;
 IF NOT FOUND THEN RETURN false; END IF;
 PERFORM 1 FROM agents WHERE id=target_agent AND principal_id=target_principal AND status='active' FOR SHARE NOWAIT;
 RETURN FOUND;
END $$;
REVOKE ALL ON FUNCTION lock_application_offer_target(uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION lock_application_offer_target(uuid,uuid,uuid) TO undergrid_app;
