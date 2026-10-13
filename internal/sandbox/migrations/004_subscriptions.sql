CREATE TABLE undergrid_sandbox.sandbox_subscriptions (
 id uuid PRIMARY KEY, actor_id uuid NOT NULL REFERENCES undergrid_sandbox.sandbox_actors,
 credential_id uuid NOT NULL, offer jsonb NOT NULL, maximum numeric(78,0) NOT NULL CHECK(maximum>0),
 period_seconds integer NOT NULL CHECK(period_seconds BETWEEN 86400 AND 7776000),
 renewal_count integer NOT NULL CHECK(renewal_count BETWEEN 1 AND 2),
 renewals_reserved integer NOT NULL DEFAULT 0 CHECK(renewals_reserved BETWEEN 0 AND renewal_count),
 expires_at timestamptz NOT NULL, next_due_at timestamptz NOT NULL,
 consent_hash text NOT NULL UNIQUE, state text NOT NULL DEFAULT 'active' CHECK(state IN('active','canceled','exhausted')),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(credential_id,actor_id) REFERENCES undergrid_sandbox.sandbox_credentials(id,actor_id), UNIQUE(id,actor_id)
);
ALTER TABLE undergrid_sandbox.sandbox_subscriptions ENABLE ROW LEVEL SECURITY;
CREATE POLICY subscription_owner ON undergrid_sandbox.sandbox_subscriptions TO undergrid_sandbox_app USING(actor_id=undergrid_sandbox.actor()) WITH CHECK(actor_id=undergrid_sandbox.actor());
CREATE POLICY subscription_worker ON undergrid_sandbox.sandbox_subscriptions TO undergrid_sandbox_worker USING(true) WITH CHECK(true);
GRANT SELECT,INSERT ON undergrid_sandbox.sandbox_subscriptions TO undergrid_sandbox_app;
GRANT UPDATE(state,updated_at) ON undergrid_sandbox.sandbox_subscriptions TO undergrid_sandbox_app;
GRANT SELECT ON undergrid_sandbox.sandbox_subscriptions TO undergrid_sandbox_worker;
GRANT UPDATE(state,renewals_reserved,next_due_at,updated_at) ON undergrid_sandbox.sandbox_subscriptions TO undergrid_sandbox_worker;
ALTER TABLE undergrid_sandbox.sandbox_payments ADD COLUMN subscription_id uuid;
ALTER TABLE undergrid_sandbox.sandbox_payments ADD COLUMN renewal_ordinal integer;
ALTER TABLE undergrid_sandbox.sandbox_payments ADD FOREIGN KEY(subscription_id,actor_id) REFERENCES undergrid_sandbox.sandbox_subscriptions(id,actor_id);
ALTER TABLE undergrid_sandbox.sandbox_payments ADD CHECK((subscription_id IS NULL AND renewal_ordinal IS NULL) OR (subscription_id IS NOT NULL AND renewal_ordinal BETWEEN 1 AND 2));
CREATE UNIQUE INDEX subscription_renewal_once ON undergrid_sandbox.sandbox_payments(subscription_id,renewal_ordinal) WHERE subscription_id IS NOT NULL;
GRANT INSERT ON undergrid_sandbox.sandbox_payments TO undergrid_sandbox_worker;
