ALTER TABLE undergrid_sandbox.sandbox_idempotency ENABLE ROW LEVEL SECURITY;
CREATE POLICY retry_actor ON undergrid_sandbox.sandbox_idempotency TO undergrid_sandbox_app
 USING(actor_id=undergrid_sandbox.actor()) WITH CHECK(actor_id=undergrid_sandbox.actor());
