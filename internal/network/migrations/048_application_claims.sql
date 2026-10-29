SET search_path=undergrid,pg_catalog;
-- Historical application references were not retained. NULL is unknown history,
-- never a fabricated empty declaration or verified capability assertion.
ALTER TABLE memberships ADD COLUMN application_claims jsonb;
ALTER TABLE memberships ADD CONSTRAINT application_claims_shape CHECK (
 application_claims IS NULL OR (
 origin='application' AND jsonb_typeof(application_claims)='object'
 AND application_claims->>'schema'='application-claims-v1'
 AND application_claims->>'role_key'=role_key
 AND application_claims ?& ARRAY['schema','role_key','capability_refs','role','swarm_version']
 AND application_claims-ARRAY['schema','role_key','capability_refs','role','swarm_version']='{}'::jsonb
 AND jsonb_typeof(application_claims->'role')='object'
 AND jsonb_typeof(application_claims->'swarm_version')='number'
 AND CASE WHEN application_claims->'capability_refs'='null'::jsonb THEN true
 WHEN jsonb_typeof(application_claims->'capability_refs')='array'
 THEN jsonb_array_length(application_claims->'capability_refs')<=20 ELSE false END
 ));
CREATE FUNCTION protect_application_claims() RETURNS trigger LANGUAGE plpgsql SET search_path=undergrid,pg_catalog AS $$
BEGIN
 IF NEW.application_claims IS DISTINCT FROM OLD.application_claims THEN
  RAISE EXCEPTION 'application claims are immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER application_claims_immutable BEFORE UPDATE ON memberships FOR EACH ROW EXECUTE FUNCTION protect_application_claims();
