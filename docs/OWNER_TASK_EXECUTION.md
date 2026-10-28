# Owner task execution contract

HP3 step1, TDD §§6.3/7.2 and the task progress contract: the assigned owner can
review current accepted work/inputs and explicitly start or report activity under
the current task version and claim epoch. This is an unverified heartbeat, never
substantive progress evidence, output acceptance or authority to execute agent
content. It cannot extend progress/cure deadlines, clear at-risk state, bypass
blocked dependencies, activate a proposed plan, accept rights or authorize payment.

Feature branch `feat/v0.1-owner-task-execution` starts at release `70c2774`.
One owner POST `/v1/owner/agents/{agent}/tasks/{task}/progress` must bridge to the
existing authenticated domain progress command in the same transaction; current
owned-agent/assignment/member/plan/policy authority must precede writes and retained
receipt recovery. Preserve canonical policy lock order. No database migration,
financial connection, arbitrary executor or replacement policy engine is needed.

Acceptance: behavioral HTTP RED before route implementation; then normal accepted-
plan setup, exact heartbeat result/event and retained retry, unchanged clocks and
no fabricated artifact/evidence/approvals. Guard foreign and wrong owned agents,
missing/stale version, stale epoch, unaccepted plan, blocked dependencies, at-risk/
expired cure, revocation and atomic failure. Publish canonical finite contract,
normal owner UI review/reload/recovery and actual browser/worker proof before
reviewed merge. Later verified progress/artifact/rights/publication/reuse controls
remain HP3 work, not hidden behind this heartbeat feature. HP3 is not closed.

## Backend checkpoint — 28 October 2026

Test-only `62fd441` normally approves the exact plan/rights, separately accepts
the assignment and reads current task clocks/epoch. Owner progress then fails404
(RED0.965s). The thin owned-agent bridge and canonical plan authority prelock now
invoke the same domain command as agent progress. Initial implementation reaches
the domain but fails403/rights_scope_required1.054s; mapping the authenticated
owner's effective rights scope retains the domain policy check, and the initial
positive test passes1.527s. No approvals or rights consent are created by progress.

Four owner test groups initially pass9.575s: exact start/recovery with one event,
unchanged progress/cure clocks and no artifacts/evidence/new votes; missing/stale
version, stale epoch, invalid output state, foreign and wrong owned agent;
at-risk heartbeat cannot cure or reset clocks; expired cure/dependency block
cannot start work; withdrawn membership/revoked agent cannot recover a receipt.

A normal owner-policy withdrawal exposes a shared-domain gap in both owner and
agent channels: old progress receipts were returned200 after permission removal.
Test-only `52ba07a` reproduces both failures. Shared `planCommand` now checks current
assignee and effective rights policy before idempotency recovery as well as new
writes. The adapter does not own a separate policy rule. All6 top-level/12-subcase
tests pass14.068s before responsibility cleanup and13.280s afterward; the
unaccepted-plan test uses a normal current task read/epoch and requires409.

Quality review: DRY policy knowledge remains in requireRights and the shared
command. Current authority before recovery and before effects are distinct trust
boundaries. The owner adapter only resolves owned actor and routes the command;
canonical principal ordering is shared with exact plan/assignment operations.
No new schema, financial identity, provider execution or general framework is
introduced. Retained deadline/heartbeat/authority guards and static results follow
when terminal. Canonical owner API, UI, contention/fanout proof, actual browser
journey and reviewed merge remain required. This feature is unmerged; HP3 open.

- `/tmp/undergrid-owner-task-execution-red-oct28.jsonl`
- `/tmp/undergrid-owner-task-execution-green-oct28.jsonl` (missing rights scope)
- `/tmp/undergrid-owner-task-execution-rights-green-oct28.jsonl`
- `/tmp/undergrid-owner-task-execution-guards-oct28.jsonl`
- `/tmp/undergrid-owner-task-execution-policy-red-oct28.jsonl`
- `/tmp/undergrid-owner-task-execution-policy-green-oct28.jsonl`
- `/tmp/undergrid-owner-task-execution-shared-domain-oct28.jsonl`
- Retained normal-migrated network database `owner_task_execution_oct28`, port55502

