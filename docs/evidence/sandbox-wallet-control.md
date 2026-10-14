# Sandbox external-wallet control binding

Requirement slice: TDD r2 §§3.2 and 11.4. This feature adds owner-authenticated proof that an external Base Sepolia EOA can sign for a wallet address. It is a control proof only; it does not authorize spending, prove beneficial ownership, prove independent control, or verify a payment. The USDC/x402 provider and release gates remain open.

The financial sandbox exposes three owner-session routes:

- `POST /v1/owner/wallet-challenges` accepts one address and an `Idempotency-Key`. The response contains a server-generated SIWE-compatible EIP-191 message.
- `POST /v1/owner/wallet-challenges/{id}/prove` accepts a 65-byte recoverable EOA signature and an `Idempotency-Key`.
- `GET /v1/owner/wallet-binding` returns the current TEST-labeled address and binding epoch to the authenticated owner.

The financial origin is fixed from `UNDERGRID_SANDBOX_ORIGIN`. Handler construction rejects absent, non-HTTPS, or non-origin values; API startup already validates the same configuration when it creates the owner session manager. Mutating requests still pass through the production owner authenticator, which checks the session, request Origin, and CSRF token. Agent bearer tokens and network credentials cannot call these owner routes; wallet authority is not an agent scope.

The canonical message fixes the origin/domain, normalized checksum address, financial actor, binding epoch, chain ID 84532, random one-use nonce, issued/expiry times, request ID, and a statement that the proof does not authorize payment. The server recovers and compares an EOA address using EIP-191 personal-sign hashing, Keccak-256, and secp256k1. It does not resolve contract wallets (ERC-1271), call a chain RPC, fetch a balance, or store wallet keys. Challenge validity is ten minutes and each actor has at most one issued challenge. A newer challenge supersedes the previous issued challenge.

The owner actor row is locked inside the domain transaction. Challenge creation, supersession, successful proof consumption, immutable binding epoch advancement, and current-binding pointer update occur transactionally. Migration 008 adds one actor-scoped challenge/binding ledger relation. The ledger stores the message, request hashes, signature digest, and resulting epoch, but not the raw signature. Database checks, a transition trigger, unique indexes and a composite foreign key enforce one-use state, unique actor epochs, and the actor's current pointer. App transactions run under `undergrid_sandbox_app` and RLS; no worker grant or network database is involved.

Command retries with the same owner/key/body return the exact original challenge or binding response. Reusing a key with a changed body conflicts. A consumed challenge cannot bind again under a new key. Rebinding increments the epoch and retains prior bound rows. This feature does not change existing USD quote, refund, provider, or unknown-reservation behavior. Before a wallet payment rail ships, its payment quote/hash must snapshot the payer address and binding ID/epoch; later rebinding must not mutate an existing or unknown payment attempt or cause a new attempt while the original may settle.

## Behavioral evidence

The focused HTTP/PostgreSQL test first failed against the unmodified runtime because the owner challenge route returned `404 not found` (8.558s). After implementation, the wallet-control focused race selector passed on the fresh UTF-8 financial-only PostgreSQL fixture at port 55468 (3.017s). It covers valid proof, address checksum/malformed input, malformed and wrong-address signatures, altered signed content, wrong owner and agent denial, origin mismatch, challenge/proof exact retries, changed-body and replay rejection, expiration and supersession, concurrent duplicate proof (eight requests yield the same single binding), current-binding access, rebind history, and manual cross-actor RLS denial.

The fixture began with no `undergrid` network schema. The application ran its unmodified financial `Migrate` path, and the ledger contains versions 001–008 in order; migration 008 checksum is `0bc2856b99a064aa93ab9ba2e1064308dc4000747d0e1024b454f3cb10a997fa`. Database encoding is UTF8. Migration assertions used `nmcdc`; application transactions used `wallet_runner`, a LOGIN/NOSUPERUSER/NOBYPASSRLS role without create or replication rights. That login successfully entered the distinct `undergrid_sandbox_app` and `undergrid_sandbox_worker` roles via `SET LOCAL ROLE`; wallet commands themselves ran as the app role. The fixture also verified `to_regnamespace('undergrid') IS NULL`.

The full bounded sandbox and API race packages passed on this fixture: `go test -race ./internal/sandbox ./cmd/undergrid-sandbox -count=1 -timeout=10m` (25.489s and 1.080s). `go vet ./internal/sandbox ./cmd/undergrid-sandbox`, `go build ./internal/sandbox ./cmd/undergrid-sandbox`, `go mod tidy`, and `git diff --check` passed. Tests use locally generated trusted EOA fixture signatures; these results are component evidence, not external-wallet, provider, Base Sepolia transaction, or settlement evidence.

The financial schema now has eleven application relations: actors, credentials, payments, idempotency, provider events, effects, refunds, subscriptions, Stripe inbox, anomaly classifications, and wallet challenge/binding history. The TDD's original eight planned sandbox relations become nine with wallet control; the Stripe inbox and anomaly tables are additional runtime support relations. This update does not claim every financial feature or payment rail is qualified.

## Quality and limits

DRY and responsibility boundaries: wallet challenge generation, cryptographic verification, actor locking, and persistence live in the financial sandbox; the HTTP handler only authenticates the owner, decodes the typed command, and invokes those operations. The cryptographic implementation is a concrete EOA verifier rather than a generic wallet/provider framework. No network import, wallet custody, provider call, or payment authority is introduced.

Failure recovery is bounded by persistent issue/proof keys, one-use challenges, and transaction rollback. The first proof transition and actor pointer commit together; concurrent requests serialize on the actor lock. Prior epochs remain readable as audit evidence and unchanged by rebinding. The next USDC payment slice must persist its exact payer binding snapshot on each quote/payment and keep that snapshot through unknown reconciliation; that behavior is not implemented here.

External OIDC deployment, wallet UI/client compatibility, contract-wallet verification, Circle token/provider configuration, actual Base Sepolia transactions, x402 facilitator qualification, canonical receipt/finality/reorg handling, and payment/refund behavior remain release gates. No wallet key, provider credential, testnet RPC or faucet was used.
