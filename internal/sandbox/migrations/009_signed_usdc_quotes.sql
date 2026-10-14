ALTER TABLE undergrid_sandbox.sandbox_payments
 ADD COLUMN wallet_binding_id uuid,
 ADD COLUMN wallet_binding_epoch bigint,
 ADD COLUMN signed_quote_payload bytea,
 ADD COLUMN signed_quote_signature bytea,
 ADD COLUMN signed_quote_key_id text,
 ADD CONSTRAINT sandbox_payment_wallet_snapshot_fk FOREIGN KEY(wallet_binding_id,actor_id,wallet_binding_epoch)
 REFERENCES undergrid_sandbox.sandbox_wallet_challenges(id,actor_id,epoch),
 ADD CONSTRAINT sandbox_signed_quote_shape CHECK(
 (asset='USD' AND wallet_binding_id IS NULL AND wallet_binding_epoch IS NULL AND signed_quote_payload IS NULL AND signed_quote_signature IS NULL AND signed_quote_key_id IS NULL)
 OR (asset='USDC' AND wallet_binding_id IS NOT NULL AND wallet_binding_epoch IS NOT NULL AND wallet_binding_epoch>0 AND signed_quote_payload IS NOT NULL AND octet_length(signed_quote_payload) BETWEEN 1 AND 8192 AND signed_quote_signature IS NOT NULL AND octet_length(signed_quote_signature)=64 AND signed_quote_key_id IS NOT NULL AND length(signed_quote_key_id) BETWEEN 1 AND 128));
CREATE FUNCTION undergrid_sandbox.guard_payment_quote() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,undergrid_sandbox AS $$
BEGIN
 IF ROW(OLD.id,OLD.actor_id,OLD.credential_id,OLD.quote,OLD.quote_hash,OLD.asset,OLD.amount,OLD.fee,OLD.provider_key,OLD.wallet_binding_id,OLD.wallet_binding_epoch,OLD.signed_quote_payload,OLD.signed_quote_signature,OLD.signed_quote_key_id)
 IS DISTINCT FROM ROW(NEW.id,NEW.actor_id,NEW.credential_id,NEW.quote,NEW.quote_hash,NEW.asset,NEW.amount,NEW.fee,NEW.provider_key,NEW.wallet_binding_id,NEW.wallet_binding_epoch,NEW.signed_quote_payload,NEW.signed_quote_signature,NEW.signed_quote_key_id) THEN
 RAISE EXCEPTION 'payment quote is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER sandbox_payment_quote_immutable BEFORE UPDATE ON undergrid_sandbox.sandbox_payments FOR EACH ROW EXECUTE FUNCTION undergrid_sandbox.guard_payment_quote();
-- The dispatcher reads the immutable original proof only; it cannot create or
-- replace owner wallet proofs.
CREATE POLICY sandbox_wallet_challenge_dispatch_read ON undergrid_sandbox.sandbox_wallet_challenges
 FOR SELECT TO undergrid_sandbox_worker USING(state='bound');
GRANT SELECT ON undergrid_sandbox.sandbox_wallet_challenges TO undergrid_sandbox_worker;
REVOKE ALL ON FUNCTION undergrid_sandbox.guard_payment_quote() FROM PUBLIC;