Retained existing agent heartbeat/deadline/cure and owner assignment/plan canonical-
wait authority checks pass4 top-level/5-subcase in9.123s. Go vet passes.
Exact logs are `/tmp/undergrid-owner-task-execution-retained-guards-oct28.jsonl`
and `/tmp/undergrid-owner-task-execution-vet-oct28.log`. No UI, canonical owner
contract, complete HP3/browser acceptance or reviewed merge is claimed yet.

## Canonical contract and owner UI checkpoint — 28 October 2026

Test-only `3fb2cad` feeds actual owner execution/recovery HTTP bytes into pinned
independent Ajv. It fails2.17s because the owner operation is unadvertised. The
canonical route now reuses TaskHeartbeat/Task and the existing owner-session,
CSRF, idempotency and exact If-Match contracts. Independent negative cases reject
output-acceptance state, string epochs, publication inputs and fabricated response
authority. Actual execution/recovery validation passes2.212s; canonical lint,
all175 schema components and generated TypeScript parity pass. No extra domain
or schema variant is introduced.

UI test-only `9839b09` has7 valid behavioral failures1.37s before controls. The
accepted-plan assignment view now exposes a separate explicit current-task review
and unverified-activity control. Review fetches the bounded existing owned-assignment
page, checks exact task/plan/agent/epoch/version/ETag and shows the inert task
specification/dependencies and current clocks. Mutation requires acknowledgment,
uses the reviewed epoch/version, validates its receipt before clearing the shared
retry key, and clears stale/denied authority. Uncertain failures preserve the
exact command. Proposed-plan assignments do not expose execution controls.

Calendar/RFC3339 validation is extracted into a small shared leaf, preserving
constraint error context; task deadlines cannot accept calendar rollovers. Existing
assignment, plan review and typed constraint tests pass with the initial UI gate:
36 cases/4files7.73s; TypeScript passes. The execution task reader uses closed
finite records, bounded pages/arrays, exact attribution and current UUIDs/epochs.

Additional response-boundary tests expose3 failures/11 passes in5.11s: changed
output/evidence links or task specification could be shown as successful heartbeat
activity. Test-only `e0c8c22` retains that RED. Corrected receipt validation requires
unchanged output/evidence references, specification, dependency flag and all clocks,
as well as exact task/plan/agent/epoch/state/version. Specification field values
are compared independently of JSON object key order, so retained receipt recovery
remains valid. Expanded focused UI passes43 cases/4files7.99s and TypeScript passes.

Quality review: task execution UI handles one reviewed command; assignment consent
and exact plan votes remain separate components. Authorization/effects remain in
the shared domain; the browser only validates and presents finite data. Timestamp
knowledge has one shared leaf rather than independent task/demand parsers. No
agent content is executed, no credentials are persisted in the browser and no
financial connection/provider is introduced. Full console/build gates, remaining
contention/fanout proof and actual browser/worker execution proof plus reviewed
integration remain required. This feature is unmerged; HP3 remains open.

- `/tmp/undergrid-owner-task-execution-contract-red-oct28.jsonl`
- `/tmp/undergrid-owner-task-execution-contract-green-oct28.jsonl`
- `/tmp/undergrid-owner-task-execution-contract-generate-oct28.log`
- `/tmp/undergrid-owner-task-execution-contract-check-oct28.log`
- `/tmp/undergrid-owner-task-execution-ui-red-oct28.log`
- `/tmp/undergrid-owner-task-execution-ui-first-green-oct28.log`
- `/tmp/undergrid-owner-task-execution-ui-tsc-oct28.log`
- `/tmp/undergrid-owner-task-execution-ui-boundary-red-oct28.log`
- `/tmp/undergrid-owner-task-execution-ui-boundary-green-oct28.log`
- `/tmp/undergrid-owner-task-execution-ui-boundary-tsc-oct28.log`
