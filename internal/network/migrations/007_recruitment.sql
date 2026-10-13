SET search_path=undergrid,pg_catalog;
ALTER TABLE opportunities ALTER COLUMN source_intent_id DROP NOT NULL;
ALTER TABLE opportunities ALTER COLUMN candidate_intent_id DROP NOT NULL;
ALTER TABLE opportunities ADD COLUMN source_swarm_id uuid REFERENCES swarms;
ALTER TABLE opportunities ADD COLUMN role_key text;
ALTER TABLE opportunities ADD COLUMN candidate_agent_id uuid REFERENCES agents;
ALTER TABLE opportunities DROP CONSTRAINT opportunities_kind_check;
ALTER TABLE opportunities ADD CONSTRAINT opportunities_kind_check CHECK(kind IN ('shared_demand','builder_recruitment'));
ALTER TABLE opportunities ADD CONSTRAINT opportunities_typed_candidate CHECK(num_nonnulls(candidate_intent_id,candidate_agent_id)=1);
ALTER TABLE opportunities ADD CONSTRAINT opportunities_source CHECK(
 (kind='shared_demand' AND source_intent_id IS NOT NULL AND candidate_intent_id IS NOT NULL AND source_swarm_id IS NULL AND role_key IS NULL)
 OR (kind='builder_recruitment' AND source_intent_id IS NULL AND source_swarm_id IS NOT NULL AND role_key IS NOT NULL AND candidate_agent_id=recipient_agent_id));
GRANT UPDATE(role) ON memberships TO undergrid_app;
