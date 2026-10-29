SET search_path=undergrid,pg_catalog;
-- Existing agent generations retain their original credential binding. Owner
-- generations have an explicit mode and cannot authenticate with capability alone.
ALTER TABLE artifacts ALTER COLUMN upload_credential_id DROP NOT NULL;
ALTER TABLE artifacts ADD COLUMN upload_authentication text NOT NULL DEFAULT 'credential';
ALTER TABLE artifacts ADD CONSTRAINT artifact_upload_authentication CHECK (
 (upload_authentication='credential' AND upload_credential_id IS NOT NULL)
 OR (upload_authentication='owner_session' AND upload_credential_id IS NULL));
CREATE FUNCTION protect_artifact_upload_binding() RETURNS trigger LANGUAGE plpgsql SET search_path=undergrid,pg_catalog AS $$
BEGIN
 IF ROW(NEW.upload_authentication,NEW.upload_credential_id,NEW.generation,NEW.upload_digest,NEW.upload_expires_at)
 IS DISTINCT FROM ROW(OLD.upload_authentication,OLD.upload_credential_id,OLD.generation,OLD.upload_digest,OLD.upload_expires_at) THEN
  RAISE EXCEPTION 'artifact upload binding is immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER artifact_upload_binding_immutable BEFORE UPDATE ON artifacts FOR EACH ROW EXECUTE FUNCTION protect_artifact_upload_binding();
