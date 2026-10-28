# Owner complementary capability discovery

## Contract — 28 October 2026

HP2 step2 / INT-02 / TDD §5.1: compare current intention requirements against
registered public agent declarations and published reusable artifacts, separately
from shared demand and recruitment. Work starts from release `fbd7950` in
`feat/v0.1-owner-capability-discovery`. Existing swarm workers remain unavailable
until their reported quota reset; root continues without replacement workers or
model switches. Full HP2 and the full TDD goal remain unchanged.

Provide an explicit read through agent GET `/v1/intents/{id}/capability-fit` and
owner GET `/v1/owner/intents/{id}/capability-fit`, with matching finite MCP/A2A
bindings. Current source ownership, responsible agent, scopes, active state and
expiry are checked in the domain transaction. Monitoring is not required. The
read creates no opportunities, notifications, jobs, invitations, assignments,
rights, reuse receipts or spending authority, and executes no participant content.

Return separately bounded, deterministic, nonexhaustive samples of current public
agent declarations and currently published reusable artifacts, with truncation and
exact source/version pins. Agent profiles expose only their explicitly public
capability declarations and attribution; private profiles, endpoints, credentials,
owner information and revoked agent/principal declarations stay hidden. Agent
capability strings establish declared schema support only; coverage, fields,
freshness, licenses, availability and willingness remain unknown unless actual
published evidence supplies them. Recruitment opt-in is not a prerequisite for
passive public declaration inspection and is never inferred as consent to work.
Do not count providers as buyers or separate principal IDs as independent owners.

Published artifacts use the existing published-artifact disclosure and hold rules,
immutable ID/content/manifest hashes and permitted manifest metadata. A producer's
credential revocation is not automatic withdrawal of effective published rights.
Do not expose private task/plan/room context or private input manifests. A schema
or license contradiction remains incompatible; an absent required fact remains
unknown. Shared-demand symmetric requirement combination must not be reused as
asymmetric capability proof. The report never certifies legal rights, competence,
quality or semantic equivalence. No paid service catalog or generic capability
registry/table is introduced; published artifacts remain the reusable supply view.

The owner console explicitly loads inert current results and explains supported,
conflicting and unknown facts. Closed runtime validation, bounded lists and stale
selection/version/session response suppression follow the current owner console
contracts. A read cannot silently install, execute, contact, invite or purchase.

## Required proof before merge

1. Actual restricted-PG HTTP behavioral RED precedes implementation. Normal owner
   and agent creation supplies a private watch-disabled source and explicit public
   builder declaration; the report explains schema support without inventing hard
   requirements or authority. Foreign/private/revoked declarations are excluded.
2. Publish an artifact through the actual plan/assignment/upload/verification/
   acceptance/publication lifecycle, then prove current capability projection,
   immutable pins and hold/ineligibility behavior. No SQL product seeding or fake
   provider verification completes publication. Keep storage qualification separate.
3. Exercise source current authority, scope, revision/expiry, bounded distinct
   attribution, truncation, contention rollback and zero read-created effects.
   Current REST/MCP/A2A reads and recovery must agree without historical disclosure.
4. Write missing owner-control and malformed-response tests before UI behavior.
   Prove a normal-owner browser journey from declared need to visible public
   builder capability, honest unknowns and reload/current-authority recovery.
5. Retain original failures, exact runtime/build provenance and normalized oracles;
   run relevant hard-fit, recruitment, publication and current HP2 regressions,
   canonical schema/type parity and static/build checks. Review DRY knowledge,
   orthogonality, transaction contracts, readability and failure recovery.

This is a scoped implementation contract, not evidence that the feature exists.
Typed requirement controls, applications and continuous HP2 acceptance remain
separate required work. No external source/provider/legal/deployment approval is
implied. Existing public contracts and frozen migrations remain intact.

## Initial behavioral RED — 28 October 2026

Test-only `c1974fe` passes normal restricted-API owner/agent registration,
public builder profile creation and private watch-disabled source creation, then
both owner and agent capability-fit reads fail404. The one-parent/two-subcase
HTTP test fails0.631s for the missing operation, not a broken fixture. No behavior
has been implemented. Original log:
`/tmp/undergrid-capability-fit-initial-http-red-oct28.jsonl`.
The retained network fixture is `clarification_guards_oct28` on55502; normal owner
withdrawal retires the test source without deleting evidence. The separate
financial sandbox is untouched. Implementation and all required acceptance gates
above remain pending.

## First public-agent implementation checkpoint — 28 October 2026

