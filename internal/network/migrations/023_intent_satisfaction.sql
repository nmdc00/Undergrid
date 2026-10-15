SET search_path=undergrid,pg_catalog;

-- Satisfaction is a terminal intention outcome, distinct from artifact acceptance.
ALTER TABLE intents DROP CONSTRAINT intents_state_check;
ALTER TABLE intents ADD CONSTRAINT intents_state_check
 CHECK (state IN ('active','paused','expired','withdrawn','satisfied')) NOT VALID;
ALTER TABLE intents VALIDATE CONSTRAINT intents_state_check;
ALTER TABLE intents ADD CONSTRAINT satisfied_intents_stop_monitoring
 CHECK (state <> 'satisfied' OR NOT watch_enabled) NOT VALID;
ALTER TABLE intents VALIDATE CONSTRAINT satisfied_intents_stop_monitoring;
