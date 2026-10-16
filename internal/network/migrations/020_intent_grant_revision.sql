SET search_path=undergrid,pg_catalog;

-- Existing grants predate immutable intention revisions. NULL deliberately
-- means unbound and is never treated as consent to the current revision.
ALTER TABLE intent_grants ADD COLUMN revision integer;
ALTER TABLE intent_grants ADD CONSTRAINT intent_grants_revision_positive
 CHECK (revision IS NULL OR revision > 0);
ALTER TABLE intent_grants ADD CONSTRAINT intent_grants_revision_fk
 FOREIGN KEY(intent_id,revision) REFERENCES intent_revisions(intent_id,revision);

-- This is the only non-owner read path into restricted intention documents.
-- The actor is taken from transaction-local authenticated context; its result
-- contains only explicitly granted allowlisted document fields.
CREATE FUNCTION granted_intent(target_id uuid)
RETURNS TABLE(document jsonb,version bigint)
LANGUAGE sql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT jsonb_build_object(
          'id',i.id,
          'version',i.version,
          'state',i.state,
          'expires_at',i.expires_at,
          'grant_expires_at',g.expires_at
        ) || COALESCE((
            SELECT jsonb_object_agg(f.field,r.document->f.field)
            FROM unnest(g.fields) AS f(field)
            WHERE f.field = ANY(ARRAY['objective','output','hard_constraints','preferences','public_summary','budget']::text[])
              AND r.document ? f.field
          ),'{}'::jsonb), i.version
 FROM intents i
 JOIN intent_revisions r ON r.intent_id=i.id AND r.revision=i.current_revision
 JOIN intent_grants g ON g.intent_id=i.id AND g.principal_id=actor_principal()
 JOIN principals owner ON owner.id=i.principal_id AND owner.status='active'
 JOIN agents owner_agent ON owner_agent.id=i.agent_id AND owner_agent.status='active'
 JOIN principals recipient ON recipient.id=g.principal_id AND recipient.status='active'
 JOIN agents reader ON reader.id=actor_agent() AND reader.principal_id=recipient.id AND reader.status='active'
 WHERE i.id=target_id
   AND i.visibility='restricted'
   AND i.state='active'
   AND i.expires_at>statement_timestamp()
   AND g.revision=i.current_revision
   AND g.expires_at>statement_timestamp()
 $$;
REVOKE ALL ON FUNCTION granted_intent(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION granted_intent(uuid) TO undergrid_app;

-- The owner write path needs a uniform availability check without exposing
-- principal rows through the ordinary RLS policy.
CREATE FUNCTION active_intent_grant_recipient(target_id uuid)
RETURNS boolean
LANGUAGE sql SECURITY DEFINER SET search_path=undergrid,pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM principals WHERE id=target_id AND status='active')
 $$;
REVOKE ALL ON FUNCTION active_intent_grant_recipient(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION active_intent_grant_recipient(uuid) TO undergrid_app;
