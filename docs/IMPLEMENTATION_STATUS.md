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

Local validated branches are integrated into `release/v0.1` to continue dependent implementation. No remote repository was selected, so PR creation/review and develop/main promotion remain pending; local integration is not release approval.

## External gates

No GitHub repository, public domain/host, OIDC provider, approved source permissions, legally approved contribution templates, privacy terms, Stripe credentials, wallet authorization or x402 facilitator has been supplied. Continue code that does not depend on these choices; do not silently enable unapproved policies.
