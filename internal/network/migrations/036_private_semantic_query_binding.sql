-- Extend the immutable observation contract to an explicitly selected private
-- query. Existing public observations and append-only revisions stay unchanged.
SET search_path=undergrid,pg_catalog,pg_temp;
CREATE OR REPLACE FUNCTION protect_intent_revision_embedding() RETURNS trigger LANGUAGE plpgsql SET search_path=undergrid,pg_catalog,pg_temp AS $$
DECLARE selected_query text;
BEGIN
 IF (NEW.intent_id,NEW.revision,NEW.document,NEW.public_projection,NEW.content_hash,NEW.created_at) IS DISTINCT FROM (OLD.intent_id,OLD.revision,OLD.document,OLD.public_projection,OLD.content_hash,OLD.created_at) THEN
  RAISE EXCEPTION 'immutable intention revision';
 END IF;
 IF OLD.embedding IS NOT NULL AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'immutable completed embedding'; END IF;
 IF NEW.embedding IS NOT NULL THEN
  IF NEW.document->>'visibility'='redacted_public' THEN
   IF COALESCE(NEW.public_projection->>'public_summary','')='' OR
    NEW.embedding_input_hash<>encode(sha256(convert_to(NEW.public_projection->>'public_summary','UTF8')),'hex') OR
    NEW.embedding_projection_hash<>encode(sha256(convert_to(NEW.public_projection::text,'UTF8')),'hex') THEN
    RAISE EXCEPTION 'embedding does not bind approved public projection';
   END IF;
  ELSIF NEW.document->>'visibility' IN ('private','restricted') THEN
   selected_query:=NEW.document#>>'{private_semantic_query,projection}';
   IF selected_query IS NULL OR length(btrim(selected_query))<3 OR octet_length(selected_query)>1000 OR
    COALESCE(NEW.document#>>'{private_semantic_query,acknowledge_external_processing}','')<>'true' OR
    NEW.embedding_profile_hash IS DISTINCT FROM NEW.document#>>'{private_semantic_query,profile_hash}' OR
    NEW.embedding_input_hash<>encode(sha256(convert_to(selected_query,'UTF8')),'hex') OR
    NEW.embedding_projection_hash<>encode(sha256(convert_to(selected_query,'UTF8')),'hex') THEN
    RAISE EXCEPTION 'embedding does not bind consented private query';
   END IF;
  ELSE
   RAISE EXCEPTION 'embedding disclosure mode unavailable';
  END IF;
 END IF;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION protect_intent_revision_embedding() FROM PUBLIC;
