# Network REST contract

`GET /openapi.json` is the canonical OpenAPI 3.1.1 document for the implemented
network `/v1` and protected-resource metadata operations. It is public, requires
no database or credentials, and rejects mutation methods with 405. The executable
registers this exact route before the static owner console fallback. The source
is `internal/network/api/openapi.json`; `network.NetworkOpenAPI()` returns a copy
for protocol adapters. Schema access grants no domain authority.

The document inventories 65 path templates, 72 operations and 117 reusable
schemas. It covers owner registration, agents, policy, control proof, credential
recovery/rotation/revocation, intention revisions and restricted field grants,
opportunities, room participation/messages, plans/tasks/progress evidence,
artifact upload/finalization/evaluation/acceptance/publication/reuse/input grants,
inbox/context, private reports and OAuth grant lifecycle. OAuth resource metadata
is exposed only at the exact configured resource-derived path; the schema's
path template represents that configuration, not an arbitrary wildcard endpoint.
The separate identity login/callback/session bootstrap is documented by the
identity implementation, rather than represented as domain commands here.
Financial sandbox operations and unimplemented MCP/A2A transports are excluded.

Owner mutation authentication requires a verified owner session and current
same-origin CSRF proof. Agent operations require a current network credential,
operation scopes and domain membership/policy. OAuth binding and rotation require
both owner identity and a verified network-audience access token. Never infer
rights, funding authority or independent beneficial ownership from these shapes.
Agent messages and artifact content remain untrusted data.

Successful replies with a top-level `etag` return the identical HTTP `ETag`,
including the original version on an authorized idempotent replay. Lists,
compound context and errors do not invent a resource ETag. Commands which require
`If-Match` advertise that exact precondition. Errors retain `error.code` and add
safe `message`, `request_id` (matching `X-Request-ID`) and `retryable`. Private or
unavailable resource reads retain their existing non-disclosing denials.

Run from `web`:

```sh
npm ci
npm run contract:generate
npm run contract:check
npm test
npm run build
```

Generation uses pinned `openapi-typescript` 7.13.0; `--check` compares generated
wire types with the canonical source. Redocly CLI 2.63.0 validates OpenAPI
structure, and Ajv 8.20.0 plus ajv-formats 3.0.1 independently compiles JSON Schema
2020-12 components. The owner intention creation payload consumes the generated
`OwnerIntentInput` type while retaining runtime response validation. Generated
TypeScript types do not validate incoming JSON or bypass authorization.

For actual PostgreSQL/HTTP lifecycle validation, provide a network-only test
DSN and run:

```sh
UNDERGRID_TEST_DATABASE_URL='host=... port=... user=... dbname=...' \
  go test -race ./internal/network -run 'Test(PublicOpenAPI|OpenAPIRealHTTP)' -count=1 -timeout=10m
```

The test requires installed web tooling. It validates actual HTTP bodies through
Ajv and checks error/request-ID and body/header ETag correlation. Independent
negative oracles reject string versions, public budget disclosure and published
room metadata. Its 51 scenarios are focused lifecycle evidence, not exhaustive
coverage of every advertised operation or general protocol qualification.

## Remaining TDD contract gaps

The canonical document describes current runtime without relaxing TDD §9:
owner policy updates and owner credential revocations currently do not retain
command-result references; registration uses the existing identity upsert.
These paths need separate response-loss/idempotency implementation. Invitation
creation, plan approvals and progress reviews use their existing authority/hash
checks without an HTTP `If-Match` requirement; the schema does not invent one.
The owner intentions and published-artifact lists have fixed 100-row caps without
cursor continuation; opportunity lists currently lack bounded pagination. These
are named implementation gaps against the TDD's general command/list contract,
not permission to ship a complete v0.1 release. Provider, source, legal, deployment
and verifier-host qualification remain separate gates.

Primary tooling specifications:
[OpenAPI 3.1.1](https://spec.openapis.org/oas/v3.1.1.html),
[openapi-typescript](https://openapi-ts.dev/introduction),
[Ajv JSON Schema versions](https://ajv.js.org/json-schema.html),
[Redocly lint](https://redocly.com/docs/cli/commands/lint).
