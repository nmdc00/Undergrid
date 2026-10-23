# Owner inbox and receipt acknowledgement

Closes the owner-return/inbox/receipt step of HP1 in
[the happy-path execution map](HAPPY_PATH_EXECUTION.md), implementing the
owner-console portion of TDD §6/§9 and SW-06. Feature branch
`feat/v0.1-owner-inbox` is based on release `852c9c3`. Local final validation,
review/merge and deployment remain pending; no complete HP1 journey is claimed.

## Runtime contract

`GET /v1/owner/agents/{agent_id}/inbox` authenticates the current owner, locks the
selected active owned agent and delegates to the same bounded `inboxCommand`
projection as the agent interface. The signed opaque cursor remains bound to the
agent and environment, with the existing 30-day expiration. `limit` is 1–50.
`wait_seconds` is explicitly rejected: manual owner reads never hold owner
principal/agent locks during a long-poll wait. Current private-resource visibility
filters remain in the shared query; hidden rows still advance the scan cursor.

`POST /v1/owner/agents/{agent_id}/inbox/{id}/ack` accepts only `{}` and an
idempotency key. It calls the existing receipt command, which sets `acked_at`
once through `COALESCE`. It introduces no new event, recovery ledger, admission,
assignment, plan, rights, publication or spending transition. Current owner and
agent status are checked on every retry. An already retained own event remains
acknowledgeable after its resource becomes private to that agent: the response
contains only receipt confirmation, never the revoked resource or new authority.
Foreign events remain inaccessible through inbox row security.

The browser selects an active owned agent and manually refreshes or loads the
next bounded page. Event type and resource reference render as untrusted text;
references are never executed, followed as callback URLs or interpolated into
HTML. A receipt is confirmed only by `acknowledged: true`, validated inside the
existing request helper before releasing its retry key. An unusable successful
response or server/network failure preserves the exact pending command for retry.
Agent/session changes discard late results and previous cursors.

On 410 the console discards the expired cursor and displayed events, then requires
an explicit reset to the retained inbox. The notice explains that this does not
recover expired history and that pending invitations/assignments require separate
review. This feature does not relabel a reset as full obligation resynchronization,
and introduces no unbounded owner snapshot or external email delivery.

## Evidence

The actual restricted-LOGIN PostgreSQL HTTP oracle first failed on the missing
owner route: 405/method_not_allowed from the existing generic agent-control
fallback, package elapsed 0.921s (`/tmp/undergrid-owner-inbox-red96.log`). The
expected success was 200; no fixture or dependency failure is labeled RED.

Before UI behavior, a typed null component scaffold rendered no Refresh inbox
control. Its three behavioral oracles failed on that missing control (14.40s,
including cold environment setup; `/tmp/undergrid-owner-inbox-ui-red.log`). The
implemented manual reader, uncertain receipt recovery, explicit expired-cursor
reset and late-agent-response handling passed those three cases in 2.26s
(`/tmp/undergrid-owner-inbox-ui-green.log`). Additional actual 500-response
recovery coverage is authored and awaits the final focused/full UI gate.

Current PostgreSQL tests cover bounded/nonblocking reads, cursor audience and
expiry, foreign attribution, closed receipt input, duplicate acknowledgement,
unchanged intention state and suspended-owner/revoked-agent retries. A separate
private-room lifecycle uses real restricted worker fanout twice, then withdrawal,
to prove current reference hiding, cursor progress and receipt-only semantics.
These final proofs are authored but not yet executed. Canonical owner schemas,
generated wire types and actual-response Ajv/negative oracles are also authored;
final checks remain pending.

## Pragmatic quality assessment

The owner adapter owns attribution and its nonblocking HTTP contract. Existing
`inboxCommand` owns receipt persistence, cursor rules and current resource
visibility, so no second authorization or delivery implementation is introduced.
Owner cookie/CSRF handling and the shared retry-key helper remain independent of
the financial sandbox and all provider credentials. There is no polling scheduler,
new database table or general notification framework. Manual refresh avoids
holding authority locks while waiting for new events. Final source review and
measured checks must confirm these paths before merging.
