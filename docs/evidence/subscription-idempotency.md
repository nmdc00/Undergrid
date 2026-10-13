# Owner subscription command recovery

Implements TDD §10 command idempotency for the §11.3 bounded recurring-consent write path. `POST /v1/owner/subscriptions` requires an `Idempotency-Key` of 1–128 bytes. The authenticated financial owner, operation and key identify a command; the canonical request hash includes the exact consent and server-selected immutable offer. A changed request conflicts. Retries recover the original subscription reference and current projection, preserving cancellation rather than creating fresh authority.

Red: two HTTP/PostgreSQL behavioral tests failed before the change (9.165s): identical requests created different subscription IDs/hashes, and an absent key created consent with HTTP 201. Green: all subscription race tests passed (6.359s). After adding restart, normalized-result-reference and cancellation assertions, the entire financial package and sandbox executable race suites passed (15.586s and 1.082s). `go vet ./...` and `go build ./...` passed. Fixture database: separate `undergrid_sandbox_subscription_retry` on port 55440, using a restricted financial application/worker test login. No network database or credentials were reused.

Migration 007 extends the existing financial idempotency table with a subscription result reference and distinct owner/credential uniqueness indexes. Existing payment/refund result constraints are retained in expanded form; exactly one correctly typed result is required. The owner context takes the actor lock before authorization and lookup. Consent and result reference commit together. Tests independently count two subscriptions for two distinct keys and exactly two normalized owner result rows, assert concurrent identical results, reject changed content and revoked credentials, and recreate the service before replay. No bearer token or response payload is stored.

Pragmatic quality review:

- DRY: the domain command owns consent creation/replay; HTTP only supplies the key. Existing actor authorization and locking remain shared. Network and financial identity stores remain separate.
- Orthogonality: provider collection and renewal preparation are unchanged; owner retries cannot create additional provider objects or recurring consent. The schema extends the current finite command-result table rather than adding a general command engine.
- Contracts and effects: current owner, policy and credential authority are checked before replay. The request hash binds normalized UTC expiry and the fixed offer. Failure rolls back both records; database uniqueness also guards multi-process retries.
- Readability and recovery: owner identity is distinguished from agent credential identity. Recovered results expose current state, so a canceled subscription remains canceled. PostgreSQL persistence, not process memory, supplies restart recovery.

Implemented and locally proven, pending PR review and release integration. Actual Stripe controlled invoice finalization/collection and test-clock qualification remain unfinished. Other financial owner commands still need their own credential issuance/policy retry contract; this feature does not claim those paths are implemented.
