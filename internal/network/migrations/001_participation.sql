CREATE ROLE undergrid_app NOLOGIN NOSUPERUSER NOBYPASSRLS;
CREATE SCHEMA undergrid;
REVOKE ALL ON SCHEMA undergrid FROM PUBLIC;
GRANT USAGE ON SCHEMA undergrid TO undergrid_app;
SET search_path = undergrid, pg_catalog;

CREATE TABLE principals (
 id uuid PRIMARY KEY, issuer text NOT NULL, subject text NOT NULL,
 name text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','suspended')),
 terms_version text NOT NULL, policy jsonb NOT NULL DEFAULT '{}',
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(issuer,subject)
);
CREATE TABLE agents (
 id uuid PRIMARY KEY, principal_id uuid NOT NULL REFERENCES principals,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 120),
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','revoked')),
 profile jsonb NOT NULL DEFAULT '{}', inbox_sequence bigint NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(id,principal_id)
);
CREATE TABLE credentials (
 id uuid PRIMARY KEY, agent_id uuid NOT NULL REFERENCES agents,
 digest bytea NOT NULL UNIQUE, scopes text[] NOT NULL,
 audience text NOT NULL CHECK(audience='undergrid-network'),
 expires_at timestamptz NOT NULL, revoked_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE intents (
 id uuid PRIMARY KEY, principal_id uuid NOT NULL, agent_id uuid NOT NULL,
 state text NOT NULL CHECK(state IN ('active','paused','expired','withdrawn')),
 visibility text NOT NULL CHECK(visibility IN ('private','restricted','redacted_public')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), current_revision int NOT NULL DEFAULT 1,
 expires_at timestamptz NOT NULL, watch_enabled boolean NOT NULL,
 watch_channels text[] NOT NULL DEFAULT '{inbox}', last_scan timestamptz, next_scan timestamptz NOT NULL DEFAULT now(),
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(agent_id,principal_id) REFERENCES agents(id,principal_id)
);
CREATE TABLE intent_revisions (
 intent_id uuid NOT NULL REFERENCES intents, revision int NOT NULL CHECK(revision>0),
 document jsonb NOT NULL, public_projection jsonb NOT NULL, content_hash text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(intent_id,revision)
);
ALTER TABLE intents ADD CONSTRAINT current_revision_fk FOREIGN KEY(id,current_revision)
 REFERENCES intent_revisions(intent_id,revision) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE intent_grants (
 intent_id uuid NOT NULL REFERENCES intents, principal_id uuid NOT NULL REFERENCES principals,
 fields text[] NOT NULL, expires_at timestamptz NOT NULL, PRIMARY KEY(intent_id,principal_id)
);
CREATE TABLE events (
 id uuid PRIMARY KEY, aggregate_kind text NOT NULL, aggregate_id uuid NOT NULL,
 version bigint NOT NULL, ordinal int NOT NULL DEFAULT 0, principal_id uuid NOT NULL REFERENCES principals,
 agent_id uuid REFERENCES agents, type text NOT NULL, facts jsonb NOT NULL,
 origin text NOT NULL CHECK(origin IN ('registered','release_test')),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(aggregate_id,version,ordinal)
);
CREATE TABLE inbox (
 agent_id uuid NOT NULL REFERENCES agents, sequence bigint NOT NULL,
 event_id uuid NOT NULL REFERENCES events, resource_path text NOT NULL CHECK(resource_path LIKE '/v1/%'),
 acked_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(agent_id,sequence), UNIQUE(agent_id,event_id)
);
CREATE TABLE idempotency (
 actor_id uuid NOT NULL REFERENCES agents, operation text NOT NULL, key text NOT NULL,
 request_hash text NOT NULL, status int NOT NULL, response jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(actor_id,operation,key)
);

CREATE FUNCTION actor_principal() RETURNS uuid LANGUAGE sql STABLE AS
 $$ SELECT nullif(current_setting('undergrid.principal_id',true),'')::uuid $$;
CREATE FUNCTION actor_agent() RETURNS uuid LANGUAGE sql STABLE AS
 $$ SELECT nullif(current_setting('undergrid.agent_id',true),'')::uuid $$;
CREATE FUNCTION owner_matches(i text,s text) RETURNS boolean LANGUAGE sql STABLE AS
 $$ SELECT i=current_setting('undergrid.owner_issuer',true) AND s=current_setting('undergrid.owner_subject',true) $$;
-- This narrow lookup cannot read content or grant authority. Commands lock and
-- recheck the returned principal, agent and credential before accessing state.
CREATE FUNCTION identify_token(d bytea) RETURNS TABLE(principal_id uuid,agent_id uuid,credential_id uuid)
 LANGUAGE sql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT a.principal_id,a.id,c.id FROM credentials c JOIN agents a ON a.id=c.agent_id
 WHERE c.digest=d AND c.audience='undergrid-network' AND c.revoked_at IS NULL AND c.expires_at>now()
 $$;
REVOKE ALL ON FUNCTION identify_token(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION identify_token(bytea) TO undergrid_app;

ALTER TABLE principals ENABLE ROW LEVEL SECURITY;
CREATE POLICY principals_access ON principals USING(id=actor_principal() OR owner_matches(issuer,subject))
 WITH CHECK(id=actor_principal() OR owner_matches(issuer,subject));
ALTER TABLE agents ENABLE ROW LEVEL SECURITY;
CREATE POLICY agents_access ON agents USING(principal_id IN (SELECT id FROM principals)) WITH CHECK(principal_id IN (SELECT id FROM principals));
ALTER TABLE credentials ENABLE ROW LEVEL SECURITY;
CREATE POLICY credentials_access ON credentials USING(agent_id IN (SELECT id FROM agents)) WITH CHECK(agent_id IN (SELECT id FROM agents));
ALTER TABLE intents ENABLE ROW LEVEL SECURITY;
CREATE POLICY intents_access ON intents USING(principal_id=actor_principal() OR visibility='redacted_public') WITH CHECK(principal_id=actor_principal());
ALTER TABLE intent_revisions ENABLE ROW LEVEL SECURITY;
CREATE POLICY revisions_private ON intent_revisions USING(intent_id IN (SELECT id FROM intents WHERE principal_id=actor_principal())) WITH CHECK(intent_id IN (SELECT id FROM intents WHERE principal_id=actor_principal()));
ALTER TABLE intent_grants ENABLE ROW LEVEL SECURITY;
CREATE POLICY grants_access ON intent_grants USING(intent_id IN (SELECT id FROM intents WHERE principal_id=actor_principal())) WITH CHECK(intent_id IN (SELECT id FROM intents WHERE principal_id=actor_principal()));
ALTER TABLE events ENABLE ROW LEVEL SECURITY;
CREATE POLICY events_access ON events USING(principal_id IN (SELECT id FROM principals)) WITH CHECK(principal_id IN (SELECT id FROM principals));
ALTER TABLE inbox ENABLE ROW LEVEL SECURITY;
CREATE POLICY inbox_access ON inbox USING(agent_id=actor_agent()) WITH CHECK(agent_id=actor_agent());
ALTER TABLE idempotency ENABLE ROW LEVEL SECURITY;
CREATE POLICY idempotency_access ON idempotency USING(actor_id=actor_agent()) WITH CHECK(actor_id=actor_agent());
GRANT SELECT,INSERT,UPDATE ON principals,agents,credentials,intents,intent_grants,inbox TO undergrid_app;
GRANT SELECT,INSERT ON intent_revisions,events,idempotency TO undergrid_app;
CREATE INDEX intents_monitoring ON intents(next_scan) WHERE state='active' AND watch_enabled;
CREATE INDEX inbox_replay ON inbox(agent_id,created_at);
