# Network OAuth resource-server profile

This change implements the network API's resource-server side for configured JWT access tokens. It does not implement an authorization server, MCP transport, A2A adapter, provider selection, or external-provider qualification. The work is a scoped authentication behavior under TDD §4 and the protocol-adapter boundary in §9; it does not complete ID-04's A2A release gate.

## Runtime contract

- `UNDERGRID_NETWORK_OAUTH_RESOURCE` and `UNDERGRID_NETWORK_OAUTH_ISSUERS` configure the canonical HTTPS resource URI and a static, bounded trusted-issuer allowlist together. A half-configured pair fails API startup. With both unset, OAuth bearer tokens fail closed while existing `ug_` API tokens continue to work.
- Startup reads OAuth authorization-server metadata only from configured issuers. The verifier requires exact issuer identity, HTTPS metadata/JWKS on that issuer host, no HTTP redirects, a five-second maximum client timeout, bounded metadata/JWKS documents, refreshes stale key sets after five minutes (failing closed if refresh fails), and rate-limits unknown-key refreshes to one per issuer per ten seconds. It accepts RS256, PS256, and ES256 JWT access tokens with `at+jwt` or `application/at+jwt`, validates signature, issuer, exact configured audience membership, subject, client ID, JTI, issue/expiry/not-before times, and a bounded unique scope list. ID tokens and other algorithms are rejected.
- RFC 9728 protected-resource metadata is served at the path derived from the configured resource URI. Bearer challenges advertise that metadata; insufficient-scope responses include the corresponding challenge error. No token, token digest, JTI, or claims are included in logs, owner responses, idempotency responses, or events.
- An owner binding POST requires the existing verified owner-session/CSRF adapter and a separately verified network-audience access token in the Authorization header. The owner chooses an active agent they own and a subset of currently owner-authorized scopes also present in the access token. The immutable `(issuer, subject, client_id)` tuple is bound to that agent; a different OAuth client is required for another agent. The response contains references only. Its expiry is capped at both the access-token expiry and 30 days. A cached idempotent response is returned only after current token scopes and owner policy are revalidated.
- OAuth grants are normalized `credentials` rows with `kind='oauth_grant'`, no token digest, and issuer/subject/client identity. That preserves credential RLS, revocation, common actor locks, and the artifact `upload_credential_id` foreign key. API-token identification and HMAC issuance/recovery/rotation remain restricted to `kind='api_token'`; API recovery does not revoke OAuth grants. Owner credential output includes the actual credential kind.
- For every API command, authority is the intersection of current stored grant scopes and verified access-token scopes. The credential row is locked and its active status/expiry rechecked in the domain transaction. Artifact upload capabilities continue through the existing live credential lock and expiry/revocation checks, so revoking the OAuth grant invalidates already-issued upload capabilities.
- Owner binding list, revoke, and rotation use owner-only row security. Revoke and rotate require the current credential ETag and idempotency key. Rotation creates a new immutable OAuth-client binding and revokes the previous grant in one transaction; it returns references only. Historical OAuth identity tuples remain unique across active, expired, and revoked rows. Rotation therefore requires the authorization server to issue a token for a distinct `client_id`; revoked tuples cannot be rebound, so an old still-valid JWT cannot resolve a replacement row. This profile does not invent a grant identifier or same-client token cutoff.

## Evidence

Real PostgreSQL HTTP lifecycle tests in `internal/network/network_oauth_test.go` cover normalized credential storage, access-token type/audience/expiry rejection, protected-resource metadata, JWKS key refresh, owner-session plus access-token proof, denial of unapproved rights scopes, idempotent reference replay with a narrower-token denial, owner isolation, truthful credential kind, effective scope intersection with no denied intent write, distinct-client rotation, permanent tuple uniqueness after revocation, immediate revocation, local grant expiry, and separation from API-token recovery.

