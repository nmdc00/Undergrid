# Undergrid v0.1 — Happy-path execution map

Mapped on 23 October 2026 against TDD `UNDERGRID_TDD_v0.1-r2.md`, its five
companions, the must-ship scope, and merged release `4db5256` (PR89).
Implementation authorization from12 October remains effective; historical design
banners do not reopen it. This changes execution order, not product scope or the
reserved legal/source/provider/deployment gates.

## Operating rule

Finish usable journeys, not isolated subsystems. One path is active at a time.
Every branch must name the step and missing acceptance outcome it closes.
Do not start work from another path merely because its code is easier.

Existing branches are retained. Finish/reconcile the inbox and dismissal work
needed for HP1; keep semantic/group/fairness work for HP2. Do not discard security
fixes or passing evidence. A defect affecting the active path is in scope; other
findings go into the owning path's backlog unless they threaten data or authority.

Use a small swarm on the active journey: implementation, end-to-end acceptance,
and integration/review. Give each coding task its own feature branch/worktree;
merge reviewed features through release → develop → main. Serialize costly
runtime gates. Hosted GitHub CI remains waived, local checks remain required.

## Definition of a closed path

For each path record these separate facts:

1. **Implemented:** every listed user step has a real runtime entrypoint; no
   manual SQL, hidden administrator action or fixture-only substitute completes it.
2. **Locally proven:** execute the complete journey through the actual browser and
   public agent interfaces where appropriate, against real isolated PostgreSQL and
   workers. Record exact source, commands, artifacts and result. Test doubles may
   represent external providers only when explicitly labeled.
3. **Merged:** reviewed feature changes and the acceptance proof are integrated
   into the release branch; relevant authority/retry/rollback tests also pass.
4. **Externally qualified:** required real provider/source/user/deployment evidence
   exists. This is never inferred from local fixture success.

Call a path locally closed only after1–3. Public release still requires4 wherever
specified and all cross-path release gates. If an external approval prevents4,
finish the authorized code and local proof, record the precise gate, and continue
independent authorized paths. Do not label the full TDD complete.

Report steps closed, next missing outcome and blockers. No impressionistic
percentage, test-count percentage or historical coverage-as-completion score.

## HP1 — The first owner gets a useful result and an ongoing watch

**Outcome:** someone can use Undergrid before other registered participants exist.

Journey:

1. Register/sign in, create a scoped agent and a structured intention with explicit
   visibility, requirements and watch expiry.
2. Inspect current permissioned capability/request leads, provenance and declared
   fit, or receive an honest gap explanation. External leads remain external.
3. Where an existing capability meets only part of the need, record partial
   satisfaction and keep the unresolved requirements monitored.
4. Leave the watch active. A later relevant permitted source update yields a
   durable opportunity and one notification; the owner returns to the inbox.
5. Review the result, acknowledge receipt and dismiss an unwanted suggestion.
   Refresh/reconnect: dismissed unchanged evidence is not notified again; genuinely
   changed evidence can produce a new suggestion.

Finish line: one repeatable browser/API journey, from empty account to useful
lead/gap to later notification and receipt/dismissal, including persistence across
worker restart. Actor/cursor/revision/source freshness are real, not mocked away.
Adjacent guards: private text does not reach publishers; stale/withdrawn leads
cannot be read; receipt/dismissal grants no work, rights or spending authority.

Current evidence at release `81e4730`: owner setup/intents/lead browsing, external
notification pipeline, suggestion dismissal (PR91) and owner inbox (PR92) are
merged. Live Chromium evidence on acceptance branch `5549003` proves real
sign-in, principal/agent creation, a private active watch, honest empty-source gap,
scoped bearer read and reload against isolated PostgreSQL. The controlled OIDC
issuer does not qualify a real provider. The harness is not yet merged, and this
is not the complete journey. Partial satisfaction and visible unmet requirements
remain open; continuous source-update/inbox/dismissal/worker-restart proof remains
pending. Do not infer journey completion from backend pieces.

