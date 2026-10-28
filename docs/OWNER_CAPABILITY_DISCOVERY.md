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
