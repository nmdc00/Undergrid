# Owner application, coordinator offer and separate admission

## Contract — 28 October 2026

HP2 step3 / SW-01/02 / TDD §6.3: finish the normal-owner path from a current
recruitment summary to an explicit application, coordinator-reviewed offer and
separate applicant acceptance/decline. Work starts from release `126ffb1` in
`feat/v0.1-owner-applications`. Established swarm workers remain quota-limited;
root continues without replacement workers or model switches. Full HP2 stays open.

Existing agent commands implement application and application-origin invitation,
but the owner bridge has neither application submission nor coordinator review.
Owner participation lists exclude applied memberships. applyToSwarm decodes
capability_refs but currently discards them, and application-origin offers need
explicit current-target/review/recovery proof. Retain the existing membership
state machine rather than building a second application system.

Add owner POST `/v1/owner/agents/{agent}/swarms/{swarm}/applications`, current
applicant pending-application reads, and coordinator-only bounded application
review/offer controls. Existing agent application/invitation wire contracts remain
compatible. Normal owner role review must pin the current room/role version using
If-Match; stale review cannot create a new application. Preserve proposed role and
untrusted capability references durably and immutably, including null legacy
claims, rather than manufacturing competence evidence. References are inert data,
not URLs or content to execute. Keep the default bounded application deadline and
current expiration/revocation policy. A purpose-specific migration is permitted;
frozen migrations and network/financial isolation remain intact.

Current recruitment summaries must disclose only permitted role/objective/profile
facts and the full-history policy. Recheck current recipient/role/room eligibility
before presenting actionable evidence; old opportunities cannot silently remain
current after their role/profile inputs change. Public summary access is never
private transcript access. Applicant and coordinator reads are separate bounded
views with current actor attribution; no foreign/private profile or owner data.

Coordinator conversion of an application creates only an offer, using the shared
invitation mutation. Review pins room and application version; current coordinator,
applicant principal/agent and application deadline remain authoritative inside the
domain transaction and before retained-response recovery. Admission still requires
the applicant's exact-version accept/decline and full-history review. Application,
offer, inbox acknowledgement or chat grants no plan approval, task assignment,
contribution rights, publication, verified skill or spending authority.

The console must support current recruitment review, reviewed application submit,
pending/reload recovery, coordinator review/offer and separate invitation response.
Use stable reviewed request bodies/keys on lost responses; clear stale agent,
room, revision and session contexts. Closed responses and inert text are required.
Never require an owner to paste SQL identifiers or use an administrator to finish
the actual acceptance journey.

## Required proof before merge

1. Restricted PostgreSQL owner HTTP RED precedes missing behavior. Normal API
   actors/room creation supplies the fixture; application yields one applied
   membership with retained role/capability claims, deadline and no admission or
   transcript access. Agent/owner paths share domain authorization and persistence.
2. Current source/coordinator/target authority, scope, role/version/expiry, immutable
   claim retention, duplicate/replay, changed-body conflict, bounded reads and lock
   contention/recovery are proven at real database seams. Do not place authorization
   solely in adapters or broaden RLS grants to expose foreign identities.
3. Coordinator review → offer preserves exact application context; applicant alone
   accepts/declines. Test expiry/revocation/stale review and response loss without
   duplicate effects or resurrected private data. Separate plan/assignment/rights
   obligations remain unaccepted. Publish canonical owner wire shapes and tests.
4. Missing UI controls/malformed responses have behavioral RED before changes.
   Actual normal-owner browser/HTTP/restricted-worker/PG journey starts with real
   recruitment, applies, recovers pending state, obtains reviewed offer and accepts
   admission, with transcript inaccessible until acceptance. Negative decline and
   original failed gates remain retained; no SQL product seed completes a step.
5. Relevant full/focused UI, canonical/static/build and retained HP2 journeys pass.
   Record exact source/build provenance, normalized oracles and pragmatic quality
   review before merging this feature. The continuous HP2 journey is a subsequent
   integrated acceptance gate, not implied by this bounded feature proof.

No implementation or qualification is claimed by this contract. Reserved external
source/provider/legal/deployment approvals remain separate explicit gates.

## Initial owner HTTP RED — 28 October 2026

