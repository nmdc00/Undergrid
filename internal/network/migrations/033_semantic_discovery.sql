-- pgvector is part of the required network stack. Missing packaged extension is
-- a failed prerequisite, never a reason to fake a ledger or silently skip SQL.
CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
SET search_path=undergrid,pg_catalog,pg_temp;
ALTER TABLE intent_revisions
 ADD COLUMN embedding public.vector,
 ADD COLUMN embedding_profile_hash text,
 ADD COLUMN embedding_input_hash text,
 ADD COLUMN embedding_projection_hash text,
 ADD COLUMN embedding_provider text,
 ADD COLUMN embedding_model text,
 ADD COLUMN embedding_model_revision text,
 ADD COLUMN embedding_dimensions integer,
 ADD COLUMN embedded_at timestamptz,
 ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (to_tsvector('simple'::regconfig,COALESCE(public_projection->>'public_summary',''))) STORED,
 ADD CONSTRAINT embedding_complete CHECK (
 (embedding IS NULL AND embedding_profile_hash IS NULL AND embedding_input_hash IS NULL AND embedding_projection_hash IS NULL AND embedding_provider IS NULL AND embedding_model IS NULL AND embedding_model_revision IS NULL AND embedding_dimensions IS NULL AND embedded_at IS NULL)
 OR (num_nonnulls(embedding,embedding_profile_hash,embedding_input_hash,embedding_projection_hash,embedding_provider,embedding_model,embedding_model_revision,embedding_dimensions,embedded_at)=9 AND embedding IS NOT NULL AND embedding_profile_hash ~ '^[a-f0-9]{64}$' AND embedding_input_hash ~ '^[a-f0-9]{64}$' AND embedding_projection_hash ~ '^[a-f0-9]{64}$' AND embedding_provider IS NOT NULL AND embedding_model IS NOT NULL AND embedding_model_revision IS NOT NULL AND embedding_dimensions BETWEEN 1 AND 4096 AND public.vector_dims(embedding)=embedding_dimensions AND embedded_at IS NOT NULL AND (embedding OPERATOR(public.<=>) embedding)=0));
CREATE INDEX intent_public_projection_fulltext ON intent_revisions USING gin(search_vector);
CREATE FUNCTION protect_intent_revision_embedding() RETURNS trigger LANGUAGE plpgsql SET search_path=undergrid,pg_catalog,pg_temp AS $$
BEGIN
 IF (NEW.intent_id,NEW.revision,NEW.document,NEW.public_projection,NEW.content_hash,NEW.created_at) IS DISTINCT FROM (OLD.intent_id,OLD.revision,OLD.document,OLD.public_projection,OLD.content_hash,OLD.created_at) THEN
  RAISE EXCEPTION 'immutable intention revision';
 END IF;
 IF OLD.embedding IS NOT NULL AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'immutable completed embedding'; END IF;
 IF NEW.embedding IS NOT NULL AND (
  COALESCE(NEW.public_projection->>'public_summary','')='' OR
  NEW.embedding_input_hash<>encode(sha256(convert_to(NEW.public_projection->>'public_summary','UTF8')),'hex') OR
  NEW.embedding_projection_hash<>encode(sha256(convert_to(NEW.public_projection::text,'UTF8')),'hex')) THEN
  RAISE EXCEPTION 'embedding does not bind approved public projection';
 END IF;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION protect_intent_revision_embedding() FROM PUBLIC;
CREATE TRIGGER intent_revision_embedding_immutable BEFORE UPDATE ON intent_revisions FOR EACH ROW EXECUTE FUNCTION protect_intent_revision_embedding();
GRANT UPDATE(embedding,embedding_profile_hash,embedding_input_hash,embedding_projection_hash,embedding_provider,embedding_model,embedding_model_revision,embedding_dimensions,embedded_at) ON intent_revisions TO undergrid_worker;
CREATE POLICY revisions_worker_embedding ON intent_revisions FOR UPDATE TO undergrid_worker USING(true) WITH CHECK(true);
