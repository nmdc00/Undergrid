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
successful receipt replay with exactly one completion event. Affected broader
artifact race regression remains pending; this checkpoint is not a release claim.