Test-only `1b1495d` completes normal restricted-API organizer/builder and room
creation, then reaches the intended missing owner application operation:404
instead of201. The single test fails in0.617s, not during fixture setup. It requires
reviewed room If-Match and checks applied/deadline persistence with no admission or
private-context access once implementation exists. Original log:
`/tmp/undergrid-owner-applications-initial-http-red-oct28.jsonl`.
The retained network fixture is `typed_requirements_final_oct28` on55502; no SQL
product seeds or financial identity/database changes occurred. Implementation and
all remaining contract gates remain pending; the full goal stays active.

## Shared submission and immutable claims checkpoint — 28 October 2026

Test-only `ed2246e` exposes a second genuine gap: the existing agent application
succeeds but its role/capability proposal is not retained (RED0.529s). New
applications now store immutable application-claims-v1 with the selected public
role snapshot, room version and unverified capability references. Historical rows
remain NULL/unknown; no fact backfill is fabricated. Migration048 is frozen at
SHA256 `cdd79ece48bb40a841c7f50aa1ad18f68bae07a4104a803f35a58893f672b516`.

The owner application route delegates to the same authorization-bearing command
as agent applications. Current active visible room/role eligibility is checked
under lock before idempotent receipt recovery; If-Match checks reviewed room
version for new writes. Existing agent requests remain compatible. Application
still creates no admission or private transcript access.

The first backend attempt failed0.940s at a genuine RLS boundary: SELECT FOR SHARE
also requires update-policy eligibility, which a nonmember must not receive.
Forward migration049 supplies only locked active visible recruitment-role context
for a current actor through a narrow app-only SECURITY DEFINER function, without
broadening table update grants. Migration049 is frozen at SHA256
`d845dad7248a02c7306b255fd30ce102ea309ef66e23d857d715122f54e3dba3`.
The two initial owner/agent cases then pass1.077s.

Additional guards pass stale/missing review and foreign attribution denial, exact
receipt recovery after a room-version change, changed-body conflict, exactly one
membership/event, SQL-enforced immutable claims and archived-room replay denial.
The existing agent application→offer→acceptance lifecycle still passes. Its
controlled checked-offer setup is fixture support, not normal-worker qualification.

An existing expiry regression correctly caught that anchoring only respond_by to
clock_timestamp broke its exact72-hour relation to created_at (failure0.715s).
Both timestamps now derive from one materialized database instant after authority
checks. The unchanged exact-duration and late-admission/sweep tests pass; no
assertion or timeout was relaxed. Final combined five-test core/lifecycle gate
passes3.759s on retained `typed_requirements_final_oct28`.

- `/tmp/undergrid-owner-applications-claims-red-oct28.jsonl`
- `/tmp/undergrid-owner-applications-first-green-oct28.jsonl` (RLS failure)
- `/tmp/undergrid-owner-applications-locked-context-green-oct28.jsonl`
- `/tmp/undergrid-owner-applications-submission-guards-oct28.jsonl`
- `/tmp/undergrid-owner-applications-agent-lifecycle-regression-oct28.jsonl`
- `/tmp/undergrid-owner-applications-deadline-regression-oct28.jsonl` (duration failure)
- `/tmp/undergrid-owner-applications-core-lifecycle-green-oct28.jsonl`

This is an unmerged backend checkpoint. Current recruitment review, bounded
applicant/coordinator reads, reviewed offer controls/current-target guards,
contention/recovery, canonical owner contracts, normal UI/browser proof and final
regressions/review remain required. Full HP2 and external qualification stay open.

## Applicant and coordinator read checkpoint — 28 October 2026

Test-only `ef25b90` reaches real owner HTTP RED for the missing read (0.721s).
Owner GET `/v1/owner/agents/{agent}/applications` now lists only that attributed
agent's pending application-origin memberships. Coordinator GET
`/v1/owner/agents/{agent}/swarms/{swarm}/applications` separately checks current
coordinator room authority before selecting its queue. Both use existing RLS,
active room/current deadline filters and deterministic100+1 keyset pages; neither
selects messages, agent profiles, credentials or owner records. The page includes
membership/version/expiry, immutable submitted claims, permitted objective and
full-history policy. A pending record is not an assertion of current offer-target
eligibility; mutation-time target guards remain pending.

The integration test proves normal submission appears in both authorized views,
retains role/capability claims, denies foreign actor attribution and prevents an
applicant from reading the coordinator queue. First attempts failed because a
routing-edit script stopped before changing dispatch (0.685s), then the outer
owner dispatch omitted the new collection path (0.690s). These are preserved
implementation failures, not extra product RED or weakened fixtures. The routed
read plus existing submission/retention/replay aggregate passes4 tests in2.584s.

