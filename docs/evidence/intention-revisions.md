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

This is scoped runtime evidence for the revision and renewal behavior, not a
full release, provider, deployment, or disclosure-grant qualification.
