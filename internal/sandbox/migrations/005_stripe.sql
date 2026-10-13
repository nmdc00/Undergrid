CREATE TABLE undergrid_sandbox.sandbox_stripe_inbox (
 account text NOT NULL,event_id text NOT NULL,api_version text NOT NULL,event_type text NOT NULL,object_id text NOT NULL,
 payload jsonb NOT NULL,payload_hash text NOT NULL,state text NOT NULL DEFAULT 'pending' CHECK(state IN('pending','processed','blocked')),
 attempts integer NOT NULL DEFAULT 0, last_error text,received_at timestamptz NOT NULL DEFAULT now(),processed_at timestamptz,
 PRIMARY KEY(account,event_id)
);
ALTER TABLE undergrid_sandbox.sandbox_stripe_inbox ENABLE ROW LEVEL SECURITY;
CREATE POLICY stripe_worker ON undergrid_sandbox.sandbox_stripe_inbox TO undergrid_sandbox_worker USING(true) WITH CHECK(true);
GRANT SELECT,INSERT ON undergrid_sandbox.sandbox_stripe_inbox TO undergrid_sandbox_worker;
GRANT UPDATE(state,attempts,last_error,processed_at) ON undergrid_sandbox.sandbox_stripe_inbox TO undergrid_sandbox_worker;
ALTER TABLE undergrid_sandbox.sandbox_payments ADD COLUMN checkout_session_id text UNIQUE;
ALTER TABLE undergrid_sandbox.sandbox_payments ADD COLUMN checkout_url text;
GRANT UPDATE(checkout_session_id,checkout_url) ON undergrid_sandbox.sandbox_payments TO undergrid_sandbox_worker;
