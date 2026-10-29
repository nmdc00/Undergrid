ALTER TABLE undergrid.evaluations
 ADD COLUMN runtime_isolation text NOT NULL DEFAULT 'legacy_unqualified_evidence',
 ADD COLUMN bubblewrap_sha256 text,
 ADD COLUMN prlimit_sha256 text,
 ADD COLUMN resource_limits_evidence text NOT NULL DEFAULT 'not_recorded_for_legacy_evaluation',
 ADD COLUMN image_identity_evidence text NOT NULL DEFAULT 'legacy_configured_digest_unverified';

ALTER TABLE undergrid.evaluations
 ADD CONSTRAINT evaluation_runtime_isolation_check CHECK(runtime_isolation IN (
  'legacy_unqualified_evidence',
  'not_run_content_unavailable',
  'development_process_not_qualified',
  'bubblewrap_cgroup_v2_private_namespaces'
 )),
 ADD CONSTRAINT evaluation_bubblewrap_hash_check CHECK(bubblewrap_sha256 IS NULL OR bubblewrap_sha256 ~ '^[0-9a-f]{64}$'),
 ADD CONSTRAINT evaluation_prlimit_hash_check CHECK(prlimit_sha256 IS NULL OR prlimit_sha256 ~ '^[0-9a-f]{64}$');
