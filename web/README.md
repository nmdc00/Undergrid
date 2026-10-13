# Owner console

The React/TypeScript client uses the network application's same-origin OIDC
session routes. Build before starting the API from the repository root:

```sh
cd web
npm ci --ignore-scripts
npm test
npm run build
cd ..
# Start the Go API with its configured restricted network DB role and OIDC provider.
```

`cmd/undergrid` already serves `web/dist` at `/`, `/auth/login`, `/auth/callback`,
`/auth/session`, `/auth/logout` and the network `/v1/` API. The UI uses HTTPS in
real operation because owner cookies are Secure and session configuration requires
an HTTPS origin. Vite's development/preview server is a local UI testing tool;
it does not replace the Go API or configured external identity provider.

Owner endpoints added by this slice:

- `GET /v1/owner/config`: current configured terms version and enabled contribution templates; verified owner session required, registration not yet required.
- `GET /v1/owner/me`: registered active owner ID.
- `GET /v1/owner/intents`: up to 100 own intentions, newest first.
- `POST /v1/owner/intents`: `{agent_id, intent}`; selects only an own active agent and calls the existing intention command.
- `GET /v1/owner/intents/{id}` and `GET .../{id}/opportunities`: own intention and current evidence.
- `PATCH /v1/owner/intents/{id}`: existing state changes, `If-Match` and `Idempotency-Key` required.

The client additionally uses existing owner principal registration, agent
issuance/list/revoke and versioned policy routes. It never issues an agent token
for browser browsing. A newly registered agent's token is displayed once in
memory, dismissible, and never saved to browser storage. If issuance loses its
response, refresh the credential list and revoke any uncollected credential
before reissuing. Intention requests retain an uncertain command key and expiry
in memory for a same-session retry; after a browser reload review server state
before creating another intention.

Browser checks run against the production bundle and explicitly mocked API wire
fixtures, while Go tests prove real encrypted session/CSRF/domain/RLS persistence.
They are not selected external OIDC provider or unfamiliar-user acceptance.

```sh
npm run build
npx playwright install chromium
npm run test:e2e
```

Set `UNDERGRID_TEST_CHROMIUM` to an existing Chromium executable if needed.
Missing host libraries must be provided by the test environment; no browser
runtime libraries are committed. Playwright artifacts and generated builds are
ignored. `npm audit --json` verifies the lockfile against the current registry.

Contribution legal templates and signup terms remain operator approval inputs.
There are no invented legal texts/provider credentials. Token rotation/control
proof, document revision/sharing/watch reconfiguration and collaboration/artifact
screens belong to the next scoped branches.
