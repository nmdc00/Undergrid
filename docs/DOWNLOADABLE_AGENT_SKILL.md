# Downloadable network agent skill

TDD §9 / interoperability: publish an installable, versioned network skill through
the actual network HTTP entrypoint, independently of database availability and
credentials. One feature branch: `feat/v0.1-downloadable-agent-skill`.

Acceptance: anonymous GET downloads the exact frontmatter-bearing SKILL.md;
HEAD returns matching metadata without a body; mutation attempts return405.
The document describes protocol discovery, explicit network credentials,
command idempotency/versioning, bounded inbox recovery and distinct consent.
Installation creates no identity, credentials, membership or financial authority.

Required proof: focused public HTTP test RED/GREEN, skill frontmatter validation,
canonical contract and generated-types checks, static/build checks and a review
of the instructions against actual routes. Provider onboarding, public hosting,
two independent client qualification and unfamiliar-owner outcome remain separate.

## Install and connect

The repository source is
[`internal/network/skills/undergrid-network/SKILL.md`](../internal/network/skills/undergrid-network/SKILL.md).
A configured network service returns the exact same file from
`/skills/undergrid-network/0.1.0/SKILL.md`, without requiring credentials or a live
database. Save the downloaded file in an `undergrid-network` directory under
your external runtime's documented skill location. Inspect the file before
loading it. No install script, shell hook or dependency download is required.
This publishes the instructions; it does not select or qualify a public host.

Use the network HTTPS origin and scoped credentials given by the owner/operator.
Read `/openapi.json`, then authenticate `GET /v1/me` to check identity. For MCP,
configure the runtime's remote Streamable HTTP transport at that origin's `/mcp`
with its secret-backed credential or supported OAuth handler, initialize, and
read `tools/list`. The [MCP contract](NETWORK_MCP.md) describes current bearer,
OAuth and error handling. For REST, use the [canonical contract](NETWORK_REST_CONTRACT.md)
for schemas and optimistic versions; the [reference listener](design-examples/README.md)
illustrates external wake-up and recovery. The A2A Agent Card is at
`/.well-known/agent-card.json`. Service-provided metadata selects the OAuth
resource identity; never derive a token audience from an endpoint URL.

Keep tokens in the runtime's secret store and financial credentials on their
separate origin. Downloading or loading this skill creates no server-side
identity or consent. It does not install a scheduler or approve participation.
