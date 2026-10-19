-- Owner API may request setup, but only the isolated provider worker can
-- establish customer/session/payment-method readiness from canonical evidence.
REVOKE INSERT ON undergrid_sandbox.sandbox_stripe_billing_setups FROM undergrid_sandbox_app;
GRANT INSERT(id,subscription_id,actor_id,consent_hash) ON undergrid_sandbox.sandbox_stripe_billing_setups TO undergrid_sandbox_app;
