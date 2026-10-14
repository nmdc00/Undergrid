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
allowlist (`public_summary`, `output`, and `hard_constraints`). This feature
branch adds expiry-bound selected-field grants for already-restricted
intentions; see [restricted intention sharing](restricted-intention-sharing.md).
It does not implement consented disclosure transitions or a generalized intent
schema.

The revision/renewal change needed no schema migration: `intent_revisions`
already denies app-role updates and allows inserts, while the deferred
current-revision foreign key permits the append and pointer advance atomically.
This sharing feature adds migration 020 to bind new grants to a revision while
leaving legacy grant rows unbound. The revision tests used real UTF-8 PostgreSQL
at port 55458. They assert old-row immutability, exact canonical hashes, current
document/projection and pointer/version, stale/wrong-owner/invalid/visibility-
change denials, owner and agent paths, pause preservation, worker-driven expiry,
renewal, replay, fresh-handler recovery, and event/inbox/scan-job idempotency.

Validation on this branch:

- Focused Go lifecycle tests with `-race`, including the unchanged
  `TestDurableParticipation` and owner isolation contract.
- `go vet ./internal/network ./internal/identity` and `go build ./...`.
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

The first release reconciliation used `origin/release/v0.1` at `67b584e` and
did not conflict. That release included migration 015; at that validation point
the branch did not yet include progress migration 016 or owner issuance
migration 017. The revision implementation and its tests were validated at source SHA
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

The later current release `f6726c6` added migrations 016 and 017. Before that
release was merged, this same fresh cluster had already applied 001–015 through
the real application migrator. After merging `f6726c6`, the focused race suite
ran on the existing 55476 database and the unmodified migrator applied 016 and
017 after the recorded 015 row. The final ledger is ordered 001–017 followed by
River readiness; the database was not reset, migrations were not replayed, and
the ledger was not edited. Migration 016 SHA-256 is
`e7c439471609dbff95f9d5f4bae48e5bda4abf43a0f90d09cc24265c71f1377b`; migration
017 SHA-256 is
`9828ddeae1accf601d67123e23e5a80da6a5c5abcdc77ff6bb37e448df1e199e`.

That post-upgrade selection passed in 73.057 seconds and covered revision edit
and renewal, expired-renewal quota denial, durable participation and owner
isolation, owner agent-issuance response-loss/replay/window/concurrency, the
progress-audience grant boundary and lifecycle, and private-grant assembly and
revocation. The new quota denial test creates an expired intention, fills the
principal's 25 active-intention slots through HTTP, and proves renewal returns
429 without changing its revision pointer, version, state, expiry, event,
inbox, or cause-specific durable job counts. The validated application source
is commit `84a46753972359edf83f01c8f12d158f5de38616`.

After the release reconciliation, `go vet ./...`, `go build ./...`,
`git diff --check`, owner-console Vitest (14 tests across 3 files) and
`npm run build` all passed. This is still focused evidence, not a full package
race, release gate, or provider/deployment qualification.

This is scoped runtime evidence for the revision and renewal behavior, not a
full release, provider, deployment, or disclosure-grant qualification.
