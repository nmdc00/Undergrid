# Owner exact-plan review and approval

Implements the owner-console portion of TDD §9 and SW-04: an authenticated
owner selects an active agent they own, reviews a sealed plan and its contribution
terms, and records an exact version/hash decision. The console vertical merged
into `release/v0.1` through PR84 (`cf4a510`). The additional policy-lock correction
below remains on its separate fix branch pending review. No deployment or full
release qualification is claimed.

The feature reconciles release `548b59d` (renewal collection and owner invitations).
The only source conflict was the participation panel import; both invitation and
plan-review controls are retained. OpenAPI was merged structurally, preserving
both feature path/schema maps, and TypeScript wire types were regenerated from
that combined canonical document.
The integrated full UI suite passed **66 tests across 9 files** in 18.78s
(`/tmp/undergrid-owner-plan-merge-ui.log`); TypeScript/Vite passed (Vite 1.24s,
`/tmp/undergrid-owner-plan-merge-build.log`), and the combined contract passed
Redocly, compilation of 135 JSON Schema components and generated-type parity
(`/tmp/undergrid-owner-plan-merge-contract.log`). The subsequent root-coordinated
exact-state PostgreSQL integration gate is recorded below.

## Runtime contract

- `GET /v1/owner/agents/{agent_id}/swarms/{id}/plans` lists up to 100 plan
  summaries, newest first. `before_version` is an exclusive pagination cursor.
- `GET /v1/owner/agents/{agent_id}/plans/{id}` returns the sealed canonical
  document, parsed specification, current task summaries, contribution-template
  hash and the configured terms URL only when its template hash still matches.
- `POST /v1/owner/agents/{agent_id}/plans/{id}/approvals` requires an idempotency
  key, exact `version` and `content_hash`, and `decision` (`approve` or `reject`).
  Approval additionally requires `acknowledge_contribution_terms: true`.

The owner-session path locks the owned active attribution agent and checks current
room authority before reading or recovering an idempotent decision. It then calls
the existing `approvePlan` transaction. That command owns current proposal/deadline,
required-principal, current rights-policy and sealed-template checks, and records
the existing separate `plan` and `rights` approval rows/events atomically. This
adapter does not duplicate those transition rules or introduce an agent bearer
credential into the browser.

The console displays the objective, exact hash/canonical document, approvers,
task dependency/output/check/deadline details, contribution terms and their hash.
Content is rendered as text. A separate unchecked contribution-rights acknowledgement
is required before approval; unavailable or unsafe terms URLs disable approval.
Changing the attributed agent/room/session clears reviewed authority. Stale or
revoked responses discard it and require another review. A 401 invokes owner logout.

Plan/contribution-rights votes preserve the existing published plan contract.
They do not accept assignments, approve a publication manifest, grant spending
authority or call the financial sandbox. The UI states these boundaries explicitly.

## Measured evidence

The real PostgreSQL owner HTTP lifecycle test is
`TestOwnerPlanReviewAndExactApprovalPersistSharedDomainEffects`:

- Initial behavioral RED: the new owner route returned 404; focused run failed
  in 2.366s (`/tmp/undergrid-owner-plan-red96.log`).
- GREEN after wiring the narrow owner-session scopes: focused race run passed
  in 2.660s (`/tmp/undergrid-owner-plan-green-fixed96.log`).
- The test observes exact sealed review/terms, owner plan discovery, foreign-owner
  denial, wrong hash/version and missing acknowledgement denial with no approval
  writes, idempotent recovery, exactly one plan and one rights vote, no assignment
  acceptance, and denial of review/replay after membership withdrawal.

The full UI suite passed **59 tests across 8 files** in 16.37s, including exact
approval, explicit acknowledgement, stale/revoked authority, unsafe terms and late
review response characterizations (`/tmp/undergrid-owner-plan-ui-final.log`).
TypeScript and the Vite build passed (Vite 1.30s,
`/tmp/undergrid-owner-plan-build.log`). The OpenAPI contract check passed Redocly,
compilation of 134 JSON Schema components, and generated-type parity
(`/tmp/undergrid-owner-plan-contract.log`). No dependencies were installed.

