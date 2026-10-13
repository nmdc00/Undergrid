# Versioned plans and explicit assignments

Scope: TDD r2 sections 6, 7.2, 8.1 and 10; partial N3, not completion of the network milestone. Local branch `feat/v0.1-versioned-plans` follows the preserved incomplete draft commit. Repository selection is now authorized; root coordinates publishing and promotion. This branch has no remote PR or deployment yet.

The HTTP domain commands now persist immutable typed plan specifications, exact canonical JSON hashes, parent revisions, dependency DAGs, separate principal plan/rights records and explicit agent assignment acceptance. All required current principal policies and all assignments must agree before a plan becomes the accepted swarm head. Concurrent approval requests acquire required principal locks in deterministic order before agent credentials, then the swarm and plan. A new accepted head supersedes the prior plan and invalidates unfinished old claims. Agent messages grant none of these permissions.

`POST /v1/owner/policy` uses owner authentication and ETag to establish bounded coordination authority. No contribution template is enabled by default. Operators can configure reviewed templates through `UNDERGRID_CONTRIBUTION_TEMPLATES_FILE`; `test_only` templates are rejected in production. Principal policy limits constrain rights templates, licenses, task category, expiry and aggregate active task count. The tests use explicitly identified fixture terms, not legal approval.

`POST /v1/swarms/{id}/plans`, `GET /v1/plans/{id}`, `POST /v1/plans/{id}/approvals`, `GET /v1/tasks/{id}`, assignment-response and progress commands share the authenticated network transaction. Progress checks assignee, ETag, claim epoch, current plan, dependencies and authority. `/v1/agent-context` recovers current assignments independently of notification retention. Withdrawal increments current claim epochs before revoking room membership; later private reads and approval replay cannot disclose the plan.

Migrations: 008 introduces plans/tasks/approvals and narrowly scoped SQL authority helpers; 009 adds the explicit withdrawn task state without editing the already-applied migration. There are no financial tables or imports.

## Test evidence

Initial draft build failed on missing task handlers and missing owner time import. After implementing handlers, `TestExactPlanApprovalAndIndependentAssignmentAcceptance` passed against actual PostgreSQL. The added withdrawal regression failed because a departed contributor's task remained `in_progress` with epoch 1; it now reports `withdrawn` with epoch 2. Context recovery initially failed because `assignments` was missing; the focused regression now passes. The lifecycle test also checks concurrent principal approvals, wrong hash, votes without accepted work, exact replay, stale ETag, canonical hash and private replay denial. Template production rejection and invalid dependency graph tests cover the configuration and immutable contract boundaries.

Commands use Go 1.27.2 and the dedicated local PostgreSQL test database with request transactions switching to the nonowner application role. This is HTTP-handler/database integration evidence, not independently operated external agents or legal approval. Full-suite result is recorded after final verification below.

## Pragmatic quality assessment

- Contracts: found and corrected withdrawal retaining a live claim and stored private-response replay bypassing current membership. Public context carries current task versions and epochs.
- DRY: template/policy checks have one domain owner; API commands reuse them. Database constraints independently enforce states and uniqueness. No generic workflow engine was added.
- Orthogonality: coordination does not import payment/provider concerns; runtime template configuration is separate from agent-authored content. REST routing remains thin while command functions own domain effects.
- Effects: state, approvals, audit records and fanout jobs share the existing request transaction. Task reads use RLS, and query rows close before further commands.
- Remaining limitations: timeout/reminder sweeps, cure/abandonment, coordinator succession/archive, explicit task replacement, and artifact acceptance/publication remain later slices. Dependency blocking is enforced when starting work; an explicit downstream blocked state is not introduced here. Source/license legality remains an external gate. No claim of full N3 or public release readiness.

Final resumed verification (2026-10-13): `go test ./... -count=1` passed, including the real PostgreSQL network suite (9.097s) and identity suite; `go vet ./...` and `git diff --check` passed. The HTTP acceptance lifecycle now independently asserts normalized accepted plan state/hash, swarm head, assignee identity, two separate plan approvals, two separate rights approvals and exactly one `plan.accepted` event. No behavior changed in this resumed review, so no new red/green claim is made.

The dedicated network database is `undergrid_network_plans`, reached through `/tmp/undergrid-pg/socket`, port 55439, local test operator `nmcdc`. Service requests switch to the existing nonowner `undergrid_app` role. No sandbox database or credential was used. Initial attempts failed TCP authentication and then existing cluster-global role creation. The clean fixture database was bootstrapped with network schema only and migration metadata from the preserved network test database; participant/event/job data was not copied. This proves request lifecycle effects on an empty fixture dataset, not fresh-cluster migration qualification. Fresh database migration in a cluster that already owns the fixed application roles remains a separate bootstrap limitation.

Current readiness: the partial versioned-plan slice is locally validated and ready for integration review. Deadline sweeps, downstream blocked-state materialization, replacement/recovery and external cohort/legal approval remain explicit follow-ups or release gates. No deployment or complete public-v0.1 readiness is asserted.

## Release integration and template restart regression

Current `release/v0.1` was merged into the feature branch with an ordinary merge, retaining verifier, CI, README and request-boundary checks. PR #4 is the feature review; root owns remote publication and promotions.

Important finding — `tasks.go`: loading new template wording under an existing template ID after restart previously allowed progress using an accepted plan sealed against different wording. The HTTP/PostgreSQL restart regression returned 200 before the fix. Both assignment and progress now compare the configured template digest with the immutable sealed digest before mutation and deny mismatches with `409 plan_terms_expired_or_changed`. The regression checks that denial preserves the task version and progress deadline. Template/license checks remain separate from legal review; fixture terms are not approved wording.

Integration checks: `go test -race -count=1 ./...`, `go vet ./...`, `go build ./...` and `git diff --check` pass against the dedicated network PostgreSQL fixture database. This includes verifier and request-boundary tests inherited from release. Hosted GitHub Actions validation remains blocked by repository/account billing; these local results do not claim hosted CI success. Root must resolve or accurately retain that promotion gate.
