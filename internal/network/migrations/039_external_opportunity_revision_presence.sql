SET search_path=undergrid,pg_catalog;

-- SQL CHECK accepts UNKNOWN: 038's revision > 0 rejects nonpositive integers,
-- but cannot by itself reject NULL. Keep the applied migration immutable and
-- require the pinned intention revision for every external observation row.
ALTER TABLE opportunities ADD CONSTRAINT opportunities_external_revision_present
CHECK(external_observation_id IS NULL OR external_intent_revision IS NOT NULL);
