# Opportunity dismissal

Requirement: HP1 step 5 and TDD §5.4. Implemented on
`feat/v0.1-opportunity-dismissal`, reconciled with release `852c9c3`
(including owner assignment response). Release merge remains pending.

## Intended behavior and acceptance

An authenticated recipient can explicitly dismiss a currently eligible suggestion.
The owner session selects an owned active agent for attribution. REST, finite MCP
and A2A commands share the domain transaction; adapters do not grant authority.
The command is `POST /v1/opportunities/{id}/dismiss` with a closed empty
JSON object and an idempotency key. The corresponding owner command is
`POST /v1/owner/agents/{agent_id}/opportunities/{id}/dismiss`.

The transaction must validate current actor, recipient, source and candidate
eligibility before recovering a retained receipt. A dismissed row remains
available for recovery of the same authorized receipt, while its recommendation
projection disappears. Foreign actors, revoked actors and stale sources cannot
recover a historical success as current authority.

Acceptance covers shared demand, external requests, external capabilities and
builder recruitment. Dismissal does not accept participation, create an assignment,
grant rights or mark an intention satisfied. Immutable evidence and the existing
recipient/kind/input-hash uniqueness remain intact: a worker restart or unchanged
rediscovery cannot produce a fresh notification. Changed eligible evidence may
produce a distinct suggestion.

Migration 040 installs narrow `UPDATE(state)` permission and a recipient
update policy constrained to the dismissed state. There are no new tables and no
application grants to mutate evidence or recipient ownership. Applied migrations
038–039 remain immutable. Network fixtures and identities remain separate from
financial runtime and deployment.

## Evidence

| Proof | Actual result | Log in `/tmp` |
| --- | --- | --- |
| Real restricted-login HTTP missing-route oracle | RED 4.497s: current shared-demand POST returned 404 | `undergrid-opportunity-dismissal-red96.log` |
| Shared demand, two controlled HTTPS source kinds and recruitment; atomic event failure rollback | PASS 12.379s | `undergrid-opportunity-dismissal-first-green96.log` |
| Owner UI dismissal command/recovery before implementation | RED, two behavioral failures, 2.25s | `undergrid-opportunity-dismissal-ui-red.log` |
| Exact malformed-success retry and late-selection suppression | Two cases PASS, 1.33s | `undergrid-opportunity-dismissal-ui-green.log` |
| Exact reconciled `d5ff1c5d2d1d1f3cfd1d43621fb4646cdc3fae96` PostgreSQL race lifecycle including actual MCP/A2A | Three top-level and two source-kind subcases PASS, 21.459s; zero failures/skips | `undergrid-opportunity-dismissal-d5ff-integration96.jsonl` |
| Exact reconciled network static analysis | PASS | `undergrid-opportunity-dismissal-d5ff-vet.log` |
| Reconciled owner UI suite | 95 tests / 12 files PASS, 24.69s | `undergrid-opportunity-dismissal-d5ff-ui.log` |
| Reconciled TypeScript and production bundle | PASS, Vite 1.50s | `undergrid-opportunity-dismissal-d5ff-build.log` |
| Reconciled OpenAPI lint, 141 schemas and generated-type parity | PASS | `undergrid-opportunity-dismissal-d5ff-contract.log` |

The PostgreSQL fixture uses distinct genuine API/worker LOGIN roles. Migration
040 was normally applied to the retained isolated network fixture. Its frozen
SHA256 is `fb101f302a57866d5a9b61c2b1462c3da0ae9a0760cba125b170e0dcce21818d`.
No applied SQL was rewritten. Controlled HTTPS fixtures are not source/provider
qualification. The exact reconciled lifecycle also proves a real candidate
revision creates a distinct suggestion, stale old receipts are denied, read-only
credentials cannot dismiss, and the application SQL role cannot reopen a row or
rewrite immutable evidence (SQLSTATE 42501). An earlier added revision oracle
incorrectly sent the whole intention instead of the published revision envelope;
that 400 fixture failure was corrected without a behavior change.

The independent MCP SDK and actual A2A HTTP path recover the same domain receipt.
An actual foreign authority lock produces an A2A protocol 500 with no committed
command receipt. After releasing the lock, the exact same message/key succeeds.
This path does not enter A2A's `command_rejected` representation, whose retryable
flag is reserved for its finite domain-rejection cases. No shared transport error
classification was changed.

For each external source kind, a fresh worker instance scans persisted dismissal
state without reopening it or emitting another available event. A controlled
HTTPS source-version update creates a distinct suggestion and invalidates the
old receipt. These tests call the actual worker transaction directly; they do
not claim a full scheduler/process restart. The combined HP1 browser/API journey
must prove that broader restart/reconnect acceptance step. The reconciled UI suite,
schema/type parity and production bundle validation also passed on this source.

The first transition records one attributed `opportunity.dismissed` event in the
same transaction as state and receipt. Repeated receipts do not create another
event or notification. A forced event-insert failure proves rollback of both the
state change and receipt. The current eligibility helper locks foreign authority
with NOWAIT, so a recipient already locked by authentication does not wait in an
opposite authority order; contention fails the whole command for retry.

## Quality and limits

The existing immutable input identity and notification insertion rules own dedup;
dismissal does not introduce a second suppression store. Domain authorization
precedes shared receipt recovery across REST and owner wrappers. The finite
protocol registry reuses canonical schemas rather than duplicating input policy.
The boolean SECURITY DEFINER helper locks and checks current foreign authority
without exporting private rows. The application can update only opportunity
state and the update policy allows only dismissed recipient rows.

No financial identities, provider actions, deployment, admission, assignment,
rights or satisfaction behavior changes. Exact merged-release validation and
full end-to-end provider qualification remain separate gates. Semantic discovery
is on an independent HP2 branch; its richer opportunity evidence requires an
explicit integration extension before semantic suggestions can be dismissed.
The applied 040 migration must remain immutable during that extension.
