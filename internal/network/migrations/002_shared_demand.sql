SET search_path=undergrid,pg_catalog;
CREATE ROLE undergrid_worker NOLOGIN NOSUPERUSER NOBYPASSRLS;
GRANT USAGE ON SCHEMA undergrid TO undergrid_worker;
CREATE TABLE opportunities (
 id uuid PRIMARY KEY, recipient_agent_id uuid NOT NULL REFERENCES agents,
 source_intent_id uuid NOT NULL REFERENCES intents, candidate_intent_id uuid NOT NULL REFERENCES intents,
 kind text NOT NULL CHECK(kind='shared_demand'), state text NOT NULL DEFAULT 'suggested' CHECK(state IN ('suggested','dismissed','stale','responded')),
 input_hash text NOT NULL, evidence jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(recipient_agent_id,kind,input_hash), CHECK(source_intent_id<>candidate_intent_id)
);
ALTER TABLE opportunities ENABLE ROW LEVEL SECURITY;
CREATE POLICY opportunities_recipient ON opportunities TO undergrid_app USING(recipient_agent_id=actor_agent()) WITH CHECK(false);
GRANT SELECT ON opportunities TO undergrid_app;
CREATE POLICY opportunities_worker ON opportunities TO undergrid_worker USING(true) WITH CHECK(true);
GRANT SELECT,INSERT,UPDATE ON opportunities TO undergrid_worker;
CREATE FUNCTION public_intents() RETURNS TABLE(id uuid,principal_id uuid,agent_id uuid,version bigint,projection jsonb,content_hash text)
 LANGUAGE sql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT i.id,i.principal_id,i.agent_id,i.version,r.public_projection,r.content_hash
 FROM intents i JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision
 JOIN principals p ON p.id=i.principal_id JOIN agents a ON a.id=i.agent_id
 WHERE i.visibility='redacted_public' AND i.state='active' AND i.expires_at>now() AND p.status='active' AND a.status='active'
 $$;
REVOKE ALL ON FUNCTION public_intents() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public_intents() TO undergrid_app,undergrid_worker;
GRANT SELECT ON principals,agents,intents,intent_revisions TO undergrid_worker;
GRANT UPDATE(version) ON principals TO undergrid_worker;
GRANT UPDATE(inbox_sequence) ON agents TO undergrid_worker;
GRANT UPDATE(state,version,watch_enabled,last_scan,next_scan) ON intents TO undergrid_worker;
GRANT INSERT ON events,inbox TO undergrid_worker;
CREATE POLICY principals_worker ON principals TO undergrid_worker USING(true) WITH CHECK(true);
CREATE POLICY agents_worker ON agents TO undergrid_worker USING(true) WITH CHECK(true);
CREATE POLICY intents_worker ON intents TO undergrid_worker USING(true) WITH CHECK(true);
CREATE POLICY revisions_worker ON intent_revisions TO undergrid_worker USING(true);
CREATE POLICY events_worker ON events FOR INSERT TO undergrid_worker WITH CHECK(true);
CREATE POLICY inbox_worker ON inbox FOR INSERT TO undergrid_worker WITH CHECK(true);
CREATE SCHEMA undergrid_queue;
REVOKE ALL ON SCHEMA undergrid_queue FROM PUBLIC;
GRANT USAGE ON SCHEMA undergrid_queue TO undergrid_app,undergrid_worker;
