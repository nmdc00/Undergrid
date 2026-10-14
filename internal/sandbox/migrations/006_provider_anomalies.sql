CREATE TABLE undergrid_sandbox.sandbox_anomalies (
 payment_id uuid NOT NULL REFERENCES undergrid_sandbox.sandbox_payments,
 kind text NOT NULL CHECK(kind IN('checkout_binding_mismatch','refund_binding_mismatch')),
 object_id text NOT NULL, evidence_class text NOT NULL CHECK(evidence_class IN('stripe_api_fixture','stripe_api_object')),
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(payment_id,kind,object_id)
);
ALTER TABLE undergrid_sandbox.sandbox_anomalies ENABLE ROW LEVEL SECURITY;
CREATE POLICY anomaly_actor ON undergrid_sandbox.sandbox_anomalies FOR SELECT TO undergrid_sandbox_app USING(payment_id IN(SELECT id FROM undergrid_sandbox.sandbox_payments WHERE actor_id=undergrid_sandbox.actor()));
CREATE POLICY anomaly_worker ON undergrid_sandbox.sandbox_anomalies TO undergrid_sandbox_worker USING(true) WITH CHECK(true);
GRANT SELECT ON undergrid_sandbox.sandbox_anomalies TO undergrid_sandbox_app;
GRANT SELECT,INSERT ON undergrid_sandbox.sandbox_anomalies TO undergrid_sandbox_worker;
