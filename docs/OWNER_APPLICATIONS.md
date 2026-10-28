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
