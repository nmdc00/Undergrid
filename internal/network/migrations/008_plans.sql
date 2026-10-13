SET search_path=undergrid,pg_catalog;
CREATE TABLE plan_versions (
 id uuid PRIMARY KEY, swarm_id uuid NOT NULL REFERENCES swarms, parent_id uuid REFERENCES plan_versions,
 version bigint NOT NULL CHECK(version>0), specification jsonb NOT NULL, content_hash text NOT NULL,
 state text NOT NULL DEFAULT 'proposed' CHECK(state IN ('proposed','accepted','rejected','expired','withdrawn','superseded')),
 respond_by timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(swarm_id,version)
);
ALTER TABLE swarms ADD COLUMN active_plan_id uuid REFERENCES plan_versions;
GRANT UPDATE(active_plan_id) ON swarms TO undergrid_app;
CREATE UNIQUE INDEX one_accepted_plan ON plan_versions(swarm_id) WHERE state='accepted';
CREATE TABLE tasks (
 id uuid PRIMARY KEY, plan_id uuid NOT NULL REFERENCES plan_versions, task_key text NOT NULL,
 specification jsonb NOT NULL, specification_hash text NOT NULL,
 state text NOT NULL DEFAULT 'offered' CHECK(state IN ('open','offered','accepted','in_progress','submitted','accepted_output','changes_requested','at_risk','abandoned','cancelled')),
 assigned_agent_id uuid NOT NULL, assigned_principal_id uuid NOT NULL,
 response_deadline timestamptz NOT NULL, progress_deadline timestamptz, final_deadline timestamptz,
 claim_epoch bigint NOT NULL DEFAULT 1 CHECK(claim_epoch>0), version bigint NOT NULL DEFAULT 1,
 UNIQUE(plan_id,task_key), FOREIGN KEY(assigned_agent_id,assigned_principal_id) REFERENCES agents(id,principal_id)
);
CREATE TABLE approvals (
 id uuid PRIMARY KEY, principal_id uuid NOT NULL REFERENCES principals, agent_id uuid NOT NULL REFERENCES agents,
 purpose text NOT NULL CHECK(purpose IN ('plan','rights')), plan_id uuid NOT NULL REFERENCES plan_versions,
 content_hash text NOT NULL, decision text NOT NULL CHECK(decision IN ('approve','reject')),
 policy_version bigint NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(plan_id,principal_id,purpose,content_hash)
);
ALTER TABLE plan_versions ENABLE ROW LEVEL SECURITY;
CREATE POLICY plan_access ON plan_versions TO undergrid_app USING(swarm_member(swarm_id)) WITH CHECK(swarm_contributor(swarm_id));
ALTER TABLE tasks ENABLE ROW LEVEL SECURITY;
CREATE POLICY task_access ON tasks TO undergrid_app USING(plan_id IN (SELECT id FROM plan_versions)) WITH CHECK(plan_id IN (SELECT id FROM plan_versions));
ALTER TABLE approvals ENABLE ROW LEVEL SECURITY;
CREATE POLICY approval_read ON approvals FOR SELECT TO undergrid_app USING(plan_id IN (SELECT id FROM plan_versions));
CREATE POLICY approval_insert ON approvals FOR INSERT TO undergrid_app WITH CHECK(principal_id=actor_principal() AND agent_id=actor_agent() AND plan_id IN (SELECT id FROM plan_versions));
GRANT SELECT,INSERT ON plan_versions,tasks,approvals TO undergrid_app;
GRANT UPDATE(state) ON plan_versions TO undergrid_app;
GRANT UPDATE(state,claim_epoch,version,progress_deadline) ON tasks TO undergrid_app;
CREATE FUNCTION principal_task_load() RETURNS bigint LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT count(*) FROM tasks WHERE assigned_principal_id=actor_principal() AND state IN ('accepted','in_progress','submitted','changes_requested','at_risk')
 $$;
REVOKE ALL ON FUNCTION principal_task_load() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION principal_task_load() TO undergrid_app;
CREATE FUNCTION current_policy_version(p uuid) RETURNS bigint LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT version FROM principals WHERE id=p AND status='active' AND (policy->>'expires_at')::timestamptz>now()
 $$;
REVOKE ALL ON FUNCTION current_policy_version(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION current_policy_version(uuid) TO undergrid_app;
GRANT SELECT ON plan_versions,tasks,approvals TO undergrid_worker;
CREATE POLICY plans_worker ON plan_versions FOR SELECT TO undergrid_worker USING(true);
CREATE POLICY tasks_worker ON tasks FOR SELECT TO undergrid_worker USING(true);
CREATE POLICY approvals_worker ON approvals FOR SELECT TO undergrid_worker USING(true);
CREATE FUNCTION lock_plan_authorities(target uuid) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
BEGIN
 PERFORM p.id FROM principals p WHERE p.id IN (
  SELECT actor_principal()
  UNION SELECT value::uuid FROM plan_versions v CROSS JOIN LATERAL jsonb_array_elements_text(v.specification->'plan'->'required_principals')
  WHERE v.id=target AND swarm_member(v.swarm_id)
 ) ORDER BY p.id FOR UPDATE;
END $$;
REVOKE ALL ON FUNCTION lock_plan_authorities(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION lock_plan_authorities(uuid) TO undergrid_app;
