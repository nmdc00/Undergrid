ALTER TABLE undergrid_sandbox.sandbox_payments
 ADD COLUMN payer_authorization jsonb,
 ADD COLUMN payer_authorization_hash text,
 ADD COLUMN payer_authorization_key text,
 ADD COLUMN payer_authorization_request_hash text,
 ADD CONSTRAINT sandbox_payer_authorization_shape CHECK(
 (payer_authorization IS NULL AND payer_authorization_hash IS NULL AND payer_authorization_key IS NULL AND payer_authorization_request_hash IS NULL)
 OR (asset='USDC' AND payer_authorization IS NOT NULL AND payer_authorization_hash IS NOT NULL AND payer_authorization_hash ~ '^[a-f0-9]{64}$'
 AND payer_authorization_key IS NOT NULL AND length(payer_authorization_key) BETWEEN 1 AND 128
 AND payer_authorization_request_hash IS NOT NULL AND payer_authorization_request_hash ~ '^[a-f0-9]{64}$'
 AND jsonb_typeof(payer_authorization)='object'
 AND COALESCE((payer_authorization->>'signature') ~ '^0x[a-f0-9]{130}$',false)
 AND COALESCE(jsonb_typeof(payer_authorization->'authorization')='object',false)
 AND COALESCE(payer_authorization->'chain_id'='84532'::jsonb,false)
 AND COALESCE(payer_authorization->>'token_address'='0x036cbd53842c5426634e7929541ec2318f3dcf7e',false)
 AND COALESCE(payer_authorization->>'quote_hash'=quote_hash,false)
 AND COALESCE(jsonb_typeof(payer_authorization->'domain')='object',false)
 AND COALESCE(jsonb_typeof(payer_authorization#>'{domain,name}')='string' AND length(payer_authorization#>>'{domain,name}') BETWEEN 1 AND 128,false)
 AND COALESCE(jsonb_typeof(payer_authorization#>'{domain,version}')='string' AND length(payer_authorization#>>'{domain,version}') BETWEEN 1 AND 128,false)
 AND COALESCE((payer_authorization#>>'{authorization,from}') ~ '^0x[a-f0-9]{40}$',false)
 AND COALESCE(payer_authorization#>>'{authorization,to}'=quote->>'payee',false)
 AND COALESCE(payer_authorization#>'{authorization,value}'=to_jsonb((amount+fee)::text),false)
 AND COALESCE(payer_authorization#>'{authorization,valid_after}'='"0"'::jsonb,false)
 AND COALESCE(jsonb_typeof(payer_authorization#>'{authorization,valid_before}')='string' AND (payer_authorization#>>'{authorization,valid_before}') ~ '^[1-9][0-9]{0,19}$',false)
 AND COALESCE((payer_authorization#>>'{authorization,nonce}') ~ '^0x[a-f0-9]{64}$',false)
 AND state IN('authorized','submitting','pending','unknown','settled','failed','canceled')));
CREATE FUNCTION undergrid_sandbox.guard_payer_authorization() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,undergrid_sandbox AS $$
BEGIN
 IF ROW(OLD.payer_authorization,OLD.payer_authorization_hash,OLD.payer_authorization_key,OLD.payer_authorization_request_hash)
 IS DISTINCT FROM ROW(NEW.payer_authorization,NEW.payer_authorization_hash,NEW.payer_authorization_key,NEW.payer_authorization_request_hash) THEN
 IF OLD.payer_authorization IS NOT NULL OR OLD.state<>'authorized' OR NEW.state<>'authorized' OR NEW.asset<>'USDC' THEN
 RAISE EXCEPTION 'payer authorization is immutable and requires an existing reservation';
 END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER sandbox_payer_authorization_guard BEFORE UPDATE ON undergrid_sandbox.sandbox_payments
 FOR EACH ROW EXECUTE FUNCTION undergrid_sandbox.guard_payer_authorization();
REVOKE ALL ON FUNCTION undergrid_sandbox.guard_payer_authorization() FROM PUBLIC;
GRANT UPDATE(payer_authorization,payer_authorization_hash,payer_authorization_key,payer_authorization_request_hash)
 ON undergrid_sandbox.sandbox_payments TO undergrid_sandbox_app;
