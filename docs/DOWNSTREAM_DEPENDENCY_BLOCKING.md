# Locked task: downstream dependency blocking

Checkpoint 2026-10-18. Owner-authorized full r2 implementation; root assigned this bounded vertical under TDD §§7.2–7.3 and SW-03. Branch `feat/v0.1-downstream-dependency-blocking` starts from merged release `2589e00`. Later release integration is pending; root reports `a5cbfea` as the newer release checkpoint.

## Contract and boundary

Voluntary member withdrawal through `POST /v1/swarms/{id}/withdraw` must durably mark unfinished transitive downstream tasks as impeded by dependencies. Preserve their existing assignment state, assignee, accepted-workload accounting and claim epoch; increment their versions and append transactional events/inbox work with the exact withdrawn ancestor task IDs and resulting epochs. Keep sealed plan wording, existing contribution rights, artifacts and acknowledged evaluations intact. No silence-as-consent, administrative assignment or private-history transfer follows withdrawal.

The planned representation is an orthogonal `tasks.dependency_blocked` property, not another task-state enum value. Network migration029 is reserved for this vertical; migrations026–028 are frozen and must not be edited. A shared task dependency guard must protect unfinished progress, progress-evidence submission/review and artifact canonical work. Blocked tasks remain obligations and retain workload; timeout processing must not blame them or silently extend deadlines. No unblock or reassignment under the old plan. Replacement uses existing explicit admission, a new sealed plan with fresh required rights/plan approvals and separate new-agent assignment acceptance. Superseded old claims remain unusable.

## Test-first checkpoint

`internal/network/dependency_blocking_test.go` contains test-only work:

- `TestWithdrawalDurablyBlocksTransitiveDependencies`: actual restricted-login HTTP withdrawal and task reads on a bounded DAG with two withdrawn roots, shared and single-root descendants and unrelated work.
- `TestWithdrawalDependencyEvidenceRecoveryAndReplacement`: exact causal IDs/epochs, single durable events/fanout and duplicate-delivery handling; current ACLs, workload, persisted deadlines, restart/context and immutable plan/rights; actual new-plan replacement and explicit assignment acceptance.
- `TestWithdrawalSerializesWithDownstreamProgress`: actual PostgreSQL row waits serialize withdrawal before a downstream progress attempt; denied progress must leave no canonical event.
- `TestWithdrawalDependencyBlockingPreservesVerifiedEvidence`: actual upload, fixed executable verifier and worker evaluation followed by contributor withdrawal; previously acknowledged artifact/evaluation rows stay unchanged while blocked output/progress upload attempts fail.

These tests compile with `go test ./internal/network -run '^$'`; that command executes no lifecycle tests and establishes no red/green result. `git diff --check` passes. No behavior changes, migration029 file or runtime PostgreSQL command have been made by this task at this checkpoint. Tests may still reveal fixture problems; those are not product-red evidence.

## PostgreSQL scheduling and fixture notes

Root owns the shared runtime schedule: settlement actual RED, then archive corrected focused checks, then this task's primary missing-behavior RED. Do not start a PostgreSQL cluster, apply migrations or run runtime tests before the slot is released. No fixture DSN has been assigned to this task yet. The primary red selector is `^TestWithdrawalDurablyBlocksTransitiveDependencies$`; run it before creating migration029 or changing behavior.

The fixture consumes the explicitly supplied `UNDERGRID_TEST_DATABASE_URL` as its separate bootstrap/assertion connection and invokes unmodified network `Migrate`. It creates distinct `undergrid_dependency_api_runner` and `undergrid_dependency_worker_runner` LOGINs with the respective nonprivileged application/worker group memberships. The existing explicit-URL DSN builder inspects actual `session_user`, superuser/RLS-bypass and ownership; it must not fall back to administrative runtime connections. The artifact lifecycle reuses the existing artifact fixture's separately asserted API/worker LOGINs and private local storage/fixed verifier. No financial database, identity or credential may be reused for this network fixture. Record the actual assigned fixture, commands, terminal red/green results, migration ledger/checksum and limitations in feature evidence after execution. Socket access remains subject to the actual sandbox permission boundary; request escalation on a denied necessary command rather than bypassing it.

## Acceptance and review

Keep this one behavior on one branch and one release-targeted PR. First record an actual missing-behavior red, then implement the smallest transactional property/guard/event change and run focused lifecycle/race plus affected existing plan/task/artifact/protocol/contract/static/build checks. Update canonical OpenAPI and generated TypeScript consistently for task, plan summaries and obligation snapshots. Review lock ordering, bounded same-plan traversal, exact root causation, current ACLs, event/job atomicity, workload and failure recovery. Required local checks remain; GitHub-hosted CI is owner-waived. Broader release validation, external legal/source/provider approvals and deployment remain separate root-owned gates. No completion, provider verification or deployment is claimed by this checkpoint.
