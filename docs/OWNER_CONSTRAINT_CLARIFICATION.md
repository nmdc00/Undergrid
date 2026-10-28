# Owner registered-demand hard-evidence clarification

## Locked contract — 28 October 2026

HP2 step1 / INT-02 / TDD r2 §5: unknown hard evidence requires visible
clarification and never counts as alignment. This feature starts from reviewed
release `24c28c4` on `feat/v0.1-owner-constraint-clarifications`, isolated at
`/tmp/undergrid-worktrees/constraint-clarifications/feat-v0.1-owner-constraint-clarifications`.
Its PR targets `release/v0.1`; root owns implementation while existing swarm
workers are unavailable until their stated quota reset. No replacement agents or
model switches bypass that limit. The complete TDD goal remains unchanged.

### Existing gap and bounded behavior

`compareDemandConstraints` already distinguishes aligned, unknown and incompatible
hard facts. `CompareDemand` also returns `needs_clarification` for differing prose
with otherwise compatible constraints. `scanIntentions` drops non-compatible
candidates. The owner therefore cannot infer missing hard evidence from an empty
opportunity list or from that status alone. This feature supplies an explicit
read-only hard-evidence report, not fabricated matching opportunities.

Add shared domain GET `/v1/intents/{id}/constraint-assessments` and owned-session
GET `/v1/owner/intents/{id}/constraint-assessments`. Resolve current actor/scope and
owned active, unexpired source in the domain transaction. Explicit inspection does
not require monitoring: a watch-disabled source remains readable, while existing
group-viewer monitoring requirements stay unchanged. Candidates come exclusively
from current permitted foreign public-intention projections; private, withdrawn,
expired or revoked evidence is never exposed. Bound a deterministic nonexhaustive
sample to20 candidates and disclose truncation/unknown common control. Separate
accounts are attribution, not beneficial-owner verification.

The closed report pins source ID/version/revision, a report scope, limits and
candidate IDs/public versions. Each candidate shows only its permitted public
summary/constraints, per-dimension actual constraint evidence, `hard_status`
(`aligned`, `unknown`, `incompatible`) and `clarification_required`. Declared outcome
comparison is separately `same_declared_outcome` or `not_evaluated`; differing prose
must not become a false missing-hard declaration or a semantic-eligibility claim.
This read performs no provider requests, semantic ranking or arbitrary URL fetch.
No report grants room origin eligibility, admission, assignment, rights or spending.

Reuse the existing hard-constraint evaluator. If complete evidence is needed for
legacy untyped constraints, expose a narrow internal evidence option while keeping
existing matching/dismissal hashes and responses unchanged. Do not duplicate policy
or weaken tests to manufacture matches. Owner/agent adapters and finite MCP/A2A
reads converge on the same authorized domain operation. New schemas/types/docs and
independent actual HTTP contract validation must describe the real wire shapes.

### Required acceptance and proof

1. Actual restricted PostgreSQL HTTP RED precedes behavior: source with declared
   coverage versus public missing coverage yields an explicit unknown/clarification
   report; hard-aligned different prose remains separately unassessed; conflicting
   licenses remain incompatible; private candidates are absent. Owner and agent
   interfaces agree. Capture normal creation and before/after normalized counts.
2. Source current authority, ownership, revision, state and expiry govern every
   read; source/agent/principal revocation and foreign owner reads fail closed.
   Candidate withdrawal/revision/revocation removes stale public evidence. Report
   reads create no opportunities, notifications, jobs, memberships or approvals.
3. The console explicitly loads and renders current hard evidence and actionable
   missing declarations as inert data, with no implication of verified competence
   or semantic fit. Clear stale views on source version/session/selection change,
   refresh/error and authority loss; late responses cannot revive old contexts.
   Validate closed finite responses using shared constraint-contract knowledge.
