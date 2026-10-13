# v0.1 implementation swarm

Current policy (2026-10-13): the owner waived GitHub CI as a merge gate. The hosted workflow is disabled and required GitHub status checks were removed from release/develop/main. PRs, administrator enforcement, conversation resolution, and protection against force pushes/deletion remain. Local behavioral, PostgreSQL lifecycle, race, build and static checks remain required. Feature PRs #4, #6 and #5 are merged into `release/v0.1` at `37477c2`; exact-release validation follows. The earlier CI billing and premerge snapshots below are historical. Full production release/deployment approvals remain open.

The owner requested swarm execution on 2026-10-12. All coding is isolated by worktree. This is an execution record, not a release approval.

Baseline: original implementation wave started at `abacd10`; repository bootstrap is integrated into `release/v0.1` at `413e4c0`. The owner selected [nmdc00/Undergrid](https://github.com/nmdc00/Undergrid) on 2026-10-13. GitHub `main` and `develop` share the documentation bootstrap and have zero divergence. Implementation is published on `release/v0.1`; [draft release PR #2](https://github.com/nmdc00/Undergrid/pull/2) targets `develop`. Feature PRs target `release/v0.1`; subsequent validated promotion is `release/v0.1 -> develop -> main`. No deployment is authorized by branch publication.

| Agent | Branch | Worktree | Acceptance and persistence | Owning evidence |
|---|---|---|---|---|
| plans | `feat/v0.1-versioned-plans` | `/tmp/undergrid-worktrees/plans/feat-v0.1-versioned-plans` | HTTP plan approval and assignment write paths; exact hashes, current rights, separate acceptance, version/epoch checks in PostgreSQL | `docs/evidence/plans.md`: TDD sections 6–8, 10; tests, limitations, quality review |
| finance | `feat/v0.1-sandbox-payments` | `/tmp/undergrid-worktrees/finance/feat-v0.1-sandbox-payments` | Separate sandbox process/database/audience; authorization, idempotent payment effects; provider evidence explicitly distinguished from fixture evidence | `docs/evidence/sandbox.md`: TDD sections 3.2, 11; tests, external blockers, quality review |
| verifier | `feat/v0.1-json-verifier` | `/tmp/undergrid-worktrees/verifier/feat-v0.1-json-verifier` | Fixed verifier executable on bounded JSON; hashes, deterministic checks, untrusted-input/process tests; no network/database access by design | `docs/evidence/verifier.md`: TDD section 8.3; process contract, tests, isolation limitations |
| quotas | `feat/v0.1-principal-quotas` | `/tmp/undergrid-worktrees/root/feat-v0.1-swarm-integration` | Durable principal admission limits enforced in domain transactions; idempotent retries and concurrent boundary proof in PostgreSQL | `docs/evidence/quotas.md`: TDD section 12, launch quota defaults; tests, limitations, quality review |
| coordination | `docs/v0.1-sprint-flow` | `/tmp/undergrid-worktrees/coordination/sprint-flow` | Reconcile sprint manifest with GitHub branch flow and release evidence | This document and `docs/evidence/ci.md`; no behavior change |

## Merge and validation order

1. Review plans against current authority and durable lifecycle requirements; integrate only after tests and evidence reconcile.
2. Review verifier independently; subsequent artifact admission/publication is a separate dependent slice, not proven by a verifier helper.
3. Review sandbox foundation independently; real Stripe and Base Sepolia receipts remain mandatory before financial release designation.
4. Reconcile CI with integrated entrypoints, then rerun integration checks on the exact release state.

Plans, sandbox authority and quotas are in progress. The fixed verifier was locally integrated at `669e34b`; container isolation and dependent artifact publication remain unqualified. CI is published, with hosted results tracked separately. No N0–N6 or F0–F4 completion is implied. Further bounded assignments follow this wave. Full release still requires UI/protocols, artifact publication/reuse, source permissions, monitoring/operations, restore/security qualification and an unfamiliar independent cohort.

## Resource and evidence controls

Plans and quotas use distinct network test databases (`undergrid_network_plans` and `undergrid_network_quotas`). Finance uses a separate sandbox test database (`undergrid_sandbox_f1`) and role names. All agents share the installed Go toolchain/module/build caches; avoid simultaneous expensive builds while disk space is constrained. No test uses production credentials or labels test identities as unrelated beneficial owners.

Unresolved external gates: hosting/domain/OIDC provider, approved rights and source permissions, provider sandbox credentials/facilitator/wallet configuration, legal review and independent cohort. These block affected release work, not independent implementation. No production financial authorization has been granted.

## Repository bootstrap and resumed wave (2026-10-13)

[PR #1](https://github.com/nmdc00/Undergrid/pull/1) published the README and design baseline after six reference-listener tests and a clean final whitespace check. The unrelated initial GitHub commit was preserved by history reconciliation, without force-pushing. Git uses the account’s existing SSH authentication because its OAuth token lacks workflow scope.

The README describes implementation and qualification limits. `main` and `develop` contain documentation only until the release is validated. Remote feature review, exact-SHA integration validation and subsequent promotion remain pending. Undergrid deployment requires its own approved hosting, identity and credentials; the Thufir deployment skill and server are not applicable.

Merge order: plans authority, sandbox authority (independent database), principal quotas, then final integrated qualification. A feature must include its proving tests and reconciled evidence before merge. The fixed verifier’s merged history is retained; its outstanding OS isolation gate is not waived.

Historical hosted CI result: the GitHub account billing lock prevented jobs from starting. The owner subsequently waived hosted CI; local checks remain explicitly local evidence.

## Review and promotion controls

GitHub protection is verified on `release/v0.1`, `develop`, and `main`: PRs required, `go-postgres` required against the current base, administrator enforcement enabled, conversation resolution required, force pushes and branch deletion disabled. This was the pre-waiver setting. Required hosted status checks were subsequently removed by explicit owner instruction; no fake passing status is used.

- Plans: [PR #4](https://github.com/nmdc00/Undergrid/pull/4), authority and assignments; review found changed terms under the same template ID could authorize progress after restart. A real failing regression led to sealed-template digest checks for assignment/progress. Local race, build and vet pass.
- Quotas: [PR #5](https://github.com/nmdc00/Undergrid/pull/5), durable principal admission; local HTTP/RLS concurrency, restart, replay and pause/reactivation tests plus all-package race/build/vet pass. Rate, storage and fair-queue requirements remain deferred.
- Sprint evidence: [PR #3](https://github.com/nmdc00/Undergrid/pull/3), documentation-only; records repository setup, isolated assignments and blockers.

The local reconciliation candidate `ce798ded40ff38303e5fb2f15ed452b73916227b` combines plans and quotas and passed `go test -race -count=1 ./...` on a completely fresh PostgreSQL 16 cluster at port 55440 (network 29.863s). The UTF-8 network database started with no copied schema, roles or participant data, so this proves the original migrations plus candidate migrations from scratch. Sandbox integration will receive a separate database and runtime role. This temporary candidate does not advance the protected release branch or establish deployment readiness.

## Final resumed-wave evidence

All task branches were published; the following table records the reviewed feature heads, now merged by the owner-authorized PR flow:

| Task | PR | Feature head | Local state | Owning document |
|---|---|---|---|---|
| Plans and assignments | [#4](https://github.com/nmdc00/Undergrid/pull/4) | `a7425a9` | Runtime/lifecycle/race/build/vet proven; partial N3 | `docs/evidence/plans.md`, TDD §§6–8, 10 |
| Sandbox authority | [#6](https://github.com/nmdc00/Undergrid/pull/6) | `2090513` | Runtime/lifecycle/race/build/vet proven; partial F1 | `docs/evidence/sandbox.md`, TDD §§2.2, 11 |
| Principal quotas | [#5](https://github.com/nmdc00/Undergrid/pull/5) | `fd7c6ef` | Domain concurrency/replay/recovery proven; partial operational limits | `docs/evidence/quotas.md`, TDD §12 |

The final combined preview is published as `reconcile/v0.1-wave1` at `9816ec99e3c5b1c6b3b2282a408111175b9ebe80`, tree `aea6267824e22524351937491c32987ac3c1423c`. All-package race tests with `CI=true`, vet, command builds, module verification, formatting, diff checks, and six separately labeled listener fixtures passed. Network and financial databases were distinct in the fresh cluster; sandbox transactions used a restricted test login. See [integration contract and retrospective](evidence/wave1-integration.md).

This preview remains review evidence rather than a replacement for exact-release validation. PRs #4, #6 and #5 were reviewed against their exact validated heads and merged in that order. The resulting release source tree is exactly `aea6267824e22524351937491c32987ac3c1423c`, matching the locally proven preview. Reviewed partial implementation can be integrated into develop through PR #2 after exact-release local validation. Production promotion to main still requires the complete release contract and external approvals. No public deployment, provider qualification or full milestone completion is implied.

## Owner-approved main checkpoint and local-file cleanup

The owner requested main promotion on 2026-10-13. This authorizes the code checkpoint through the established release/develop/main PR flow; it does not designate a complete v0.1 release or authorize service deployment, provider/source/legal approvals, or production credentials. Those gates remain explicit. Hosted CI remains disabled by the prior owner decision; configured local tests remain required.

[PR #7](https://github.com/nmdc00/Undergrid/pull/7) removes AGENTS.md from tracking while preserving the local instructions and ignores analogous local tooling, credential and generated paths. Portable samples remain tracked, including an explicit exception for .env.sandbox.example. No history rewrite was performed.

Coverage measurement exposed a reproducible shutdown defect. [PR #8](https://github.com/nmdc00/Undergrid/pull/8) configures a five-second River drain/cancellation window inside the unchanged ten-second stop bound. Real locked-transaction red/green and normalized retained-job/intention evidence are in [worker shutdown](evidence/worker-shutdown.md). The complete local race/coverage suite passes after the fix; [coverage](evidence/coverage.md) reports measured limits. Earlier pending-main status snapshots describe the state before this owner instruction.
