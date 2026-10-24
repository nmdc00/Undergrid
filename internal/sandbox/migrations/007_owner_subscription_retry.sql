-- Extend the existing command-result table; owner commands carry no agent
-- credential identity, and their result is an immutable subscription reference.
ALTER TABLE undergrid_sandbox.sandbox_idempotency DROP CONSTRAINT sandbox_idempotency_pkey;
ALTER TABLE undergrid_sandbox.sandbox_idempotency ALTER COLUMN credential_id DROP NOT NULL;
ALTER TABLE undergrid_sandbox.sandbox_idempotency ADD COLUMN subscription_id uuid REFERENCES undergrid_sandbox.sandbox_subscriptions;
ALTER TABLE undergrid_sandbox.sandbox_idempotency DROP CONSTRAINT sandbox_idempotency_check;
ALTER TABLE undergrid_sandbox.sandbox_idempotency DROP CONSTRAINT sandbox_idempotency_check1;
ALTER TABLE undergrid_sandbox.sandbox_idempotency ADD CONSTRAINT idempotency_single_result CHECK(num_nonnulls(payment_id,refund_id,subscription_id)=1);
ALTER TABLE undergrid_sandbox.sandbox_idempotency ADD CONSTRAINT idempotency_result_kind CHECK (
 (operation IN('refund-request','refund-authorize') AND refund_id IS NOT NULL) OR
 (operation='subscription.create' AND subscription_id IS NOT NULL) OR
 (operation NOT IN('refund-request','refund-authorize','subscription.create') AND payment_id IS NOT NULL)
);
ALTER TABLE undergrid_sandbox.sandbox_idempotency ADD CONSTRAINT idempotency_subject CHECK (
 (credential_id IS NOT NULL AND subscription_id IS NULL AND operation <> 'subscription.create') OR
 (credential_id IS NULL AND payment_id IS NULL AND refund_id IS NULL AND subscription_id IS NOT NULL AND operation='subscription.create')
);
CREATE UNIQUE INDEX sandbox_agent_command_key ON undergrid_sandbox.sandbox_idempotency(credential_id,operation,key) WHERE credential_id IS NOT NULL;
CREATE UNIQUE INDEX sandbox_owner_command_key ON undergrid_sandbox.sandbox_idempotency(actor_id,operation,key) WHERE credential_id IS NULL;
