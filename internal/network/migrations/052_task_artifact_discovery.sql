SET search_path=undergrid,pg_catalog;

-- Bounded keyset discovery for the returning assignee; no policy changes.
CREATE INDEX artifacts_task_agent_id ON artifacts(task_id,agent_id,id);
