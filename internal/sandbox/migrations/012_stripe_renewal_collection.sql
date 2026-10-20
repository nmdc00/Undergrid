SET search_path=undergrid_sandbox,pg_catalog;

CREATE TABLE sandbox_stripe_billing_setups (
 id uuid PRIMARY KEY, subscription_id uuid NOT NULL UNIQUE REFERENCES sandbox_subscriptions(id),
 actor_id uuid NOT NULL REFERENCES sandbox_actors(id), consent_hash text NOT NULL,
 state text NOT NULL DEFAULT 'requested' CHECK(state IN('requested','customer_unknown','customer_ready','session_unknown','pending','ready')),
 account text, price_id text, customer_id text UNIQUE, session_id text UNIQUE, setup_url text,
 setup_intent_id text UNIQUE, payment_method_id text, evidence jsonb,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK(state<>'ready' OR (account IS NOT NULL AND price_id IS NOT NULL AND customer_id IS NOT NULL AND session_id IS NOT NULL AND setup_intent_id IS NOT NULL AND payment_method_id IS NOT NULL AND evidence IS NOT NULL))
);
ALTER TABLE sandbox_stripe_billing_setups ENABLE ROW LEVEL SECURITY;
CREATE POLICY billing_setup_owner ON sandbox_stripe_billing_setups TO undergrid_sandbox_app USING(actor_id=actor()) WITH CHECK(actor_id=actor());
CREATE POLICY billing_setup_worker ON sandbox_stripe_billing_setups TO undergrid_sandbox_worker USING(true) WITH CHECK(true);
GRANT SELECT,INSERT ON sandbox_stripe_billing_setups TO undergrid_sandbox_app;
GRANT SELECT ON sandbox_stripe_billing_setups TO undergrid_sandbox_worker;
GRANT UPDATE(state,account,price_id,customer_id,session_id,setup_url,setup_intent_id,payment_method_id,evidence,updated_at) ON sandbox_stripe_billing_setups TO undergrid_sandbox_worker;

CREATE TABLE sandbox_stripe_renewal_claims (
 payment_id uuid PRIMARY KEY REFERENCES sandbox_payments(id), setup_id uuid NOT NULL REFERENCES sandbox_stripe_billing_setups(id),
 quote_hash text NOT NULL, provider_key text NOT NULL UNIQUE,
 state text NOT NULL CHECK(state IN('invoice_unknown','draft','item_unknown','ready','finalize_unknown','open','pay_unknown','settled')),
 invoice_id text UNIQUE, item_id text UNIQUE, payment_intent_id text UNIQUE, charge_id text UNIQUE,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
ALTER TABLE sandbox_stripe_renewal_claims ENABLE ROW LEVEL SECURITY;
CREATE POLICY renewal_claim_worker ON sandbox_stripe_renewal_claims TO undergrid_sandbox_worker USING(true) WITH CHECK(true);
GRANT SELECT,INSERT ON sandbox_stripe_renewal_claims TO undergrid_sandbox_worker;
GRANT UPDATE(state,invoice_id,item_id,payment_intent_id,charge_id,updated_at) ON sandbox_stripe_renewal_claims TO undergrid_sandbox_worker;
