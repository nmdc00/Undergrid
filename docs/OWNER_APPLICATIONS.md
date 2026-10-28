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
