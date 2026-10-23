# Owner assignment review and response

Implements the owner-console portion of TDD §7/§9, SW-03/SW-04: an authenticated
owner reviews responsibilities assigned to a selected owned agent and explicitly
accepts or declines. This feature is on `feat/v0.1-owner-assignment-response`, based
on release `ab330e3`. PR review/merge and deployment remain pending. No new migration, financial provider or external approval is introduced.

## Runtime contract

- `GET /v1/owner/agents/{agent_id}/plans/{id}/assignments` returns at most 50 task
  projections assigned to this agent, under current accepted room authority.
  The existing `taskResult` filters credited evidence for the current principal.
- `POST /v1/owner/agents/{agent_id}/tasks/{id}/assignment-response` requires the
  task `If-Match`, exact `plan_hash`, an idempotency key, and `decision:
  accept|decline`. Acceptance additionally requires
  `acknowledge_responsibility_and_terms: true` and the existing domain rights
  policy. The acknowledgement does not manufacture a policy grant or vote.

The canonical sorted required-principal lock set precedes individual owner and
agent locks. Task-to-plan resolution requires proven owned-agent attribution and
current room visibility. Owner and agent status, assignment attribution and room
membership are rechecked after waiting and before retained idempotent recovery.
The original owner body, including its acknowledgement, remains in the recovery
fingerprint; only the payload passed to `updateTask` is translated.

The shared task transaction owns deadlines, ETag/hash, workload, rights policy,
task transition and atomic event/fanout. Assignment acceptance can satisfy the
last prerequisite of an already approved plan; it creates no plan, rights,
publication or spending approval rows. Admission remains a separate decision.
Task events do not increment the separate chat sequence.

The console embeds assignment review in exact plan review, displays task
specification/dependencies/checks/deadlines as untrusted text and requires a
separate responsibility/terms checkbox before acceptance. Decline needs no such
acknowledgement. Selection/session changes invalidate late responses. Unusable
success, network failure or 5xx preserves the reviewed task and original retry
key/body while clearing acknowledgement. Definitive 4xx discards the review; 401
also invokes owner logout. Result parsing verifies exact task/plan/agent,
version+1, matching ETag and decision-consistent state before releasing the key.

## Measured evidence

The actual PostgreSQL/HTTP missing-route RED returned 404/not_found in 13.347s
(`/tmp/undergrid-owner-assignment-red96.log`). The service uses an actual restricted
application LOGIN without worker membership.

The corrected PostgreSQL race gate passed **3 top-level cases and 6 subcases** in
**8.305s**, zero failures/skips
(`/tmp/undergrid-owner-assignment-affected96.jsonl`):

- exact review, acceptance and retained retry; foreign owner/different owned agent,
  missing acknowledgement, wrong hash and missing/stale task ETag denials;
- zero approval votes after assignment acceptance; withdrawn-member replay denial;
- decline, expired offer and controlled queue failure, observing normalized task
  state/version, events, command recovery rows and unchanged chat sequence;
- actual canonical-lock waits observed with `pg_blocking_pids`, then revoked agent,
  suspended owner or withdrawn membership denial without votes or command rows.

The initial affected run failed an incorrect test expectation that task events
increment chat sequence (package 6.848s). The observed decline state/version,
event and recovery row were correct. Only the oracle was corrected; domain
behavior was not changed to satisfy that assertion. The final integrated gate below also proves two-member own-task filtering.

Sequential `go vet ./internal/network` passed
(`/tmp/undergrid-owner-assignment-vet.log`). No Go runtime source changes after
that gate. Canonical Redocly validation, compilation of **140 schema components**
and generated TypeScript parity passed
(`/tmp/undergrid-owner-assignment-contract.log`). The HTTP lifecycle also runs
Ajv against actual responses and independent acknowledgement/closed-input attacks.

Focused UI proof passed **8 tests in 1.84s**
(`/tmp/undergrid-owner-assignment-ui-final-focused.log`), including malformed 200
and actual 500 responses for both decisions, identical retry key/body/ETag,
reaffirmed acceptance acknowledgement, malformed ETag and 401/403/409 clearing.
The ETag review oracle first failed (1 failed, 2 passed, 2.90s;
`/tmp/undergrid-owner-assignment-ui-etag-red.log`), then passed after requiring the
exact quoted task version. Initial UI tests were authored before the component
but first executed after implementation; no initial UI behavioral RED is claimed.
Full UI, build and integrated legacy plan/assignment gates subsequently passed as recorded below.

These are local application/fixture results, not provider qualification, external
legal/source approval, deployment or full TDD completion evidence.

## Quality assessment

DRY: owner and agent entrypoints share `updateTask` and its rights/workload and
conditional plan-acceptance rules. The owner adapter owns only cookie attribution,
explicit human acknowledgement and its closed owner request shape. Both use the
same canonical policy lock helper. Orthogonality: network owner-session routing,
CSRF/retry handling and audience-safe task projection have no dependency on the
financial sandbox or provider credentials. Existing request recovery owns key
lifecycle; the feature's validator owns its decision-specific result policy.

No generic policy engine or new storage is introduced. The bounded SQL collection
is closed before individual task projections run, and errors propagate to the
existing outer transaction rollback. Root review found no material authorization
or response-recovery issue in these paths. The final checks below include affected legacy plan-lock callers.


## Final integrated validation

The final exact working-tree runtime passed **8 top-level PostgreSQL race cases
and 8 subcases in 15.490s**, zero failures/skips
(`/tmp/undergrid-owner-assignment-final-integration96.jsonl`). This adds a real
two-member revised plan with separately assigned tasks and verifies that each
owner's selected attribution agent receives only its own assignment. The same
gate includes existing owner plan review/approval, final-vote peer-policy
serialization, owner/agent post-wait revocation and independent agent assignment
acceptance. Sequential final vet passed
(`/tmp/undergrid-owner-assignment-final-vet.log`).

The full owner console suite passed **93 tests across 11 files in 23.55s**
(`/tmp/undergrid-owner-assignment-full-ui.log`). TypeScript checking and Vite build
passed (Vite 1.26s; `/tmp/undergrid-owner-assignment-build.log`). Final canonical
contract checks passed Redocly, 140 compiled schema components and generated type
parity (`/tmp/undergrid-owner-assignment-contract-final.log`). No runtime source
changes followed these gates; this evidence recording changes documentation only.
PR merge and release/deployment qualification remain separate gates.