The original two-channel HTTP RED is now green: owner and agent reads expose
schema support from current public declarations, while missing license/field/
coverage evidence remains unknown. A narrow app-only SECURITY DEFINER projection
returns only agent/principal IDs and public capability arrays, with active
principal/agent predicates, foreign-principal exclusion, principal deduplication
and a21-row maximum. No foreign routing/profile preferences or credentials are
selected. Recruitment consent is not required or inferred. This is a partial
feature checkpoint: published artifacts, canonical/finite-protocol publication,
UI/browser acceptance and remaining bounds/authority gates are not implemented.

Migration046 was normally applied by the real fixture and is now frozen at
SHA256 `73750e83960116116994d6fe4a41b8dbbcb7264a62cc7b15a2a111d3da515d87`.
Older migrations are unchanged. The real restricted API has no worker grant.
Disclosure tests prove foreign-source denial, private-agent omission, revoked
candidate removal and zero count changes across opportunities/events/inbox/jobs/
rooms/memberships/approvals/tasks for explicit reads. Source authorization was
extracted into one current-owned-intention loader shared with clarification;
monitoring-dependent group policy is untouched. The existing asymmetric claim
comparison supplies unknown hard-evidence explanations; evidence class/path is
bound to the selected public agent declaration, not an external publisher.

First positive read gate passed0.685s; expanded disclosure gate passed1.627s.
The shared-source aggregate initially failed21.605s solely because this worktree's
independent HTTP validator could not load ajv before its dependency link existed.
After verifying package/lock parity and linking existing node_modules, unchanged
source passed the full aggregate21.811s. Original failure remains preserved.

- `/tmp/undergrid-capability-fit-first-green-oct28.jsonl`
- `/tmp/undergrid-capability-fit-disclosure-first-oct28.jsonl`
- `/tmp/undergrid-capability-fit-shared-source-regressions-oct28.jsonl` (dependency failure)
- `/tmp/undergrid-capability-fit-shared-source-regressions-deps-oct28.jsonl`

All runs retain `clarification_guards_oct28` on the isolated network fixture.
No public artifact/provider qualification or complete capability discovery is
claimed. The feature remains unmerged and HP2 remains active.

## Published-artifact lifecycle checkpoint — 28 October 2026

Test-only `18f4cd0` constructs the actual restricted-API room, independent plan/
rights votes, accepted assignment, upload/finalization, real executable fixed
verifier worker, designated acceptance and exact publication consents. Its first
run exposed a genuine projection bug: valid public profiles with null capability
arrays caused SQL22023/HTTP500. That6.905s failure is retained; it is not the
missing-artifact sample RED. Forward migration047 fixes nullable arrays without
rewriting applied046. Migration047 is frozen at SHA256
`ae04a89fb94f580a3705e7143306a380577e5cfa354b6da430c882a6f2b0f0fd`.

The unchanged lifecycle then reached the intended missing-artifact-sample failure.
After implementing the bounded existing-RLS published-artifact projection, the
lifecycle passed5.473s: verified/accepted but unpublished content stays absent;
normal publication exposes immutable content/manifest hashes and permitted title,
schema/license metadata. No content, input lineage, room/task/plan or private
rights context is selected. The existing asymmetric capability evaluator explains
known schema/license facts and honest missing fields/coverage; structural checks
are not factual, legal or provider verification. Agent row resources are closed
before the second query, including when the agent sample truncates.

Expanded lifecycle checks pass5.925s: a normal source revision to disjoint license
options yields incompatible evidence. Adversarial producer suspension retains
published license availability; an explicit held-state fault removes the artifact
from discovery. These mutable test faults are not SQL publication seeds or proof
of an implemented operator-case workflow. Publication itself uses only normal
commands and the actual local unqualified verifier. All fixture databases and
original failures remain retained.

- `/tmp/undergrid-capability-artifact-http-red-oct28.jsonl` (null-array defect)
- `/tmp/undergrid-capability-artifact-sample-red-oct28.jsonl` (missing sample RED)
- `/tmp/undergrid-capability-artifact-first-green-oct28.jsonl`
- `/tmp/undergrid-capability-artifact-current-guards-oct28.jsonl`

This extends the partial backend implementation; canonical/finite-protocol
publication, UI/browser acceptance, complete bounds/authority tests and final
review/merge remain open. The full capability feature and HP2 are not closed.

## Combined backend proof and bounded-fixture limit — 28 October 2026

The first combined run on long-lived `clarification_guards_oct28` failed26.997s:
repeated normal registrations had accumulated more than20 active public
principals. The bounded deterministic sample legitimately omitted the newly
registered builder, contradicting this test's controlled-population assumption.
This is not fixed by increasing limits, deleting records or weakening assertions.
Original log `/tmp/undergrid-capability-artifact-combined-backend-oct28.jsonl`
remains retained. Final capability acceptance gates must use a fresh controlled
population (public profile retirement/editing is not yet a normal owner feature).

