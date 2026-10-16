SET search_path=undergrid,pg_catalog;
CREATE TABLE artifacts (
 id uuid PRIMARY KEY, swarm_id uuid NOT NULL REFERENCES swarms, plan_id uuid NOT NULL REFERENCES plan_versions,
 task_id uuid NOT NULL REFERENCES tasks, principal_id uuid NOT NULL REFERENCES principals, agent_id uuid NOT NULL REFERENCES agents,
 claim_epoch bigint NOT NULL CHECK(claim_epoch>0), generation uuid NOT NULL UNIQUE,
 state text NOT NULL DEFAULT 'quarantine' CHECK(state IN ('quarantine','sealed','verified','failed','published','held')),
 size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 1 AND 10485760), media_type text NOT NULL CHECK(media_type='application/json'),
 upload_digest bytea NOT NULL, upload_credential_id uuid NOT NULL REFERENCES credentials, upload_expires_at timestamptz NOT NULL,
 sha256 text, manifest jsonb, manifest_bytes bytea, manifest_hash text, plan_hash text, agreement_hash text,
 license_id text, required_rights uuid[], version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), sealed_at timestamptz, published_at timestamptz,
 CHECK((state='quarantine') OR (sha256 IS NOT NULL AND manifest_hash IS NOT NULL AND manifest_bytes IS NOT NULL AND agreement_hash IS NOT NULL AND required_rights IS NOT NULL))
);
CREATE TABLE artifact_inputs (
 artifact_id uuid NOT NULL REFERENCES artifacts, input_artifact_id uuid NOT NULL REFERENCES artifacts,
 input_sha256 text NOT NULL, purpose text NOT NULL CHECK(purpose='assembly'), rights_grant_hash text NOT NULL,
 PRIMARY KEY(artifact_id,input_artifact_id), CHECK(artifact_id<>input_artifact_id)
);
CREATE TABLE evaluations (
 id uuid PRIMARY KEY, artifact_id uuid NOT NULL REFERENCES artifacts,
 request_key text NOT NULL, generation uuid NOT NULL, evidence jsonb NOT NULL,
 result text NOT NULL CHECK(result IN ('passed','failed','error','unsupported')),
 input_sha256 text NOT NULL, manifest_sha256 text NOT NULL, spec_sha256 text NOT NULL,
 verifier_sha256 text NOT NULL, image_sha256 text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(artifact_id,request_key)
);
ALTER TABLE tasks ADD COLUMN submitted_artifact_id uuid REFERENCES artifacts;
GRANT UPDATE(submitted_artifact_id) ON tasks TO undergrid_app;
ALTER TABLE approvals DROP CONSTRAINT approvals_purpose_check;
ALTER TABLE approvals ADD COLUMN artifact_id uuid REFERENCES artifacts;
ALTER TABLE approvals ADD COLUMN evaluation_id uuid REFERENCES evaluations;
ALTER TABLE approvals ADD CONSTRAINT approvals_purpose_check CHECK(purpose IN ('plan','rights','acceptance','publication'));
ALTER TABLE approvals ADD CONSTRAINT approval_subject CHECK((purpose IN ('plan','rights') AND artifact_id IS NULL AND evaluation_id IS NULL) OR (purpose='acceptance' AND artifact_id IS NOT NULL AND evaluation_id IS NOT NULL) OR (purpose='publication' AND artifact_id IS NOT NULL AND evaluation_id IS NULL));
ALTER TABLE approvals DROP CONSTRAINT approvals_plan_id_principal_id_purpose_content_hash_key;
CREATE UNIQUE INDEX plan_approval_unique ON approvals(plan_id,principal_id,purpose,content_hash) WHERE artifact_id IS NULL;
CREATE UNIQUE INDEX artifact_approval_unique ON approvals(artifact_id,principal_id,purpose,content_hash) WHERE artifact_id IS NOT NULL;
ALTER TABLE artifacts ENABLE ROW LEVEL SECURITY;
CREATE POLICY artifact_read ON artifacts FOR SELECT TO undergrid_app USING(state='published' OR swarm_member(swarm_id));
CREATE POLICY artifact_insert ON artifacts FOR INSERT TO undergrid_app WITH CHECK(principal_id=actor_principal() AND agent_id=actor_agent() AND swarm_contributor(swarm_id));
CREATE POLICY artifact_update ON artifacts FOR UPDATE TO undergrid_app USING(swarm_member(swarm_id)) WITH CHECK(swarm_member(swarm_id));
CREATE POLICY artifact_worker ON artifacts TO undergrid_worker USING(true) WITH CHECK(true);
ALTER TABLE artifact_inputs ENABLE ROW LEVEL SECURITY;
CREATE POLICY input_read ON artifact_inputs FOR SELECT TO undergrid_app USING(artifact_id IN (SELECT id FROM artifacts));
CREATE POLICY input_insert ON artifact_inputs FOR INSERT TO undergrid_app WITH CHECK(artifact_id IN (SELECT id FROM artifacts WHERE agent_id=actor_agent() AND state='quarantine'));
CREATE POLICY input_worker ON artifact_inputs FOR SELECT TO undergrid_worker USING(true);
ALTER TABLE evaluations ENABLE ROW LEVEL SECURITY;
CREATE POLICY evaluation_read ON evaluations FOR SELECT TO undergrid_app USING(artifact_id IN (SELECT id FROM artifacts));
CREATE POLICY evaluation_worker ON evaluations TO undergrid_worker USING(true) WITH CHECK(true);
GRANT SELECT,INSERT ON artifacts,artifact_inputs TO undergrid_app;
GRANT UPDATE(state,sha256,manifest,manifest_bytes,manifest_hash,plan_hash,agreement_hash,license_id,required_rights,version,sealed_at,published_at) ON artifacts TO undergrid_app;
GRANT SELECT ON evaluations TO undergrid_app;
GRANT SELECT ON artifacts,artifact_inputs,evaluations TO undergrid_worker;
GRANT INSERT ON evaluations TO undergrid_worker;
GRANT UPDATE(state,version) ON artifacts TO undergrid_worker;
-- Immutable blob identity and manifest survive verifier/publication transitions.
CREATE FUNCTION protect_sealed_artifact() RETURNS trigger LANGUAGE plpgsql SET search_path=undergrid,pg_catalog AS $$
BEGIN
 IF OLD.state<>'quarantine' AND (NEW.swarm_id,NEW.plan_id,NEW.task_id,NEW.principal_id,NEW.agent_id,NEW.claim_epoch,NEW.generation,NEW.size_bytes,NEW.media_type,NEW.sha256,NEW.manifest_bytes,NEW.manifest_hash,NEW.plan_hash,NEW.agreement_hash,NEW.license_id,NEW.required_rights,NEW.sealed_at,NEW.manifest) IS DISTINCT FROM (OLD.swarm_id,OLD.plan_id,OLD.task_id,OLD.principal_id,OLD.agent_id,OLD.claim_epoch,OLD.generation,OLD.size_bytes,OLD.media_type,OLD.sha256,OLD.manifest_bytes,OLD.manifest_hash,OLD.plan_hash,OLD.agreement_hash,OLD.license_id,OLD.required_rights,OLD.sealed_at,OLD.manifest) THEN RAISE EXCEPTION 'sealed artifact is immutable'; END IF;
 IF OLD.state<>'quarantine' AND NEW.state='quarantine' THEN RAISE EXCEPTION 'sealed generation cannot be reopened'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER sealed_artifact_immutable BEFORE UPDATE ON artifacts FOR EACH ROW EXECUTE FUNCTION protect_sealed_artifact();
