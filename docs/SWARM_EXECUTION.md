# v0.1 implementation swarm

The owner requested swarm execution on 2026-10-12. All coding is isolated by worktree. This is an execution record, not a release approval.

Baseline: `release/v0.1` at `abacd10`. Local `main` and `develop` have zero divergence. No remote is configured; remote parity, feature PRs, release PR and protected-branch promotion are blocked pending repository selection. Feature PRs must target `release/v0.1` when available. No PR URLs exist yet.

| Agent | Branch | Worktree | Acceptance and persistence | Owning evidence |
|---|---|---|---|---|
| plans | `feat/v0.1-versioned-plans` | `/tmp/undergrid-worktrees/plans/feat-v0.1-versioned-plans` | HTTP plan approval and assignment write paths; exact hashes, current rights, separate acceptance, version/epoch checks in PostgreSQL | `docs/evidence/plans.md`: TDD sections 6–8, 10; tests, limitations, quality review |
| finance | `feat/v0.1-sandbox-payments` | `/tmp/undergrid-worktrees/finance/feat-v0.1-sandbox-payments` | Separate sandbox process/database/audience; authorization, idempotent payment effects; provider evidence explicitly distinguished from fixture evidence | `docs/evidence/sandbox.md`: TDD sections 3.2, 11; tests, external blockers, quality review |
| verifier | `feat/v0.1-json-verifier` | `/tmp/undergrid-worktrees/verifier/feat-v0.1-json-verifier` | Fixed verifier executable on bounded JSON; hashes, deterministic checks, untrusted-input/process tests; no network/database access by design | `docs/evidence/verifier.md`: TDD section 8.3; process contract, tests, isolation limitations |
| root | `feat/v0.1-swarm-integration` | `/tmp/undergrid-worktrees/root/feat-v0.1-swarm-integration` | Integration orchestration and CI qualification; actual PostgreSQL tests required, no mocked persistence fallback | This document and `docs/evidence/ci.md`: TDD section 12; checks and deployment limits |

## Merge and validation order

1. Review plans against current authority and durable lifecycle requirements; integrate only after tests and evidence reconcile.
2. Review verifier independently; subsequent artifact admission/publication is a separate dependent slice, not proven by a verifier helper.
3. Review sandbox foundation independently; real Stripe and Base Sepolia receipts remain mandatory before financial release designation.
4. Reconcile CI with integrated entrypoints, then rerun integration checks on the exact release state.

Each assignment is in progress. No N0–N6 or F0–F4 completion is implied. Further bounded assignments follow this wave. Full release still requires UI/protocols, artifact publication/reuse, source permissions, monitoring/operations, restore/security qualification and an unfamiliar independent cohort.

## Resource and evidence controls

The plans agent owns network-database testing during this wave. Finance uses a separate sandbox test database and role names. All agents share the installed Go toolchain/module/build caches; avoid simultaneous expensive builds while disk space is constrained. No test uses production credentials or labels test identities as unrelated beneficial owners.

Unresolved external gates: GitHub target, hosting/domain/OIDC provider, approved rights and source permissions, provider sandbox credentials/facilitator/wallet configuration, legal review and independent cohort. These block affected release work, not independent implementation. No production financial authorization has been granted.
