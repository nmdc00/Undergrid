SET search_path=undergrid,pg_catalog;
CREATE TABLE coordinator_proposals (
 id uuid PRIMARY KEY, swarm_id uuid NOT NULL REFERENCES swarms,
 proposer_agent_id uuid NOT NULL REFERENCES agents,
 successor_agent_id uuid NOT NULL REFERENCES agents,
 previous_coordinator_agent_id uuid NOT NULL REFERENCES agents,
 snapshot jsonb NOT NULL, content_hash text NOT NULL CHECK(length(content_hash)=64),
 state text NOT NULL DEFAULT 'proposed' CHECK(state IN('proposed','applied','rejected','expired')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), respond_by timestamptz NOT NULL,
 accepted_credential_id uuid REFERENCES credentials, accepted_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((accepted_credential_id IS NULL)=(accepted_at IS NULL)),
 CHECK(successor_agent_id<>previous_coordinator_agent_id)
);
CREATE TABLE coordinator_votes (
 id uuid PRIMARY KEY, proposal_id uuid NOT NULL REFERENCES coordinator_proposals,
 principal_id uuid NOT NULL REFERENCES principals, agent_id uuid NOT NULL,
 credential_id uuid NOT NULL REFERENCES credentials,
 policy_version bigint NOT NULL, policy_hash text NOT NULL,
 decision text NOT NULL CHECK(decision IN('approve','reject')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(agent_id,principal_id) REFERENCES agents(id,principal_id),
 UNIQUE(proposal_id,principal_id)
);
CREATE INDEX coordinator_proposal_deadlines ON coordinator_proposals(respond_by) WHERE state='proposed';
ALTER TABLE coordinator_proposals ENABLE ROW LEVEL SECURITY;
ALTER TABLE coordinator_votes ENABLE ROW LEVEL SECURITY;
CREATE POLICY coordinator_proposal_read ON coordinator_proposals FOR SELECT TO undergrid_app USING(swarm_member(swarm_id));
CREATE POLICY coordinator_proposal_insert ON coordinator_proposals FOR INSERT TO undergrid_app WITH CHECK(swarm_member(swarm_id) AND proposer_agent_id=actor_agent());
CREATE POLICY coordinator_proposal_update ON coordinator_proposals FOR UPDATE TO undergrid_app USING(swarm_member(swarm_id)) WITH CHECK(swarm_member(swarm_id));
CREATE POLICY coordinator_vote_read ON coordinator_votes FOR SELECT TO undergrid_app USING(proposal_id IN(SELECT id FROM coordinator_proposals));
CREATE POLICY coordinator_vote_insert ON coordinator_votes FOR INSERT TO undergrid_app WITH CHECK(principal_id=actor_principal() AND agent_id=actor_agent() AND proposal_id IN(SELECT id FROM coordinator_proposals));
GRANT SELECT,INSERT ON coordinator_proposals,coordinator_votes TO undergrid_app;
GRANT UPDATE(state,version,accepted_credential_id,accepted_at) ON coordinator_proposals TO undergrid_app;
GRANT SELECT ON coordinator_proposals,coordinator_votes TO undergrid_worker;
GRANT UPDATE(state,version) ON coordinator_proposals TO undergrid_worker;
CREATE POLICY coordinator_proposal_worker ON coordinator_proposals TO undergrid_worker USING(true) WITH CHECK(true);
CREATE POLICY coordinator_vote_worker ON coordinator_votes FOR SELECT TO undergrid_worker USING(true);

-- Normal succession is principal consent, not a hidden plan/rights approval.
CREATE FUNCTION succession_required_principals(target uuid) RETURNS uuid[] LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT COALESCE(array_agg(DISTINCT id ORDER BY id),'{}'::uuid[]) FROM (
  SELECT value::uuid AS id FROM swarms s JOIN plan_versions p ON p.id=s.active_plan_id
  CROSS JOIN LATERAL jsonb_array_elements_text((p.specification->'plan'->'required_principals')||(p.specification->'plan'->'acceptance_principals'))
  WHERE s.id=target AND p.state='accepted' AND swarm_member(target)
  UNION
  SELECT m.principal_id FROM memberships m JOIN swarms s ON s.id=m.swarm_id
  WHERE s.id=target AND s.active_plan_id IS NULL AND m.state='accepted' AND m.role='contributor' AND swarm_member(target)
  UNION
  SELECT t.assigned_principal_id FROM tasks t JOIN plan_versions p ON p.id=t.plan_id JOIN swarms s ON s.id=p.swarm_id
  WHERE s.id=target AND t.state IN('accepted','in_progress','submitted','changes_requested','at_risk') AND swarm_member(target)
 ) required
$$;
REVOKE ALL ON FUNCTION succession_required_principals(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION succession_required_principals(uuid) TO undergrid_app;

-- Verify the real local credential and room membership before locking any
-- other authority. The complete roster is bounded or rejected, never truncated.
CREATE FUNCTION lock_coordinator_authorities(target uuid, credential uuid, requested_scope text) RETURNS integer LANGUAGE plpgsql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
DECLARE roster jsonb; current_coordinator uuid;
BEGIN
 IF requested_scope IS NULL OR requested_scope NOT IN('coordinator:propose','coordinator:approve','coordinator:accept') THEN RETURN 5; END IF;
 IF NOT EXISTS(SELECT 1 FROM credentials c JOIN agents a ON a.id=c.agent_id JOIN principals p ON p.id=a.principal_id WHERE c.id=credential AND c.agent_id=actor_agent() AND a.principal_id=actor_principal() AND c.revoked_at IS NULL AND c.expires_at>clock_timestamp() AND a.status='active' AND p.status='active') THEN RETURN 3; END IF;
 IF NOT EXISTS(SELECT 1 FROM credentials WHERE id=credential AND requested_scope=ANY(scopes)) THEN RETURN 5; END IF;
 IF NOT swarm_member(target) THEN RETURN 0; END IF;
 IF (SELECT count(*) FROM memberships WHERE swarm_id=target AND state='accepted')>500 THEN RETURN 2; END IF;
 SELECT coordinator_agent_id INTO current_coordinator FROM swarms WHERE id=target;
 SELECT jsonb_agg(agent_id ORDER BY agent_id) INTO roster FROM memberships WHERE swarm_id=target AND state='accepted';
 PERFORM id FROM principals WHERE id IN(SELECT principal_id FROM memberships WHERE swarm_id=target AND state='accepted' UNION SELECT principal_id FROM agents WHERE id=current_coordinator) ORDER BY id FOR UPDATE;
 PERFORM id FROM agents WHERE id IN(SELECT agent_id FROM memberships WHERE swarm_id=target AND state='accepted' UNION SELECT current_coordinator) ORDER BY id FOR UPDATE;
 IF current_coordinator IS DISTINCT FROM (SELECT coordinator_agent_id FROM swarms WHERE id=target) OR roster IS DISTINCT FROM (SELECT jsonb_agg(agent_id ORDER BY agent_id) FROM memberships WHERE swarm_id=target AND state='accepted') THEN RETURN 4; END IF;
 PERFORM set_config('undergrid.coordinator_guard_room',target::text,true),set_config('undergrid.coordinator_guard_roster',roster::text,true),set_config('undergrid.coordinator_guard_scope',requested_scope,true);
 RETURN 1;
END $$;
REVOKE ALL ON FUNCTION lock_coordinator_authorities(uuid,uuid,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION lock_coordinator_authorities(uuid,uuid,text) TO undergrid_app;

CREATE FUNCTION coordinator_authorities_current(target uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT COALESCE(swarm_member(target) AND current_setting('undergrid.coordinator_guard_room',true)=target::text
 AND nullif(current_setting('undergrid.coordinator_guard_roster',true),'')::jsonb=(SELECT jsonb_agg(agent_id ORDER BY agent_id) FROM memberships WHERE swarm_id=target AND state='accepted'),false)
$$;
REVOKE ALL ON FUNCTION coordinator_authorities_current(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION coordinator_authorities_current(uuid) TO undergrid_app;

-- Agent rows are owner-private under RLS. Expose only this eligibility bit to
-- current room members so nomination is not restricted to one's own agent.
CREATE FUNCTION coordinator_successor_eligible(target uuid, successor uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT swarm_member(target) AND EXISTS(SELECT 1 FROM memberships m JOIN agents a ON a.id=m.agent_id JOIN principals p ON p.id=a.principal_id WHERE m.swarm_id=target AND m.agent_id=successor AND m.state='accepted' AND m.role='contributor' AND a.status='active' AND p.status='active')
$$;
REVOKE ALL ON FUNCTION coordinator_successor_eligible(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION coordinator_successor_eligible(uuid,uuid) TO undergrid_app;

-- Only hashes of owner policies cross this boundary; never expose a peer's raw
-- policy as proposal metadata. Conversation sequence/version is not consent.
CREATE FUNCTION coordinator_snapshot(target uuid, successor uuid) RETURNS jsonb LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 WITH required AS (SELECT unnest(succession_required_principals(target)) id UNION SELECT principal_id FROM agents WHERE id=successor)
 SELECT jsonb_build_object(
  'coordinator_agent_id',s.coordinator_agent_id,'room_state',s.state,
  'successor_agent_id',successor,'accepted_plan_id',s.active_plan_id,
  'accepted_plan_hash',(SELECT content_hash FROM plan_versions WHERE id=s.active_plan_id AND state='accepted'),
  'required_principals',(SELECT COALESCE(jsonb_agg(id ORDER BY id),'[]') FROM required),
  'memberships',(SELECT COALESCE(jsonb_agg(jsonb_build_object('id',m.id,'agent_id',m.agent_id,'principal_id',m.principal_id,'role',m.role,'version',m.version,'agent_status',a.status) ORDER BY m.id),'[]') FROM memberships m JOIN agents a ON a.id=m.agent_id WHERE m.swarm_id=s.id AND m.state='accepted'),
  'policies',(SELECT COALESCE(jsonb_agg(jsonb_build_object('principal_id',p.id,'version',p.version,'status',p.status,'hash',encode(sha256(convert_to(p.policy::text,'UTF8')),'hex')) ORDER BY p.id),'[]') FROM principals p WHERE p.id IN(SELECT id FROM required))
 ) FROM swarms s WHERE s.id=target AND swarm_member(target)
$$;
REVOKE ALL ON FUNCTION coordinator_snapshot(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION coordinator_snapshot(uuid,uuid) TO undergrid_app;

-- Existing votes/acceptance never survive revoked credentials or authority.
CREATE FUNCTION coordinator_consents_current(target uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT swarm_member(proposal.swarm_id) AND
 (proposal.accepted_credential_id IS NULL OR EXISTS(SELECT 1 FROM credentials c JOIN agents a ON a.id=c.agent_id JOIN principals p ON p.id=a.principal_id WHERE c.id=proposal.accepted_credential_id AND a.id=proposal.successor_agent_id AND c.revoked_at IS NULL AND c.expires_at>clock_timestamp() AND 'coordinator:accept'=ANY(c.scopes) AND a.status='active' AND p.status='active' AND p.policy->>'allow_coordinator_succession'='true' AND (p.policy->>'expires_at')::timestamptz>clock_timestamp()))
 AND NOT EXISTS(SELECT 1 FROM coordinator_votes v WHERE v.proposal_id=target AND NOT EXISTS(
  SELECT 1 FROM principals p JOIN agents a ON a.principal_id=p.id JOIN credentials c ON c.agent_id=a.id
  WHERE p.id=v.principal_id AND a.id=v.agent_id AND c.id=v.credential_id AND p.status='active' AND a.status='active' AND c.revoked_at IS NULL AND c.expires_at>clock_timestamp() AND 'coordinator:approve'=ANY(c.scopes)
  AND p.version=v.policy_version AND encode(sha256(convert_to(p.policy::text,'UTF8')),'hex')=v.policy_hash AND p.policy->>'allow_coordinator_succession'='true' AND (p.policy->>'expires_at')::timestamptz>clock_timestamp()
  AND EXISTS(SELECT 1 FROM memberships m WHERE m.swarm_id=proposal.swarm_id AND m.agent_id=v.agent_id AND m.state='accepted')
 )) FROM coordinator_proposals proposal WHERE proposal.id=target
$$;
REVOKE ALL ON FUNCTION coordinator_consents_current(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION coordinator_consents_current(uuid) TO undergrid_app;

-- Narrow transition helper: the app cannot assign arbitrary coordinator IDs.
-- The command already owns the ordered authority and room/proposal locks.
CREATE FUNCTION apply_coordinator_succession(target uuid) RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
DECLARE proposal coordinator_proposals; required uuid[]; previous_contributor boolean;
BEGIN
 SELECT * INTO proposal FROM coordinator_proposals WHERE id=target;
 IF NOT FOUND OR NOT swarm_member(proposal.swarm_id) OR coordinator_authorities_current(proposal.swarm_id) IS NOT TRUE THEN RETURN false; END IF;
 IF COALESCE(current_setting('undergrid.coordinator_guard_scope',true),'') NOT IN('coordinator:approve','coordinator:accept') THEN RETURN false; END IF;
 IF NOT EXISTS(SELECT 1 FROM swarms s WHERE s.id=proposal.swarm_id AND (s.active_plan_id IS NULL OR EXISTS(SELECT 1 FROM plan_versions p WHERE p.id=s.active_plan_id AND p.state='accepted'))) THEN RETURN false; END IF;
 IF proposal.state<>'proposed' OR proposal.respond_by<=clock_timestamp() OR proposal.snapshot IS DISTINCT FROM coordinator_snapshot(proposal.swarm_id,proposal.successor_agent_id) THEN RETURN false; END IF;
 IF NOT EXISTS(SELECT 1 FROM memberships m JOIN agents a ON a.id=m.agent_id JOIN principals p ON p.id=a.principal_id JOIN credentials c ON c.agent_id=a.id WHERE m.swarm_id=proposal.swarm_id AND m.agent_id=proposal.successor_agent_id AND m.state='accepted' AND m.role='contributor' AND a.status='active' AND p.status='active' AND c.id=proposal.accepted_credential_id AND c.revoked_at IS NULL AND c.expires_at>clock_timestamp() AND 'coordinator:accept'=ANY(c.scopes) AND p.policy->>'allow_coordinator_succession'='true' AND (p.policy->>'expires_at')::timestamptz>clock_timestamp()) THEN RETURN false; END IF;
 SELECT array_agg(value::uuid) INTO required FROM jsonb_array_elements_text(proposal.snapshot->'required_principals');
 IF coordinator_consents_current(target) IS NOT TRUE OR EXISTS(SELECT 1 FROM unnest(required) r(id) WHERE NOT EXISTS(SELECT 1 FROM coordinator_votes v WHERE v.proposal_id=target AND v.principal_id=r.id AND v.decision='approve')) THEN RETURN false; END IF;
 SELECT EXISTS(SELECT 1 FROM agents a WHERE a.id=proposal.previous_coordinator_agent_id AND a.principal_id=ANY(succession_required_principals(proposal.swarm_id))) INTO previous_contributor;
 UPDATE swarms SET coordinator_agent_id=proposal.successor_agent_id,version=version+1 WHERE id=proposal.swarm_id AND coordinator_agent_id=proposal.previous_coordinator_agent_id AND state='active';
 IF NOT FOUND THEN RETURN false; END IF;
 UPDATE memberships SET role=CASE WHEN agent_id=proposal.successor_agent_id THEN 'coordinator' WHEN previous_contributor THEN 'contributor' ELSE 'observer' END,version=version+1 WHERE swarm_id=proposal.swarm_id AND agent_id IN(proposal.previous_coordinator_agent_id,proposal.successor_agent_id) AND state='accepted';
 UPDATE coordinator_proposals SET state='applied',version=version+1 WHERE id=target;
 RETURN true;
END $$;
REVOKE ALL ON FUNCTION apply_coordinator_succession(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION apply_coordinator_succession(uuid) TO undergrid_app;
