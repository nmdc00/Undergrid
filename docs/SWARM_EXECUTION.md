# v0.1 implementation swarm

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

Hosted CI blocker: GitHub account billing lock prevents jobs from starting. Feature PRs and draft release PR remain open; preserve required checks and rerun after account recovery. Local tests continue as explicitly local evidence.

## Review and promotion controls

GitHub protection is verified on `release/v0.1`, `develop`, and `main`: PRs required, `go-postgres` required against the current base, administrator enforcement enabled, conversation resolution required, force pushes and branch deletion disabled. Hosted CI billing is an external account blocker; no override or fake passing status is used. `main` and `develop` remain identical at the documentation baseline.

- Plans: [PR #4](https://github.com/nmdc00/Undergrid/pull/4), authority and assignments; review found changed terms under the same template ID could authorize progress after restart. A real failing regression led to sealed-template digest checks for assignment/progress. Local race, build and vet pass.
- Quotas: [PR #5](https://github.com/nmdc00/Undergrid/pull/5), durable principal admission; local HTTP/RLS concurrency, restart, replay and pause/reactivation tests plus all-package race/build/vet pass. Rate, storage and fair-queue requirements remain deferred.
- Sprint evidence: [PR #3](https://github.com/nmdc00/Undergrid/pull/3), documentation-only; records repository setup, isolated assignments and blockers.

The local reconciliation candidate `ce798ded40ff38303e5fb2f15ed452b73916227b` combines plans and quotas and passed `go test -race -count=1 ./...` on a completely fresh PostgreSQL 16 cluster at port 55440 (network 29.863s). The UTF-8 network database started with no copied schema, roles or participant data, so this proves the original migrations plus candidate migrations from scratch. Sandbox integration will receive a separate database and runtime role. This temporary candidate does not advance the protected release branch or establish deployment readiness.

## Final resumed-wave evidence

All task branches are published; feature PRs remain unmerged while hosted CI is blocked:

| Task | PR | Feature head | Local state | Owning document |
|---|---|---|---|---|
| Plans and assignments | [#4](https://github.com/nmdc00/Undergrid/pull/4) | `a7425a9` | Runtime/lifecycle/race/build/vet proven; partial N3 | `docs/evidence/plans.md`, TDD §§6–8, 10 |
| Sandbox authority | [#6](https://github.com/nmdc00/Undergrid/pull/6) | `2090513` | Runtime/lifecycle/race/build/vet proven; partial F1 | `docs/evidence/sandbox.md`, TDD §§2.2, 11 |
| Principal quotas | [#5](https://github.com/nmdc00/Undergrid/pull/5) | `fd7c6ef` | Domain concurrency/replay/recovery proven; partial operational limits | `docs/evidence/quotas.md`, TDD §12 |

The final combined preview is published as `reconcile/v0.1-wave1` at `9816ec99e3c5b1c6b3b2282a408111175b9ebe80`, tree `aea6267824e22524351937491c32987ac3c1423c`. All-package race tests with `CI=true`, vet, command builds, module verification, formatting, diff checks, and six separately labeled listener fixtures passed. Network and financial databases were distinct in the fresh cluster; sandbox transactions used a restricted test login. See [integration contract and retrospective](evidence/wave1-integration.md).

This preview is review evidence, not a replacement for feature PRs or exact-release validation. After account recovery, rerun checks, merge feature PRs into release in the documented order with current-base validation, validate the resulting release SHA, then proceed through develop and main only when the complete release contract and external approvals allow it. No protected merge, public deployment, provider qualification or full milestone completion occurred.