### Active HP1 swarm assignments

| Responsibility | Branch and isolated worktree | Acceptance and owning documentation |
| --- | --- | --- |
| Partial satisfaction implementation (`external_oct23`) | `feat/v0.1-intention-partial-satisfaction`; `/tmp/undergrid-worktrees/partial-satisfaction/feat-v0.1-intention-partial-satisfaction` | TDD §5.3: explicit owner-selected fulfilled fields produce an immutable revision with visible unmet remainder and active watch. Prove authorization, source/version pins, retry/rollback and private audit through real domain transactions. Update feature evidence and this path's status before merge. |
| Continuous acceptance (`profile_oct23`) | `test/v0.1-hp1-live-journey`; `/tmp/undergrid-worktrees/hp1-live/test-v0.1-hp1-live-journey` | Actual browser/API/PostgreSQL/source refresher/worker journey; no product-row SQL setup. `docs/HP1_ACCEPTANCE.md` owns step-level results and artifacts. Integrate reviewed product changes before full-path proof. |
| Independent review (`assignment_oct23`) | Read-only inspection of those branches | Review domain authority, immutable revisions, private projections, idempotency and actual harness boundaries. Findings must identify a proving check. |
| Integration (`root`) | `release/v0.1`; documentation on `docs/v0.1-hp1-progress` | Review scoped PRs, merge dependency order and retain exact test evidence. No production deployment or promotion is implied by a local milestone. |

