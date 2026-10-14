SET search_path=undergrid,pg_catalog;
ALTER TABLE approvals DROP CONSTRAINT approvals_purpose_check;
ALTER TABLE approvals ADD CONSTRAINT approvals_purpose_check CHECK(purpose IN ('plan','rights','acceptance','publication','assembly_grant','evaluation_grant'));
ALTER TABLE approvals ADD COLUMN grant_recipient_agent_id uuid REFERENCES agents;
ALTER TABLE approvals ADD COLUMN grant_recipient_principal_id uuid REFERENCES principals;
ALTER TABLE approvals ADD COLUMN grant_target_task_id uuid REFERENCES tasks;
ALTER TABLE approvals ADD COLUMN grant_target_plan_id uuid REFERENCES plan_versions;
ALTER TABLE approvals ADD COLUMN grant_scope jsonb;
ALTER TABLE approvals ADD COLUMN grant_expires_at timestamptz;
ALTER TABLE approvals ADD COLUMN grant_survives_withdrawal boolean;
ALTER TABLE approvals ADD COLUMN grant_state text CHECK(grant_state IN ('active','revoked'));
ALTER TABLE approvals DROP CONSTRAINT approval_subject;
ALTER TABLE approvals ADD CONSTRAINT approval_subject CHECK(
 (purpose IN ('plan','rights') AND artifact_id IS NULL AND evaluation_id IS NULL AND grant_scope IS NULL)
 OR (purpose='acceptance' AND artifact_id IS NOT NULL AND evaluation_id IS NOT NULL AND grant_scope IS NULL)
 OR (purpose='publication' AND artifact_id IS NOT NULL AND evaluation_id IS NULL AND grant_scope IS NULL)
 OR (purpose IN ('assembly_grant','evaluation_grant') AND artifact_id IS NOT NULL AND evaluation_id IS NULL AND grant_scope IS NOT NULL AND grant_recipient_agent_id IS NOT NULL AND grant_recipient_principal_id IS NOT NULL AND grant_target_task_id IS NOT NULL AND grant_target_plan_id IS NOT NULL AND grant_expires_at IS NOT NULL AND grant_survives_withdrawal IS NOT NULL AND grant_state IS NOT NULL));
CREATE INDEX input_grants_recipient ON approvals(artifact_id,grant_recipient_agent_id,grant_target_task_id,purpose) WHERE grant_scope IS NOT NULL;
CREATE POLICY input_grant_actor_read ON approvals FOR SELECT TO undergrid_app USING(grant_scope IS NOT NULL AND (principal_id=actor_principal() OR grant_recipient_agent_id=actor_agent()));
CREATE POLICY input_grant_actor_revoke ON approvals FOR UPDATE TO undergrid_app USING(grant_scope IS NOT NULL AND principal_id=actor_principal()) WITH CHECK(grant_scope IS NOT NULL AND principal_id=actor_principal());
GRANT UPDATE(grant_state) ON approvals TO undergrid_app;
CREATE FUNCTION protect_input_grant() RETURNS trigger LANGUAGE plpgsql SET search_path=undergrid,pg_catalog AS $$
BEGIN
 IF OLD.grant_scope IS NOT NULL AND (to_jsonb(NEW)-'grant_state') IS DISTINCT FROM (to_jsonb(OLD)-'grant_state') THEN RAISE EXCEPTION 'input grant is immutable'; END IF;
 IF OLD.grant_state='revoked' AND NEW.grant_state<>'revoked' THEN RAISE EXCEPTION 'revoked input grant cannot reopen'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_input_grant BEFORE UPDATE ON approvals FOR EACH ROW EXECUTE FUNCTION protect_input_grant();