4. Prove a normal-owner Chromium/API/real-PG journey from missing declaration to
   visible clarification, then a normal owner revision that resolves the missing
   declaration. Verify updated evidence/reload and zero report-created authority
   effects independently; no SQL product seeds complete the browser flow.
5. Retain actual RED/failures separately, exact runtime/build provenance, meaningful
   lifecycle/authority and existing opportunity/group regressions, canonical
   lint/schema/type parity and configured static checks. Review DRY knowledge,
   orthogonality, contracts, readability and recovery before merge.

At this checkpoint behavior is not implemented or proven. No legal, provider,
source, cohort or deployment approval follows from local fixture proof. HP2 still
requires complementary capabilities, typed controls, applications and its complete
continuous journey after the remaining verticals integrate.

## First core checkpoint — 28 October 2026

Test-only `1f8afcd` reached actual restricted PostgreSQL RED: both owner and agent
report requests returned404 (one top-level/two subcases, package0.961s), after
normal source/candidate creation succeeded. Original log:
`/tmp/undergrid-constraint-clarification-1f8afcd-http-red-oct28.jsonl`;
retained database `clarification_red_oct28` on the isolated55502 network fixture.

The first core implementation passed the same behavioral matrix on the frozen
working tree subsequently committed with this checkpoint: owner and agent agree,
coverage absence is unknown, different prose with aligned hard constraints remains
separately unassessed, incompatible licenses stay incompatible, private metadata
is absent, watch-disabled explicit inspection works, and no report read creates
opportunities/events/inbox/jobs/memberships. One top-level/two subcases passed
2.012s; `/tmp/undergrid-constraint-clarification-first-green-oct28.jsonl`, genuine
fresh `clarification_green_oct28`. These are focused core results, not release proof.

A complete-evidence option in the existing evaluator serves the new report while
its old wrapper retains historical evidence shape. A golden byte-contract guard
and existing hard/typed-constraint cases passed in1.038s:
`/tmp/undergrid-constraint-clarification-legacy-unit-oct28.jsonl`.
Public candidates use the existing `public_intents()` projection boundary; the
new bounded query adds no broad grants or migration. Source identity/scope/state
checks are inside the shared domain transaction. No provider call is involved.

Remaining: broader current-authority/withdrawal/expiry/concurrency and bounded
sampling tests, canonical schemas/generated types/actual wire validation, finite
MCP/A2A read adapters, owner UI with stale-response recovery, actual normal-owner
clarification-to-revision browser proof, retained runtime/static gates and final
review/merge. The feature is unmerged and HP2 is still open.

## Authority and protocol checkpoint — 28 October 2026

Six top-level cases/14 subcases passed10.647s against real restricted PostgreSQL:
source ownership/state/expiry/revocation, current publisher revision/withdrawal/
revocation, explicit monitoring-off inspection, writer-only vs owner attribution,
and a bounded held-source-lock500/rollback/read-recovery case. Fixture retirement
uses normal owner withdrawals without deleting evidence. Log:
`/tmp/undergrid-constraint-clarification-authority-first-oct28.jsonl`.

Test-only `88e6e37` then produced actual protocol RED14.866s: MCP unknown tool and
A2A unregistered operation. The existing finite registry now delegates the new
read to the same domain operation. Two canonical paths and two shared report
schemas bring the contract to162 schemas; lint/compilation/generated parity pass.
The first generation command used the wrong working directory and did not run;
its log is retained separately from successful generation and is not behavioral RED.

Actual MCP/A2A parity and captured HTTP contract cases passed19.012s. Extended
coverage then passed eight top-level/16 subcases in32.649s, including foreign MCP
and A2A receipt denials, closed tool arguments, current A2A resource snapshots
after publisher revision, minimal receipt storage and source-pause suppression.
Independent HTTP validation rejects private extra fields, contradictory evidence,
incorrect identity/scope, invalid versions and oversized reports. Logs:

