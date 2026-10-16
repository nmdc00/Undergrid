# Owner self-service console — ID-01/ID-03, INT-01/INT-04, TDD §§4/9

Branch: `feat/v0.1-owner-console`, based on `release/v0.1` e17ae5a.
This evidence covers the owner-console behavior only, not complete v0.1 UI or
external signup/provider/legal qualification.

## Result and actual runtime

The React/TypeScript production bundle builds to the already served `web/dist`
route in `cmd/undergrid/main.go`. Real API operation uses the application's
configured HTTPS origin and OIDC provider, encrypted Secure/HttpOnly owner
cookies, `/auth/session` CSRF bootstrap and `/auth/logout`. No owner identity is
derived from supplied principal IDs. Browser API requests use same-origin cookies,
no stored agent bearer token and no redirect following.

After verified login, unregistered owners see the configured terms version and
an explicit unchecked acceptance control. Registered owners can list/register
agents, view scoped credential IDs, issue one one-time token and revoke only
own credentials. The UI exposes typed enabled-template/license/category/workload/
UTC-expiry policy controls with reviewed `If-Match`; it cannot grant authority.
Owners create private source-catalog JSON intentions attributed to an own active
agent, browse own documents/current opportunity evidence and pause/resume/withdraw
using existing versioned commands. Intention list is bounded to 100 most recent
records. Empty results are described as a gap rather than inferred demand.

New owner routes and browser test setup are documented in `web/README.md`.
No migrations, unused tables, provider credentials, sandbox identity reuse or
legal/provider defaults were introduced. Registration refers to the operator's
configured approved terms; actual text/link/operator qualification remains external.

## Red → green evidence

- `TestOwnerConsoleRegistrationAndIntentionIsolation` first failed through HTTP
  with `404 not_found` for the missing owner configuration endpoint. Added owner
  configuration/me/intention routes, then green against fresh UTF8 PostgreSQL
  `/tmp/undergrid-pg-frontend` port 55444. Cross-owner private reads/opportunity
  reads/selected agents return 404, stale edits 409. Replayed creation returns the
  same ID and independently queried durable event count remains one.
- Opportunity browsing extension first failed 404, then calls the existing
  evidence query only after own intention and active agent checks.
- Method-contract review added a failing test: POST to an intention detail route
  returned 201 and created a second intention. Explicit collection/detail method
  guards now reject it with 405 before mutation.
- Four executable React DOM behavioral tests initially failed on an empty App;
  configured terms/CSRF, OIDC sign-in link, browsing/logout and token dismissal/no
  persistence became green. A fifth test failed on missing typed policy fields,
  then proved the explicit bounded policy document plus its reviewed ETag.
- `api.test.ts` first observed different idempotency keys after response loss.
  Uncertain same-session intention retry now retains its key; creation expiry
  stays stable until success. Six frontend behavioral tests pass in total.

## Final checks actually run

- `go test -race ./internal/network ./internal/identity -count=1`: passed, real
  isolated PostgreSQL, all network and identity tests, after final domain changes.
- `TestBrowserOwnerDomainCSRFAndIsolation`: passed again through a freshly created
  distinct `LOGIN NOSUPERUSER NOBYPASSRLS` role with `undergrid_app` membership,
  encrypted owner cookies, real owner domain transaction/RLS and durable revocation.
  Wrong origin, wrong/missing CSRF return 401; foreign owner cannot list/revoke the
  other's credential. The fixture role is dropped during cleanup. This is a local
  cryptographic session fixture, not external OIDC-provider verification.
- `npm test`: six tests pass. `npm run build`: strict TypeScript checking and
  Vite 7.3.7 production bundle pass.
- `npm run test:e2e`: two Chromium tests pass on the final production bundle.
  They prove live browser execution, untrusted HTML remains text, no injected image,
  same-session CSRF request headers, one-time token/no browser storage/dismissal,
  logout and unauthenticated OIDC link. HTTP API responses here are explicit wire
  fixtures; real cookie/domain/RLS proof is the Go test above. No claim of full
  browser→selected-provider→production-host qualification is made.
- `go vet` for touched network/identity packages, formatting and `git diff --check`:
  passed. npm registry audit of the final locked dependency tree: zero known
  vulnerabilities. Initial Vite/Vitest/esbuild advisories were corrected to
  Vite 7.3.7, Vitest 4.1.11 and esbuild 0.28.1.

Chromium initially could not start because host NSS/audio libraries were missing.
Downloaded distribution packages were extracted into `/tmp/undergrid-browser-libs`
and used through test-only `LD_LIBRARY_PATH`; no system install or repository
binary was added. Generated build/browser results and local instruction files
remain ignored.

## Quality and recovery assessment

DRY: browser intentions call the existing `mutateIntent`/`getIntent`/
`listOpportunities` domain functions inside the owner transaction, retaining
quota, validation, immutable revision, version, idempotency, events/inbox and
transactional enqueue behavior. The browser cannot duplicate those policies.
Orthogonality: static UI, interactive session identity, network authority and
financial sandbox remain separate. Contracts: own active agent attribution and
principal lock/context precede commands; object versions stay server-enforced.
Effects/resources: rows/connections/transactions close, no agent content executes,
responses render as text, token and CSRF stay in memory. Readability: typed policy
controls were extracted into their own component; all frontend source is formatted.
Recovery: stale writes ask the user to refresh/review; intention retries retain
an uncertain key in memory and cannot silently repeat a committed command.

Concrete limitation: agent creation predates this slice and is not server-
idempotent. An issuance response loss requires refreshing/reviewing/revoking an
uncollected credential before reissuing; the UI does not silently auto-retry it.
Browser reload loses uncertain-key memory, so owner must review server state
before creating another intention. These limitations are documented to avoid
false exactly-once claims. UI is the fixed private JSON/source-catalog vertical,
not a generalized intention designer.

## Remaining dependent work and gates

Agent control proof/rotation, full typed intention revision/share/watch changes,
collaboration/invitation/plan/task/artifact screens and separate financial sandbox
UI remain next scoped branches in FULL_IMPLEMENTATION.md. Existing state pause
turns off watching and resume does not itself re-enable it; this UI reports the
actual watch document and does not claim watch reconfiguration. External OIDC
selection/signup, actual approved legal texts, production hosting and unfamiliar
owner end-to-end qualification remain explicit gates. Main/deploy/release status
is determined by the root's review/integration flow, not this feature evidence.