-- A grantor can inspect only this minimal target packet, never target room history.
CREATE FUNCTION input_grant_target(source uuid,target uuid) RETURNS jsonb LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT jsonb_build_object('task_id',t.id,'task_epoch',t.claim_epoch,'plan_id',p.id,'plan_hash',p.content_hash,'agent_id',t.assigned_agent_id,'principal_id',t.assigned_principal_id,'template_hash',p.specification->>'template_hash','grant_policy_hash',p.specification->>'input_grant_policy_hash','template_id',p.specification->'plan'->'rights'->>'template_id','license_id',p.specification->'plan'->'rights'->>'license_id','schema_family',t.specification->>'output_schema')
 FROM tasks t JOIN plan_versions p ON p.id=t.plan_id JOIN swarms s ON s.id=p.swarm_id
 JOIN agents agent ON agent.id=t.assigned_agent_id JOIN principals owner ON owner.id=t.assigned_principal_id
 WHERE t.id=target AND p.state='accepted' AND s.active_plan_id=p.id AND s.state='active' AND t.state IN ('accepted','in_progress','at_risk','changes_requested') AND agent.status='active' AND owner.status='active'
 AND EXISTS(SELECT 1 FROM memberships m WHERE m.swarm_id=p.swarm_id AND m.agent_id=t.assigned_agent_id AND m.state='accepted' AND m.role<>'observer')
 AND EXISTS(SELECT 1 FROM artifacts a WHERE a.id=source AND swarm_member(a.swarm_id) AND actor_principal()=ANY(a.required_rights))
