# Resumed wave integration proof — 2026-10-13

Candidate: `reconcile/v0.1-wave1` at `9816ec99e3c5b1c6b3b2282a408111175b9ebe80`; tree `aea6267824e22524351937491c32987ac3c1423c`. Published review branch combines plans `a7425a9`, sandbox `2090513`, and quotas `fd7c6ef` with release base `413e4c0`. This is a local qualification candidate, not the protected release or promotion SHA.

## Contract and proof

| Invariant | Actual entrypoint | Named proof | Status |
|---|---|---|---|
| Exact approved plan plus separate assignment acceptance determines accepted head; changed template digest denies later task work | HTTP plan approvals, assignment-response and progress through domain transactions | `TestExactPlanApprovalAndIndependentAssignmentAcceptance` checks normalized plan/head/assignee/approval/event rows, private replay, epochs and restart with changed terms | Proven locally; partial N3 |
| Principal admissions cannot exceed durable resource limits through sibling-agent concurrent commands | Owner agent create; intention create/reactivate; swarm create | `TestPrincipalAgentAdmissionQuota`, `TestPrincipalIntentAdmissionConcurrencyAndReplay`, `TestPrincipalSwarmAdmissionAcrossAgents`, `TestPrincipalIntentQuotaPauseAndReactivation` | Proven locally; rates/storage/fairness deferred |
| Sandbox quote and authorization derive authority from its own credential/audience and server-selected product | Separate sandbox HTTP handler and transaction-owning Store | `TestSandboxHTTPEnrollmentToAuthorization`, `TestSandboxHTTPStrictCommands` | Proven locally; external login/provider gates open |
| Unknown provider outcomes retain reservations and cannot cause a new dispatch; economic facts and events have separate immutable identity | Store authorization, BeginDispatch, MarkUnknown and RecordEffect worker seams | `TestAuthorityReservationsAndReconciliation`, `TestConcurrentCapAndCrossPrincipalIsolation` | Proven with fixture facts; actual providers unqualified |
| Sandbox roles/credentials cannot use privileged runtime paths or substitute network configuration | Sandbox command startup, migration and transaction role validation | `TestSandboxRequiresDedicatedConfiguration`, `TestSandboxRejectsPrivilegedAndCombinedRuntimeLogin`, `TestSandboxRoleCatalogSafety` | Proven locally; deployment identities not provisioned |
| Original plus candidate migrations operate on empty databases without copied schema/fixture data | Network Migrate and actual sandbox migrate command | Fresh PostgreSQL cluster and configured full suite | Proven locally; hosted runner not exercised |

## Exact checks

A new disposable PostgreSQL 16 cluster was initialized under `/tmp/undergrid-integration-pg`, socket `/tmp/undergrid-integration-pg/socket`, port 55440. `undergrid_network_fresh` and `undergrid_sandbox_fresh` are distinct UTF-8 databases. Network migrations ran from scratch with no prior roles/schema/participant data. The real `cmd/undergrid-sandbox -mode=migrate` initialized only the financial database. A separate NOSUPERUSER/NOBYPASSRLS/NOCREATEROLE/NOCREATEDB/NOREPLICATION sandbox test runner received app and worker memberships solely for tests; actual API startup rejects that combined membership. Deployment requires independent app-only and worker identities.

On the exact candidate, `CI=true go test -race -count=1 -timeout=10m ./...` passed with network migration/test admin configuration and both sandbox admin/runtime fixture URLs. Network package: 31.716s; sandbox: 7.178s. Command startup, real executable verifier and cryptographic OIDC fixture packages also passed. `go vet ./...`, `go build ./cmd/...`, `go mod verify`, tracked Go formatting and `git diff --check` passed. Six reference-listener tests passed separately (13.386s); those are fake-transport reference evidence.

Earlier copied-schema databases used by individual task agents were not the source of this fresh-cluster proof. Their setup limitation remains stated in each feature's evidence. The earlier combined plans/quota candidate also passed from scratch before adding sandbox code.

## Release and production limits

GitHub annotation on run [38386613425](https://github.com/nmdc00/Undergrid/actions/runs/38386613425) says the job did not start because the account is locked due to billing. Required `go-postgres` checks remain on release/develop/main; no administrative bypass or invented passing status was used. Feature PRs #4/#6/#5 and evidence PR #3 remain open; release PR #2 remains draft. Main and develop retain the identical documentation bootstrap.

No real provider receipt, sandbox dispatch loop, approved rights/source material, container isolation qualification, external cohort, restore/security qualification or public deployment is established. Full N0–N6/F0–F4 gates remain incomplete. After feature merges, exact release validation must run again on the resulting SHA; this candidate cannot stand in for promotion-SHA or post-deploy proof.

Post-deploy checks must be executed only after approved Undergrid deployment configuration exists: read canonical task context and inspect normalized plan/head/hash/approval/claim-epoch records; compare quota admission rejection with persisted principal counts; verify a sandbox unknown attempt retains its provider key and gross reservation; inspect immutable effect identities against actual verified provider receipts; verify runtime role catalog attributes and isolation. Health checks alone are insufficient. These checks are specified here and have not been executed in production.

## Session retrospective

Learned: a broad green plan suite missed template changes across restart; cross-review produced a failing real lifecycle regression and digest enforcement. A schema-cloned database did not prove first migration safety, so a fresh cluster supplied independent proof. Hosted CI can fail before any test executes due to account state.

Institutionalize: no new skill is necessary; existing Undergrid, lifecycle and release-contract skills cover these patterns. Keep isolated branches and document fixture/provider/deployment evidence separately.

Amend: a future sprint-skill update should preflight hosted runner/account availability before treating published CI as executed qualification. No installed skills were edited during this session.

Testing: retain restart/changed-template and unsafe-role catalog tests. Missing sandbox runtime fixture configuration now fails in CI instead of skipping or falling back to admin. Full configured lifecycle checks are required after integration.

Automation: the financial feature's workflow now migrates the separate sandbox, provisions a disposable restricted runtime login, and supplies its DSN. A repeatable fresh-cluster local bootstrap script is a useful follow-up; copying migration metadata must remain explicitly weaker evidence, not a migration gate substitute.
