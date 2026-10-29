-- Explicit pg_temp placement prevents temporary relations from taking precedence
-- over the trusted schema inside privileged worker-only helpers. Earlier applied
-- migrations remain immutable; this correction has its own ledger entry.
ALTER FUNCTION undergrid.swarm_accepted_progress_at(uuid) SET search_path=undergrid,pg_catalog,pg_temp;
ALTER FUNCTION undergrid.lock_swarm_deadline_room(uuid) SET search_path=undergrid,pg_catalog,pg_temp;
