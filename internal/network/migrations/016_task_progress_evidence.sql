SET search_path=undergrid,pg_catalog;

ALTER TABLE artifacts ADD COLUMN purpose text NOT NULL DEFAULT 'output' CHECK(purpose IN ('output','progress'));
CREATE OR REPLACE FUNCTION protect_sealed_artifact() RETURNS trigger LANGUAGE plpgsql SET search_path=undergrid,pg_catalog AS $$
BEGIN
 IF OLD.state<>'quarantine' AND (NEW.swarm_id,NEW.plan_id,NEW.task_id,NEW.principal_id,NEW.agent_id,NEW.generation,NEW.purpose,NEW.claim_epoch,NEW.size_bytes,NEW.media_type,NEW.sha256,NEW.manifest_bytes,NEW.manifest_hash,NEW.plan_hash,NEW.agreement_hash,NEW.license_id,NEW.required_rights,NEW.sealed_at,NEW.manifest) IS DISTINCT FROM (OLD.swarm_id,OLD.plan_id,OLD.task_id,OLD.principal_id,OLD.agent_id,OLD.generation,OLD.purpose,OLD.claim_epoch,OLD.size_bytes,OLD.media_type,OLD.sha256,OLD.manifest_bytes,OLD.manifest_hash,OLD.plan_hash,OLD.agreement_hash,OLD.license_id,OLD.required_rights,OLD.sealed_at,OLD.manifest) THEN RAISE EXCEPTION 'sealed artifact is immutable'; END IF;
 IF OLD.state<>'quarantine' AND NEW.state='quarantine' THEN RAISE EXCEPTION 'sealed generation cannot be reopened'; END IF;
 RETURN NEW;
END $$;

CREATE TABLE task_progress_evidence (
 id uuid PRIMARY KEY,
 swarm_id uuid NOT NULL REFERENCES swarms,
 plan_id uuid NOT NULL REFERENCES plan_versions,
 task_id uuid NOT NULL REFERENCES tasks,
 artifact_id uuid NOT NULL REFERENCES artifacts,
 evaluation_id uuid NOT NULL REFERENCES evaluations,
 claim_epoch bigint NOT NULL CHECK(claim_epoch>0),
 plan_hash text NOT NULL CHECK(plan_hash ~ '^[0-9a-f]{64}$'),
 artifact_sha256 text NOT NULL CHECK(artifact_sha256 ~ '^[0-9a-f]{64}$'),
 report text NOT NULL CHECK(length(report) BETWEEN 1 AND 4000),
 report_sha256 text NOT NULL CHECK(report_sha256 ~ '^[0-9a-f]{64}$'),
 audience_principal_ids uuid[] NOT NULL CHECK(cardinality(audience_principal_ids) BETWEEN 1 AND 50),
 reviewer_principal_id uuid NOT NULL REFERENCES principals,
 submitted_by_principal_id uuid NOT NULL REFERENCES principals,
 submitted_by_agent_id uuid NOT NULL REFERENCES agents,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','accepted','rejected')),
 created_at timestamptz NOT NULL DEFAULT now(),
 reviewed_at timestamptz,
 UNIQUE(task_id,claim_epoch,artifact_id,evaluation_id,report_sha256),
 CHECK(reviewer_principal_id=ANY(audience_principal_ids)),
 CHECK((state='pending' AND reviewed_at IS NULL) OR (state IN ('accepted','rejected') AND reviewed_at IS NOT NULL))
);
ALTER TABLE tasks ADD COLUMN credited_progress_evidence_id uuid REFERENCES task_progress_evidence;
GRANT UPDATE(credited_progress_evidence_id) ON tasks TO undergrid_app;