This is local application/fixture evidence, not external legal approval,
provider qualification, a full release gate or production validation.

## Quality assessment and remaining scope

DRY: owner HTTP and agent APIs share `approvePlan`; the owner adapter adds only
owner attribution, exact-version and explicit human acknowledgement requirements.
Authorization remains in the same transaction as voting and idempotent recovery.
Orthogonality: the browser uses owner cookies/CSRF and the existing request retry
handling; the financial sandbox has no dependency on this path. No migration or
new policy framework is introduced. Runtime response parsing rejects malformed
plan/hash/task shapes, and generation counters discard late responses after a
selection/session change.

This vertical provides console approval and rejection. Rejection submits the exact
reviewed version/hash with `decision: reject`, without an approval acknowledgement,
and shares the same busy/stale/revoked-authority handling. The existing domain
records its plan and rights rejection votes; rejection grants no contribution
rights. The UI rejection oracle first failed on the absent button (1.96s,
`/tmp/undergrid-owner-plan-rejection-red.log`). The final full UI suite passed
**60 tests across 8 files** in 16.35s, including the rejection payload and separate
success state (`/tmp/undergrid-owner-plan-rejection-green.log`).
Final TypeScript/Vite build also passed (Vite 1.35s,
`/tmp/undergrid-owner-plan-rejection-build.log`). The rejection UI changes no
OpenAPI or Go behavior; the contract and HTTP persistence checks above remain
the evidence for those unchanged paths.

Plan creation and revision editing, assignment decisions and publication review
remain separate work. Full TDD and release/provider/deployment gates remain open.

## Confirmed decision response and recovery

The branch reconciles release `110e26f`, including the shared owner-command
response-recovery helper. A reviewer found that the plan control treated any
successful JSON object as confirmation; `200 {}` therefore falsely reported an
approval or rejection. Two actual UI oracles failed before correction (2 failed,
7 passed in 4.67s; `/tmp/undergrid-owner-plan-malformed-red.log`).

Review and decision responses now share a base-plan parser. The decision response
must contain usable plan/specification/task structure, the exact reviewed plan ID,
room, version, hash and sealed canonical document, plus a decision-consistent state.
The existing `approvePlan` command unconditionally sets a rejected plan to
`rejected`; approval may leave it `proposed` while other votes or assignment
acceptances remain outstanding, or return `accepted`. These are the actual command
states, not a new inference of assignment or publication approval. The base Plan
response contains no vote ledger: confirmation relies on the successful exact
command contract; separate PostgreSQL lifecycle proof observes its persisted votes.

Validation runs inside the request helper before the idempotency key is released.
An unusable success, network error or server failure leaves the reviewed projection
available for an exact retry with its original key/body, but clears contribution
acknowledgement so approval must be reaffirmed. A definitive stale/current-authority
denial still discards the review. A further 500-response oracle reproduced the old
review-clearing recovery gap before its correction (2 failed in 1.48s,
`/tmp/undergrid-owner-plan-unknown-red.log`; other tests were intentionally excluded
from that RED run). The combined focused GREEN passed all 11 UI cases in 2.33s
(`/tmp/undergrid-owner-plan-recovery-focused-green.log`).