CREATE FUNCTION artifact_upload_identity(target uuid, capability bytea) RETURNS TABLE(principal_id uuid,agent_id uuid,credential_id uuid) LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT a.principal_id,a.agent_id,c.id FROM artifacts a JOIN credentials c ON c.id=a.upload_credential_id
 WHERE a.id=target AND a.upload_digest=capability AND a.upload_expires_at>now() AND a.state='quarantine' AND c.revoked_at IS NULL AND c.expires_at>now() AND 'artifacts:submit'=ANY(c.scopes) ORDER BY c.id LIMIT 1
$$;
REVOKE ALL ON FUNCTION artifact_upload_identity(uuid,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION artifact_upload_identity(uuid,bytea) TO undergrid_app;
CREATE FUNCTION principal_artifact_storage() RETURNS bigint LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT COALESCE(sum(size_bytes),0) FROM artifacts WHERE principal_id=actor_principal()
$$;
REVOKE ALL ON FUNCTION principal_artifact_storage() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION principal_artifact_storage() TO undergrid_app;
CREATE FUNCTION artifact_third_party(target uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT NOT EXISTS(SELECT 1 FROM artifacts a JOIN plan_versions p ON p.id=a.plan_id WHERE a.id=target AND (a.principal_id=actor_principal() OR actor_principal()=ANY(a.required_rights) OR p.specification->'plan'->'required_principals' ? actor_principal()::text OR p.specification->'plan'->'acceptance_principals' ? actor_principal()::text))
$$;
REVOKE ALL ON FUNCTION artifact_third_party(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION artifact_third_party(uuid) TO undergrid_app;
CREATE FUNCTION lock_artifact_authorities(target uuid) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
BEGIN
 PERFORM p.id FROM principals p WHERE p.id IN (
  SELECT actor_principal() UNION SELECT unnest(a.required_rights) FROM artifacts a WHERE a.id=target AND swarm_member(a.swarm_id)
  UNION SELECT value::uuid FROM artifacts a JOIN plan_versions v ON v.id=a.plan_id CROSS JOIN LATERAL jsonb_array_elements_text(v.specification->'plan'->'required_principals') WHERE a.id=target AND swarm_member(a.swarm_id)
 ) ORDER BY p.id FOR UPDATE;
END $$;
REVOKE ALL ON FUNCTION lock_artifact_authorities(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION lock_artifact_authorities(uuid) TO undergrid_app;
GRANT INSERT ON events TO undergrid_worker;
CREATE POLICY artifact_evaluation_event ON events FOR INSERT TO undergrid_worker WITH CHECK(type='artifact.evaluated' AND aggregate_id IN (SELECT id FROM artifacts WHERE principal_id=events.principal_id AND agent_id=events.agent_id));
