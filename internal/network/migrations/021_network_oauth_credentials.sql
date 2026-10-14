SET search_path=undergrid,pg_catalog;

-- OAuth access grants are normalized credentials so every existing actor lock,
-- revocation, RLS policy and artifact upload FK continues to cover them.
ALTER TABLE credentials ALTER COLUMN digest DROP NOT NULL;
ALTER TABLE credentials ADD COLUMN kind text NOT NULL DEFAULT 'api_token'
 CHECK(kind IN ('api_token','oauth_grant'));
ALTER TABLE credentials ADD COLUMN oauth_issuer text;
ALTER TABLE credentials ADD COLUMN oauth_subject text;
ALTER TABLE credentials ADD COLUMN oauth_client_id text;
ALTER TABLE credentials DROP CONSTRAINT credentials_audience_check;
ALTER TABLE credentials ADD CONSTRAINT credentials_audience_check CHECK(length(audience) BETWEEN 1 AND 512);
ALTER TABLE credentials ADD CONSTRAINT credentials_kind_identity_check CHECK (
 (kind='api_token' AND digest IS NOT NULL AND oauth_issuer IS NULL AND oauth_subject IS NULL AND oauth_client_id IS NULL AND audience='undergrid-network') OR
 (kind='oauth_grant' AND digest IS NULL AND length(oauth_issuer) BETWEEN 1 AND 512 AND length(oauth_subject) BETWEEN 1 AND 512 AND length(oauth_client_id) BETWEEN 1 AND 255)
);
CREATE UNIQUE INDEX credentials_oauth_identity ON credentials(oauth_issuer,oauth_subject,oauth_client_id) WHERE kind='oauth_grant';

CREATE OR REPLACE FUNCTION identify_token(d bytea) RETURNS TABLE(principal_id uuid,agent_id uuid,credential_id uuid)
 LANGUAGE sql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT a.principal_id,a.id,c.id FROM credentials c JOIN agents a ON a.id=c.agent_id
 WHERE c.kind='api_token' AND c.digest=d AND c.audience='undergrid-network' AND c.revoked_at IS NULL AND c.expires_at>now()
 $$;
REVOKE ALL ON FUNCTION identify_token(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION identify_token(bytea) TO undergrid_app;

CREATE FUNCTION identify_oauth(i text,s text,client text,resource text)
 RETURNS TABLE(principal_id uuid,agent_id uuid,credential_id uuid)
 LANGUAGE sql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT a.principal_id,a.id,c.id FROM credentials c JOIN agents a ON a.id=c.agent_id
 WHERE c.kind='oauth_grant' AND c.oauth_issuer=i AND c.oauth_subject=s AND c.oauth_client_id=client
 AND c.audience=resource AND c.revoked_at IS NULL AND c.expires_at>now() AND a.status='active'
 $$;
REVOKE ALL ON FUNCTION identify_oauth(text,text,text,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION identify_oauth(text,text,text,text) TO undergrid_app;

CREATE OR REPLACE FUNCTION immutable_credential_issuance() RETURNS trigger LANGUAGE plpgsql SET search_path=undergrid,pg_catalog AS $$
BEGIN
 IF ROW(NEW.id,NEW.agent_id,NEW.digest,NEW.kind,NEW.oauth_issuer,NEW.oauth_subject,NEW.oauth_client_id,NEW.scopes,NEW.audience,NEW.expires_at,NEW.generation,NEW.issuance_nonce,NEW.issuance_key_id,NEW.issuance_replay_until,NEW.predecessor_id)
 IS DISTINCT FROM ROW(OLD.id,OLD.agent_id,OLD.digest,OLD.kind,OLD.oauth_issuer,OLD.oauth_subject,OLD.oauth_client_id,OLD.scopes,OLD.audience,OLD.expires_at,OLD.generation,OLD.issuance_nonce,OLD.issuance_key_id,OLD.issuance_replay_until,OLD.predecessor_id) THEN
 RAISE EXCEPTION 'immutable credential issuance metadata';
 END IF;
 IF OLD.revoked_at IS NOT NULL AND NEW.revoked_at IS DISTINCT FROM OLD.revoked_at THEN RAISE EXCEPTION 'credential revocation is terminal'; END IF;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION immutable_credential_issuance() FROM PUBLIC;
