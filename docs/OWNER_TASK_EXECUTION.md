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