ALTER TABLE approvals ADD COLUMN progress_evidence_id uuid REFERENCES task_progress_evidence;
ALTER TABLE approvals DROP CONSTRAINT approvals_purpose_check;
DO $$
DECLARE has_grant_columns boolean;
BEGIN
 SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='undergrid' AND table_name='approvals' AND column_name='grant_scope') INTO has_grant_columns;
 IF has_grant_columns THEN
  ALTER TABLE approvals ADD CONSTRAINT approvals_purpose_check CHECK(purpose IN ('plan','rights','acceptance','publication','assembly_grant','evaluation_grant','progress'));
 ELSE
  ALTER TABLE approvals ADD CONSTRAINT approvals_purpose_check CHECK(purpose IN ('plan','rights','acceptance','publication','progress'));
 END IF;
END $$;
DO $$
DECLARE current_check text;
DECLARE check_body text;
DECLARE progress_subject text;
DECLARE has_grant_columns boolean;
BEGIN
 SELECT pg_get_constraintdef(oid) INTO current_check FROM pg_constraint WHERE conname='approval_subject' AND conrelid='undergrid.approvals'::regclass;
 IF current_check IS NULL OR left(current_check,7)<>'CHECK (' OR right(current_check,1)<>')' THEN
  RAISE EXCEPTION 'unexpected approvals approval_subject constraint';
 END IF;
 check_body := substr(current_check,8,length(current_check)-8);
 SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='undergrid' AND table_name='approvals' AND column_name='grant_scope') INTO has_grant_columns;
 progress_subject := '(purpose=''progress'' AND progress_evidence_id IS NOT NULL AND artifact_id IS NOT NULL AND evaluation_id IS NOT NULL';
 IF has_grant_columns THEN
  progress_subject := progress_subject || ' AND grant_scope IS NULL';
 END IF;
 progress_subject := progress_subject || ')';
 ALTER TABLE approvals DROP CONSTRAINT approval_subject;
 EXECUTE 'ALTER TABLE approvals ADD CONSTRAINT approval_subject CHECK((' || check_body || ') OR ' || progress_subject || ')';
END $$;
CREATE UNIQUE INDEX progress_approval_unique ON approvals(progress_evidence_id,principal_id,purpose,content_hash) WHERE progress_evidence_id IS NOT NULL;

ALTER TABLE task_progress_evidence ENABLE ROW LEVEL SECURITY;
CREATE POLICY task_progress_evidence_read ON task_progress_evidence FOR SELECT TO undergrid_app
 USING(swarm_member(swarm_id) AND actor_principal()=ANY(audience_principal_ids));
CREATE POLICY task_progress_evidence_insert ON task_progress_evidence FOR INSERT TO undergrid_app
 WITH CHECK(submitted_by_principal_id=actor_principal() AND submitted_by_agent_id=actor_agent() AND swarm_contributor(swarm_id));
CREATE POLICY task_progress_evidence_review ON task_progress_evidence FOR UPDATE TO undergrid_app
 USING(swarm_member(swarm_id) AND reviewer_principal_id=actor_principal())
 WITH CHECK(swarm_member(swarm_id) AND reviewer_principal_id=actor_principal());
GRANT SELECT,INSERT ON task_progress_evidence TO undergrid_app;
GRANT UPDATE(state,reviewed_at) ON task_progress_evidence TO undergrid_app;

CREATE FUNCTION protect_task_progress_evidence() RETURNS trigger LANGUAGE plpgsql SET search_path=undergrid,pg_catalog AS $$
BEGIN
 IF (to_jsonb(NEW)-ARRAY['state','reviewed_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','reviewed_at']) THEN
  RAISE EXCEPTION 'task progress evidence is immutable';
 END IF;
 IF OLD.state<>'pending' OR NEW.state NOT IN ('accepted','rejected') OR NEW.reviewed_at IS NULL THEN
  RAISE EXCEPTION 'task progress evidence review is final';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_task_progress_evidence BEFORE UPDATE ON task_progress_evidence FOR EACH ROW EXECUTE FUNCTION protect_task_progress_evidence();
