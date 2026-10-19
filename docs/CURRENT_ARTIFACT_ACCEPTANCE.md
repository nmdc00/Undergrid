# Current-submission acceptance guard

TDD §8.3 requires canonical task acceptance to refer to its exact current
submission. One bounded fix branch: `fix/v0.1-current-artifact-acceptance`.

Observed seam: `acceptArtifact` writes an approval, then updates a task guarded
by `submitted_artifact_id`, but ignores the affected-row count before appending
`task.output_accepted`. Current common artifact authority does not check that
pointer because private input grants and verification history have distinct
rules. Do not widen that shared guard or invalidate acknowledged verification.

Proving scenario: create/accept plan and assignment, upload/seal/evaluate a real
output, record one legitimate acceptance. Inject a persisted recovery fault
(current submission pointer absent) through the fixture administrator. The
second acceptance through HTTP must fail atomically without approval/event/task
effects. Restore the pointer and retry; exact current acceptance must complete
once. This fault injection does not claim that the present public API can
produce the inconsistent pointer through an ordinary successful command.

Red before behavior, then a narrow acceptance-specific current-submission
precondition and checked state transition. Existing readable evidence, fixed
verification and published licensed reuse must remain intact. No migration or
provider action is required. Run affected artifact lifecycle/race regressions
and reconcile the owning TDD/evidence before integration.

Actual fault-injection RED on PostgreSQL96 failed30.634s: the second vote
returned200 for an output whose current task submission reference was absent.
The narrow guard now checks submission identity and submitted/accepted-output
state before any approval, and requires exactly one row from the terminal
state/version/submission UPDATE before emitting completion. Shared artifact
read, grant and verification authority remains unchanged.

Focused actual-login lifecycle GREEN passed4.054s, including atomic denial,
restoring the current pointer, retry with the same key after denial and repeated
successful receipt replay with exactly one completion event.

At source `e3d3261`, six affected actual-PostgreSQL96 lifecycle tests passed
with race detection in62.810s: the recovery fault, canonical publication through
the executable verifier worker, exact private-input publication consent,
immutable progress-input audiences, archived public licensed reuse and failed
fixed-check denial. Log: `/tmp/undergrid-current-artifact-acceptance-affected96.log`.
This is scoped local evidence, not provider or production qualification.

Release dependency-blocking integration `135b3fe` was reconciled cleanly in
`a1e5c5a`. The acceptance guard remains specific to acceptance; shared artifact
reads, verification history, publication rights and dependency authority keep
their existing owners. The task is locked before the acceptance transaction,
and the checked state/version/submission update prevents a false completion
event. No schema migration is introduced by this fix.

On integrated source `a1e5c5a`, the seven-case actual-PostgreSQL96 race gate
passed98.995s with no skips. It additionally covers preserved verified evidence
after dependency withdrawal; normal migration applied029 before the tests.
Log: `/tmp/undergrid-current-artifact-acceptance-integrated96.log`. Network and
executable vet and diff checks passed. These checks do not establish whole
release coverage or a production deployment.

Post-deployment qualification must repeat the denied-current-pointer recovery
scenario in an authorized fixture and verify normalized task state/version,
approval counts and exactly one `task.output_accepted` event after restoration.
Do not inject administrative faults into a live collaboration.