- `/tmp/undergrid-constraint-clarification-88e6e37-protocol-red-oct28.jsonl`
- `/tmp/undergrid-constraint-clarification-protocol-contract-green-oct28.jsonl`
- `/tmp/undergrid-constraint-clarification-authority-protocol-contract-oct28.jsonl`
- `/tmp/undergrid-constraint-clarification-contract-first-oct28.log`

These are local controlled-fixture protocol checks, not external-client/provider
qualification. Owner UI, actual browser flow, bounded-sampling tests and final
retained/static/review/merge gates remain pending.

## Owner UI checkpoint — 28 October 2026

The actual App entrypoint test on `848eaa0` first failed because the explicit
requirement-gap control was absent (one failure,28 skipped,3.03s). The console now
mounts an explicit read for active intentions even with monitoring off. Closed
response validation, inert publisher text, source/version/session invalidation,
and refresh failure handling keep this read separate from matching authority.
The existing group viewer and new report share only declared-constraint parsing;
the group's monitoring policy is unchanged.

The first combined UI gate passed89 cases across App, compatible groups and
clarifications in14.46s. TypeScript then exposed two unsupported test-query options;
anchored names corrected that fixture issue, and the App rerun passed29 cases
in10.45s. Expanded late-response checks cover source selection, revision and
session changes, including stale401 responses:23 component cases passed in2.04s.
Standalone TypeScript and diff whitespace checks pass. Exact logs:

- `/tmp/undergrid-constraint-clarification-848eaa0-app-red-oct28.log`
- `/tmp/undergrid-constraint-clarification-ui-first-green-oct28.log`
- `/tmp/undergrid-constraint-clarification-ui-tsc-first-oct28.log` (fixture errors)
- `/tmp/undergrid-constraint-clarification-app-corrected-oct28.log`
- `/tmp/undergrid-constraint-clarification-session-guards-oct28.log`
- `/tmp/undergrid-constraint-clarification-ui-tsc-final-checkpoint-oct28.log`

This is an unmerged implementation checkpoint. Candidate-limit/deduplication
runtime proof, the normal-owner missing-to-resolved browser journey, final build
provenance, regression gates and review remain required before feature closure.

## Bounded sampling proof — 28 October 2026

Test-only `fe325ef` adds normal API-created public declarations to the restricted
PostgreSQL fixture. Multiple declarations from one publisher consume one slot;
the source owner's public declaration is excluded. Two foreign principals produce
two entries without truncation;21 foreign principals produce20 distinct-principal
entries with truncation. Normal owner withdrawal retires fixtures without deleting
history. The characterization gate passed6.483s, then passed7.500s after deriving
SQL's extra-row sampling bound from the report's declared candidate limit.

Logs: `/tmp/undergrid-constraint-clarification-bounds-oct28.jsonl` and
`/tmp/undergrid-constraint-clarification-bounds-refactor-oct28.jsonl`.
Both use retained `clarification_guards_oct28`; no worker/provider qualification
is inferred. The actual owner browser journey and final feature gates remain open.

## Live browser harness checkpoint — 28 October 2026

`393cfae` adds the normal three-owner missing-coverage → publisher revision →
refresh/reload scenario, a separate HP2 mode, and independent normalized revision,
watch and zero-opportunity/collaboration oracles. Tagged harness compilation,
TypeScript and the coupled production build passed. The first browser run failed
108.121s: the harness reviewed C's public projection before adding coverage, so
current product review invalidation correctly disabled submission. `4464501`
moves public review after all edits and also reviews the revised publication.
Its build passed; the second browser run failed94.044s because the harness tried
to edit B without first opening the intention detail. `2e8fa2d` adds that normal
review step; it has not yet been rerun. These are harness failures, not product
RED or passing journey evidence. Existing90-second browser and three-minute
helper bounds remain unchanged.

