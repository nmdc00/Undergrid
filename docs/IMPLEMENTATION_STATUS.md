# Implementation evidence

Owner authorized the complete r2 implementation on 2026-10-12. This record reports observed results, not design aspirations.

## Active slice

`feat/v0.1-durable-participation`: an authenticated agent can persist an intention; retries cannot duplicate writes; private state cannot cross principal boundaries; revoked credentials cannot perform subsequent actions. Test through HTTP with real PostgreSQL and nonowner roles. N1 is not complete until self-service OIDC/UI qualification also passes.

## Milestone gates

| Gate | Status |
|---|---|
| N0 | In progress; product/policy, permitted sources, identity provider and hosting choices requested |
| N1–N6 | Not complete |
| F0–F4 | Not complete; provider credentials and wallet/facilitator configuration requested |

## Environment evidence

Go 1.27.2 official archive verified against SHA-256 `ecbadb99091a3f46e31f5f934b068b1864eafa7995211b39eaddf76996045fe5`. Local PostgreSQL 16.11 started for isolated integration tests. Docker is unavailable. Toolchain installation encountered a full 24 GB filesystem; disposable cache cleanup was required.

The six existing reference-listener checks passed before application implementation. They use fake transport and do not prove service acceptance. No public deployment, external cohort, Stripe test receipt or USDC testnet receipt exists yet.

## Durable participation slice evidence

Failing test observed before implementation: undefined service/migration contracts. Then `TestDurableParticipation` passed against PostgreSQL 16, including HTTP handlers, persisted revision/event counts, retry conflicts, private access, stale writes, new-handler recovery and credential revocation. `TestDatabaseIsolationAndAuditImmutability` proves a non-superuser/non-bypass application role sees no private rows without actor context and cannot update audit events. A separate failing-then-passing budget test rejects live commitment claims.

Quality assessment: actor authority is derived once and locked in principal → agent → credential order; transaction-local RLS state cannot leak between connections. Immutable revisions and append-only events separate current state from evidence. Budgets carry no payment capability. Current limitations: connections are per request until a tested pool is introduced; owner authentication is an injected boundary and the test provider is a fixture, not external OIDC proof; intention editing, expiry processing and public projection reads are still outstanding. No claim of complete N1 or public readiness.

## Resumable inbox slice evidence

`TestInboxReceiptAndRecovery` failed before cursor/inbox implementation, then passed against PostgreSQL with the participation/security regressions. It proves actor-bound signed cursors, duplicate receipt acknowledgment, unchanged business state after acknowledgment, empty incremental replay, canonical watch recovery and HTTP 410 on expired cursors. Long polling releases the transaction between polls and reauthenticates each poll. Pending swarm obligations are not implemented yet. Inbox retention and 100-listener load qualification are still open. `go vet ./...` and `git diff --check` passed.

Local validated branches are integrated into `release/v0.1` to continue dependent implementation. The owner selected nmdc00/Undergrid on 2026-10-13. Implementation is published on release/v0.1 with draft release PR #2 into develop; feature review and develop/main promotion remain pending. Branch publication is not release approval.

## Interactive identity slice evidence

The Go API binary builds. Owner login uses OIDC authorization code plus PKCE, nonce, issuer/audience/signature checks, verified email and recent authentication. Encrypted host-only Secure/HttpOnly cookies expire after 15 minutes; mutating owner requests require the configured Origin and CSRF token. API startup rejects database owners/superusers/RLS-bypass logins. Migrations are a separate command and credential. Agent ID-token authentication is not exposed.

Local TLS cryptographic fixture tests pass for a valid flow and wrong nonce/audience/unverified email. Session tests pass for tamper/expiry/CSRF/origin rejection. `TestOwnerCanReviewAndRevokeOnlyOwnCredentials` first failed with HTTP 405, then passed; token issuance/revocation writes immutable credential events and lists expose neither tokens nor digests. Code review moved owner operations out of the transport file and centralized credential audit facts. All current tests and `go vet` pass. The fixture is not an external provider test. Credential rotation UI, owner policy editing, provider recovery and load/security qualification remain open.

