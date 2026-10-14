# Intention revisions — INT-01 / ID-03, TDD §§4–5

`PATCH /v1/intents/{id}` and the owner-console equivalent accept the existing
`{"state": ...}` mutation or an explicit `{"revision": <IntentInput>}`. Revision
requests require the existing exact `If-Match` ETag and idempotency key. The
domain command locks the owner-scoped intent, validates the currently supported
typed input, and appends a revision document, projection, and canonical-input
hash. It advances `current_revision`, version, expiry, and effective watch in
the same transaction as the event, inbox item, and durable discovery work.
Replays return the committed response without duplicating those effects.

Edits to active and paused intentions preserve their lifecycle state. Renewal
of an expired intention requires a valid future expiry and active-intention
quota, then resumes it as active under the appended revision. Withdrawn
intentions remain terminal. Existing state-only pause/resume/withdraw requests
retain their behavior. The owner path uses the same domain command; the console
can revise an objective or expiry while preserving all other current typed
fields, and offers renewal for expired records.

Read paths already exclude expired timestamps from public demand, opportunities,
and agent watch context, even before the worker records `state='expired'`. A
revision submitted while the stored state is still active keeps that state and
its existing quota slot. The quota admission check applies when renewing a row
whose stored state is expired, using the existing state-based active-intent
count.

Revision changes keep the existing visibility value. A request that changes
visibility fails with `visibility_change_requires_grant`; revision editing does
not imply disclosure consent. Redacted-public projections retain the existing
allowlist (`public_summary`, `output`, and `hard_constraints`). Field-scoped,
expiry-bound sharing grants and consented disclosure transitions remain deferred
under ID-03. This branch implements the currently supported `IntentInput`
contract and does not claim a generalized intent schema.

No migration was needed: `intent_revisions` already denies app-role updates and
allows inserts, while the deferred current-revision foreign key permits the
append and pointer advance to commit atomically. The actual HTTP tests use the
restricted application role and real UTF-8 PostgreSQL at port 55458. They assert
old-row immutability, exact canonical hashes, current document/projection and
pointer/version, stale/wrong-owner/invalid/visibility-change denials, owner and
agent paths, pause preservation, worker-driven expiry, renewal, replay, fresh-
handler recovery, and event/inbox/scan-job idempotency.

Validation on this branch:

- Focused Go lifecycle tests with `-race`, including the unchanged
  `TestDurableParticipation` and owner isolation contract.
- `go vet ./...` and `go build ./...`.
- Owner-console Vitest suite and TypeScript/Vite production build.

A broader `go test ./...` attempt at the time did not pass. The network package
reported a failure in `TestCommittedDiscoverySurvivesBlockedRecruitment`: the
controlled recruitment-lock wait was not reached, and its application-name
connection variant could not parse the URL-form test DSN. The queue-fairness
fixture and durable-worker scheduling were subsequently corrected through PR
#26 and follow-up PRs #29 and #30. The latest full network race on integrated
release 014+015 passed in 583.538 seconds (74.1%); that result does not establish
the full suite with migrations 016 or 017. Sandbox tests also require separate
sandbox admin/runtime database DSNs, which were not configured in the earlier
network-only run. No provider or financial-sandbox qualification is claimed.

## Release reconciliation and current-source proof

This feature branch was reconciled with `origin/release/v0.1` at `67b584e`
without conflicts. That release includes migration 015; this branch does not
include progress migration 016 or owner issuance migration 017. The revision
implementation and its tests were validated at source SHA
`40fd64613be22cd2caec6e6dfa90db36d3090b6a` on a fresh PostgreSQL 16.11 database
on port 55476, created with UTF8 and `C.UTF-8` collation/character type. The
unmodified application migrator created migrations 001–015 in order and River
readiness; no SQL was manually replayed and no migration ledger was edited.
`undergrid_app` and `undergrid_worker` are `NOSUPERUSER NOCREATEROLE NOCREATEDB
NOBYPASSRLS`.

The following actual HTTP/PostgreSQL tests passed under `-race` in 15.001s:
`TestIntentionRevisionIsImmutableAndOwnerCommandSharesDomainPath`,
`TestExpiredIntentionCanBeRenewedByAppendingRevision`,
`TestDurableParticipation`, and
`TestOwnerConsoleRegistrationAndIntentionIsolation`. On that source tree,
`go vet ./...`, `go build ./...`, and `git diff --check` also passed. No owner
issuance or progress-016 integration is claimed by this evidence.

This is scoped runtime evidence for the revision and renewal behavior, not a
full release, provider, deployment, or disclosure-grant qualification.