The artifact lifecycle test uses the existing accepted-task runtime: a bound OAuth credential creates an artifact upload, the persisted `upload_credential_id` points to that OAuth credential, bytes enter quarantine through the actual upload URL, and an upload capability issued before grant revocation is rejected afterward. These are fixture-based resource-profile tests, not external provider verification.

The test authorization server is a local TLS fixture. It proves this implementation's JWT/resource-server behavior only. Provider-specific interoperation, independently operated OAuth clients, Streamable HTTP MCP transport, authorization-server behavior, and release qualification remain unproven.

The 55466 PostgreSQL fixture preserves network migrations `001`–`018` plus `021_network_oauth_credentials.sql`; the sibling `019` and `020` migrations are absent from this isolated worktree. Its 20 ledger rows include `river-0.49.0-ready`, so this is not a claim of contiguous migration qualification. Root integration must apply the combined ordered migration set on its integration fixture before merge review.

A direct app-role contract test then exposed SQL three-valued-logic behavior in the OAuth identity CHECK: against the pre-fix 021 on preserved UTF8 PostgreSQL 16 fixture 55473, an `undergrid_app` INSERT with `oauth_issuer=NULL` succeeded despite valid owner/agent RLS visibility (red). The fixture remains preserved with its old 021 ledger entry; it was not edited or replayed. Migration 021 now requires `oauth_issuer`, `oauth_subject`, and `oauth_client_id` to be explicitly non-NULL before their length checks. On a separate fresh UTF8 PostgreSQL 16 cluster at 55475, the application's unmodified `Migrate` path applied its available migrations in order; the same test rejected each of the three NULL tuple variants specifically at `credentials_kind_identity_check`. The corrected file checksum is `32e9538514d5d597dd61e94c6f9b516cd6afbf06e90af419709c1a3c644a827e`. The 55475 ledger has 20 rows including the River readiness marker and includes 001–018 plus 021; 019/020 remain absent on this feature worktree, so the test is not a claim of contiguous migration qualification.

After the correction, focused real-PostgreSQL OAuth, owner grant/recovery and policy tests passed, including the race run: `go test ./internal/network -run '^(TestOAuth|TestOwnerCanReviewAndRevokeOnlyOwnCredentials|TestCredentialRotationReplayRestartAndRecovery|TestOwnerAgentIssuanceRequiresPolicyForProtectedArtifactGrantScope)$' -count=1` and the same command with `-race`. `go vet ./internal/network ./cmd/undergrid`, `go build ./...`, and `git diff --check` also passed. Full-suite integration remains with the root-owned gate.

