# Owner command response recovery

This focused console fix preserves the original command identity when an HTTP
success contains an unusable result. It supports TDD §4 idempotency/recovery and
the owner workflows in §9. It does not change backend authorization or command
effects. Branch: `fix/v0.1-owner-command-response-recovery`, based on release
`548b59d`; merge and deployment remain pending.

## Failure and resulting behavior

The server may commit a room or invitation even when the browser receives a
successful `{}` response. Previously `request()` cleared its pending idempotency
key before the component checked required fields. The component then reported an
error and offered a same-form retry, which generated a new key. That could create
a second room or lose recovery of the original invitation.

The request helper now accepts an optional synchronous result validator. It runs
after JSON/object parsing and before clearing the pending key, including for a
204 response. Creation and invitation controls supply their existing structural
checks there. An invalid successful result preserves the draft, body, original
key and precondition for an exact retry; a usable response clears the key so a
later intentional command obtains a new identity. Network errors, malformed JSON
and server errors retain the existing recovery behavior; explicit client errors
continue to clear the key. The bounded in-memory retry map and owner cookie/CSRF
identity remain unchanged. No keys or bearer credentials are persisted in browser
storage.

The caller inventory also found credential rotation/recovery: the App only shows
an issued secret when `result.token` exists, but previously accepted `{}` and
refreshed the agent list. That could discard recovery of a newly issued credential
whose predecessor was already revoked. The helper now requires a nonempty ID and
token for the exact owner rotation/recovery POST routes, as it already did for
agent creation, and rejects 204 for those issuance commands. These checks occur
before key release and before the App refresh. Backend replay remains bounded by
the existing credential replay lifetime and current-authority checks.

Other existing mutation callers were inspected: intention revisions/status/grants
carry `If-Match`, so a fresh key after a malformed result does not bypass version
checks or duplicate effects. Their broader rendering validation is outside this
bounded fix. Membership decisions reload authoritative participation rather than
using the mutation result. Plan and message feature changes remain separate;
this branch introduces no message composer or worker behavior.

## Measured evidence

- New room-creation UI oracle: actual RED on different keys after `201 {}` and
  same-form retry, 1 failed/6 passed in 2.70s
  (`/tmp/undergrid-owner-create-malformed-red.log`).
- New credential issuance request oracles: rotation and recovery each accepted
  unusable 201/204 before the fix, 4 failed/8 passed in 999ms
  (`/tmp/undergrid-owner-credential-malformed-red.log`).
- Invitation and callback tests were reused from the stable message-worker
  review fix. Its prior actual invitation RED is
  `/tmp/undergrid-owner-invite-malformed-ui-red.log`; this task did not rerun that
  historical RED or claim authorship of it. The message-specific RED remains
  separate (`/tmp/undergrid-owner-message-malformed-ui-red.log`).
- Focused GREEN: 28 tests across API, creation and invitation files passed in
  6.07s (`/tmp/undergrid-owner-command-focused-green.log`). This includes original
  key/body preservation for malformed success, successful recovery, and key
  release after validated completion.

Full UI GREEN: **67 tests across 8 files** passed in 56.31s
(`/tmp/undergrid-owner-command-full-ui.log`). TypeScript (`tsc --noEmit`) and the
Vite build passed (Vite 1.65s, `/tmp/undergrid-owner-command-build.log`).
No PostgreSQL/backend code, migrations, dependencies or external provider calls
change in this fix; it is browser/request behavior evidence, not a new backend
replay qualification or provider result.

## Quality review

DRY: recovery-key ownership stays in the request helper, while each component
retains its resource-specific result contract through a small callback. Issuance
validation is centralized because all issuance paths must retain recoverable
one-time secrets. Orthogonality: UI validation cannot authorize backend effects;
the existing transaction, preconditions and current identity still decide them.
The callback runs before key cleanup and before component state changes; a thrown
validation error leaves recovery state intact. No framework, new retry policy or
unrelated component refactoring is introduced.
