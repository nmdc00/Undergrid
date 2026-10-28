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