- `/tmp/undergrid-owner-application-reads-red-oct28.jsonl`
- `/tmp/undergrid-owner-application-reads-first-green-oct28.jsonl` (routing edit failure)
- `/tmp/undergrid-owner-application-reads-routed-green-oct28.jsonl` (outer dispatch failure)
- `/tmp/undergrid-owner-application-reads-submission-green-oct28.jsonl`

Dedicated read pagination/expiry/revocation/zero-effect guards, current recruitment
review, reviewed coordinator offer/current-target enforcement, canonical contracts,
UI/browser proof and final gates remain open. The feature is unmerged; HP2 remains
active and no external qualification is inferred.

## Shared application-offer target guard — 28 October 2026

Test-only `e734681` exposes a real agent-domain gap in two phases: revoked
applicants could receive a new application-origin offer or recover an old offer
receipt. Both fail the required denial before implementation. Current target
principal/agent authority is now checked before generic invitation idempotency
recovery. The same validated invitation input type is used for parsing and
mutation, avoiding independently maintained body knowledge. The existing owner
ordinary invitation adapter cannot supply application_id; the new owner
application-offer command must use this shared guard too.

Narrow migration050 is frozen at SHA256
`f00c1f1739c643f73a77034fd793aceac322d8221dea5dff9c254d529f66b96a`.
Its app-only SECURITY DEFINER function checks the current coordinator, accepted
coordinator membership, active room and live application, then locks active target
principal/agent rows FOR SHARE NOWAIT. It returns only a boolean; no foreign
profile/owner data or broad table grant is exposed. NOWAIT avoids cycles behind
already-held actor/room authority and makes contention retryable rather than
silently authorizing stale targets. Mutation-time application expiry now uses the
actual database clock as well. Dedicated contention/principal-revocation cases
remain to be added; no unrun concurrency proof is claimed.

The new two-phase denial plus unchanged normal application→offer→admission and
expiry gates pass3 top-level/two subcases in3.048s. Denial preserves the applied
state with zero offers before creation, or exactly one existing offer after
receipt recovery is denied. Logs:

- `/tmp/undergrid-application-offer-authority-red-oct28.jsonl`
- `/tmp/undergrid-application-offer-authority-green-oct28.jsonl`

Reviewed owner-offer version/history controls, complete read/current-recruitment
and contention guards, canonical owner contracts, UI/browser proof and final
integration/review remain open. This is an unmerged security/backend checkpoint,
not completion of owner applications or full HP2.

## Reviewed owner-offer command checkpoint — 28 October 2026

Test-only `25b0be5` reaches the missing reviewed owner-offer operation RED0.659s.
POST `/v1/owner/agents/{agent}/swarms/{swarm}/applications/{application}/offer`
now checks current coordinator authority, resolves the submitted applicant from
its membership, reuses the shared current-target guard before idempotency recovery,
and requires full-history policy acknowledgement. New writes pin reviewed room
If-Match and application_version under transaction locks before delegating to the
existing invitation mutation. No caller-supplied target identity is trusted.

The actual owner lifecycle passes0.685s: an offer retains the original application
ID, advances to offered/version2 and grants no private history. Only the applicant's
separate exact-version acceptance changes membership to accepted. Scoped database
oracles assert no plans, tasks or approval votes are created for that room; an
initial unused global count was replaced with this meaningful room-scoped oracle.

Additional actual HTTP guards prove missing room review428, stale room/application
review409, wrong full-history policy422 and foreign owner404; retries return the
exact same offer and one event, a fresh key cannot reuse stale application review,
and target revocation denies retained owner-offer recovery. The combined existing
application/read/claim/replay/target/expiry/admission gate passes in7.554s on the
retained restricted network fixture. No assertion or timeout was weakened.

- `/tmp/undergrid-owner-application-offer-red-oct28.jsonl`
- `/tmp/undergrid-owner-application-offer-first-green-oct28.jsonl`
- `/tmp/undergrid-owner-application-offer-reviewed-guards-oct28.jsonl`

Current recruitment review, read pagination/expiry/zero-effect and explicit
foreign-lock contention cases, canonical owner contracts and normal-owner UI/
browser journey remain required. No fresh independent-agent review, external
qualification or complete owner-application/HP2 closure is claimed. The feature
remains on its isolated unmerged branch.

## Deterministic contention, pagination and bounded claim disclosure — 28 October 2026