Original logs: `/tmp/undergrid-clarification-browser-393cfae-oct28.jsonl` and
`/tmp/undergrid-clarification-browser-4464501-oct28.jsonl`. Retained artifact
folders: `/tmp/undergrid-hp2-browser-2717199456` and
`/tmp/undergrid-hp2-browser-1299341547`. No fixture reset or product SQL seed
completed either journey. Final live journey proof remains required.

## Actual normal-owner journey — 28 October 2026

Frozen `ecead15` passes the complete new Chromium/API/restricted-worker/real-PG
scenario: package16.634s, browser case13.6s. A privately declares EU coverage with
monitoring off; B publicly omits coverage; C publicly declares EU coverage but a
conflicting license. A explicitly reads unknown B and incompatible C. B opens the
normal intention detail, edits coverage and reviews the final public projection;
A refreshes and reloads, seeing aligned B at version2 while C remains incompatible
and source version stays1. Separate controlled OIDC accounts prove attribution,
not independent beneficial ownership. No SQL product seeds complete the journey.

Independent normalized checks prove four immutable intention revisions, B current
revision2 and EU coverage, A monitoring off, zero opportunities and zero combined
rooms/memberships/plans/approvals/tasks. The actual restricted worker starts and
stops under the existing bounds; this is local runtime proof, not provider proof.

- Log: `/tmp/undergrid-clarification-browser-ecead15-oct28.jsonl`
- Build: `/tmp/undergrid-clarification-browser-ecead15-build-oct28.log`
- Retained DB: `hp1_run_68fc79199b744311a224c446fda58cab`
- Artifacts: `/tmp/undergrid-hp2-browser-1697825902`
- Frontend source SHA256: `239a79547f9f25328e83450415c0612177706a3b31d3faed830d1880fbd635d8`
- Production JS SHA256: `7a4e087c0ca5dc3190f2551440c43032c44370f4645462e299230ae4a4c0231b`

This closes the missing-to-resolved browser acceptance step. Final regressions,
review and merge remain pending; the complete continuous HP2 journey remains open.

## Final-feature regression checkpoint — 28 October 2026

At runtime/frontend `ecead15`, all233 UI cases across21 files passed76.19s.
The complete clarification domain/authority/finite-protocol/actual-HTTP-contract
suite passed9 top-level cases and16 subcases in24.676s on the retained restricted
network fixture. Canonical lint, all162 schemas and generated-type parity pass.

- `/tmp/undergrid-clarification-ecead15-full-ui-oct28.log`
- `/tmp/undergrid-clarification-ecead15-authority-protocol-contract-oct28.jsonl`
- `/tmp/undergrid-clarification-ecead15-contract-oct28.log`

Quality review confirms REST, owner, MCP and A2A route to one authorization-bearing
transactional read; the shared hard-evidence evaluator preserves historical
matching bytes under characterization. Candidate disclosure stays behind existing
`public_intents()` rather than a new privileged repository. Shared frontend
constraint parsing centralizes contract knowledge while monitoring and current
read policy remain distinct. Late responses cannot revive old session/source
contexts. This read creates no opportunity or collaboration authority. No migration,
provider request, financial credential or deployment is introduced.

The retained group/room/plan browser aggregate passes67.397s at `ecead15`, with
individual cases18.83s/18.13s/30.42s. Tagged `go vet` and `go build ./...` pass;
six pure typed-hard/nontransitive-group regressions pass as well. Logs:

- `/tmp/undergrid-clarification-ecead15-retained-browsers-oct28.jsonl`
- `/tmp/undergrid-clarification-ecead15-vet-oct28.log`
- `/tmp/undergrid-clarification-ecead15-go-build-oct28.log`
- `/tmp/undergrid-clarification-ecead15-hard-group-unit-oct28.jsonl`

No fresh independent agent review is claimed while workers remain quota-limited.
Root reviewed the scoped diff and actual proof under the implementation skill;
remote PR integration and exact merged-release checks are still pending.
