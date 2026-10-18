# Swarm inactivity lifecycle

Task: TDD §7.3 long inactivity, one feature branch
`feat/v0.1-swarm-inactivity`, based on release `a5cbfea`.

Observable behavior: warn once after 30 days without accepted progress and
terminally archive after 45 days. Creation time is the initial baseline;
explicitly accepted progress evidence or accepted output can advance it.
Messages, heartbeats, pending evidence, verifier outcomes, votes and assignment
acceptance do not advance it. Recheck the baseline and active state under the
same room lock used by commands, so racing acceptance and archival serialize.

Entrypoint: the persisted coordination-deadline worker and its restart sweep.
Persist warning/archive events and fanout atomically with versioned state.
Use bounded fair discovery and durable warning deduplication per inactivity
episode. An offline restart already past day 45 may archive immediately; it
must not require an operator to run a day-30 sweep first. Archive grants no
new consent and preserves acknowledged verification/history and authorized
exports under the existing explicit archival contract.

Worker events must say they were initiated by the inactivity policy; attribution
to a room's owner must not imply that owner explicitly requested archival.
Reuse the explicit archive transition after its feature is merged, keeping
coordinator scope/ETag checks confined to the human command.

Test-first checkpoint: `TestSwarmInactivityWarningAndArchiveSurviveRestart`
authored; actual PostgreSQL RED and further accepted-progress/concurrency,
fanout, first-sweep-overdue and stale-work tests remain pending. No runtime
behavior or migration has changed. Network migration029 belongs exclusively
to downstream dependency blocking. Do not edit applied migrations.

Required evidence before merge: actual app-only/worker-only LOGIN lifecycle
tests, reviewed accepted-progress boundary, repeated/restarted sweeps, atomic
event/job effects, affected archive/progress/deadline race regression and
TDD/evidence reconciliation. Legal retention approval remains a separate gate.
