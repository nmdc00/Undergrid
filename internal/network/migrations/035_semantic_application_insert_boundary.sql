-- A table-level INSERT grant also covers future columns. Keep participant
-- revision creation separate from worker-only provider observation metadata.
REVOKE INSERT ON undergrid.intent_revisions FROM undergrid_app;
GRANT INSERT(intent_id,revision,document,public_projection,content_hash) ON undergrid.intent_revisions TO undergrid_app;
