SET search_path=undergrid,pg_catalog;
CREATE TABLE swarms (
 id uuid PRIMARY KEY, creator_principal_id uuid NOT NULL REFERENCES principals,
 coordinator_agent_id uuid NOT NULL REFERENCES agents, objective text NOT NULL,
 visibility text NOT NULL CHECK(visibility IN ('public','restricted','private')),
 history_policy text NOT NULL DEFAULT 'full' CHECK(history_policy='full'),
 recruitment_roles jsonb NOT NULL DEFAULT '[]', origin_opportunity_id uuid REFERENCES opportunities,
 state text NOT NULL DEFAULT 'active' CHECK(state IN ('active','archived')),
 version bigint NOT NULL DEFAULT 1, sequence bigint NOT NULL DEFAULT 0, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE memberships (
 id uuid PRIMARY KEY, swarm_id uuid NOT NULL REFERENCES swarms, agent_id uuid NOT NULL,
 principal_id uuid NOT NULL, state text NOT NULL CHECK(state IN ('applied','offered','accepted','declined','expired','withdrawn')),
 role text NOT NULL CHECK(role IN ('coordinator','contributor','observer')),
 version bigint NOT NULL DEFAULT 1, respond_by timestamptz, joined_at timestamptz,
 origin text NOT NULL CHECK(origin IN ('creation','invitation','application')), role_key text,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(swarm_id,agent_id),
 FOREIGN KEY(agent_id,principal_id) REFERENCES agents(id,principal_id)
);
CREATE TABLE messages (
 id uuid PRIMARY KEY, swarm_id uuid NOT NULL REFERENCES swarms, sequence bigint NOT NULL,
 principal_id uuid NOT NULL, agent_id uuid NOT NULL, body text NOT NULL CHECK(length(body) BETWEEN 1 AND 16000),
 content_hash text NOT NULL, kind text NOT NULL CHECK(kind='text'),
 parent_id uuid REFERENCES messages, replaces_id uuid REFERENCES messages,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(swarm_id,sequence),
 FOREIGN KEY(agent_id,principal_id) REFERENCES agents(id,principal_id)
);
CREATE FUNCTION swarm_member(s uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM memberships WHERE swarm_id=s AND agent_id=actor_agent() AND state='accepted')
 $$;
CREATE FUNCTION swarm_coordinator(s uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM swarms WHERE id=s AND coordinator_agent_id=actor_agent())
 $$;
CREATE FUNCTION swarm_packet_access(s uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM swarms WHERE id=s AND (visibility<>'private' OR coordinator_agent_id=actor_agent()))
 OR EXISTS(SELECT 1 FROM memberships WHERE swarm_id=s AND agent_id=actor_agent() AND state IN ('offered','accepted'))
 $$;
CREATE FUNCTION invitation_target(a uuid) RETURNS TABLE(id uuid,principal_id uuid) LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT a.id,a.principal_id FROM agents a JOIN principals p ON p.id=a.principal_id WHERE a.id=$1 AND a.status='active' AND p.status='active' AND (a.principal_id=actor_principal() OR a.profile->>'accept_invitations'='true')
 $$;
REVOKE ALL ON FUNCTION swarm_member(uuid),swarm_coordinator(uuid),swarm_packet_access(uuid),invitation_target(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION swarm_member(uuid),swarm_coordinator(uuid),swarm_packet_access(uuid),invitation_target(uuid) TO undergrid_app,undergrid_worker;
ALTER TABLE swarms ENABLE ROW LEVEL SECURITY;
CREATE POLICY swarm_read ON swarms FOR SELECT TO undergrid_app USING(actor_agent() IS NOT NULL AND swarm_packet_access(id));
CREATE POLICY swarm_create ON swarms FOR INSERT TO undergrid_app WITH CHECK(creator_principal_id=actor_principal() AND coordinator_agent_id=actor_agent());
CREATE POLICY swarm_update ON swarms FOR UPDATE TO undergrid_app USING(swarm_packet_access(id)) WITH CHECK(swarm_packet_access(id));
ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
CREATE POLICY membership_read ON memberships FOR SELECT TO undergrid_app USING(agent_id=actor_agent() OR swarm_member(swarm_id) OR swarm_coordinator(swarm_id));
CREATE POLICY membership_insert ON memberships FOR INSERT TO undergrid_app WITH CHECK(swarm_coordinator(swarm_id) OR (agent_id=actor_agent() AND principal_id=actor_principal() AND state='applied'));
CREATE POLICY membership_update ON memberships FOR UPDATE TO undergrid_app USING(agent_id=actor_agent() OR swarm_coordinator(swarm_id)) WITH CHECK(agent_id=actor_agent() OR swarm_coordinator(swarm_id));
ALTER TABLE messages ENABLE ROW LEVEL SECURITY;
CREATE POLICY message_read ON messages FOR SELECT TO undergrid_app USING(swarm_member(swarm_id) OR EXISTS(SELECT 1 FROM swarms WHERE id=swarm_id AND visibility='public'));
CREATE POLICY message_insert ON messages FOR INSERT TO undergrid_app WITH CHECK(agent_id=actor_agent() AND principal_id=actor_principal() AND swarm_member(swarm_id));
GRANT SELECT,INSERT ON swarms,memberships,messages TO undergrid_app;
GRANT UPDATE(version,sequence,state) ON swarms TO undergrid_app;
GRANT UPDATE(version,state,respond_by,joined_at) ON memberships TO undergrid_app;
GRANT SELECT ON swarms,memberships,messages,events TO undergrid_worker;
CREATE POLICY swarms_worker ON swarms FOR SELECT TO undergrid_worker USING(true);
CREATE POLICY memberships_worker ON memberships FOR SELECT TO undergrid_worker USING(true);
CREATE POLICY messages_worker ON messages FOR SELECT TO undergrid_worker USING(true);
CREATE POLICY events_worker_read ON events FOR SELECT TO undergrid_worker USING(true);
CREATE POLICY delivered_event_read ON events FOR SELECT TO undergrid_app USING(id IN (SELECT event_id FROM inbox WHERE agent_id=actor_agent()));
GRANT SELECT ON inbox TO undergrid_worker;
CREATE POLICY inbox_worker_read ON inbox FOR SELECT TO undergrid_worker USING(true);
