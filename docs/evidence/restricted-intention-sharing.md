# Restricted intention sharing — ID-03 / TDD §§4–5

This branch adds explicit field-scoped reads for already-restricted intentions.
An owner can list grants, name one existing active recipient principal, choose a
nonempty allowlisted field set, and set an expiry after now and no later than
the source intention expiry. The command requires the current `If-Match` ETag
and an idempotency key. Grant or revoke updates the parent version and writes
the matching event and owner inbox item in the same transaction. Replays return
the stored result; a changed body under the same key conflicts.

The command never changes intention visibility. Private and redacted-public
intentions cannot receive these grants. Creating an intention in the owner
console now offers private or restricted visibility and defaults to private.
The restricted grant editor requires a recipient principal ID and explicit
field selection; budget is unchecked by default. The budget is absent from a
recipient response unless that exact field is selected.

Recipient reads keep `intent_grants` and `intent_revisions` owner-only under
RLS. A fixed-search-path `SECURITY DEFINER` function derives the recipient from
transaction-local authenticated principal and agent context, requires the
source and recipient to remain active, the source to remain active and
unexpired, and the grant to remain unexpired and bound to the current revision.
It returns only a small metadata envelope plus allowlisted fields named in the
grant, including separate source and grant expiry timestamps. Other agents
under that same principal need their own `intents:read` scope; the grant does
not bypass credential scopes.

Migration 020 adds a nullable revision FK to the existing grant table. It does
not backfill prior grants: NULL-revision legacy rows are inert. A new revision
makes prior grants stale at read time without deleting their audit row. The
owner must explicitly grant the new current revision. Revocation expires the
grant row in place. A read that began before a concurrent revoke commit may
finish from its earlier statement snapshot; a fresh request after that commit
receives the same 404 as an absent grant. Grant and revoke do not widen shared
search, public projections, opportunity evidence, or cross-visibility
transitions.

Validation on this branch:

- Implementation code and tests are committed at `a1c77c67d5f7e5d7c0039b425b211c7897d2c5a2`.
  The focused `go test -race ./internal/network ./internal/identity` selection
  passed against fresh database `undergrid_sharing_proof` on port 55467, after
  initializing a new PostgreSQL 16.11 cluster with UTF-8 encoding and
  `C.UTF-8` locale. The cluster initially had no Undergrid roles or migration
  ledger. The embedded migrations created `undergrid_app` in 001 and
  `undergrid_worker` in 002; both are `NOLOGIN`, non-superuser, and
  `NOBYPASSRLS`, and neither has role memberships. The exact Undergrid ledger
  entries were `001_participation.sql` through `014_agent_control_rotation.sql`
  and `020_intent_grant_revision.sql`; the test harness also recorded
  `river-0.49.0-ready`. This proves migration 020 from raw cluster state with
  the migrations available on this branch. It does not claim contiguous
  migrations 001–020; migrations 015–019 are absent here and remain an
  integration check for the root owner.
- These tests cover HTTP owner grant/list/revoke,
  wrong owner and agent-credential denial, browser CSRF denials before any row
  is written, exact field masking, explicit budget selection, raw foreign-row
  RLS denial, scope enforcement, stale and missing ETags, changed-body replay,
  handler-restart replay, invalid fields and expiries, source pause/expiry,
  revision invalidation, legacy NULL-row inactivity and explicit regrant, and
  concurrent read/revoke followed by a denied fresh read. Event/inbox/version
  effects are checked against PostgreSQL.
- The unchanged owner-console/revision lifecycle and durable participation
  tests passed with `-race`.
- `go vet ./internal/network ./internal/identity` and `go build ./...` passed.
  The owner-console Vitest suite (12 tests) and TypeScript/Vite production
  build passed before the implementation commit.
- The initial red HTTP test returned 405 for the owner grant path before the
  route existed; after implementation the same test completed the database
  lifecycle. An early invalid test budget was corrected to the published
  `stated_budget` shape before treating the route failure as red evidence.

Migration files 015–019 belong to concurrent branches and are not included in
this isolated branch. The combined sorted migration gate remains an integration
check for the root owner. The identity test uses a signed local session fixture;
it proves the CSRF/owner transaction boundary, not external OIDC-provider
verification. No provider, source-permission, deployment, legal, financial, or
full-release qualification is claimed. The branch remains local and unmerged.
