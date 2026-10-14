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

- Final reconciled implementation SHA: `ce5433e` (release baseline `06fc773`).
  Fresh PostgreSQL 16.11 cluster `/tmp/undergrid-pg-sharing-55479/data`, socket
  `/tmp/undergrid-pg-sharing-55479/socket`, port 55479, database `postgres`,
  UTF-8 / `C.UTF-8`, initially had no migration ledger or Undergrid roles.
  The unmodified application `Migrate` recorded raw network migrations
  001–017 plus 020 and `river-0.49.0-ready` (19 ledger rows). Migrations
  018 and 019 are absent; this is explicitly not contiguous 001–020 proof.
  No SQL replay, role omission, or migration-ledger rewrite was used.
  `undergrid_app` and `undergrid_worker` are NOLOGIN, NOSUPERUSER,
  NOBYPASSRLS with no role memberships. This includes the final progress
  privacy migration 016 (`e7c439471609dbff95f9d5f4bae48e5bda4abf43a0f90d09cc24265c71f1377b`)
  and owner recovery 017 (`9828ddeae1accf601d67123e23e5a80da6a5c5abcdc77ff6bb37e448df1e199e`).
  Migration 020 SHA-256 is
  `2f9c2401d74bb346964267ea81fd337e0180397af086a875dfece05475398b6b`.
  Financial migration 008 remains byte-identical
  (`5fe8b8871772fa9d0a99ccd4ae61c4ca2187b5ddf8ab1e2a0acb207695f0cfe4`);
  no financial migrations were applied to this network fixture.
- `go test -race ./internal/network ./internal/identity -run
  'Test(Restricted|GrantRequires|IntentionRevision|ExpiredIntention|Owner)'
  -count=1 -timeout=10m` passed: network 64.992s, identity 1.022s.
- These tests cover HTTP owner grant/list/revoke,
  wrong owner and agent-credential denial, browser CSRF denials before any row
  is written, exact field masking, explicit budget selection, raw foreign-row
  RLS denial, scope enforcement, stale and missing ETags, changed-body replay,
  handler-restart replay, invalid fields and expiries, source pause/expiry,
  revision invalidation, legacy NULL-row inactivity and explicit regrant, and
  concurrent read/revoke followed by a denied fresh read. Event/inbox/version
  effects are checked against PostgreSQL.
- Unchanged owner-console registration/isolation and durable participation
  passed separately with `-race` (2.895s).
- `go vet ./internal/network ./internal/identity ./cmd/undergrid` and
  `go build ./cmd/undergrid` passed. The owner-console Vitest suite passed
  all 17 tests and the TypeScript/Vite production build passed. An earlier
  unchanged run timed out starting the App test worker; it executed eight
  other tests but did not qualify the full suite. The successful unchanged
  retry is the final evidence.
- The initial red HTTP test returned 405 for the owner grant path before the
  route existed; after implementation the same test completed the database
  lifecycle. An early invalid test budget was corrected to the published
  `stated_budget` shape before treating the route failure as red evidence.

The combined sorted migration 018/019 integration gate remains pending for
root review. Earlier raw 55467 proof covered only 001–014 plus 020 and is
superseded by the reconciled 55479 proof above. DRY review keeps grant policy
in its transactional command and the restricted-read SQL projection; UI field
selection cannot authorize access. SQL and Go allowlists defend separate trust
boundaries. Read/revoke semantics explicitly use committed PostgreSQL snapshots
rather than promising to cancel an already executing read. The identity test uses a signed local session fixture;
it proves the CSRF/owner transaction boundary, not external OIDC-provider
verification. No provider, source-permission, deployment, legal, financial, or
full-release qualification is claimed. The branch remains local and unmerged.