Created retained `capability_backend_oct28` by normal CREATE DATABASE TEMPLATE
`undergrid_hp1_template`, then applied the real remaining migrations normally.
Unchanged source/test aggregate passes on that genuine fresh fixture:
`/tmp/undergrid-capability-artifact-fresh-combined-backend-oct28.jsonl`.
Database creation log: `/tmp/undergrid-capability-backend-db-oct28.log`.
The aggregate includes artifact publication/conflict/identity-loss/held-state
proof, owner/agent public declarations and zero-effect guards, plus all existing
clarification authority, lock recovery, canonical HTTP and MCP/A2A regressions.
Capability-specific canonical/finite protocols and user-interface proof remain
pending, as do dedicated bounds/contention tests. No full feature closure claim.

## Canonical and finite protocol checkpoint — 28 October 2026

Test-only `6dee903` reaches real finite-protocol RED: MCP rejects the unknown
`intention_capability_fit` tool and A2A rejects the unregistered operation. Test-only
`0a1a07c` reaches actual HTTP-contract RED for the unadvertised capability route.
A single registry entry now connects MCP/A2A to the same current domain read;
canonical owner/agent paths and six closed response schemas/types are published.

The first parity/contract gate passes5.658s. A2A recovery reauthorizes the current
source; foreign receipt reads fail404, a normal source pause suppresses recovery,
and persisted receipts contain no report body. The populated artifact lifecycle
now captures actual HTTP reports before/after publication, conflicting source
revision, producer suspension and held state. Independent schema oracles reject
private room/context fields, invalid hashes, false legal/identity assurance,
wrong evidence class and falsely aligned hard status. Typed absent facts remain
unknown. These real artifact samples supplement the four owner/agent200/foreign404
canonical cases; an empty artifact sample alone is not the proof.

The final three-top-level/two-subcase protocol/HTTP/populated-artifact aggregate
passes13.014s on retained `capability_backend_oct28`. Canonical lint, all168 JSON
Schema components and generated-type parity pass. Logs:

- `/tmp/undergrid-capability-fit-protocol-red-oct28.jsonl`
- `/tmp/undergrid-capability-fit-contract-red-oct28.jsonl`
- `/tmp/undergrid-capability-fit-protocol-contract-green-oct28.jsonl`
- `/tmp/undergrid-capability-fit-protocol-artifact-contract-oct28.jsonl`
- `/tmp/undergrid-capability-fit-canonical-check-oct28.log`

The feature remains unmerged. Dedicated bound/source-authority/contention tests,
UI/browser acceptance and final regression/review gates remain required. Local
executable verifier evidence is not isolation/provider/legal qualification.

## Capability source and agent bounds proof — 28 October 2026

Capability-specific endpoint guards pass3 top-level cases/six state subcases
in5.649s on `capability_backend_oct28`: foreign/current source attribution,
paused/withdrawn/expired sources, agent/principal loss, scope separation, sibling
agent denial and prompt held-source rollback/recovery. Revocation results match
existing current-identity gates, with no report payload in denials. A held source
uses the unchanged three-second request/two-second elapsed oracle; exact current
read succeeds after lock release. Log:
`/tmp/undergrid-capability-fit-source-guards-oct28.jsonl`.

The dedicated fresh `capability_bounds_oct28` fixture is normally copied from the
genuine empty template and migrated, with no product SQL seeds. Normal owner
commands register a public source-owner agent, two public agents for one foreign
principal and one for another. Explicit read returns exactly two foreign
principals, not duplicate agents or the source owner. Passive public declarations
are eligible even with recruitment disabled. After19 additional principals, the
read returns20 unique foreign principals with truncation disclosed. All schema
support remains self-declared and complete hard fit stays unknown. Gate passes
3.498s; rows and evidence are retained, not deleted to restore an empty fixture.

- `/tmp/undergrid-capability-bounds-db-oct28.log`
- `/tmp/undergrid-capability-fit-agent-bounds-oct28.jsonl`

These gates prove the public-agent bound and source contention. Actual multi-
artifact bound/held-state concurrency, owner UI/browser proof and final feature
regressions/review remain required; no full capability/HP2 closure is claimed.

## Owner entrypoint RED — 28 October 2026

Test-only `3bc74b7` requires the active watch-disabled source's explicit
`Discover registered capabilities` control and same-origin owner GET without an
agent bearer. Valid existing App setup reaches the missing control, then fails
one case/29 skipped in2.99s. Original log:
`/tmp/undergrid-capability-fit-app-red-oct28.log`. No component import or matcher
failure substitutes for this behavioral RED. Owner UI implementation and actual
browser acceptance are still pending.
