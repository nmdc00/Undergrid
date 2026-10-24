ALTER TABLE undergrid_sandbox.sandbox_payments ADD UNIQUE(id,actor_id);
ALTER TABLE undergrid_sandbox.sandbox_credentials ADD UNIQUE(id,actor_id);
CREATE TABLE undergrid_sandbox.sandbox_refunds (
 id uuid PRIMARY KEY, actor_id uuid NOT NULL REFERENCES undergrid_sandbox.sandbox_actors,
 payment_id uuid NOT NULL, credential_id uuid NOT NULL, asset text NOT NULL, amount numeric(78,0) NOT NULL CHECK(amount>0),
 refund_hash text NOT NULL, state text NOT NULL DEFAULT 'requested' CHECK(state IN('requested','authorized','pending','unknown','succeeded','failed')),
 policy_version bigint, authority_proof jsonb, provider_key text NOT NULL UNIQUE, provider_reference text,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(payment_id,actor_id) REFERENCES undergrid_sandbox.sandbox_payments(id,actor_id),
 FOREIGN KEY(credential_id,actor_id) REFERENCES undergrid_sandbox.sandbox_credentials(id,actor_id)
);
ALTER TABLE undergrid_sandbox.sandbox_refunds ENABLE ROW LEVEL SECURITY;
CREATE POLICY refund_actor ON undergrid_sandbox.sandbox_refunds TO undergrid_sandbox_app USING(actor_id=undergrid_sandbox.actor()) WITH CHECK(actor_id=undergrid_sandbox.actor());
CREATE POLICY refund_worker ON undergrid_sandbox.sandbox_refunds TO undergrid_sandbox_worker USING(true) WITH CHECK(true);
GRANT SELECT,INSERT ON undergrid_sandbox.sandbox_refunds TO undergrid_sandbox_app;
GRANT UPDATE(state,policy_version,authority_proof,updated_at) ON undergrid_sandbox.sandbox_refunds TO undergrid_sandbox_app;
GRANT SELECT ON undergrid_sandbox.sandbox_refunds TO undergrid_sandbox_worker;
GRANT UPDATE(state,provider_reference,updated_at) ON undergrid_sandbox.sandbox_refunds TO undergrid_sandbox_worker;
ALTER TABLE undergrid_sandbox.sandbox_idempotency ALTER COLUMN payment_id DROP NOT NULL;
ALTER TABLE undergrid_sandbox.sandbox_idempotency ADD COLUMN refund_id uuid REFERENCES undergrid_sandbox.sandbox_refunds;
ALTER TABLE undergrid_sandbox.sandbox_idempotency ADD CHECK(num_nonnulls(payment_id,refund_id)=1);
ALTER TABLE undergrid_sandbox.sandbox_idempotency ADD CHECK((operation IN('refund-request','refund-authorize') AND refund_id IS NOT NULL) OR (operation NOT IN('refund-request','refund-authorize') AND payment_id IS NOT NULL));
ALTER TABLE undergrid_sandbox.sandbox_provider_events ADD COLUMN refund_id uuid REFERENCES undergrid_sandbox.sandbox_refunds;
ALTER TABLE undergrid_sandbox.sandbox_effects ADD COLUMN refund_id uuid REFERENCES undergrid_sandbox.sandbox_refunds;
CREATE UNIQUE INDEX one_effect_per_refund ON undergrid_sandbox.sandbox_effects(refund_id) WHERE refund_id IS NOT NULL;
ALTER TABLE undergrid_sandbox.sandbox_effects ADD CHECK(refund_id IS NULL OR kind='refund');
CREATE POLICY effect_actor_read ON undergrid_sandbox.sandbox_effects FOR SELECT TO undergrid_sandbox_app USING(payment_id IN(SELECT id FROM undergrid_sandbox.sandbox_payments WHERE actor_id=undergrid_sandbox.actor()));
GRANT SELECT ON undergrid_sandbox.sandbox_effects TO undergrid_sandbox_app;
