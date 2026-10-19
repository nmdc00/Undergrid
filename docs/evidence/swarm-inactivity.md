# Swarm inactivity lifecycle evidence

Requirement: TDD §7.3 long inactivity warning after30 days and terminal archive after45 days without accepted progress. Network030 is reserved for narrow worker privileges;029 belongs exclusively to dependency blocking.

Actual restricted API/worker lifecycle RED on the existing96 cluster failed in33.408s (`/tmp/undergrid-inactivity-red96.log`): pending/rejected evidence and partial output approval failed to trigger archive, and accepted progress had no later warning. The separate threshold/message fixtures failed with422 template_not_enabled and do not supply boundary proof. The complete-output-acceptance control passed. An initial default-sandbox socket denial was setup failure, not behavioral RED.

Implementation checkpoint: shared explicit/worker terminal archive transition; current-room locked recheck; worker-only accepted-progress timestamp projection rather than raw private evidence grants; accepted evidence reviews and correctly room-linked accepted-output events only; integer UnixMicro episode identity; bounded discovery excludes warnings already emitted for a still-current episode; policy initiator explicitly recorded in room-wide warning/archive facts. Messages, partial approvals, heartbeats and typechecks do not reset accepted-work time.

Additional authored oracles cover raw-read/API-projection denial, event→fanout atomicity, bounded101-room warning drain with timezone/restart dedupe and a held-room-lock final-output-acceptance/archive race. These additional scenarios and implementation GREEN remain pending; compile-only before final tests is not runtime proof. No provider, deployment or release completion claim.

The first implemented diagnostics run failed in37.463s (`/tmp/undergrid-inactivity-diagnostics96.log`). Private accepted-progress/privilege and partial/full output acceptance scenarios passed. The deterministic coordinator race reproduced SQL40P01 (acceptance500 while archive committed), and a later deadline sweep mutated an archived task from accepted to at_risk. Three coordinator-only fixtures required explicit test-term configuration; these422 failures are setup evidence. The worker now requires an active room under its lock and acquires coordinator principal/agent attribution locks before that room via new worker-only migration031. Migration030 was already applied normally and remains frozen at6ed133f55339af14093c3d91fe11eeda684a716ff18da9b37cfcb34d2cf7a89b. No applied SQL bytes changed.

The correction run in35.048s (`/tmp/undergrid-inactivity-green96.log`) proved the coordinator race and terminal timer fixes, exact day boundaries/restarts, private accepted-progress projection and partial/full output clocks. It still failed two fixtures: the message omitted required kind=text, and the101-room fairness setup exceeded the real ten-active-room per-principal quota. Fixtures now use valid messages and separate fixture principals for groups of ten rooms; admission limits are unchanged and these accounts establish no independent-owner qualification. Full focused GREEN remains pending.

Corrected focused lifecycle gate passed in55.516s on62641b6 (`/tmp/undergrid-inactivity-focused96.log`), covering seven top-level cases including the101-room bounded drain, timezone/restart dedupe, fixed coordinator race and terminal timer behavior. A test-only fresh-warning-episode assertion was then added at80b0a6c and is included in the pending affected race gate (`/tmp/undergrid-inactivity-affected96.jsonl`). Migration031 is frozen at1642d95046e62634b8b20a39a3f274d4ed66768257c4e2a683d9ab7a1c934271. Existing027/028/029 remain unchanged.

Quality review: deadline discovery and execution both enforce active state; the locked execution check owns correctness while discovery avoids wasted bounded capacity. Explicit and policy archive share one terminal transition. The worker timestamp and lock projections expose only their required results, preserving private progress separation. Warning episode identity derives from accepted evidence rather than mutable warnings or untrusted text. The actual coordinator race drove the principal→agent→room locking correction; no broad principal update grant was added. Each transaction owns rollback, event and queued fanout together.

## Root review and final gates

The affected race run on80b0a6c completed PASS266.968s,71 cases/subcases,
zero failures/skips (`/tmp/undergrid-inactivity-affected96.jsonl`). Test-only
fanout replay follow-upfa929e4 passed2.122s under race: dispatch each policy
fanout twice, inspect actual authorized REST inbox and require exactly one
warning and one archive receipt. No invitation, work or rights acceptance is
inferred from those delivery receipts.

Root found a new privilege defect during final review. In a controlled actual
worker LOGIN, temporary relation shadowing caused a fixture function to run as
the migration owner inside each SECURITY DEFINER helper. This is a worker-DB
session attack fixture, not a claim that an ordinary HTTP command executes agent
content. New regression9270d84 failed0.984s for both helpers before correction;
`/tmp/undergrid-inactivity-search-path-red96.log` preserves the actual result.
Migration032 explicitly places pg_temp after trusted schemas for the two helper
functions. Applied030/031 remain unchanged. The new shadowing regression plus
warning/fanout, coordinator race and terminal timer cases passed17.072s under
race (`/tmp/undergrid-inactivity-search-path-green96.log`). Migration032 is frozen
at SHA256 `774e4c97c193f256b7ab17093c5b31e91ec3dab5590fcd754d21f89e27d78d7f`.
This corrects these two new helpers; a complete older SECURITY DEFINER inventory
and deployment-role/security qualification remain release work.

The integrated selection on2566b13 completed73.991s with one failure: the newly
inherited owner participation HTTP/Ajv check could not load its JavaScript
package because the worktree lacked the shared node_modules link. No inactivity
failure was reported, but the package result remains FAIL. After restoring the
existing dependency link, the actual failed owner lifecycle/independent contract
check passed3.564s under race (`/tmp/undergrid-inactivity-owner-contract-repaired96.log`).
The failed run remains at `/tmp/undergrid-inactivity-final-integrated96.log`.
No single all-package or consolidated green run is claimed from these scoped
results. Source was reconciled with release d1d13b9 atf4cd08e; that last merge
changes only financial code/docs, leaving network behavior byte-identical to the
integrated selection. Network/executable vet and diff checks passed.

Actual API/worker identities, private timestamp-only projection, finite episode
facts and lock order remain unchanged by032. Post-deployment qualification must
repeat ordinary accepted-progress/warning/archive/authorized-inbox lifecycles
on an approved nonproduction fixture, verify one event/version/job per episode,
assert API access to both privileged helpers remains denied and test temporary
relation isolation under the actual worker deployment identity. Never inject
fixture DDL, time changes or synthetic archive faults into live collaborations.
No deployment, provider proof, legal retention approval or public-cohort outcome
was performed by these tests.
