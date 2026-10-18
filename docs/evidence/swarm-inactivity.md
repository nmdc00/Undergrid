# Swarm inactivity lifecycle evidence

Requirement: TDD §7.3 long inactivity warning after30 days and terminal archive after45 days without accepted progress. Network030 is reserved for narrow worker privileges;029 belongs exclusively to dependency blocking.

Actual restricted API/worker lifecycle RED on the existing96 cluster failed in33.408s (`/tmp/undergrid-inactivity-red96.log`): day30 warnings absent, overdue rooms remained active, pending/rejected evidence and partial output approval failed to trigger archive, and accepted progress had no later warning. The complete-output-acceptance control passed. An initial default-sandbox socket denial was setup failure, not behavioral RED.

Implementation checkpoint: shared explicit/worker terminal archive transition; current-room locked recheck; worker-only accepted-progress timestamp projection rather than raw private evidence grants; accepted evidence reviews and correctly room-linked accepted-output events only; integer UnixMicro episode identity; bounded discovery excludes warnings already emitted for a still-current episode; policy initiator explicitly recorded in room-wide warning/archive facts. Messages, partial approvals, heartbeats and typechecks do not reset accepted-work time.

Additional authored oracles cover raw-read/API-projection denial, event→fanout atomicity, bounded101-room warning drain with timezone/restart dedupe and a held-room-lock final-output-acceptance/archive race. These additional scenarios and implementation GREEN remain pending; compile-only before final tests is not runtime proof. No provider, deployment or release completion claim.
