SET search_path=undergrid_sandbox,pg_catalog;

CREATE TABLE sandbox_usdc_claims (
 payment_id uuid PRIMARY KEY REFERENCES sandbox_payments(id),
 chain_id bigint NOT NULL CHECK(chain_id=84532), token text NOT NULL CHECK(token='0x036cbd53842c5426634e7929541ec2318f3dcf7e'),
 payer text NOT NULL CHECK(payer ~ '^0x[a-f0-9]{40}$'), nonce text NOT NULL CHECK(nonce ~ '^0x[a-f0-9]{64}$'),
 quote_hash text NOT NULL, authorization_hash text NOT NULL, provider_key text NOT NULL UNIQUE,
 start_block numeric(78,0) CHECK(start_block>=0), scan_block numeric(78,0) NOT NULL DEFAULT 0 CHECK(scan_block>=0), created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(chain_id,token,payer,nonce)
);
CREATE TABLE sandbox_usdc_observations (
 sequence bigserial PRIMARY KEY, id uuid NOT NULL UNIQUE, payment_id uuid NOT NULL REFERENCES sandbox_payments(id),
 transaction_hash text NOT NULL CHECK(transaction_hash ~ '^0x[a-f0-9]{64}$'),
 block_number numeric(78,0) NOT NULL CHECK(block_number>=0), block_hash text NOT NULL CHECK(block_hash='' OR block_hash ~ '^0x[a-f0-9]{64}$'),
 finality text NOT NULL CHECK(finality IN('candidate','provisional','safe','finalized','orphaned')),
 evidence jsonb NOT NULL CHECK(jsonb_typeof(evidence)='object'), checked_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sandbox_usdc_payment_observations ON sandbox_usdc_observations(payment_id,sequence DESC);
ALTER TABLE sandbox_effects ADD COLUMN reverses_fact_id text;
ALTER TABLE sandbox_effects ADD CHECK(reverses_fact_id IS NULL OR kind='reorg');
ALTER TABLE sandbox_effects ADD FOREIGN KEY(provider,account,environment,reverses_fact_id) REFERENCES sandbox_effects(provider,account,environment,fact_id);

ALTER TABLE sandbox_usdc_claims ENABLE ROW LEVEL SECURITY;
ALTER TABLE sandbox_usdc_observations ENABLE ROW LEVEL SECURITY;
CREATE POLICY usdc_claim_worker ON sandbox_usdc_claims TO undergrid_sandbox_worker USING(true) WITH CHECK(true);
CREATE POLICY usdc_observation_worker ON sandbox_usdc_observations TO undergrid_sandbox_worker USING(true) WITH CHECK(true);
CREATE POLICY usdc_observation_actor ON sandbox_usdc_observations FOR SELECT TO undergrid_sandbox_app
 USING(payment_id IN(SELECT id FROM sandbox_payments WHERE actor_id=actor()));
GRANT SELECT,INSERT ON sandbox_usdc_claims,sandbox_usdc_observations TO undergrid_sandbox_worker;
GRANT UPDATE(scan_block) ON sandbox_usdc_claims TO undergrid_sandbox_worker;
GRANT USAGE ON SEQUENCE sandbox_usdc_observations_sequence_seq TO undergrid_sandbox_worker;
GRANT SELECT(sequence,id,payment_id,transaction_hash,block_number,block_hash,finality,checked_at) ON sandbox_usdc_observations TO undergrid_sandbox_app;

-- Chain facts remain immutable even across forks. Only a payment's latest
-- finalized canonical observation makes its exact tx/block facts refundable.
-- Re-canonicalization reactivates the same fact instead of inventing a transfer.
CREATE VIEW sandbox_refundable_effects WITH(security_invoker=true) AS
 SELECT e.* FROM sandbox_effects e
 WHERE e.provider<>'x402_test' OR EXISTS(
  SELECT 1 FROM sandbox_payments p
  JOIN LATERAL(SELECT o.transaction_hash,o.block_hash,o.finality FROM sandbox_usdc_observations o WHERE o.payment_id=p.id ORDER BY o.sequence DESC LIMIT 1) latest ON true
  WHERE p.id=e.payment_id AND p.state='settled' AND latest.finality='finalized'
   AND latest.transaction_hash=e.evidence->>'transaction_hash' AND latest.block_hash=e.evidence->>'block_hash'
 );
GRANT SELECT ON sandbox_refundable_effects TO undergrid_sandbox_app,undergrid_sandbox_worker;
