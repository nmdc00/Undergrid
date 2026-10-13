# F1 sandbox authority slice

Requirement: TDD r2 §§2.2, 11.1–11.2 and threat-model network/sandbox and sandbox/provider boundaries. This branch implements the authority/reservation substrate for one fixed USD Stripe TEST product. It does not complete the finance release gate.

## Observable behavior

The separate `cmd/undergrid-sandbox` API/migrate process imports no network domain. Its configuration uses only `UNDERGRID_SANDBOX_*` names. API startup rejects superusers, bypass-RLS identities, table owners, combined app/worker logins, and databases containing the network schema. Migration refuses the network database. Runtime deployment must provision a separate host, database, login, HTTPS origin, session secret and OIDC client/audience; provisioning and production deployment remain pending. The shared identity package is a reviewed OIDC/session security utility, not network storage or credentials.

An interactive owner enrolls an actor, sets a versioned USD/UTC-day cap and exact product/payee allowlist, and issues/revokes scoped `ugs_` credentials with the `undergrid-sandbox` audience. Agent HTTP quotes select only the configured product; amount/payee/terms cannot be supplied by an agent. Domain transactions rederive credential identity, apply RLS, serialize actor policy and enforce scopes. Exact actor/credential/nonce-bound quote hashes are required for authorization. Authorization and quote retries retain their original payment; changed payloads conflict. Catalog-derived expiry remains the original expiry on response-loss retry.

Reservations count amount plus fee across authorized/submitting/pending/unknown/settled payments in the UTC period. Concurrent 600-unit purchases against a 1000-unit cap admit exactly one. Unknown outcomes retain the reservation and stable provider key. Worker-domain dispatch claims recheck policy version, credential revocation, quote expiry and period before committing submitting; no external provider call exists. Valid late monetary facts remain recordable after authority changes. Event identity is bound to payment, economic fact, kind, asset, amount and evidence; economic-fact identity dedupes distinct events. Refund facts cannot exceed verified charge facts and never replenish the gross cap.

## Evidence and checks

Real PostgreSQL fixture: `/tmp/undergrid-pg/socket`, port 55439, database `undergrid_sandbox_f1`; network databases were not touched. Migration/assertion connection uses fixture administrator `nmcdc`. Domain tests use distinct NOSUPERUSER/NOBYPASSRLS login `undergrid_sandbox_f1_runner`, with app and worker memberships solely to exercise both domain seams. Transactions set the exact restricted role. The actual API rejects this combined-role test login; deployment needs an app-only login and a separately provisioned worker identity.

Behavioral tests cover HTTP owner enrollment/policy/credential/quote/authorization, server-selected prices, strict duplicate/oversized JSON rejection, network-token rejection, exact retries, concurrent cap admission, cross-principal access, unset-actor RLS, runtime role privilege checks, revocation before dispatch, unknown retention, economic replay, partial refund and over-refund rejection. Startup tests prove network configuration is never substituted and privileged/combined logins are rejected.

Two observed red/green corrections during resumed work:

- Same provider event/evidence with changed refund kind/amount was accepted; regression failed, event economic fields were persisted and compared, and regression passed.
- Retrying HTTP quote generation recomputed expiry and returned 409; regression failed, catalog request identity was separated from the generated expiry, and the original quote now replays.

Commands (set `GOCACHE=/tmp/undergrid-gocache`, `GOMODCACHE=/tmp/undergrid-gomod` and the two fixture URLs described above): `go test ./internal/sandbox ./cmd/undergrid-sandbox -count=1`; `go vet ./...`; `go test -race ./internal/sandbox ./cmd/undergrid-sandbox ./internal/identity -count=1` (all pass, shared OIDC fixture included). Full unconfigured `go test ./...` correctly fails network tests requiring their real database and sandboxed OIDC sockets; the root integration gate must run with a dedicated configured network fixture.

## Quality review and remaining gates

DRY: HTTP and worker paths call transaction-owning domain commands; catalog retry shares quote validation and persistence with direct quotes. Network and sandbox persistence remain separate despite superficial similarity. Contracts: actor-first locking makes authorization/revocation/cap checks serial; immutable quote fields, append-only effects privileges, numeric(78,0) values and economic-event binding remain explicit. Failure recovery: unknown attempts keep reservations, repeat dispatch conflicts, late monetary truth is retained and refunds do not overwrite charges. No editable balances or revenue claims exist.

Important boundaries: the six currently used sandbox tables are a bounded subset of the TDD's eight; independent refund request/authorization state and subscription consent tables are not installed empty. Only charge/refund fixture facts are supported; fee/reorg qualification and signed receipts remain unimplemented. VerifiedEffect is an internal worker-domain input, not HTTP ingestion; passing fixtures are not Stripe verification. Provider events currently reconcile synchronously, not a separately scheduled durable inbox processor. Authorized rows are durable work candidates, but no queue/dispatch loop is shipped. Unexpected settlement is retained; dedicated anomaly flagging/cancellation after dispatch is still pending.

Actual Stripe Checkout/raw-body webhook signature/object verification/idempotency-window handling, provider failure/reorder/refund exercises, bounded Billing/test clocks, USDC Base Sepolia/x402 wallet proof/finality/reorgs, and provider account/test-product approvals remain blocked/unimplemented. OIDC fixture tests verify the shared utility; a deployed sandbox owner login and its separate client/audience are not qualified. No financial provider credentials, network credentials, deployment identities or legal terms are created here. A TEST string is a designation, not a verified settlement receipt. No production deployment or provider verification is claimed.

## Integration and CI qualification follow-up

The feature merged `release/v0.1` without rewriting history. CI now migrates its isolated sandbox database before tests, provisions a disposable NO SUPERUSER/CREATEDB/CREATEROLE/REPLICATION/BYPASSRLS login with sandbox app+worker memberships, and sets `UNDERGRID_SANDBOX_TEST_RUNTIME_DATABASE_URL`. CI missing database/runtime fixtures causes failures, not skips or administrator fallback. Runtime-login privilege metadata is asserted explicitly; the API still rejects the combined-role test runner.

Role metadata is checked after migration, at API startup, and before every transaction's SET ROLE. Preexisting sandbox groups must be NOLOGIN and lack superuser, RLS bypass, role/database creation, replication and network-app/worker membership. Migration fails closed and does not rewrite role permissions. Catalog tests use an actual unsafe login and missing role; shared PostgreSQL role attributes were never altered to manufacture the test.

The final configured full race suite uses dedicated database `undergrid_network_finance` plus `undergrid_sandbox_f1` and the separate sandbox runtime login; network fixture schema/migration markers were copied read-only from the finished quotas fixture. No plans/integration/network-quotas database was written. `CI=true go test -race ./... -count=1` passes all command, identity, network, sandbox and verifier packages. `go vet ./...`, `go build ./cmd/...`, and `go mod verify` pass. All six reference listener recovery tests also pass. The separate `.env.sandbox.example` documents deployment identity boundaries without supplying credentials.

Billing remains blocked: no provider account/configuration qualification or test-clock evidence enforces reservation before each of at most two test renewals. No subscription, unbounded debit, production entitlement or provider verification is claimed.
