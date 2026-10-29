SET search_path=undergrid,pg_catalog;
ALTER TABLE agents ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 ADD COLUMN signing_key bytea CHECK(signing_key IS NULL OR octet_length(signing_key)=32),
 ADD COLUMN control_nonce_hash text,
 ADD COLUMN control_expires_at timestamptz,
 ADD COLUMN control_pending_key bytea CHECK(control_pending_key IS NULL OR octet_length(control_pending_key)=32),
 ADD COLUMN key_control_verified_at timestamptz,
 ADD COLUMN endpoint text,
 ADD COLUMN endpoint_control_verified_at timestamptz;
ALTER TABLE credentials ADD COLUMN generation bigint NOT NULL DEFAULT 1 CHECK(generation>0),
 ADD COLUMN issuance_nonce bytea CHECK(issuance_nonce IS NULL OR octet_length(issuance_nonce)=32),
 ADD COLUMN issuance_key_id text,
 ADD COLUMN issuance_replay_until timestamptz,
 ADD COLUMN predecessor_id uuid REFERENCES credentials;
CREATE FUNCTION immutable_credential_issuance() RETURNS trigger LANGUAGE plpgsql SET search_path=undergrid,pg_catalog AS $$
BEGIN
 IF ROW(NEW.id,NEW.agent_id,NEW.digest,NEW.scopes,NEW.audience,NEW.expires_at,NEW.generation,NEW.issuance_nonce,NEW.issuance_key_id,NEW.issuance_replay_until,NEW.predecessor_id)
 IS DISTINCT FROM ROW(OLD.id,OLD.agent_id,OLD.digest,OLD.scopes,OLD.audience,OLD.expires_at,OLD.generation,OLD.issuance_nonce,OLD.issuance_key_id,OLD.issuance_replay_until,OLD.predecessor_id) THEN
 RAISE EXCEPTION 'immutable credential issuance metadata';
 END IF;
 IF OLD.revoked_at IS NOT NULL AND NEW.revoked_at IS DISTINCT FROM OLD.revoked_at THEN RAISE EXCEPTION 'credential revocation is terminal'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER credentials_immutable BEFORE UPDATE ON credentials FOR EACH ROW EXECUTE FUNCTION immutable_credential_issuance();
REVOKE ALL ON FUNCTION immutable_credential_issuance() FROM PUBLIC;
