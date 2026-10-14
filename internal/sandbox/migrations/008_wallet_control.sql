ALTER TABLE undergrid_sandbox.sandbox_actors
 ADD COLUMN wallet_binding_id uuid,
 ADD COLUMN wallet_binding_epoch bigint NOT NULL DEFAULT 0 CHECK(wallet_binding_epoch>=0);

CREATE TABLE undergrid_sandbox.sandbox_wallet_challenges (
 id uuid PRIMARY KEY,
 actor_id uuid NOT NULL REFERENCES undergrid_sandbox.sandbox_actors,
 address text NOT NULL CHECK(address=lower(address) AND address ~ '^0x[0-9a-f]{40}$'),
 chain_id bigint NOT NULL CHECK(chain_id=84532),
 expected_epoch bigint NOT NULL CHECK(expected_epoch>=0),
 nonce text NOT NULL UNIQUE CHECK(nonce ~ '^[a-f0-9]{64}$'),
 message text NOT NULL CHECK(length(message) BETWEEN 1 AND 2048),
 issue_key text NOT NULL CHECK(length(issue_key) BETWEEN 1 AND 128),
 issue_request_hash text NOT NULL CHECK(issue_request_hash ~ '^[a-f0-9]{64}$'),
 state text NOT NULL DEFAULT 'issued' CHECK(state IN('issued','superseded','bound')),
 issued_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL,
 proof_key text,
 proof_request_hash text,
 proof_hash text,
 epoch bigint,
 bound_at timestamptz,
 UNIQUE(actor_id,issue_key),
 UNIQUE(id,actor_id,epoch),
 CHECK(expires_at>issued_at),
 CHECK((state='bound' AND epoch IS NOT NULL AND epoch=expected_epoch+1 AND proof_key IS NOT NULL AND length(proof_key) BETWEEN 1 AND 128 AND proof_request_hash IS NOT NULL AND proof_request_hash ~ '^[a-f0-9]{64}$' AND proof_hash IS NOT NULL AND proof_hash ~ '^[a-f0-9]{64}$' AND bound_at IS NOT NULL)
    OR (state<>'bound' AND epoch IS NULL AND proof_key IS NULL AND proof_request_hash IS NULL AND proof_hash IS NULL AND bound_at IS NULL))
);
CREATE UNIQUE INDEX sandbox_wallet_proof_key ON undergrid_sandbox.sandbox_wallet_challenges(actor_id,proof_key) WHERE proof_key IS NOT NULL;
CREATE UNIQUE INDEX sandbox_wallet_one_issued_challenge ON undergrid_sandbox.sandbox_wallet_challenges(actor_id) WHERE state='issued';
CREATE UNIQUE INDEX sandbox_wallet_epoch ON undergrid_sandbox.sandbox_wallet_challenges(actor_id,epoch) WHERE state='bound';
CREATE INDEX sandbox_wallet_actor_history ON undergrid_sandbox.sandbox_wallet_challenges(actor_id,epoch DESC) WHERE state='bound';
ALTER TABLE undergrid_sandbox.sandbox_actors
 ADD CONSTRAINT sandbox_actor_wallet_binding_fk FOREIGN KEY(wallet_binding_id,id,wallet_binding_epoch)
 REFERENCES undergrid_sandbox.sandbox_wallet_challenges(id,actor_id,epoch),
 ADD CONSTRAINT sandbox_actor_wallet_binding_epoch CHECK(
   (wallet_binding_id IS NULL AND wallet_binding_epoch=0) OR
   (wallet_binding_id IS NOT NULL AND wallet_binding_epoch>0));

CREATE FUNCTION undergrid_sandbox.guard_wallet_challenge() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,undergrid_sandbox AS $$
BEGIN
 IF OLD.id<>NEW.id OR OLD.actor_id<>NEW.actor_id OR OLD.address<>NEW.address OR
    OLD.chain_id<>NEW.chain_id OR OLD.expected_epoch<>NEW.expected_epoch OR
    OLD.nonce<>NEW.nonce OR OLD.message<>NEW.message OR OLD.issue_key<>NEW.issue_key OR
    OLD.issue_request_hash<>NEW.issue_request_hash OR OLD.issued_at<>NEW.issued_at OR
    OLD.expires_at<>NEW.expires_at THEN
   RAISE EXCEPTION 'wallet challenge identity is immutable';
 END IF;
 IF OLD.state<>'issued' OR NEW.state NOT IN('superseded','bound') THEN
   RAISE EXCEPTION 'wallet challenge transition is not allowed';
 END IF;
 IF NEW.state='superseded' AND (NEW.proof_key IS NOT NULL OR NEW.proof_request_hash IS NOT NULL OR
    NEW.proof_hash IS NOT NULL OR NEW.epoch IS NOT NULL OR NEW.bound_at IS NOT NULL) THEN
   RAISE EXCEPTION 'superseded challenge cannot contain a proof';
 END IF;
 IF NEW.state='bound' AND (NEW.epoch IS DISTINCT FROM OLD.expected_epoch+1 OR NEW.proof_key IS NULL OR
    NEW.proof_request_hash IS NULL OR NEW.proof_hash IS NULL OR NEW.bound_at IS NULL) THEN
   RAISE EXCEPTION 'wallet proof transition is incomplete';
 END IF;
 RETURN NEW;
END;
$$;
REVOKE ALL ON FUNCTION undergrid_sandbox.guard_wallet_challenge() FROM PUBLIC;
CREATE TRIGGER sandbox_wallet_challenge_guard BEFORE UPDATE ON undergrid_sandbox.sandbox_wallet_challenges
 FOR EACH ROW EXECUTE FUNCTION undergrid_sandbox.guard_wallet_challenge();

ALTER TABLE undergrid_sandbox.sandbox_wallet_challenges ENABLE ROW LEVEL SECURITY;
CREATE POLICY sandbox_wallet_challenge_actor ON undergrid_sandbox.sandbox_wallet_challenges
 FOR ALL TO undergrid_sandbox_app USING(actor_id=undergrid_sandbox.actor())
 WITH CHECK(actor_id=undergrid_sandbox.actor());
GRANT SELECT,INSERT ON undergrid_sandbox.sandbox_wallet_challenges TO undergrid_sandbox_app;
GRANT UPDATE(state,proof_key,proof_request_hash,proof_hash,epoch,bound_at)
 ON undergrid_sandbox.sandbox_wallet_challenges TO undergrid_sandbox_app;
GRANT SELECT(wallet_binding_id,wallet_binding_epoch) ON undergrid_sandbox.sandbox_actors TO undergrid_sandbox_app;
GRANT UPDATE(wallet_binding_id,wallet_binding_epoch) ON undergrid_sandbox.sandbox_actors TO undergrid_sandbox_app;