$$;
REVOKE ALL ON FUNCTION input_grant_target(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION input_grant_target(uuid,uuid) TO undergrid_app;
-- Expand the artifact's sorted authority lock set with source grantors for its
-- named task. This applies before actor/room locks, including quarantine outputs.
CREATE OR REPLACE FUNCTION lock_artifact_authorities(target uuid) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
BEGIN
 PERFORM owner.id FROM principals owner WHERE owner.id IN (
 SELECT actor_principal() UNION SELECT unnest(a.required_rights) FROM artifacts a WHERE a.id=target AND (swarm_member(a.swarm_id) OR has_scoped_input_read(a.id))
 UNION SELECT value::uuid FROM artifacts a JOIN plan_versions p ON p.id=a.plan_id CROSS JOIN LATERAL jsonb_array_elements_text(p.specification->'plan'->'required_principals') WHERE a.id=target AND swarm_member(a.swarm_id)
 UNION SELECT g.principal_id FROM artifacts a JOIN approvals g ON g.grant_target_task_id=a.task_id WHERE a.id=target AND swarm_member(a.swarm_id) AND g.grant_recipient_agent_id=actor_agent()
 ) ORDER BY owner.id FOR UPDATE;
END $$;
SET search_path=undergrid,pg_catalog;
ALTER TABLE artifact_inputs ADD COLUMN grant_approval_ids uuid[];
CREATE FUNCTION valid_input_grants(source uuid,target uuid,wanted text) RETURNS SETOF approvals LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT g.* FROM approvals g JOIN artifacts a ON a.id=g.artifact_id JOIN plan_versions sp ON sp.id=a.plan_id
 JOIN tasks t ON t.id=g.grant_target_task_id JOIN plan_versions p ON p.id=t.plan_id JOIN swarms s ON s.id=p.swarm_id
 JOIN principals owner ON owner.id=g.principal_id JOIN agents agent ON agent.id=g.agent_id
 WHERE a.id=source AND t.id=target AND p.state='accepted' AND s.active_plan_id=p.id AND s.state='active'
 AND t.state IN ('accepted','in_progress','submitted','accepted_output','at_risk','changes_requested')
 AND a.state IN ('sealed','verified','published') AND g.purpose=wanted||'_grant' AND g.decision='approve' AND g.grant_state='active' AND g.grant_expires_at>clock_timestamp()
 AND g.grant_recipient_agent_id=t.assigned_agent_id AND g.grant_recipient_principal_id=t.assigned_principal_id AND g.grant_target_plan_id=p.id AND g.principal_id=ANY(a.required_rights)
 AND g.policy_version=current_policy_version(g.principal_id) AND (owner.policy->>'expires_at')::timestamptz>clock_timestamp() AND owner.status='active' AND agent.status='active'
 AND (g.grant_survives_withdrawal OR EXISTS(SELECT 1 FROM memberships m WHERE m.swarm_id=a.swarm_id AND m.agent_id=g.agent_id AND m.state='accepted'))
 AND g.grant_scope->>'artifact_sha256'=a.sha256 AND g.grant_scope->>'manifest_hash'=a.manifest_hash AND g.grant_scope->>'source_plan_hash'=a.plan_hash AND (g.grant_scope->>'source_claim_epoch')::bigint=a.claim_epoch
 AND g.grant_scope->>'target_plan_hash'=p.content_hash AND (g.grant_scope->>'target_claim_epoch')::bigint=t.claim_epoch
 AND g.grant_scope->>'template_hash'=a.agreement_hash AND g.grant_scope->>'template_hash'=p.specification->>'template_hash'
 AND g.grant_scope->>'grant_policy_hash'=sp.specification->>'input_grant_policy_hash' AND g.grant_scope->>'grant_policy_hash'=p.specification->>'input_grant_policy_hash'
 AND g.grant_scope->>'license_id'=a.license_id AND g.grant_scope->>'license_id'=p.specification->'plan'->'rights'->>'license_id'
$$;
REVOKE ALL ON FUNCTION valid_input_grants(uuid,uuid,text) FROM PUBLIC;
CREATE FUNCTION effective_input_grant_count(source uuid,target uuid,wanted text) RETURNS bigint LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT count(DISTINCT g.principal_id) FROM valid_input_grants(source,target,wanted) g JOIN tasks t ON t.id=g.grant_target_task_id
 WHERE t.assigned_agent_id=actor_agent() AND t.assigned_principal_id=actor_principal() AND EXISTS(SELECT 1 FROM memberships m WHERE m.swarm_id=(SELECT swarm_id FROM plan_versions WHERE id=t.plan_id) AND m.agent_id=actor_agent() AND m.state='accepted')
$$;
REVOKE ALL ON FUNCTION effective_input_grant_count(uuid,uuid,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION effective_input_grant_count(uuid,uuid,text) TO undergrid_app;
CREATE FUNCTION has_scoped_input_read(source uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM artifacts a JOIN approvals g ON g.artifact_id=a.id WHERE a.id=source AND g.grant_recipient_agent_id=actor_agent() AND g.purpose IN ('assembly_grant','evaluation_grant') AND effective_input_grant_count(a.id,g.grant_target_task_id,split_part(g.purpose,'_',1))=cardinality(a.required_rights))
$$;
REVOKE ALL ON FUNCTION has_scoped_input_read(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION has_scoped_input_read(uuid) TO undergrid_app;
CREATE POLICY artifact_scoped_input_read ON artifacts FOR SELECT TO undergrid_app USING(has_scoped_input_read(id));
CREATE FUNCTION input_grant_evidence(source uuid,target uuid,wanted text,selected uuid[]) RETURNS jsonb LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 WITH eligible AS (
 SELECT DISTINCT ON(g.principal_id) g.id,g.principal_id,g.content_hash,g.grant_scope FROM valid_input_grants(source,target,wanted) g JOIN tasks t ON t.id=g.grant_target_task_id
 WHERE (selected IS NULL OR g.id=ANY(selected)) AND (
 (t.assigned_agent_id=actor_agent() AND t.assigned_principal_id=actor_principal() AND swarm_member((SELECT swarm_id FROM plan_versions WHERE id=t.plan_id)))
 OR EXISTS(SELECT 1 FROM artifacts output JOIN artifact_inputs edge ON edge.artifact_id=output.id WHERE output.task_id=target AND output.state IN ('sealed','verified','published') AND edge.input_artifact_id=source AND swarm_member(output.swarm_id)))
 ORDER BY g.principal_id,g.content_hash)
 SELECT jsonb_build_object('ids',COALESCE(jsonb_agg(id ORDER BY principal_id),'[]'::jsonb),'hashes',COALESCE(jsonb_agg(content_hash ORDER BY principal_id),'[]'::jsonb),'policy_hashes',COALESCE(jsonb_agg(grant_scope->>'grant_policy_hash' ORDER BY principal_id),'[]'::jsonb),'count',count(*)) FROM eligible
$$;
REVOKE ALL ON FUNCTION input_grant_evidence(uuid,uuid,text,uuid[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION input_grant_evidence(uuid,uuid,text,uuid[]) TO undergrid_app;
CREATE FUNCTION lineage_input_context(output uuid,source uuid) RETURNS SETOF artifacts LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT a.* FROM artifacts a JOIN artifact_inputs edge ON edge.input_artifact_id=a.id JOIN artifacts result ON result.id=edge.artifact_id
 WHERE result.id=output AND a.id=source AND swarm_member(result.swarm_id) AND result.state IN ('sealed','verified','published') AND edge.input_sha256=a.sha256
$$;
REVOKE ALL ON FUNCTION lineage_input_context(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION lineage_input_context(uuid,uuid) TO undergrid_app;
