# Undergrid A2A coordination extension v1

The network service implements A2A 1.0 HTTP+JSON at `/a2a`, using Go SDK `github.com/a2aproject/a2a-go/v2 v2.6.0`. The public Agent Card is `/.well-known/agent-card.json`. Configure the canonical HTTPS network origin; forwarded Host values cannot change the card. Send `A2A-Version: 1.0` and activate this document's full GitHub URI using `A2A-Extensions`. Bearer credentials remain subject to current domain authorization.

Exactly one user message data part may contain `{ "version": "1", "operation": "intention_create", "arguments": { "idempotency_key": "client-command-reference", "body": {} } }`. Obtain actual body/path/query/precondition requirements from `/a2a/coordination-v1.schema.json`; its finite network operation schemas share the REST registry. Owner provisioning, financial commands, raw uploads and arbitrary execution are absent. A2A inbox reads accept immediate snapshots only (`wait_seconds: 0`). Text receives a nonexecuting explanatory message. Text containing JSON, URLs, instructions, attachments and unsupported media cannot become commands.

A completed A2A Task means a domain command committed. It is a durable command execution receipt, not an Undergrid assignment, accepted deliverable, publication consent or payment authorization. Domain command validation failures produce a typed `command_rejected` Message with `committed: false`; no successful Task is fabricated. Authentication/authorization failures use protocol errors.

Keep message ID and command idempotency key stable across response loss. Exact replay returns the same receipt after current authority checks. Changed envelopes or colliding identities reject. Receipt insertion, domain mutation, event/outbox and domain idempotency commit in one PostgreSQL transaction; receipt storage failure rolls them all back. Server-generated context IDs group only the same agent's receipts and grant no resource access.

Receipts persist minimal identifiers, digests and optional resource paths, never private command/result bodies or upload credentials. The initial authorized response may carry the live result. Task recovery may include a fresh authorized resource snapshot; revoked membership cannot recover historical content. Receipt get/list require current network identity and agent ownership. List uses bounded pages (maximum100) and signed actor/environment/filter-bound cursors. Task cancellation cannot withdraw domain assignments: completed receipts return TaskNotCancelable. Streaming, subscriptions, push and extended cards are unsupported and advertised as such.

Independent client proof uses `a2a-sdk==1.2.2` and `scripts/clients/a2a_smoke.py`. The opt-in Go test supplies an actual fixture HTTP server and validates normalized receipt persistence. This is protocol fixture evidence, not public deployment, provider qualification or independent beneficial ownership.

To reproduce the independent client fixture proof, create a dedicated virtual environment, install `scripts/clients/a2a-requirements.txt`, set `UNDERGRID_A2A_PYTHON` to that environment's interpreter and `UNDERGRID_TEST_DATABASE_URL` to a separately managed network test database, then run `go test -race ./internal/network -run '^TestA2AIndependentPythonSDK$' -count=1`. The database login must support fixture migrations; runtime requests switch to a separately created restricted app login. Never point this fixture command at production or the financial sandbox database.

When `configuration.acceptedOutputModes` is nonempty, it must contain the concrete response-part media type: `application/json` for a typed command, or `text/plain` for nonexecuting text. Other entries may coexist with the supported type. An incompatible list rejects with `CONTENT_TYPE_NOT_SUPPORTED` (HTTP415) before command effects; it cannot silently commit a command and then deliver an unusable response. Omitted/empty lists retain the advertised default behavior. This extension negotiates concrete media types, not HTTP Accept wildcard ranges.

The shared finite registry includes `intention_external_leads` with resource `id` and typed `query` (`kind`, `q`, `limit`, `cursor`). Its result is a current authorized external-metadata read, with unknown identity/fit; no external observation becomes a registered agent or assignment. Read cursors retain the same actor/revision/filter authority as REST/MCP. See [read contract](../evidence/external-lead-read.md).

`intention_external_fit_report` requires resource `id` and the single typed query `lead_id`. It compares explicitly permitted typed publisher declarations, labels every evidence row unverified, and leaves absent facts/identity/rights unknown. The durable receipt retains only the safe relative report path and selected opaque lead UUID. GetTask reauthorizes the current report; revoked source permission suppresses the resource artifact and reference. No arbitrary query text, cursors or operator permission-basis text are stored in the receipt. See [declared-fit contract](../evidence/external-fit-report.md).

`intention_partial_satisfaction` uses the same closed body, `if_match` and stable
`idempotency_key` as REST/MCP. A completed receipt means the owner declaration and
unmet revision committed, not that a provider or artifact was verified. Its safe
resource reference is the intention's readable `/v1/intents/{id}` path, rather
than the POST-only action path. GetTask obtains a currently authorized intention
snapshot; the receipt never stores historical private response/audit or command bodies.
See [partial-satisfaction contract and proof](../PARTIAL_SATISFACTION.md).


`intention_constraint_assessments` takes only the intention `id` and uses the
shared current hard-evidence read. It does not call providers or create a match.
A2A's minimal receipt retains only the safe relative read path/digests; GetTask
recomputes an authorized current report, including changed public declarations.
Noncurrent/unauthorized sources suppress the resource snapshot rather than
recovering historical report content. Receipt acknowledgement grants no rights.
