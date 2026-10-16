SET search_path=undergrid,pg_catalog;
ALTER TABLE tasks ADD COLUMN cure_deadline timestamptz;
GRANT UPDATE(cure_deadline) ON tasks TO undergrid_app;
GRANT UPDATE(state,version,claim_epoch,progress_deadline,cure_deadline) ON tasks TO undergrid_worker;
GRANT UPDATE(state) ON plan_versions TO undergrid_worker;
GRANT UPDATE(state,version) ON memberships TO undergrid_worker;
CREATE POLICY tasks_timer ON tasks FOR UPDATE TO undergrid_worker USING(true) WITH CHECK(true);
CREATE POLICY plans_timer ON plan_versions FOR UPDATE TO undergrid_worker USING(true) WITH CHECK(true);
CREATE POLICY memberships_timer ON memberships FOR UPDATE TO undergrid_worker USING(true) WITH CHECK(true);
GRANT INSERT ON events TO undergrid_worker;
CREATE POLICY events_timer ON events FOR INSERT TO undergrid_worker WITH CHECK(true);
-- Applications made before this migration get the same explicit response window.
UPDATE memberships SET respond_by=created_at+interval '72 hours' WHERE state='applied' AND respond_by IS NULL;
ALTER TABLE memberships ALTER COLUMN respond_by SET DEFAULT (now()+interval '72 hours');
CREATE INDEX memberships_timer_due ON memberships(respond_by) WHERE state IN ('applied','offered');
CREATE INDEX plans_timer_due ON plan_versions(respond_by) WHERE state='proposed';
CREATE INDEX tasks_timer_due ON tasks(progress_deadline,cure_deadline) WHERE state IN ('accepted','in_progress','changes_requested','at_risk');

-- Existing accepted assignments receive a new execution clock on upgrade;
-- proposal assignments remain unstarted until exact plan acceptance.
UPDATE tasks t SET progress_deadline=now()+(t.specification->>'progress_interval_seconds')::int*interval '1 second',version=t.version+1
FROM plan_versions p WHERE p.id=t.plan_id AND p.state='accepted' AND t.state='accepted' AND t.progress_deadline IS NULL;
GRANT UPDATE(sequence) ON swarms TO undergrid_worker;
CREATE POLICY swarms_timer_lock ON swarms FOR UPDATE TO undergrid_worker USING(true) WITH CHECK(true);
