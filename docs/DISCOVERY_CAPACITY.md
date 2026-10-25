# HP2 committed discovery capacity

Work in progress on `feat/v0.1-discovery-capacity`, from release `c0087c0`.
Owning requirements are HP2 step 1, TDD §5.4 and the existing five-second
committed-discovery contract. This feature changes scheduling ownership, not
matching policy, identity, permissions or opportunity evidence. It imports no
parked semantic code and needs no application migration.

## Runtime behavior under implementation

Normal committed intention changes receive independent fresh direct-scan,
watcher-fanout and watcher-pair lanes. The existing queues remain active retained
lanes. Startup and one-second maintenance move eligible jobs older than thirty
seconds in at most 256-row `SKIP LOCKED` batches. Only queue ownership changes;
job identity, arguments, kind, state, attempts/errors, scheduling, priority,
maximum attempts and other metadata remain intact. Running, fresh, unique-key
and unrelated jobs are excluded. Future-backoff schedules stay future-backoff
schedules; queue movement never makes them immediately eligible.

Fanout dispatch derives descendant lanes from the actual persisted River job
queue. A fresh fanout moves pairs and pagination into fresh lanes; an aged fanout
that has become retained cannot refill the fresh lanes indefinitely.

One concrete registration map also determines pool sizing: seventeen queues
plus the existing ten-connection allowance for completion and maintenance.
Production source configuration, the concrete source-refresher seam, six-hour
source schedule, authority transactions and bounded shutdown remain intact.
The pinned River 0.49.0 fetch orders by priority, scheduled time and ID and has
no public age-filter/queue-move operation; the narrow queue-only SQL relies on
that pinned schema. Excluding non-null unique keys avoids rewriting queue-bound
unique-job identity.

## Test-first evidence and preserved attempts

The parked semantic-dependent capacity branch remains untouched. The new branch
first adapted only its behavioral reproducer to current-release restricted
network fixtures and normal HTTP commands, before changing runtime behavior.

1. Attempt one failed compilation because a fixture extraction omitted the
   eligible-retained selection. No runtime ran, so it is not behavioral RED.
   `/tmp/undergrid-capacity-current-release-red-20261025.jsonl` and the empty
   `hp2_capacity_red_20261025_0120` database on network 96 are preserved.
2. Attempt two failed normal migration 001 because cluster-global roles already
   existed on network 96. No role/migration was rewritten or deleted and no ledger
   row was fabricated. This is fixture failure, not behavioral RED.
   `/tmp/undergrid-capacity-current-release-red-attempt2-20261025.jsonl` and
   `hp2_capacity_red_20261025_0125` are preserved.
3. Attempt three used a genuine PostgreSQL template copy of the previously
   normally migrated, empty `undergrid_hp1_template` on isolated network 55500.
   **Actual RED: package 27.338 seconds.** Both legacy and aged-fresh restart
   phases missed the unchanged five-second initial-scan bound; only one of the
   two known eligible unattempted retained jobs recovered within five seconds.
   Runtime was unchanged release `c0087c0`.
   `/tmp/undergrid-capacity-current-release-red-attempt3-20261025.jsonl` and
   `hp2_capacity_red_20261025_0128` are preserved. Actual restricted API and worker
   `session_user` values and their non-superuser/non-bypass flags are checked.

4. The first candidate gate was interrupted by workspace restart. Its first
   three cases passed, but the descendant-routing case has no terminal result;
   the package is not counted as GREEN. Its log and database are retained:
   `/tmp/undergrid-capacity-green-attempt1-20261025.jsonl` and
   `hp2_capacity_green_20261025_a1`. Before retrying, static fixture inspection
   corrected the descendant test's assumption that one 256-row age-out batch
   must reach its newly inserted target after 257 HTTP watcher setups. It now
   uses the actual startup drain; no runtime behavior changed for that correction.
5. Candidate attempt two completed all four cases but the package failed,
   65.873 seconds. Backlog isolation, full-row/batch/exclusion preservation and
   persisted descendant routing passed. The actual post-start maintenance case
   recovered 34 of its forty accepted scans inside the unchanged five-second
   bound. At final worker shutdown, cohort job IDs 1009–1048 contained 38
   completed rows and two available, unattempted rows, with no retries or errors;
   the initial setup scan (1003) is not part of that cohort. Database
   `hp2_capacity_green_20261025_a2` and
   `/tmp/undergrid-capacity-green-attempt2-20261025.jsonl` are retained.
   The retained direct lane inherited River's 100-millisecond fetch cooldown
   and one-second periodic polling. Its correction uses one-millisecond
   completion-driven cooldown and 100-millisecond polling while preserving two
   workers, accepted schedules/attempts and the original recovery assertion.

6. The corrected and release-reconciled candidate `d2a476d` passed the complete
   four-case PostgreSQL race gate: **78.297 seconds**, four top-level cases and
   both legacy/restart subcases, zero failures/skips. The original five-second
   initial/result/recovery and twelve-second shutdown assertions remain intact.
   All forty post-start maintenance scans complete after the authority lock is
   released. Whole-row preservation, bounded `256/41/0` batches with locked rows,
   all exclusions and persisted fresh/retained pagination/pair routing also pass.
   `/tmp/undergrid-capacity-green-attempt3-20261025.jsonl` and genuine template
   copy `hp2_capacity_green_20261025_a3` are retained on network 55500.

## Candidate acceptance and remaining integration

The candidate retains the original two-phase five-second scan/result and
twelve-second shutdown assertions and checks all accepted direct/fanout IDs and
arguments. Recovery uses known eligible unattempted work; normal worker retry
backoff is never reset. Additional oracles compare the entire persisted job row
except queue while consumers are paused, prove bounded batches and `SKIP LOCKED`,
exclude running/fresh/unique/unrelated states, exercise actual post-start runtime
maintenance, and dispatch persisted fresh/retained River jobs through paginated
fanout to check descendant scheduling ownership.

Implementation and the focused acceptance gate are proven; affected retained
runtime/HP1 regressions, static checks, review and release integration remain
pending. Controlled accounts prove separate
credential authority, not independent beneficial ownership. No semantic/provider,
real-cohort, source, legal or deployment qualification is established by these
queue tests. HP2 remains open.
