# Canonical network REST contract — TDD §§9/10

Implementation source SHA `5cddae9c490f54d3cc7b0b666b2debbbc2263a60`, reconciled
with release `a3349d2` (network migrations 001–021 and response ETags). Canonical
source `internal/network/api/openapi.json` SHA-256:
`5f7898627fa006f0dbaf39a7d49f0f8e816fa30f477f34c233ccf930c2096c88`.
There is no schema migration for this behavior. This evidence commit changes
only documentation; the implementation remains local, unmerged and undeployed.

The public document is served before database or credential handling. Actual red
GET `/openapi.json` with an unavailable DSN returned 500 `internal_error`; green
returns the canonical document. A second red POST attempted the database and
returned 500; mutation methods now return safe 405 before database handling.
Executable mux wiring is covered by `TestExecutableRoutesPublicDocumentWithoutDatabase`
so the document cannot fall through to the static console unnoticed.

Fresh cluster `/tmp/undergrid-pg-openapi-55482/data`, socket
`/tmp/undergrid-pg-openapi-55482/socket`, port 55482, database `postgres`, uses
PostgreSQL 16.11, UTF-8 and `C.UTF-8`. It began with no Undergrid roles or migration
ledger. The unmodified `Migrate` applied all sorted network migrations 001–021
plus `river-0.49.0-ready` (22 ledger rows). No SQL replay, ledger rewrite or role
creation omission was used. Final progress privacy migration 016 SHA-256 is
`e7c439471609dbff95f9d5f4bae48e5bda4abf43a0f90d09cc24265c71f1377b`;
OAuth migration 021 is
`32e9538514d5d597dd61e94c6f9b516cd6afbf06e90af419709c1a3c644a827e`.
The app/worker roles are NOLOGIN/NOSUPERUSER/NOBYPASSRLS with no memberships.
Actual runtime fixture logins `undergrid_artifact_api_runner` and
`undergrid_artifact_worker_runner` are LOGIN/NOSUPERUSER/NOBYPASSRLS and belong
only to `undergrid_app` and `undergrid_worker`, respectively. The fixture proves
these local authority boundaries, not production identity/deployment approval.
No financial migrations or credentials were applied to this network fixture;
financial migration 008 remains unchanged at
`5fe8b8871772fa9d0a99ccd4ae61c4ca2187b5ddf8ab1e2a0acb207695f0cfe4`.

At the implementation SHA, the focused combined race run passed:

```sh
go test -race ./internal/network ./internal/identity ./cmd/undergrid \
  -run '^Test(PublicOpenAPI|OpenAPIRealHTTP|DurableParticipation|OwnerConsoleRegistrationAndIntentionIsolation|OwnerSessionRequiresAuthenticatedSameOriginAction|ResponseETag|ExecutableRoutesPublicDocument)' \
  -count=1 -timeout=10m -v
```

Network 14.475s, identity 1.024s and executable 1.029s. The actual HTTP lifecycle
was independently validated by pinned Ajv JSON Schema 2020-12: final run captured
52 request/response exchanges (51 in the previous passing run; worker evaluation
poll count varies). Exchanges include current identity/config/policy, intent
create/replay/conflicting request/stale ETag/private outsider denial, owner
lists, inbox/context, room/plan/task reads, real sealed upload + verifier worker,
acceptance/publication and public projection, restricted field grants and
revocation, private report receipt/read/scope denial, and configured OAuth
binding/read/revoke with old-token 401. Request/body schemas, error request-ID
correlation, ETag header/body correlation and independently constructed negative
oracles (string version, public budget, published room metadata) are checked.
This is focused lifecycle proof, not runtime coverage of all 72 operations.

`npm run contract:check` passed Redocly CLI 2.63.0 OpenAPI structure validation,
Ajv 8.20.0 + formats 3.0.1 compilation of all 117 components, and pinned
openapi-typescript 7.13.0 generation drift check. The owner console consumes the
generated intention input type, retaining runtime response validation; the
finite visibility type exposed and corrected the prior generic-string state.
All 17 Vitest tests and TypeScript/Vite production build passed. Go vet for
network/identity/executable and Go executable build passed; `git diff --check`
passed. A temporary generator API/CLI comment-header mismatch was a check-harness
issue; the final native `--check` proves exact deterministic output.

Quality assessment: one canonical JSON document owns network wire types; the UI
consumes generated types rather than repeating the intention knowledge. Domain
validation, transactions and independent schema validation remain separate trust
boundaries. Public and private artifact schemas preserve disclosure differences;
the schema and Go accessor cannot grant domain authority. Test-only HTTP observers
capture actual bytes without altering service behavior. Identity bootstrap,
network resource authority and financial credentials remain separate. No generic
framework or arbitrary agent execution was introduced.

`docs/NETWORK_REST_CONTRACT.md` inventories current endpoints and explicitly names
remaining TDD idempotency/list/precondition gaps. This work does not waive those
requirements or claim MCP/A2A implementation, external OAuth provider verification,
legal/source approvals, qualified verifier hard limits, financial rail execution,
production deployment, or full-release completion.