## Exact shared-demand slice evidence

PostgreSQL tests now prove an initially empty result, matching of disclosed public demand across distinct test principals, private budget/objective redaction, hard-license rejection, unknown-license clarification, deduplicated opportunities and a committed River job producing a real opportunity. The worker uses a separate non-bypass role, transactional queue insertion and a 15-minute repair schedule. Application migrations and River 0.49.0 migrations commit separately because PostgreSQL enum additions cannot be used inside the same migration transaction; the startup readiness marker follows both. A regression caught public metadata readable with no actor context; migration 003 binds those reads to an authenticated actor.

All current tests and static checks pass. This is exact matching only: differing descriptions explicitly report `semantic_comparison_pending`; pgvector/semantic retrieval and permitted external sources remain unimplemented. Matching tests prove separate account identities, not unrelated human ownership. The 1,000-intention sweep bound and worker throughput still require load qualification; no N2 completion claim.

## Swarm participation slice evidence

Real PostgreSQL HTTP tests pass for restricted/private room creation, opted-in invitations, exact-version admission, denial before admission, durable message replay, stranger denial, withdrawal denial and invitation recovery from canonical context. Agent text remains data. The observer-write test first demonstrated HTTP 201 and now receives 403; the database also requires contributor membership. Tightening the lock policy exposed an incorrectly qualified SQL correlation; forward migration 006 fixes it without rewriting an applied migration.

Room events enqueue fanout transactionally; the worker acquires only one recipient's authority locks per delivery, avoiding cross-principal locks inside a sender's command. Inbox reads recheck membership and advance past inaccessible events. Shared idempotency logic replaces duplicated retry rules while retaining the existing intention request-hash format. Static checks and current regressions pass. Remaining N3 work includes recruitment, planning, assignment/deadlock/timeout handling, concurrency stress and public-client qualification. River may emit a context-canceled maintenance log during intentional test shutdown; no job loss assertion has been waived.

## Builder recruitment slice evidence

`TestBuilderDiscoveryApplicationAndAdmission` failed before implementation, then passed with PostgreSQL: opted-in public capability profiles receive typed builder opportunities; an application conveys no history access; a coordinator's offer increments the membership version; only explicit acceptance admits the builder. New public profiles and room roles enqueue recruitment work, with periodic repair. Evidence labels capabilities `self_declared`, separate from registered buyer demand. Existing discovery, identity and swarm regressions and static checks pass. Role/profile scanning remains bounded to the initial qualification workload; pagination/load qualification and complementary capability search beyond recruitment remain open.

Permission review corrected an early broad `swarms:write` scope to the TDD's separate `swarms:create`, `swarms:apply`, `members:invite` and `members:respond` scopes, plus explicit withdrawal authority. `TestSwarmCreationDoesNotGrantInvitationAuthority` now proves a creator-only credential cannot invite another agent. Existing lifecycle fixtures request the corresponding explicit permissions; no blanket compatibility alias grants extra authority.

## External gates

No public domain/host, OIDC provider, approved source permissions, legally approved contribution templates, privacy terms, Stripe credentials, wallet authorization or x402 facilitator has been supplied. Continue code that does not depend on these choices; do not silently enable unapproved policies.

## Full implementation checkpoint — 2026-10-14

Release450c833 includes reviewed progress/evidence, owner issuance recovery, report intake, intention revisions and restricted sharing, correlated errors/ETags, configured OAuth resource credentials, fail-closed verifier isolation implementation, wallet control and signed USDC quotes. See FULL_IMPLEMENTATION.md and feature evidence for actual runtime gates. Current combined all-package race/coverage is running; no current full-coverage result is asserted. Canonical REST, MCP and exact external payer authorization are in active isolated branches. Hard-cgroup/provider/legal/source/production storage/deployment qualification and remaining TDD code stay incomplete. Main/develop have not received this full implementation wave.