Protocol references: [RFC 9068](https://www.rfc-editor.org/rfc/rfc9068.html), [RFC 9728](https://www.rfc-editor.org/rfc/rfc9728.html), and the [MCP authorization specification](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization).

## Final ordered integration proof

Source `1524658e6041e254169342fcc578f6fa6a4d0026` integrates release `ce0ab76`, final progress/privacy 016, owner recovery 017, verifier 018, reports 019 and restricted sharing 020. The released error renderer remains the only renderer: early invalid JWT and transactional insufficient scope return message, server request ID, retryable=false and resource challenge; insufficient scope creates no intention. Metadata now truthfully declares the live reports:read/write scopes.

A completely fresh network-only PostgreSQL cluster `/tmp/undergrid-pg-oauth-final-55481` was initialized with UTF8/C locale. Unmodified application Migrate applied raw network001–021 in order plus River readiness (22 ledger rows); no SQL replay, ledger alteration or omitted CREATE ROLE occurred. The first focused race run exposed a released error-test fixture using a non-API bearer for an intentionally invalid DSN: OAuth correctly rejected that as invalid JWT before database connection. The fixture now explicitly uses ug_ API-token syntax to exercise its intended database-error path. This is a fixture correction, not a weakened authentication rule. The complete focused OAuth/owner/error race selector passed on final source in **12.411 seconds**.

Coverage includes RFC9068 access-token validation and ID-token rejection, HTTPS trusted discovery/JWKS restrictions, RSA/ES256 keys and refresh, normalized credential foreign keys during actual artifact capability upload, post-revocation upload denial, scope intersection after domain locking, owner proof/policy/idempotent replays, immutable historical issuer/subject/client bindings, old JWT denial after rotation/revoke and attempted rebinding, NULL tuple rejection through app privileges, metadata and shared HTTP error fields. Fixtures are a synthetic HTTPS authorization server, not selected-provider interoperability qualification. No custom authorization server or secret-bearing synthetic API credential was introduced.

The app/worker roles are NOLOGIN/NOSUPERUSER/NOBYPASSRLS; separately exercised artifact API/worker roles are LOGIN/NOSUPERUSER/NOBYPASSRLS. Vet network/API, build all commands and diff checks passed. Old fixture75 was untouched. Real authorization-server/provider selection and compatibility, MCP transport, deployment, production image/storage and legal/source gates remain separate and unqualified.

Final migration SHA-256 manifest:

```text
efb10be411cfff46746246c12fc8077cc46d1971cde7a5959ddfa18f3826435d  001_participation.sql
9e6a312d8cdb792c92bb8b88ad0becf56093b3b943d89bf83cd54b33fbe6d473  002_shared_demand.sql
e854e3c6d2ea4bf29d6dbbde8fe5761638f01926fc7abb1d7d008880ce95aa2e  003_bound_public_reads.sql
42ee55b4613f16ce8d471af6ba241f1be913a94f126a171e79058677f735eeac  004_swarms.sql
f298adb7b16409ae4f72eba8cb6d8cc28b665b378186109fc501ca67de0c8bb3  005_contributor_roles.sql
af6c33ce60ec22ae07af894e350485f58997826fd703b50de9d19471f63c74d9  006_offer_lock_visibility.sql
9a5f6ca7c514d7ce3b7243f85089a5404dacd222697474120f60dd3327b0d1c0  007_recruitment.sql
2a06e32a597e4ec950297259e23988b5d055e37899953bb3b02bd4753a7d343d  008_plans.sql
7435e0a9bb975b2eda945b0f36d0b70b657b203958d3db0529a9d047d5095fa7  009_task_withdrawal.sql
3ccd95b365441b668f28153f3eee95634840d43f69744cd16091bcc4f448afcb  010_principal_quotas.sql
b135b3a2b3490cfa16135ad6d4fce5854f69b4ad44bb9bea14e34a06ac4e6fb2  011_coordination_deadlines.sql
c6762cdd3beed62f089a3094e30ab796d7af95275efcc2217e36f93509c7d540  012_artifacts.sql
79f41576f4e380486891aecf0026952b08ffc487d2caf0948388860f18d32f4c  013_artifact_authority.sql
32ca6ad8e60fb9886a8ca2b782175359454f94205a7452dc4e86e620f9541163  014_agent_control_rotation.sql
1e8f7e0d0a08100b2cdd0abca67b1ffca8688a504cc526e908d84bc323ea05a2  015_private_artifact_grants.sql
e7c439471609dbff95f9d5f4bae48e5bda4abf43a0f90d09cc24265c71f1377b  016_task_progress_evidence.sql
9828ddeae1accf601d67123e23e5a80da6a5c5abcdc77ff6bb37e448df1e199e  017_owner_agent_issuance_recovery.sql
3c873b2a27a3bfc83669e73deee326992110a619650dfb5320d3003b4be7d537  018_verifier_isolation_evidence.sql
89bb49a9182de558293263cb5b79796858801f849351fa3d3e3b1543563e4558  019_private_report_intake.sql
2f9c2401d74bb346964267ea81fd337e0180397af086a875dfece05475398b6b  020_intent_grant_revision.sql
32e9538514d5d597dd61e94c6f9b516cd6afbf06e90af419709c1a3c644a827e  021_network_oauth_credentials.sql
```
