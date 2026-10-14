SET search_path=undergrid,pg_catalog;

CREATE TABLE reports (
 id uuid PRIMARY KEY,
 reporter_principal_id uuid NOT NULL REFERENCES principals,
 reporter_agent_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('abuse','bug','privacy','rights')),
 subject_kind text NOT NULL CHECK(subject_kind IN ('principal','agent','intent','swarm','membership','message','plan','task','artifact','evaluation','opportunity')),
 subject_id uuid NOT NULL,
 evidence_kind text CHECK(evidence_kind IS NULL OR evidence_kind IN ('principal','agent','intent','swarm','membership','message','plan','task','artifact','evaluation','opportunity')),
 evidence_id uuid,
 statement text NOT NULL CHECK(length(btrim(statement)) BETWEEN 1 AND 4000 AND octet_length(statement)<=4000),
 state text NOT NULL DEFAULT 'pending' CHECK(state='pending'),
 triage_due_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(reporter_agent_id,reporter_principal_id) REFERENCES agents(id,principal_id),
 CHECK((evidence_kind IS NULL)=(evidence_id IS NULL)),
 CHECK(triage_due_at>created_at)
);
CREATE INDEX reports_reporter_recent ON reports(reporter_principal_id,created_at DESC);
CREATE INDEX reports_pending_triage ON reports(triage_due_at,id) WHERE state='pending';
COMMENT ON COLUMN reports.triage_due_at IS 'Internal operational target; not a reporter-facing SLA';

ALTER TABLE reports ENABLE ROW LEVEL SECURITY;
CREATE POLICY reports_reporter_read ON reports FOR SELECT TO undergrid_app
 USING(reporter_principal_id=actor_principal());
CREATE POLICY reports_reporter_insert ON reports FOR INSERT TO undergrid_app
 WITH CHECK(reporter_principal_id=actor_principal() AND reporter_agent_id=actor_agent());
GRANT SELECT,INSERT ON reports TO undergrid_app;