Four real locked-target cases cover principal/agent contention through both agent
and owner offer endpoints. The actual request uses a three-second context and
must return retryable500 within two seconds, with the application still applied
and zero offer events. After rollback of the held lock, the exact body/key twice
returns the same single offer and one event, without admission. Gate passes2.894s.
Principal suspension now has the same before-write/before-receipt coverage as
agent revocation, with exact409 denial assertions; their combined two-parent/eight-
subcase identity/contention gate passes5.326s.

A coordinator queue test creates101 applications through normal agent commands,
not SQL product seeds. Its first/second pages contain100/1 unique sorted IDs;
an applicant sees only its own application. Reads create no events/inbox/jobs.
A controlled deadline fault removes that application from both applicant and
coordinator pending reads without consuming another page, and malformed cursor
returns400. Original pagination gate passes28.023s. The large controlled cohort
is release-fixture attribution only, not independent or organic demand.

A concrete long-reference test reached RED for unbounded inline claim disclosure.
Legacy valid input remains accepted and immutable in storage. Collection reads
now include at most16KiB of PostgreSQL JSONB text per claim; larger payloads become
application_claims:null with claims_state:retained_over_budget and a SHA256 over
the UTF8 PostgreSQL JSONB text. Normal claims use included; missing legacy claims
use historical_unknown with no invented digest. SQL projects the limited payload
rather than fetching all large bodies into application memory. The page/hash
metadata does not certify the submitted claims. The test independently recomputes
the returned hash in Go over actual stored JSONB text and proves the full original
reference remains intact. First budget/read proof passes1.486s; final pagination,
identity/contention and read-budget aggregate passes with unchanged limits.
Page and preview limits are defined once in the domain read.

- `/tmp/undergrid-application-offer-contention-oct28.jsonl`
- `/tmp/undergrid-application-offer-identity-contention-oct28.jsonl`
- `/tmp/undergrid-owner-application-pagination-oct28.jsonl`
- `/tmp/undergrid-owner-application-claim-budget-red-oct28.jsonl`
- `/tmp/undergrid-owner-application-claim-budget-green-oct28.jsonl`
- `/tmp/undergrid-application-read-contention-final-oct28.jsonl`

Current recruitment review, remaining actor/read recovery guards, canonical owner
contracts, complete normal-owner UI/browser proof and final integration/review
remain required. This feature and full HP2 remain unmerged/open.

## Current owner recruitment review checkpoint — 28 October 2026

Test-only `6c8a87a` normally creates a visible room and a registered builder, then
runs the actual recruitment scan under a separate restricted worker login on fresh
retained `owner_recruitment_reads_oct28`. No opportunity SQL seed is used. After
that valid setup, missing owner review fails404 (RED1.040s). Direct scan invocation
is backend fixture proof, not a claim that the normal browser/worker-loop journey
has completed or that providers/independent users are qualified.

Owner GET `/v1/owner/agents/{agent}/recruitment` and selected
`/v1/owner/agents/{agent}/recruitment/{opportunity}` now expose only current own-
recipient recruitment evidence, permitted objective/role, visibility/full-history
policy and reviewed room version/ETag. The list uses a bounded50-candidate UUID
window and next_after advances by scanned candidate ID, including stale candidates;
this permits continued traversal without claiming that an empty window is the
whole network. Rows are closed before current-evidence queries. Current authority
locks remain NOWAIT and errors are not disguised as stale evidence.

The existing room-origin helper delegates to a shared current-opportunity evidence
method, preserving optional origin semantics while giving this read the same
role/profile/hash and locked current-room checks. A controlled recruitment-profile
opt-out invalidates the selected packet and removes the stale list item. The first
attempt failed1.255s because a pointer error match missed this codebase's value-
returned problem type, propagating404 instead of an empty list. That implementation
failure remains retained. Corrected current-review gate passes1.239s; existing
room-origin recruitment eligibility regression passes8.339s.

- `/tmp/undergrid-owner-recruitment-db-oct28.log`
- `/tmp/undergrid-owner-recruitment-review-red-oct28.jsonl`
- `/tmp/undergrid-owner-recruitment-review-first-green-oct28.jsonl` (error-match failure)
- `/tmp/undergrid-owner-recruitment-review-current-green-oct28.jsonl`
- `/tmp/undergrid-owner-recruitment-origin-regression-oct28.jsonl`

Before feature closure, align the older generic agent recruitment listing with
current role/profile evidence without changing its published response shape;
finish stale-window/pagination/authority proof, canonical owner contracts and the
normal-owner UI/browser lifecycle. The feature remains unmerged and HP2 open.
