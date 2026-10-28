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
