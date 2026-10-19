# Swarm inactivity lifecycle

TDD §7.3: the persisted coordination-deadline worker warns once after30 days
without accepted progress and terminally archives after45 days. The periodic
job runs at startup and each minute; its restart sweep uses persisted state.
Creation is the initial clock. Accepted private progress reviews and correctly
room-linked completed output acceptance can advance it. Messages, heartbeats,
pending/rejected evidence, verification outcomes, partial approvals and
assignment acceptance cannot. Historical accepted progress remains evidence
across later plan or claim changes; no heartbeat manufactures completion.

Discovery considers at most100 currently due active rooms per sweep. A warning
already emitted for the current episode is excluded until the archive deadline
or fresh accepted progress, so a bounded drain reaches later rooms. The episode
key is the accepted-progress instant as integer Unix microseconds, independent
of timezone or JSON timestamp formatting. A restart beyond45 days can archive
immediately without an earlier warning run.

Under the room lock, the worker rechecks activity and terminal state. Migration031
acquires coordinator principal/agent key-share locks before the room to match
command lock order and event foreign-key attribution. A coordinator change while
waiting skips that candidate for safe rediscovery. Archived rooms admit no later
deadline-driven task, membership or plan mutation. Explicit and automatic archive
share one terminal transition; human scope/ETag checks remain in the command.

Warning/archive events and fanout jobs commit atomically with versioned state.
Their principal identifies the accountable room coordinator, while
`initiated_by: inactivity_policy` explicitly identifies the worker policy as
initiator. Facts contain the room resource and episode clock, never private
evidence IDs, task IDs, hashes, audiences or report text. Migration030 gives the
worker a timestamp-only SECURITY DEFINER projection; it grants no raw private
progress reads and is unavailable to the application role.

Archival grants no consent or completion and preserves acknowledged verification,
current-authorized history and licensed exports under [explicit archive](SWARM_ARCHIVE.md).
Legal retention approval, erasure and deployment remain separate gates.

Normal migrations030/031 were applied after029 on the preserved isolated96
cluster; applied migration bytes are frozen. Semantic discovery now reserves032.
[Evidence](evidence/swarm-inactivity.md) records the actual missing-policy RED,
proven deadlock/terminal-timer RED, fixture corrections and focused GREEN55.516s.
The affected race gate and exact release integration remain pending.