The unchanged Go implementation on source `2dc9b41` also passed the root-coordinated
exact PostgreSQL integration gate: 3 top-level owner-plan/invitation lifecycle cases,
zero failures/skips, package elapsed 6.537s
(`/tmp/undergrid-owner-plan-exact-integration96.jsonl`). This is prior exact-backend
evidence, not a claim that the new UI changes have a new backend test result.
Final integrated full UI suite: **78 tests across 9 files passed in 20.12s**
(`/tmp/undergrid-owner-plan-recovery-full-ui.log`). Final TypeScript (`tsc --noEmit`)
and Vite build passed (Vite 1.33s,
`/tmp/undergrid-owner-plan-recovery-build.log`). No migration or backend contract
changed in the recovery correction; Go and OpenAPI match the exact-backend-tested
source above. The release flow still requires PR review/merge; this is not a
deployment or full TDD completion claim.

## Plan-wide policy locks for owner decisions

A controlled HTTP/PostgreSQL race reproduced a missing authority lock in the
owner adapter. Agent plan/task mutations already call `lock_plan_authorities`
before their individual actor locks. Owner decisions instead locked only the
owner principal and agent before entering the shared plan transaction. A final
owner vote could count another principal's current policy grant, pause before
accepting the plan, and then commit an accepted head after that other owner had
already committed a policy revocation.

Actual RED: `TestOwnerPlanFinalApprovalSerializesPeerPolicyRevocation` failed in
3.078s (`/tmp/undergrid-owner-plan-authority-red96.log`). The peer's real owner
policy HTTP command returned 200 before final acceptance; independent persistence
reads found an accepted plan and active head with the peer's recorded approval at
policy version 2 while its current policy was version 3. A fixture-only trigger
pauses the plan transition after policy counting to make the ordering deterministic.
It is a controlled timing seam, not a public API capability or a provider result.

Only exact owner plan-decision POSTs now resolve verified owner identity and an
owned active agent without taking row locks, install that proven attribution,
and acquire the existing canonical sorted required-principal lock set. They then
recheck the current owner under lock and continue through the existing locked
agent, room, exact-plan and rights-policy checks. Foreign attribution is rejected
before invoking the privileged lock helper. Unsupported routes and other owner
commands retain their prior locking path. Calling the helper later, after locking
one owner, would invert the agent path's lock order and risk deadlock; the fix is
at the authentication/transaction boundary instead.

The same race oracle then passed with race detection in 2.883s
(`/tmp/undergrid-owner-plan-authority-green96.log`): the peer policy mutation waits
until the accepted-plan transaction commits. A later policy revocation remains
possible; this fix serializes acceptance against revocation rather than erasing
historical approvals. The controlled trigger, advisory lock and HTTP contexts are
bounded and cleaned up on failure/cancellation.

The affected race gate passed **6 top-level cases and 2 post-wait subcases** in
12.959s, zero failures/skips
(`/tmp/undergrid-owner-plan-authority-affected96.jsonl`). It includes loss of the
attributed agent or owner while the real HTTP request waits on the canonical lock
set; both deny with 403 and leave zero votes and idempotency rows. Existing owner
plan, invitation/participation and independent agent assignment/plan lifecycle
tests also pass. `go vet ./internal/network` passed
(`/tmp/undergrid-owner-plan-authority-vet.log`).

After reconciling release `d1e866d` (owner room messages), the exact runtime source
`fdeddf265f5bc4e9f8f7c682af24877b33a68199` passed **9 top-level cases and 2
post-wait subcases** with race detection in 16.263s, zero failures/skips
(`/tmp/undergrid-owner-plan-authority-reconciled96.jsonl`). This adds all three
owner-message attribution/current-authority/queue-rollback lifecycles to the
affected plan/participation/assignment checks. Vet passed again
(`/tmp/undergrid-owner-plan-authority-reconciled-vet.log`). Subsequent evidence
recording changes documentation only; no runtime source changes after that gate.

No migration or wire response changes. DRY: both owner and agent adapters
use the same canonical authority-lock function and shared domain command; no new
policy evaluator is introduced. Orthogonality: only owner plan-decision routing
changes, and all effects remain in its existing network transaction. This
checkpoint is on `fix/v0.1-owner-plan-authority-locks`; it is not yet merged or
deployed and does not complete the TDD.
