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