All feature PRs target `release/v0.1`; release integration PR is
[PR62](https://github.com/nmdc00/Undergrid/pull/62). On 23 October, fetched
`main...develop` divergence is `0 1` (develop one commit ahead, no divergent main
commit). Hosted CI is waived; required local gates remain. Costly Go/PostgreSQL
and browser suites run serially. External source, provider, legal and deployment
qualification remains separate from these local implementation tasks.

Public qualification: real usable permitted capability and request sources in the
chosen niche, plus the no-admin unfamiliar-owner useful result within ten minutes.
A truthful local gap proves the gap behavior, not satisfaction of the real-source
release gate. Provider/source approval remains explicit.

Owning requirements: ID-01/02/03, INT-01/02/03/04/05, SW-06; TDD §§4–6,9,12.4.

## HP2 — Participants discover one another and agree on work

**Outcome:** distinct participants move from unmet demand to an explicitly agreed
collaboration. Separate account IDs alone do not establish independent ownership.

Journey:

1. Publish eligible intentions; explain current hard/semantic matches or missing
   hard evidence. Inspect a compatible group without assuming transitive fit.
2. Discover an appropriate registered builder/reusable capability, publish an
   opt-in role, and deliver an attributable recruitment opportunity.
3. Create a room with reviewed visibility/history policy. Invite/apply, review and
   explicitly accept admission; exchange persistent messages.
4. Create/revise a bounded plan and task dependencies. Each required participant
   reviews the exact hash and contribution terms and records their own decision.
5. Each assignee separately accepts responsibility. The accepted plan head and
   task state agree; participants can reconnect and recover pending obligations.

Finish line: buyers A/B and builder C reach one accepted plan with independently
recorded admission, exact plan/rights votes and assignment acceptance. Browser
owner actions and public agent commands share authoritative domain behavior.
Adjacent guards: unknown hard requirements need clarification, incompatible needs
never align due to semantic score, revocation blocks current reads/replay,
canonical locks prevent stale policy acceptance, and receipt/chat never grants
participation or rights. Prove restart, decline and no duplicate effects.

Current evidence: room participation/messages and owner plan/assignment decisions
are merged. Semantic/groups/viewer are on dependent branches. Combined runtime
integration exposed a five-second initial-scan starvation defect behind retained
jobs; preserve the bound and fix durable fairness before closing the discovery
steps. Complementary discovery and missing owner plan-creation/revision controls
must be resolved against the actual intended user journey.

Public qualification: externally operated participants and independent REST/MCP/A2A
clients with actual outcomes, clearly labeled release-test evidence.

Owning requirements: ID-01/03/04, INT-02/03/04, SW-01/02/03/04/06; TDD §§4–7,9–10.

## HP3 — The collaboration delivers, publishes and is reused

**Outcome:** the HP2 collaboration produces useful reusable supply, rather than
stopping at an approved plan.

Journey:

1. The assignee reviews responsibility and inputs, starts work and records bounded
   progress/evidence; Undergrid never executes the participant's agent content.
2. Upload the supported JSON dataset and input lineage/rights manifest; finalize
   immutable server-observed bytes and hashes.
3. Run the fixed verifier and inspect evidence. The designated parties accept the
   exact artifact, then separately authorize the exact publication manifest.
4. Publish permitted content and a reusable capability listing. A fourth principal
   retrieves it and explicitly acknowledges reuse of that immutable version.
5. Mark the original need satisfied, or retain a truthful unresolved remainder.

Finish line: from accepted plan to verified, explicitly accepted and published
artifact to a recorded third-party reuse receipt, with actual user interfaces and
public protocol paths. Retrieval alone is not confirmed reuse.
Adjacent guards: private input grants, stale manifests/votes, failed checks,
withdrawal/abandoned work, rights holds and coordinator exit/recovery cannot be
bypassed. No arbitrary verifier execution. Keep evidence and consent separate.

Current evidence: many backend artifact/verifier/rights/publication/reuse paths
exist. Complete owner contribution/artifact/evidence/publication interfaces and
storage/verifier-host qualification remain open; a combined user journey is not
claimed complete.

Public qualification: approved legal templates/license policy, qualified object
storage/verifier isolation, and the real four-principal discovery-to-reuse cohort.

Owning requirements: SW-03/04/06, DEL-01/02/03/04, INT-01/02; TDD §§7–10,12.4.

## HP4 — A sandbox user completes the Stripe TEST lifecycle

**Outcome:** explicitly authorized TEST billing is usable and reconciled in the
separate financial product.

Journey: authenticate to the sandbox → set scoped test authority/limits → create
an exact quote → complete Stripe test checkout → observe one verified settlement
→ exercise bounded renewal where authorized → request and reconcile a refund.
The owner can inspect status/evidence without a network credential or browser
silently crossing into sandbox authority.

Finish line: separate sandbox UI/API/database/identity, real Stripe test evidence
and matching normalized effects. Adjacent gates include duplicate/out-of-order
webhooks, unknown outcomes, revocation, expiry, replay, cadence, reservation and
refund recovery. No fresh charge on an unknown result.

Current evidence: substantial isolated backend and renewal/recovery fixture proof;
financial UI and actual selected-provider qualification remain open.

Owning requirements: INT-05, SW-04, DEL-04, TDD §§2.2,11.1–11.3,12.4.

## HP5 — A sandbox user completes the USDC TEST lifecycle

**Outcome:** a controlled testnet authorization becomes independently verified
settlement with visible recovery/reconciliation.

Journey: authenticate to the isolated sandbox → prove wallet control → review a
pinned quote → explicitly authorize its exact test payment → submit/observe
canonical settlement → inspect finality/evidence → reconcile reversal/refund or
changed canonical chain state where required.

Finish line: actual approved Sepolia/x402 test-rail evidence, correct token/chain/
recipient/amount binding, and one durable normalized effect. A local cryptographic
fixture is not testnet qualification. Adjacent gates cover mismatches, timeouts,
reorgs, duplicate dispatch, expired/revoked credentials and unknown settlement;
reservations cannot be released just to make an unknown payment look resolved.

Current evidence: isolated wallet/quote/authorization/canonical recovery backend
exists; sandbox user interface and actual selected-rail qualification remain open.
No mainnet or live-money authority follows from this path.

Owning requirements: INT-05, SW-04, DEL-04, TDD §§2.2,11.4,12.4.

## HP6 — An operator can launch, support and recover the service

**Outcome:** the completed journeys can be operated publicly and users can obtain
support or exercise privacy/rights controls.

Journey:

1. Deploy reviewed immutable network and sandbox releases with isolated secrets,
   origins and runtime identities; publish onboarding/API/skill/client docs.
2. A user submits an abuse, bug, privacy or rights report; an authorized operator
   reviews it, records a decision and applies the required bounded hold/action.
3. A user requests export/erasure; approved retention/hold rules govern data,
   revisions, vectors and blobs, with a truthful visible result.
4. Operate quotas, dead-letter/health alerts and incident contacts. Back up data
   off-host, restore into isolation, replay deletion tombstones, and rerun the
   product journeys before reopening.

Finish line: deployment/rollback and restore evidence, report/privacy workflows,
current credential revocation and host isolation, usable public docs/status/contact,
and exact release gates. Relevant backup/recovery/privacy failures are integral
acceptance cases, not reasons to grow unrelated frameworks.

Current evidence: local isolation, quotas, process modes, report intake and many
runtime guards exist. Operator cases/holds, retention/export/erasure, production
storage/deployment/restore and public outcome qualification remain incomplete.

External gates: selected approved hosts/providers/operators, legal retention and
rights policy. Preparing reviewable code/configuration does not authorize an
unselected production deployment. Hosted GitHub CI remains explicitly waived.

Owning requirements: ID-01/02/03/04, SW-06, DEL-04; TDD §§2–4,6,9,12–13 and all
five companions.

## Coverage and order

| Must-ship area | Owning paths |
|---|---|
| Identity and private visibility/grants | HP1; rechecked HP2/3/6 |
| Demand, semantic/hard fit, counts, watches | HP1/2; satisfaction HP3 |
| Permissioned first-user sources | HP1 |
| Collaboration/plan/tasks/exit/recovery | HP2/3 |
| Inbox/listener/ack/resync and owner alerts | HP1/2/6 |
| Rights/lineage/verifier/publication/reuse | HP3; holds HP6 |
| REST/MCP/A2A, Agent Card, installable skill | HP1/2/3; onboarding HP6 |
| Human network interface | HP1/2/3/6 |
| Isolated financial UI and two actual TEST rails | HP4/5 |
| Public operations/privacy/moderation/restore | HP6 |
| First-user/cohort/provider/release outcomes | HP1/2/3/4/5/6 qualification gates |

PRD ID-01–04 map to HP1/2/6; INT-01–05 to HP1/2/3/4/5; SW-01–04/06
to HP2/3 (SW-06 also HP1/6); DEL-01–04 to HP3/6 (DEL-04 financial refund also
HP4/5). SW-05 remains the explicitly deferred nested-delegation feature. The
companion deferred inventory stays deferred; this map introduces no new product
variants. Approval, architecture, threat-model and operating constraints continue
to apply to every owning path.

Execution order: **HP1 → HP2 → HP3 → HP4 → HP5 → HP6**. Prepare only prerequisites
of the active path; cross-cutting release qualifications remain tracked from day
one. If an explicit external gate blocks final qualification, continue the next
independent authorized path after recording completed code/local proof and the
remaining gate. Do not start multiple unrelated happy paths in parallel.

## Immediate checkpoint

The active path is **HP1**. Inventory and finish current owner inbox/dismissal
branches, then close the partial-satisfaction/unmet-remainder and explicit-gap/
clarification user steps. Build one complete HP1 acceptance walkthrough. Record
actual missing steps before assigning further branches. Preserve the HP2 semantic
queue-fairness failure/reproducer and resume it when its path is active or when it
is demonstrably a prerequisite for HP1. No complete path is newly claimed by
writing this mapping.
