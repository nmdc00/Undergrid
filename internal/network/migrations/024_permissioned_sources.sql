SET search_path=undergrid,pg_catalog;
CREATE TABLE external_sources (
 id uuid PRIMARY KEY, config jsonb NOT NULL, next_refresh timestamptz NOT NULL DEFAULT now(), last_success timestamptz,
 validators jsonb NOT NULL DEFAULT '{}', last_error text, CHECK(jsonb_typeof(config)='object')
);
CREATE TABLE external_observations (
 id uuid PRIMARY KEY, source_id uuid NOT NULL REFERENCES external_sources(id), source_key text NOT NULL,
 source_version text NOT NULL, canonical_url text NOT NULL, kind text NOT NULL CHECK(kind IN ('external_capability','external_request')),
 metadata jsonb NOT NULL CHECK(jsonb_typeof(metadata)='object'), content_hash text NOT NULL,
 status text NOT NULL CHECK(status IN ('active','closed','deleted','taken_down')), first_seen timestamptz NOT NULL DEFAULT now(),
 last_seen timestamptz NOT NULL DEFAULT now(), checked_at timestamptz NOT NULL DEFAULT now(), UNIQUE(source_id,source_key)
);
CREATE VIEW eligible_external_observations AS SELECT o.* FROM external_observations o JOIN external_sources s ON s.id=o.source_id WHERE o.status='active' AND o.checked_at>now()-interval '24 hours' AND s.last_success>now()-interval '24 hours';
REVOKE ALL ON external_sources,external_observations,eligible_external_observations FROM PUBLIC,undergrid_app;
GRANT SELECT,INSERT,UPDATE ON external_sources,external_observations TO undergrid_worker;
GRANT SELECT ON eligible_external_observations TO undergrid_worker;
