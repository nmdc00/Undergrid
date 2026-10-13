SET search_path=undergrid,pg_catalog;
ALTER TABLE tasks DROP CONSTRAINT tasks_state_check;
ALTER TABLE tasks ADD CONSTRAINT tasks_state_check CHECK(state IN ('open','offered','accepted','in_progress','submitted','accepted_output','changes_requested','at_risk','withdrawn','abandoned','cancelled'));
