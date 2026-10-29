SET search_path=undergrid,pg_catalog;
-- Workers need a clock, never the private reports or their audience metadata.
CREATE FUNCTION swarm_accepted_progress_at(target uuid) RETURNS timestamptz
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT GREATEST(s.created_at,
  (SELECT max(e.reviewed_at) FROM task_progress_evidence e WHERE e.swarm_id=s.id AND e.state='accepted'),
  (SELECT max(e.created_at) FROM events e JOIN tasks t ON t.id=e.aggregate_id JOIN plan_versions p ON p.id=t.plan_id
   WHERE p.swarm_id=s.id AND e.type='task.output_accepted' AND e.facts->>'swarm_id'=s.id::text AND t.state='accepted_output'))
 FROM swarms s WHERE s.id=target
$$;
REVOKE ALL ON FUNCTION swarm_accepted_progress_at(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION swarm_accepted_progress_at(uuid) TO undergrid_worker;
GRANT UPDATE(state,version) ON swarms TO undergrid_worker;
CREATE INDEX swarms_inactivity_active ON swarms(created_at,id) WHERE state='active';
CREATE INDEX swarm_inactivity_warning_episode ON events(aggregate_id,((facts->>'inactivity_since_us')::bigint)) WHERE type='swarm.inactivity_warning';
