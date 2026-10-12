SET search_path=undergrid,pg_catalog;
ALTER POLICY intents_access ON intents TO undergrid_app
 USING(principal_id=actor_principal() OR (actor_agent() IS NOT NULL AND visibility='redacted_public'))
 WITH CHECK(principal_id=actor_principal());
