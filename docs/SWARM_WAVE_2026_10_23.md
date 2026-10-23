# Undergrid continuation wave — 23 October 2026

Full approved r2 implementation remains the objective. This wave does not narrow
completion to its current branches. The Undergrid implementation, isolated-worktree
and sprint branching contracts apply. Routine implementation, tests and feature
PR work are authorized; actual legal/source/provider/deployment gates remain.

## Authoritative release and promotion

At recovery the clean release is `release/v0.1` at `648c078` (PR87 owner policy-lock
fix). Feature PRs target this release. [Release PR62](https://github.com/nmdc00/Undergrid/pull/62)
targets develop. Fetched main/develop ancestry remains0:1; no main-only commit.
Promotion remains release → develop → main after exact integration/release-contract
qualification. Hosted GitHub CI is waived by the owner; local checks remain required.
There is no deployment or provider-verification claim.

## Assignment map and gate order

| Owner | Branch / isolated worktree | Behavior and evidence | Next gate / document |
|---|---|---|---|
| Root | `feat/v0.1-semantic-discovery`, `/tmp/undergrid-worktrees/semantic/feat-v0.1-semantic-discovery` | Current semantic pairwise groups and successful model cutover; six-case race proof19.776s. Ordinary opportunity reads still need fresh operator-profile eligibility; new assertion awaits actual RED | Read RED/implementation/GREEN; `SEMANTIC_DISCOVERY.md` and TDD §5.4 |
| external_oct23 | `feat/v0.1-external-opportunity-notifications`, `/tmp/undergrid-worktrees/external-notifications/feat-v0.1-external-opportunity-notifications` | Recovered exact b807 integration12 cases33.751s; final worker41-case race89.981s and frozen038/039. Current source filtering, authority and continuation proofs remain scoped | Final contract/build/doc proof, PR/review/merge; `EXTERNAL_OPPORTUNITY_NOTIFICATIONS.md` |
| profile_oct23 | `feat/v0.1-semantic-profile-authority`, `/tmp/undergrid-worktrees/semantic-profile-authority/feat-v0.1-semantic-profile-authority` | Six actual policy-revocation RED subcases; fresh guard before provider POST and durable observation is authored | Focused lifecycle/private/withdrawal GREEN+vet, merge dependent feature into semantic after review; `SEMANTIC_PROFILE_AUTHORITY.md` |
| assignment_oct23 | `feat/v0.1-owner-assignment-response`, `/tmp/undergrid-worktrees/owner-assignment/feat-v0.1-owner-assignment-response` | Actual HTTP oracle authored before behavior; exact reviewed task/plan and separate responsibility acknowledgement required | HTTP RED, domain/UI/contracts, current-authority and rollback proofs; `OWNER_ASSIGNMENT_RESPONSE.md` |
| Completed dependent UI | `feat/v0.1-owner-compatible-groups-ui`, `/tmp/undergrid-worktrees/owner-compatible-groups/feat-v0.1-owner-compatible-groups-ui` | Clean pushed8f03ecd;115 UI tests, build and147-schema checks passed. Depends on semantic baseline | Dependency integration before release PR; `OWNER_COMPATIBLE_GROUPS.md` |

Only one broad Go/PostgreSQL gate runs at a time. Deadline-sensitive worker gates
also reserve quiet broad Node capacity. Observe the original process handle until
terminal; observation expiry is not a reason to restart. Preserve failures, old
jobs, credentials, database ledgers and worktrees. Applied migrations are immutable.

## Release invariants and remaining scope

Domain transactions check current identity and membership before retained replay.
Owner plan and assignment commands acquire the canonical sorted principal set
before individual owner locks. Assignment, admission, plan/contribution votes,
publication consent and financial authority remain distinct.

Semantic comparison cannot override missing or conflicting hard evidence. Every
group checks all pairs plus global feasible constraints/license intersection;
principal IDs do not prove independent beneficial owners. Private selected input
stays owner-authorized and never becomes a public candidate. Provider fixture
vectors prove runtime/numeric wiring only. Fresh profile authority must guard
provider I/O, durable evidence and discovery reads independently.

Notifications must bind current revisions/source snapshots and deduplicate
transactional opportunities/events/inbox without losing candidate tails or hiding
queue starvation behind longer deadlines. Their observations remain external,
unverified demand/capability evidence, not registered participants.

Open work beyond this wave includes authorized suggestion dismissal and unchanged-
evidence recovery, user-visible hard-unknown clarification, complementary discovery,
partial satisfaction, additional collaboration/artifact/rights controls, owner inbox
and reporting/email, isolated financial UI/reconciliation, privacy/retention/restore,
production storage/verifier isolation and full operational/cohort/provider gates.
Full completion still requires requirement-by-requirement proof, not case counts.

## Progress after recovery

External notification PR88 merged to release `ab330e3` after exact reconciled
12-case proof,41-case worker regression and current contract/build qualification.
Root current-profile discovery read reproduced stale public results (3.405s), then
passed the six-case21.577s race slice and vet. The profile-authority worker proved
fresh provider/write-policy guards (four top-level tests, six subcases13.126s), and
then reproduced a distinct already-embedded stale-worker notification write in
all three revocation cases; that additional fix is not yet qualified. Assignment
review has an actual missing-route RED and is under focused backend/UI checks.

The external worker now owns `feat/v0.1-opportunity-dismissal` in
`/tmp/undergrid-worktrees/opportunity-dismissal/feat-v0.1-opportunity-dismissal`:
all suggestion kinds, explicit authorized dismissal, unchanged-evidence
suppression/recovery and current-authority replay. Its owning artifact is
`OPPORTUNITY_DISMISSAL.md`; network040 is reserved. No dismissal behavior or
migration has been implemented at this checkpoint; the actual HTTP oracle is
unrun. Existing038/039 remain frozen.

Combined semantic/external source `d735921` passed37/38 top-level race cases;
initial watcher scan missed its five-second deadline behind349 retained FIFO
jobs. No deadline or fixture was weakened. A dedicated dependent
`feat/v0.1-fresh-discovery-capacity` worktree now owns the reproducer and queue
fairness repair; profile_oct23 owns that task. See `SEMANTIC_DISCOVERY.md` for
actual job timing and scope. Its owning document is `FRESH_DISCOVERY_CAPACITY.md`.
The unchanged full TDD objective remains active.
