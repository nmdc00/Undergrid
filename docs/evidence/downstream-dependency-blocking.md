# Durable downstream dependency blocking

TDD §§7.2–7.3 / SW-03. [Locked behavior and replacement contract](../DOWNSTREAM_DEPENDENCY_BLOCKING.md). Branch `feat/v0.1-downstream-dependency-blocking`, initially based on merged release2589e00. Test-only checkpoint6ca4e97 preceded behavior, followed by releasea5cbfea reconciliation276b454 and implementationcd22c54. Archive/skill release0126c25 reconciliation produced790f007 without editing frozen migrations026–029.

## Actual red and focused proof

The first default-sandbox run failed0.008s solely because connecting to the assigned PostgreSQL94 Unix socket returned operation-not-permitted. It is setup evidence only (`/tmp/undergrid-dependency-primary-red94.log`). The necessary escalated command waited for approval, then ran as session14689 and reached the real behavior. **Actual red: FAIL5.619s**, GETmiddle200 after successful member withdrawal lacked `dependency_blocked` while retaining accepted assignment/epoch1. Log `/tmp/undergrid-dependency-primary-red94-escalated.log`. No migration029 existed before that terminal missing-behavior result.

After implementation, the actual restricted-login four-test gate with `-race -count=1` **passed28.502s**, no failures/skips (`/tmp/undergrid-dependency-focused94.log`, session40344). The compiled source is recorded ascd22c54; committing those same bytes while the run executed did not change its source. Tests:

- `TestWithdrawalDurablyBlocksTransitiveDependencies`: real HTTP withdrawal and current-authorized reads mark shared/transitive/single-root descendants, preserving assignment state/assignee/epoch.
- `TestWithdrawalDependencyEvidenceRecoveryAndReplacement`: normalized flag/version/epoch and exact reachable causal root sets; one event/job per transition; actual worker fanout repeated without duplicate inbox effects; current private ACLs; unchanged accepted workload and historical deadlines; monotonic-property database enforcement; restart and obligation snapshot; real new-plan approvals and separate admitted replacement assignment acceptance; stale old claims cannot act, old plan/rights remain.
- `TestWithdrawalSerializesWithDownstreamProgress`: actual observed room/principal-lock waits serialize withdrawal before progress; denied progress writes no canonical event.
- `TestWithdrawalDependencyBlockingPreservesVerifiedEvidence`: actual private upload, fixed executable verification and River worker evaluation pass before root withdrawal. Exact acknowledged artifact/evaluation rows and current submitted-artifact pointer remain; blocked output/progress upload attempts fail and removed contributors cannot replay private canonical work.

Normal River cancellation/soft-stop messages occurred during orderly artifact worker cleanup; the lifecycle and bounded cleanup completed successfully. These are not provider receipts, unrelated-owner qualification or kernel-enforced verifier isolation proof.

## Database and migration boundaries

Assigned network-only fixture: `host=/tmp/undergrid-pg-coordinator-55494/socket port=55494 user=nmcdc dbname=postgres`. This is a retained network fixture; no new fresh-cluster bootstrap is claimed. The tests invoke unmodified `network.Migrate`; no SQL was replayed, omitted or manually recorded. A separate read-only ledger inspection confirmed contiguous network001–029 plus`river-0.49.0-ready`. The ledger records filenames, not SQL checksums. Independent checksums:

- Frozen028: `718089e0efe5c4dac59b6a8054bdcc0d60794c1bab0a9b200b7e4318e1778f06`.
- Applied/frozen029: `391ce455647947c267cdfdde9b1d14acd39df130f45d6bde940dc1512f1999e7`.

The actual column is boolean/NOT NULL/defaultfalse. Tests connect API commands as`undergrid_dependency_api_runner` and fanout/deadline work as`undergrid_dependency_worker_runner`, with distinct app/worker memberships, no superuser/RLS bypass or table ownership. The existing explicit DSN builder independently inspects actual session identity. Read-only role inspection also confirmed both LOGINs have superuser=false and bypassrls=false. Bootstrap/assertions use the fixture's separate administrative connection. The artifact scenario independently asserts its existing separate artifact API/worker identities. No financial database, credential or deployment identity was reused. Retained fixture identities establish local authorization boundaries, not deployed identity qualification.

## Static, contract and quality assessment

On implementationcd22c54: network/command vet and command build passed, canonical contract validation passed127 schemas/generated-type drift, all41 UI tests passed and TypeScript/Vite build passed. After archive/skill reconciliation790f007: canonical validation passed128 schemas/generated TypeScript parity, TypeScript/Vite build, network/command vet/build and diff checks passed. A prior contract command used the web directory with repository-relative paths and failed setup before generation; correcting its working directory and dependency link produced the passing checks. No failing setup command is counted as behavior proof.

DRY: one dependency guard owns durable impediment plus current prerequisite knowledge across progress and artifact canonical commands. Existing transaction/event/fanout and new-plan approval/assignment paths remain the effect owners. Orthogonality: assignment state, dependency impediment, progress evidence, rights and sandbox finance stay separate. No extra state enum, generic workflow, execution of agent text, new financial import or authority-bearing adapter was introduced. Bounded same-plan traversal and plan-before-task locks preserve recovery and concurrency; a malformed oversized graph fails the entire withdrawal transaction rather than committing a partial mark. Property monotonicity prevents old-claim unblocking even through the app role. Private reads/fanout use current membership; no replacement can see history before explicit admission.

No automatic assignment replacement, rights transfer, new license decision or universal liveness guarantee is claimed. The existing noncurrent-artifact acceptance-pointer issue identified during archive review is outside this vertical and remains root-owned; this feature explicitly preserves the current submitted-artifact pointer. Full release validation on the promotion SHA, unfamiliar-user/cohort evidence, hard-resource verifier/storage qualification, legal/source/provider approvals and deployment remain separate gates. GitHub-hosted CI is owner-waived; required local checks remain.

## Affected gate / review status

The focused feature gate above is complete. The broader existing plan/task/artifact/protocol/archive affected race gate is queued behind the financial integration run. No full release, PR merge, promotion or deployment is claimed at this checkpoint.
