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
