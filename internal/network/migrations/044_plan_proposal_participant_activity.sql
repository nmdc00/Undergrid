SET search_path=undergrid,pg_catalog;

-- New proposals can name accepted participants from other principals, whose
-- private identity rows are not readable by the API role. Confine the check and
-- locks to this current contributor's room; return no foreign identity fields.
-- NOWAIT avoids waiting on peer authority while the caller owns its own actor
-- and room locks. Known-success receipt recovery does not call this helper.
CREATE FUNCTION lock_plan_proposal_participants(target uuid,selected_agents uuid[],selected_required uuid[],selected_publication uuid[]) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,undergrid,pg_temp AS $$
DECLARE principal_ids uuid[]; agent_ids uuid[]; chosen_principal uuid; chosen_agent uuid;
BEGIN
 IF COALESCE(cardinality(selected_agents),0) NOT BETWEEN 1 AND 50
  OR COALESCE(cardinality(selected_required),0) NOT BETWEEN 1 AND 50
  OR COALESCE(cardinality(selected_publication),0)>50
  OR NOT undergrid.swarm_contributor(target)
  OR NOT EXISTS(SELECT 1 FROM undergrid.swarms WHERE id=target AND state='active')
  OR NOT EXISTS(SELECT 1 FROM undergrid.agents a JOIN undergrid.principals p ON p.id=a.principal_id
   WHERE a.id=undergrid.actor_agent() AND p.id=undergrid.actor_principal() AND a.status='active' AND p.status='active') THEN RETURN false; END IF;
 IF EXISTS(SELECT 1 FROM unnest(selected_agents) chosen(id) WHERE NOT EXISTS(
   SELECT 1 FROM undergrid.memberships m WHERE m.swarm_id=target AND m.agent_id=chosen.id
    AND m.state='accepted' AND m.role<>'observer' AND m.principal_id=ANY(selected_required)))
  OR EXISTS(SELECT 1 FROM unnest(selected_required) chosen(id) WHERE NOT EXISTS(
   SELECT 1 FROM undergrid.memberships m WHERE m.swarm_id=target AND m.principal_id=chosen.id AND m.state='accepted' AND m.role<>'observer'))
  OR EXISTS(SELECT 1 FROM unnest(selected_publication) chosen(id) WHERE NOT EXISTS(
   SELECT 1 FROM undergrid.memberships m WHERE m.swarm_id=target AND m.principal_id=chosen.id AND m.state='accepted')) THEN RETURN false; END IF;
 SELECT array_agg(id ORDER BY id) INTO principal_ids FROM (
  SELECT DISTINCT id FROM unnest(selected_required||COALESCE(selected_publication,'{}'::uuid[])||ARRAY[undergrid.actor_principal()]) chosen(id)) ids;
 SELECT array_agg(id ORDER BY id) INTO agent_ids FROM (
  SELECT DISTINCT id FROM unnest(selected_agents||ARRAY[undergrid.actor_agent()]) chosen(id)) ids;
 PERFORM 1 FROM undergrid.principals WHERE id=ANY(principal_ids) ORDER BY id FOR SHARE NOWAIT;
 IF EXISTS(SELECT 1 FROM undergrid.principals WHERE id=ANY(principal_ids) AND status<>'active') THEN RETURN false; END IF;
 -- A non-assigned approver or publication principal still needs one active
 -- accepted attribution agent. Required approvers need a contributor; a
 -- publication-only principal may have an observer. Pick and lock an eligible
 -- witness, rather than disqualifying every agent when one was revoked.
 FOR chosen_principal IN SELECT DISTINCT id FROM unnest(selected_required||COALESCE(selected_publication,'{}'::uuid[])) chosen(id) ORDER BY id LOOP
  SELECT a.id INTO chosen_agent FROM undergrid.memberships m JOIN undergrid.agents a ON a.id=m.agent_id AND a.principal_id=m.principal_id
   WHERE m.swarm_id=target AND m.principal_id=chosen_principal AND m.state='accepted' AND a.status='active'
    AND (NOT chosen_principal=ANY(selected_required) OR m.role<>'observer')
   ORDER BY a.id LIMIT 1 FOR SHARE OF m,a NOWAIT;
  IF NOT FOUND THEN RETURN false; END IF;
 END LOOP;
 PERFORM 1 FROM undergrid.agents WHERE id=ANY(agent_ids) ORDER BY id FOR SHARE NOWAIT;
 RETURN NOT EXISTS(SELECT 1 FROM undergrid.agents WHERE id=ANY(agent_ids) AND status<>'active')
  AND EXISTS(SELECT 1 FROM undergrid.agents a WHERE a.id=undergrid.actor_agent() AND a.principal_id=undergrid.actor_principal() AND a.status='active');
 -- Do not catch 55P03: transient contention retains the existing retryable
 -- server-error contract and must not masquerade as a permanent stale choice.

END;
$$;
REVOKE ALL ON FUNCTION lock_plan_proposal_participants(uuid,uuid[],uuid[],uuid[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION lock_plan_proposal_participants(uuid,uuid[],uuid[],uuid[]) TO undergrid_app;
