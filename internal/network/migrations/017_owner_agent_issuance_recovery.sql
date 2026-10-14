SET search_path=undergrid,pg_catalog;

-- Owner creation happens before the new agent exists, so its command identity
-- cannot use the agent-only actor_id key. Keep one table with disjoint keys.
ALTER TABLE idempotency DROP CONSTRAINT idempotency_pkey;
ALTER TABLE idempotency ALTER COLUMN actor_id DROP NOT NULL;
ALTER TABLE idempotency ADD COLUMN owner_principal_id uuid REFERENCES principals;
ALTER TABLE idempotency ADD CONSTRAINT idempotency_one_actor CHECK(num_nonnulls(actor_id,owner_principal_id)=1);
CREATE UNIQUE INDEX idempotency_agent_command_key ON idempotency(actor_id,operation,key) WHERE actor_id IS NOT NULL;
CREATE UNIQUE INDEX idempotency_owner_command_key ON idempotency(owner_principal_id,operation,key) WHERE owner_principal_id IS NOT NULL;

DROP POLICY idempotency_access ON idempotency;
CREATE POLICY idempotency_access ON idempotency
 USING (
   (actor_id=actor_agent() AND owner_principal_id IS NULL)
   OR (actor_id IS NULL AND owner_principal_id=actor_principal()
       AND actor_agent() IS NULL
       AND current_setting('undergrid.owner_verified',true)='true')
 )
 WITH CHECK (
   (actor_id=actor_agent() AND owner_principal_id IS NULL)
   OR (actor_id IS NULL AND owner_principal_id=actor_principal()
       AND actor_agent() IS NULL
       AND current_setting('undergrid.owner_verified',true)='true')
 );
