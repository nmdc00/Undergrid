SET search_path=undergrid,pg_catalog;

-- A selected lead is not necessarily a persisted opportunity, so 040's
-- opportunity-only guard cannot authorize this command. Reuse the established
-- fit projection after locking its source, without exporting private config.
CREATE FUNCTION lock_partial_satisfaction_source(target uuid,selected_lead uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,undergrid,pg_temp AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM undergrid.intents i JOIN undergrid.principals p ON p.id=i.principal_id JOIN undergrid.agents a ON a.id=i.agent_id
  WHERE i.id=target AND i.principal_id=undergrid.actor_principal() AND i.agent_id=undergrid.actor_agent()
  AND i.state='active' AND i.watch_enabled AND i.expires_at>clock_timestamp() AND p.status='active' AND a.status='active') THEN RETURN false; END IF;
 PERFORM 1 FROM undergrid.external_observations o JOIN undergrid.external_sources s ON s.id=o.source_id
  WHERE o.id=selected_lead AND o.kind='external_capability' FOR SHARE OF o,s NOWAIT;
 IF NOT FOUND THEN RETURN false; END IF;
 RETURN EXISTS(SELECT 1 FROM undergrid.external_fit_source(target,selected_lead) l
  JOIN undergrid.external_observations o ON o.id=l.id JOIN undergrid.external_sources s ON s.id=l.source_id
  WHERE l.origin='external_capability' AND o.checked_at>clock_timestamp()-interval '24 hours' AND s.last_success>clock_timestamp()-interval '24 hours');
END;
$$;
REVOKE ALL ON FUNCTION lock_partial_satisfaction_source(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION lock_partial_satisfaction_source(uuid,uuid) TO undergrid_app;
