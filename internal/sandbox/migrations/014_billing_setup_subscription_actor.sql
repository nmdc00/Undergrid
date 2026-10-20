-- The request role chooses its own actor under RLS, but must not consume or
-- provision the unique billing slot belonging to a different subscription owner.
SET search_path=undergrid_sandbox,pg_catalog,pg_temp;
ALTER TABLE sandbox_stripe_billing_setups
 ADD CONSTRAINT billing_setup_subscription_actor_fk
 FOREIGN KEY(subscription_id,actor_id) REFERENCES sandbox_subscriptions(id,actor_id);
