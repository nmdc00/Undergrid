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

## Actual owner browser and transaction proof — 28 October 2026

Frozen `0df8fb26fe2f44ed3bf5b61be14dcd67e790d2f7` passes the actual three-owner
Chromium/session/API/PostgreSQL/worker journey in44.479s. It extends the full
continuous HP2 path rather than seeding an accepted plan: normal demand/group/
capability discovery, room creation/recovery, recruitment, application and peer
admissions, messages, exact plan/rights decisions, separate dependent assignment
acceptances, revision and real worker restart/recovery all precede execution.
C reviews the currently accepted collect task through owner controls, deliberately
loses a committed activity response, recovers the same body/key/version/epoch,
and reloads the same in_progress task with unchanged progress/cure clocks.
Independent SQL requires exactly one collect execution at plan-v2/task-v3/epoch1,
one activity event, no artifacts/evidence, the other task still accepted, and
unchanged exact plan/rights votes. This proves only the first HP3 execution slice;
it is not contribution/evaluation/publication/reuse or provider qualification.

- `/tmp/undergrid-owner-task-execution-live-build-oct28.log`
- `/tmp/undergrid-owner-task-execution-live-first-oct28.jsonl` (PASS44.479s)
- Retained isolated network database: hp1_run_d3d474095e0f42f8bba8b552e9955afc on port 55502
- Retained HP2 browser artifacts: /tmp/undergrid-hp2-browser-687255846

Frontend source SHA256 `afa53700992c72a2bd93554fc638566e0ec3ebdb3c5d9561d33c6f8837d22131`;
built JavaScript SHA256 `6ae7c1f317107265221bc1444c98b16bba5f1ef80d05b9f672b0b80030071dde`.
The coupled build records source/HEAD/assets together; checks run with frozen HEAD.

Two additional deterministic transaction tests pass3.156s. A queue trigger fails
only this task's activity fanout: task state/version, event and receipt all roll
back; removing the fault and retrying the same key twice yields exactly one event
and receipt with unchanged clocks. A separate normal accepted task waits on the
actual canonical plan-authority principal lock; a controlled policy withdrawal
commits before release, and the request returns403/owner_policy_denied with zero
effects. These are real restricted-role HTTP/PostgreSQL transactions, not mocks.
Exact log: `/tmp/undergrid-owner-task-execution-transactions-oct28.jsonl`.

## Environment recovery and final gates — 29 October 2026

The environment restarted after the full UI gate launch. Its Oct28 log is empty
and the process is confirmed missing; this is interruption, not a product failure
or a passing gate. Recreated only disposable memory-backed caches. The retained
network PostgreSQL data directory was stopped and recovered normally; no database,
WAL, retained blob/evidence or migration ledger was reset/deleted. The first
restart omitted the dedicated port and failed against default5432; corrected
startup uses network55502 and the retained socket/data directory. The PostgreSQL
recovery log is `/tmp/undergrid-pg-semantic-55502/restart-oct29.log`. The original
execution fixture and browser-proof databases remain present.

Restarted full console gate passes300 tests/25files in100.04s; canonical175-schema
lint/compile/generated parity passes. Logs:
`/tmp/undergrid-owner-task-execution-full-ui-oct29.log` and
`/tmp/undergrid-owner-task-execution-final-contract-oct29.log`.
The fresh final restricted PostgreSQL/race/static gates and review follow when
terminal. This feature remains unmerged and HP3 remains open.

## Release integration contract

| Invariant | Entrypoint and direct proving check |
| --- | --- |
| Current owned assignee records activity only under accepted work | Owner POST progress → shared planCommand/updateTask; owner execution and current epoch/version/plan/actor tests. |
| Current rights policy precedes receipt recovery in both channels | Shared pre-recovery requireRights/assignee check; normal owner policy withdrawal owner/agent test and canonical-wait test. |
| Heartbeat is not substantive progress or output consent | Unchanged clocks/at-risk state and zero artifact/evidence/new approval oracles in backend and actual browser case. |
| Failed fanout has no partial task/event/receipt effects | Task-specific transaction fault test, followed by exact same-key recovery with one event/receipt. |
| Review and uncertain commands cannot silently change authority/data | UI context/epoch/version/finite-shape checks; invalid clock/output/evidence/specification receipt tests and exact-key/body retry. |
| Runtime wiring and integration survive merge | Actual same-owner browser/worker journey on the exact release SHA plus coupled build/source identity and focused restricted HTTP guard checks. |

No production deployment is performed or claimed. For an approved future isolated
runtime check, read the task's normalized state/version/epoch/clocks, exact
plan/hash/member attribution, and count task.progress_recorded by aggregate ID;
compare its idempotency key's single receipt and event linkage. Confirm the same
command did not create artifacts/progress evidence/approvals. These direct queries
are modeled in owner_task_execution_test.go and the transaction/browser oracles;
a health check alone is not equivalent. Product data and financial identities
remain separate. Current proof is local; external qualification remains open.

## Final scoped review — 29 October 2026

Fresh `owner_task_execution_final_oct29` normal migration/restricted-role race gate
passes12 top-level cases/17 subcases in53.206s, including actual owner execution/
retained receipt schema validation, stale version/epoch/foreign identity, blocked/
at-risk/expired cure, inactive agent/member, current policy withdrawal in both
owner and agent channels, canonical-wait withdrawal, atomic fanout rollback and
exact recovery, and retained assignment/plan canonical-wait/deadline behavior.
No race detector failure is reported. Tagged vet and Go build both pass.

- `/tmp/undergrid-owner-task-execution-final-backend-race-oct29.jsonl`
- `/tmp/undergrid-owner-task-execution-final-vet-oct29.log`
- `/tmp/undergrid-owner-task-execution-final-go-build-oct29.log`

Root review traced owned-actor attribution, canonical principal lock order, shared
pre-replay rights/assignee checks, effect-owning domain transition, immutable plan/
epoch and clock behavior, event/receipt rollback, finite read/response validation,
exact uncertain-command keys and parent-key/generation invalidation. No material
in-scope finding remains. Historical receipts confirm recorded effects, not new
execution/rights authority; subsequent mutations retain current domain checks.
No production migration or financial source/identity changes are present. API
schemas/types and owning docs match the implemented behavior. This is root review,
not a new independent agent review; established workers remain quota-unavailable.
No replacement agent/model was used to evade that quota. Release merge and exact-
release checks remain pending. All other HP3 delivery/progress-evidence/upload/
acceptance/publication/reuse outcomes and external qualification remain open.
