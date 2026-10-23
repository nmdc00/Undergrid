# Owner inbox and receipt acknowledgement

Closes the owner-return/inbox/receipt step of HP1 in
[the happy-path execution map](HAPPY_PATH_EXECUTION.md), implementing the
owner-console portion of TDD §6/§9 and SW-06. Feature branch
`feat/v0.1-owner-inbox` reconciles release `78b2e5b` (dismissal PR91). Local focused/integrated checks
passed as recorded below; review/merge and deployment remain pending. No complete
HP1 journey is claimed.

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
HTML. Discovery notifications have the fixed human label “New discovery result”
and a fixed local anchor to the existing intention-review panel, with instructions
to select the relevant intention and inspect current evidence/source details.
The raw event resource path never controls this navigation. A receipt is confirmed only by `acknowledged: true`, validated inside the
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
(`/tmp/undergrid-owner-inbox-ui-green.log`). The final focused/full UI gates also cover actual 500-response recovery.

Current PostgreSQL tests cover bounded/nonblocking reads, cursor audience and
expiry, foreign attribution, closed receipt input, duplicate acknowledgement,
unchanged intention state and suspended-owner/revoked-agent retries. A separate
private-room lifecycle uses real restricted worker fanout twice, then withdrawal,
to prove current reference hiding, cursor progress and receipt-only semantics.
These proofs passed on the exact reconciled runtime as recorded below. Actual
HTTP/Ajv validation rejects receipt authority fields, private content additions,
trusted-content relabeling and more than 50 owner events.

## Pragmatic quality assessment

The owner adapter owns attribution and its nonblocking HTTP contract. Existing
`inboxCommand` owns receipt persistence, cursor rules and current resource
visibility, so no second authorization or delivery implementation is introduced.
Owner cookie/CSRF handling and the shared retry-key helper remain independent of
the financial sandbox and all provider credentials. There is no polling scheduler,
new database table or general notification framework. Manual refresh avoids
holding authority locks while waiting for new events. Root source review found no material authorization/recovery issue. Its UX finding
that raw event/resource codes alone were insufficient guidance was corrected with
the fixed discovery label and existing-panel navigation above.


## Reconciled final validation

Exact runtime source `994e443a5b0743a77c8b2d5df43a19ad0bce4759` includes merged
dismissal release `78b2e5b`. OpenAPI conflicts were resolved through a structural
three-way JSON merge retaining both features, and TypeScript types were regenerated
from that combined canonical document. App integration retains dismissal refresh
and the fixed inbox-to-intention-review anchor.

The race-enabled PostgreSQL gate passed **3 top-level cases in 5.870s**, zero
failures/skips (`/tmp/undergrid-owner-inbox-reconciled96.jsonl`):
`TestOwnerInboxReceiptAndBoundCursor`,
`TestOwnerInboxRechecksPrivateResourceAndReceiptRemainsSeparate` and the existing
`TestInboxReceiptAndRecovery`. This includes real restricted worker duplicate
fanout, private membership withdrawal, a hidden event advancing the scan cursor,
receipt persistence and current owner/agent denial. Sequential vet passed
(`/tmp/undergrid-owner-inbox-vet.log`). An earlier private-case attempt stopped
at a fixture message request missing the agent contract's `kind: text` (422);
correcting that test input does not represent a domain defect or behavioral RED.

The guidance oracle first failed on the absent human discovery label/link
(1 failed, 4 excluded, 2.32s;
`/tmp/undergrid-owner-inbox-guidance-red.log`), then all **5 focused UI cases**
passed in **1.60s** (`/tmp/undergrid-owner-inbox-guidance-green.log`). The full
merged console suite passed **100 tests across 13 files in 26.19s**
(`/tmp/undergrid-owner-inbox-full-ui.log`). TypeScript and Vite build passed
(Vite 1.28s; `/tmp/undergrid-owner-inbox-build.log`). Final Redocly, compilation of
**142 schema components** and generated-type parity passed
(`/tmp/undergrid-owner-inbox-contract-final.log`).

No runtime source changed after these gates; subsequent evidence recording is
documentation only. This closes the feature's local checks, not the full HP1
browser/worker-restart journey, real-source qualification, deployment or the TDD.
