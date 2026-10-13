# Coordination deadline repair — SW-01/03/06

Scope: TDD r2 §§6.3, 7.2–7.3 and §9. Implements persisted UTC participation, proposal, assignment and execution deadlines. This is a timer slice, not complete coordination, meaningful-progress verification, replacement, succession or archive qualification.

## Runtime behavior

`RunWorker` registers `coordination_deadlines_v1`, runs repair at startup and every minute, with ten bounded retry attempts. `Service.SweepCoordinationDeadlines` reconnects as `undergrid_worker`, reads current persisted deadlines and processes at most 100 rooms, 100 pending memberships and 100 due plans per room (plans already bound to 50 tasks). Each room transaction locks the room, then plans before tasks in the same order as commands. Conditional state/deadline updates serialize timely responses and renewed progress with stale jobs; workers hold no cached consent or claim authority.

Applications persist an explicit 72-hour response deadline (including a migration backfill). Expired applications cannot become offers even before repair. Offered membership expiry never joins the target. Proposal expiry never creates approval; its offered tasks return open and already accepted tentative assignments cancel. Assignment response expiry returns open, increments version/claim epoch and appends `task.assignment_expired`; silence never starts work.

Execution progress deadlines begin only on atomic full plan acceptance, using the immutable approved task interval. Accepted historical tasks lacking a deadline get a new upgrade clock and version increment. A progress timeout records `at_risk`, preserving claim epoch and storing a 48-hour cure deadline measured from the missed progress deadline. After cure, repair records abandonment and increments epoch. Commands reject progress after elapsed cure even before the sweep, including unswept in-progress tasks. A behavioral red reproduced the prior worker-downtime bypass (HTTP 200 after 49 hours); the domain timestamp guard now denies it. Accepted outputs, submitted evidence, license grants and dependencies are not fabricated or revoked by timers.

Task GET, plan task snapshots and agent recovery obligations expose persisted response/progress/cure timestamps and current epoch. Events and fanout jobs commit with transitions; uniqueness plus state predicates prevents repeat events. Expired membership notification reaches both its target and current members. New fanout and timer repair use the bounded `coordination` queue so matching/recruitment cannot consume their worker slots; already queued default jobs remain serviceable.

## Tests and evidence

- Test-first red: `TestCoordinationDeadlineSilenceAndRestart` initially failed to compile because the timer entrypoint did not exist. This is recorded as an unavailable entrypoint red, not a provider or completed behavior result.
- `TestCoordinationDeadlineSilenceAndRestart`: actual startup periodic River dispatch expires a SQL-due invitation and actual fanout persists two inbox entries; replay/restart creates one normalized expiry event/version. Timely public HTTP acceptance concurrently with stale repair stays accepted; private expired packet access fails closed.
- `TestProgressDeadlineCureAndExpiredPlan`: assignment approval cannot start the progress clock; full plan approval does. Real persisted cure is exactly 48 hours from the missed deadline (PostgreSQL microsecond precision), restart recovery retains deadlines/epoch, current progress cures, concurrent stale sweeps cannot undo renewal, repeated expired-cure repair abandons once and old epoch fails.
- `TestProposalAndAssignmentExpiryPersistence`: expired offer is open/version2/epoch2 with one event, proposal expires once and late public HTTP approval fails.
- `TestApplicationDeadlineCannotAdmitAfterExpiry`: actual HTTP application has a persisted 72-hour default, overdue application cannot become invitation before repair and replay produces one expiry version.

Own fresh UTF8 PostgreSQL clusters: lifecycle port55443 and zero-state replay port55447. No other agent fixtures were reset. API tests run transactions as `undergrid_app`; repair/fanout connections explicitly switch to non-superuser, non-BYPASSRLS `undergrid_worker`. A fresh-migration check caught the missing worker row-lock grant; the migration was corrected, then replayed from zero. Additional real dispatcher tests reproduced queue starvation; dedicated coordination slots corrected this rather than weakening deadline assertions.

Validation: full `go test -race ./internal/network -count=1`, module checksums, `go vet ./...`, `go build ./...`, `git diff --check`, and six Python reference-listener recovery tests. Financial/provider suites are unchanged and require the integration owner's separate financial fixture; this evidence does not claim those provider or deployment gates.

## Quality review and remaining gates

Policy lives in domain transactions and the timer; adapters do not grant admission or approvals. A single cure-window constant feeds both timer SQL and command denial. Room/plan/task lock order, conditional persisted predicates, transactional outbox and bounded batches provide restart/concurrency recovery without a generic timer framework. Network worker credentials/storage remain separate from sandbox authority.

Explicit limitation: the existing progress command reports a state/epoch, which proves neither useful progress nor output quality. Evidence-bearing progress/cure is a next vertical behavior, as are explicit replacement, contributor succession/archive and downstream blocked projections. Current explicit task progress interval is required by the plan contract; this slice does not silently introduce omitted-interval defaulting or alter exact plan hashes. Fixture rights are not approved production legal terms. No production deployment or external-provider qualification occurred.

## Release reconciliation

Reconciled `feat/v0.1-coordination-deadlines` onto release integration baseline `efafe57` in `reconcile/v0.1-coordination-deadlines`, without protected-branch writes. Conflicts were limited to `jobs.go` and `tasks.go`: preserved artifact environment configuration and the artifact worker/verification queue, reused the configured service for the deadline worker, and retained both submitted-artifact and cure-deadline fields in task storage/projections. No artifact implementation or verification invariant was removed.

Own port55447 fixture was reset for combined zero-state migrations after an initial old-fixture queue-backlog dispatch timeout. Combined focused race tests for artifact public publication through the executable worker, actual deadline startup dispatch/fanout and progress/cure passed in40.644s. Static vet/build passed. Full combined network race suite is the reconciliation gate; financial fixtures/providers/deployment remain separately owned.

### Concrete integration quality findings

Important — worker scheduling/pool capacity: the full combined suite reproduced the unchanged15-second startup expiry/fanout failure in162.179s. At observation,225 fanouts were completed and230 remained available;13 artifact evaluations were completed;2 deadline jobs were completed and7 were available. PostgreSQL showed WALSync/WALWrite pressure and principal transaction waits. Three two-worker queues shared a five-connection River pool. Fresh-fixture success did not prove liveness under that backlog.

Correction: keep ordinary coordination fanout and artifact verification intact, but reserve two `coordination_deadlines` worker slots for timer repair and its transition notifications. Timer jobs have priority1; ordinary fanout cannot consume those slots. River pool capacity is10 for four queue fetchers, completions and periodic/leader maintenance. The identical bounded deadline assertion then passed on the existing loaded fixture in10.828s; no queue reset, deadline extension or weaker assertion was used for that red/green. All worker-side deadline and domain expiry guards remain unchanged. General large-cohort queue fairness, host resource admission and overload qualification remain release-relevant operational gates; this correction proves only the observed seam.

Orthogonality/contracts: merge conflicts preserve both normalized task pointers (`submitted_artifact_id`, `cure_deadline`) and their public read projection; configured artifact service and immutable verifier path remain registered. Timers operate on current persisted states/deadlines and never accept output or consent. Queue routing is concrete and bounded rather than a general scheduling framework. Domain guards deny elapsed admission/assignment/cure even when worker repair is delayed.

The runtime-lifecycle-testing and release-contract-validation skills were applied to the actual periodic dispatch, atomic expiry event/outbox, normalized version/epoch/cure state and reconstructed obligations. The resulting reconciliation commit must pass the complete network race command on its exact SHA before handoff; the handoff records that SHA and result. No production lifecycle was exercised, and no main/deployment readiness is inferred.
