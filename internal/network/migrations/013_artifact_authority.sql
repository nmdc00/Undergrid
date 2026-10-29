SET search_path=undergrid,pg_catalog;
-- Counts expose no foreign owner policy or private credential information. The
-- owning artifact's exact sealed required set is selected inside this boundary.
CREATE FUNCTION artifact_current_approval_count(target uuid, wanted text) RETURNS bigint LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT count(*) FROM approvals x JOIN artifacts output ON output.id=x.artifact_id JOIN plan_versions p ON p.id=output.plan_id
 JOIN agents agent ON agent.id=x.agent_id JOIN principals owner ON owner.id=x.principal_id
 WHERE output.id=target AND swarm_member(output.swarm_id) AND x.purpose=wanted AND x.decision='approve'
 AND x.content_hash=CASE WHEN wanted='acceptance' THEN output.sha256 ELSE output.manifest_hash END
 AND x.policy_version=current_policy_version(x.principal_id) AND agent.status='active' AND owner.status='active'
 AND owner.policy->'template_ids' ? (p.specification->'plan'->'rights'->>'template_id') AND owner.policy->'license_ids' ? output.license_id
 AND owner.policy->'task_categories' ? 'json_dataset'
 AND EXISTS(SELECT 1 FROM memberships m WHERE m.swarm_id=output.swarm_id AND m.agent_id=x.agent_id AND m.state='accepted')
 AND ((wanted='publication' AND x.principal_id=ANY(output.required_rights)) OR
 (wanted='acceptance' AND p.specification->'plan'->'acceptance_principals' ? x.principal_id::text AND EXISTS(
 SELECT 1 FROM evaluations e WHERE e.id=x.evaluation_id AND e.artifact_id=output.id AND e.generation=output.generation AND e.input_sha256=output.sha256 AND e.manifest_sha256=output.manifest_hash AND e.result='passed')))
$$;
REVOKE ALL ON FUNCTION artifact_current_approval_count(uuid,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION artifact_current_approval_count(uuid,text) TO undergrid_app;
CREATE FUNCTION artifact_current_plan_rights(target uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT NOT EXISTS(
 SELECT 1 FROM artifacts a JOIN plan_versions p ON p.id=a.plan_id CROSS JOIN LATERAL jsonb_array_elements_text(p.specification->'plan'->'required_principals') required
 WHERE a.id=target AND swarm_member(a.swarm_id) AND NOT EXISTS(
 SELECT 1 FROM approvals x JOIN agents agent ON agent.id=x.agent_id JOIN principals owner ON owner.id=x.principal_id
 WHERE x.plan_id=p.id AND x.artifact_id IS NULL AND x.principal_id=required.value::uuid AND x.purpose='rights' AND x.content_hash=p.content_hash AND x.decision='approve'
 AND x.policy_version=current_policy_version(x.principal_id) AND owner.status='active' AND agent.status='active'
 AND EXISTS(SELECT 1 FROM memberships m WHERE m.swarm_id=a.swarm_id AND m.agent_id=x.agent_id AND m.state='accepted')))
$$;
REVOKE ALL ON FUNCTION artifact_current_plan_rights(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION artifact_current_plan_rights(uuid) TO undergrid_app;
