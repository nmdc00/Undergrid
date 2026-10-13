DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='undergrid_sandbox_app') THEN CREATE ROLE undergrid_sandbox_app NOLOGIN NOSUPERUSER NOBYPASSRLS; END IF;
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='undergrid_sandbox_worker') THEN CREATE ROLE undergrid_sandbox_worker NOLOGIN NOSUPERUSER NOBYPASSRLS; END IF;
END $$;
CREATE SCHEMA undergrid_sandbox;
REVOKE ALL ON SCHEMA undergrid_sandbox FROM PUBLIC;
GRANT USAGE ON SCHEMA undergrid_sandbox TO undergrid_sandbox_app,undergrid_sandbox_worker;
CREATE TABLE undergrid_sandbox.sandbox_actors (
 id uuid PRIMARY KEY, owner_hash text NOT NULL UNIQUE, policy jsonb NOT NULL DEFAULT '{}', version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE undergrid_sandbox.sandbox_credentials (
 id uuid PRIMARY KEY, actor_id uuid NOT NULL REFERENCES undergrid_sandbox.sandbox_actors, agent_name text NOT NULL, digest text NOT NULL UNIQUE,
 audience text NOT NULL CHECK(audience='undergrid-sandbox'), scopes text[] NOT NULL, expires_at timestamptz NOT NULL, revoked_at timestamptz
);
CREATE TABLE undergrid_sandbox.sandbox_payments (
 id uuid PRIMARY KEY, actor_id uuid NOT NULL REFERENCES undergrid_sandbox.sandbox_actors, credential_id uuid NOT NULL REFERENCES undergrid_sandbox.sandbox_credentials,
 quote jsonb NOT NULL, quote_hash text NOT NULL, asset text NOT NULL, amount numeric(78,0) NOT NULL CHECK(amount>0),fee numeric(78,0) NOT NULL CHECK(fee>=0),
 state text NOT NULL DEFAULT 'quoted' CHECK(state IN('quoted','authorized','submitting','pending','unknown','settled','failed','canceled')),
 policy_version bigint, authority_proof jsonb, reservation_period date, provider_key text NOT NULL UNIQUE, provider_reference text,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE undergrid_sandbox.sandbox_idempotency (
 actor_id uuid NOT NULL REFERENCES undergrid_sandbox.sandbox_actors,credential_id uuid NOT NULL REFERENCES undergrid_sandbox.sandbox_credentials,operation text NOT NULL,key text NOT NULL,request_hash text NOT NULL,payment_id uuid NOT NULL REFERENCES undergrid_sandbox.sandbox_payments,
 PRIMARY KEY(credential_id,operation,key)
);
CREATE TABLE undergrid_sandbox.sandbox_provider_events (
 provider text NOT NULL,account text NOT NULL,environment text NOT NULL CHECK(environment='test'),event_id text NOT NULL,payment_id uuid NOT NULL REFERENCES undergrid_sandbox.sandbox_payments,evidence jsonb NOT NULL,fact_id text NOT NULL,kind text NOT NULL,asset text NOT NULL,amount numeric(78,0) NOT NULL,received_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(provider,account,environment,event_id)
);
CREATE TABLE undergrid_sandbox.sandbox_effects (
 provider text NOT NULL,account text NOT NULL,environment text NOT NULL CHECK(environment='test'),fact_id text NOT NULL,payment_id uuid NOT NULL REFERENCES undergrid_sandbox.sandbox_payments,
 kind text NOT NULL CHECK(kind IN('charge','refund','fee','reorg')),asset text NOT NULL,amount numeric(78,0) NOT NULL CHECK(amount>0),evidence jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(provider,account,environment,fact_id)
);
CREATE FUNCTION undergrid_sandbox.actor() RETURNS uuid LANGUAGE sql STABLE AS $$ SELECT nullif(current_setting('undergrid_sandbox.actor',true),'')::uuid $$;
CREATE FUNCTION undergrid_sandbox.identify_owner() RETURNS uuid LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,undergrid_sandbox AS $$ SELECT id FROM sandbox_actors WHERE owner_hash=current_setting('undergrid_sandbox.owner',true) $$;
CREATE FUNCTION undergrid_sandbox.identify_token(d text) RETURNS uuid LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,undergrid_sandbox AS $$ SELECT actor_id FROM sandbox_credentials WHERE digest=d AND audience='undergrid-sandbox' AND revoked_at IS NULL AND expires_at>now() $$;
REVOKE ALL ON FUNCTION undergrid_sandbox.identify_owner(),undergrid_sandbox.identify_token(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION undergrid_sandbox.identify_owner(),undergrid_sandbox.identify_token(text) TO undergrid_sandbox_app;
ALTER TABLE undergrid_sandbox.sandbox_actors ENABLE ROW LEVEL SECURITY;
CREATE POLICY actor_access ON undergrid_sandbox.sandbox_actors TO undergrid_sandbox_app USING(id=undergrid_sandbox.actor() OR owner_hash=current_setting('undergrid_sandbox.owner',true)) WITH CHECK(id=undergrid_sandbox.actor() OR owner_hash=current_setting('undergrid_sandbox.owner',true));
CREATE POLICY worker_actor ON undergrid_sandbox.sandbox_actors TO undergrid_sandbox_worker USING(true);
ALTER TABLE undergrid_sandbox.sandbox_credentials ENABLE ROW LEVEL SECURITY;
CREATE POLICY credential_access ON undergrid_sandbox.sandbox_credentials TO undergrid_sandbox_app USING(actor_id=undergrid_sandbox.actor()) WITH CHECK(actor_id=undergrid_sandbox.actor());
CREATE POLICY worker_credential ON undergrid_sandbox.sandbox_credentials TO undergrid_sandbox_worker USING(true);
ALTER TABLE undergrid_sandbox.sandbox_payments ENABLE ROW LEVEL SECURITY;
CREATE POLICY payment_access ON undergrid_sandbox.sandbox_payments TO undergrid_sandbox_app USING(actor_id=undergrid_sandbox.actor()) WITH CHECK(actor_id=undergrid_sandbox.actor());
CREATE POLICY worker_payment ON undergrid_sandbox.sandbox_payments TO undergrid_sandbox_worker USING(true) WITH CHECK(true);
ALTER TABLE undergrid_sandbox.sandbox_idempotency ENABLE ROW LEVEL SECURITY;
CREATE POLICY idem_access ON undergrid_sandbox.sandbox_idempotency TO undergrid_sandbox_app USING(actor_id=undergrid_sandbox.actor()) WITH CHECK(actor_id=undergrid_sandbox.actor());
ALTER TABLE undergrid_sandbox.sandbox_provider_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY worker_events ON undergrid_sandbox.sandbox_provider_events TO undergrid_sandbox_worker USING(true) WITH CHECK(true);
ALTER TABLE undergrid_sandbox.sandbox_effects ENABLE ROW LEVEL SECURITY;
CREATE POLICY worker_effects ON undergrid_sandbox.sandbox_effects TO undergrid_sandbox_worker USING(true) WITH CHECK(true);
GRANT SELECT,INSERT ON undergrid_sandbox.sandbox_actors,undergrid_sandbox.sandbox_credentials,undergrid_sandbox.sandbox_payments,undergrid_sandbox.sandbox_idempotency TO undergrid_sandbox_app;
GRANT UPDATE(policy,version) ON undergrid_sandbox.sandbox_actors TO undergrid_sandbox_app;
GRANT UPDATE(revoked_at) ON undergrid_sandbox.sandbox_credentials TO undergrid_sandbox_app;
GRANT UPDATE(state,policy_version,authority_proof,reservation_period,updated_at) ON undergrid_sandbox.sandbox_payments TO undergrid_sandbox_app;
GRANT SELECT ON undergrid_sandbox.sandbox_actors,undergrid_sandbox.sandbox_credentials,undergrid_sandbox.sandbox_payments,undergrid_sandbox.sandbox_effects,undergrid_sandbox.sandbox_provider_events TO undergrid_sandbox_worker;
GRANT UPDATE(state,provider_reference,updated_at) ON undergrid_sandbox.sandbox_payments TO undergrid_sandbox_worker;
GRANT UPDATE(id) ON undergrid_sandbox.sandbox_actors TO undergrid_sandbox_worker;
GRANT INSERT ON undergrid_sandbox.sandbox_effects,undergrid_sandbox.sandbox_provider_events TO undergrid_sandbox_worker;
