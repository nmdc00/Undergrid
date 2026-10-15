# Discovery queue isolation

Owning requirement: TDD §12.2 operational fairness and §5 durable discovery. This slice implements foreground discovery isolation from recruitment; it does not qualify all public admission, load or dead-letter requirements.

Committed intentions enqueue a cause-specific discovery job and a coalesced watcher refresh atomically with their domain transaction. Two dedicated discovery slots cannot be consumed by recruitment. One separate repair slot refreshes existing watches and performs periodic repair. Recruitment has its own periodic repair on the existing default queue. The River pool has capacity for six queue fetchers plus completion and maintenance. Previously persisted default-queue scan jobs preserve their original watcher sweep and recruitment effects during upgrades.

## Behavioral evidence

- `TestCommittedDiscoverySurvivesBlockedRecruitment` first observes two running recruitment jobs and two tagged worker connections blocked transitively by a controlled principal lock, then commits matching intentions through HTTP and requires discovery within five seconds. Before routing isolation it failed with unrelated recruitment consuming discovery capacity (11.994s including cleanup). Earlier setup failures were fixture mistakes and are not behavioral red evidence. Its running-job count is global; the tagged connection/blocker observations supply the causal link, without directly correlating River rows to backend PIDs.
- `TestCommittedDiscoveryRefreshesExistingWatcher` proves a previously scanned watch discovers a subsequently committed candidate through the actual worker.
- `TestFailedDiscoveryRefreshDoesNotAbsorbNewCommit` persists a delayed retryable refresh and requires an immediately available follow-up after a new command. The initial coalescer failed (0.496s); available-only coalescing corrected it. This is a persisted retry-state component scenario, not a claim that a worker failure was induced.
- `TestLegacyDefaultDiscoveryJobPreservesWatcherAndRecruitment` persists a release-shaped default-queue job, invokes its registered worker entrypoint, and observes both old-watch and recruitment output. The cause-only implementation failed with an empty watch (1.165s); compatibility routing corrected it. This test invokes the worker directly; the other discovery tests and unchanged durable-worker test execute River dispatch.

Combined focused race tests, including the unchanged durable discovery and shutdown assertions, passed in 39.918s on network-only UTF-8 PostgreSQL port 55453. Command: `go test -race ./internal/network -run 'TestCommittedDiscovery|TestFailedDiscoveryRefresh|TestLegacyDefaultDiscovery|TestCommittedIntentionRunsThroughDurableWorker|TestWorkerShutdown' -count=1 -timeout=2m`. `go vet ./...` passed. `go build ./...` passed. The complete network race suite passed in 268.286s on the same loaded fixture without resetting queues or extending the durable discovery assertion: `go test -race ./internal/network -count=1 -timeout=20m`, implementation SHA `b667170adfd314605ee4fc2757bacc4efaf2daa1`. The subsequent handoff commit changes only this evidence document.

## Loaded-fixture setup correction

The later full network race gate on the integrated grants fixture ran for
636.248s and failed only because `TestCommittedDiscoverySurvivesBlockedRecruitment`
could not establish its controlled lock precondition; the unchanged durable
discovery worker assertion and remaining tests passed. On that loaded fixture,
the default queue had over 1,000 available priority-1 rows. The test now chooses
the first active public recruiting source-catalog agent ordered by agent ID,
excluding its own coordinator and the two new intent owners, then locks that
candidate principal. It inserts exactly two test-owned recruitment jobs for
the test room at priority 1 and schedules them one second before the earliest
due priority-1 default job across all states. It records those two job IDs
and requires both rows to be durably running plus two tagged worker connections
blocked transitively on the exact locker PID. No existing queue rows are updated
or cleared.

The first local correction attempt exposed an accidental same-connection
transaction interaction: beginning the job transaction on the connection
already holding the principal lock was not nested, and committing the job
transaction released that lock. Moving the inserts to a separate connection
keeps queue setup committed before the lock transaction begins. Once that
precondition passed, the earlier oracle also showed a source-order race: its
first intent scan could finish before the matching peer committed. The test now
commits the candidate first and the observed source second, then polls that
second intent. This preserves both HTTP commits after the blocked-recruitment
precondition and the original five-second discovery deadline while proving the
new cause-specific foreground job sees a committed candidate.

`testWorkerDSN` is shared with `TestWorkerShutdownCancelsBlockedTransactionAndPreservesQueuedWork`;
it adds `application_name` through URL query parsing or keyword-DSN parameters.
Appending a keyword to a URL had prevented the controlled worker from starting
with a valid tagged connection. Both DSN forms now pass the controlled fairness
test and shared shutdown test under `go test -race` on the same preserved UTF8
PostgreSQL 16 cluster at port 55449. That database was not reset: at handoff its
network migration ledger contained SQL migrations 001–015 plus the River-ready
entry (16 ledger rows total), with no 016 row. The focused tests pass after the
fixture correction; the complete network race suite has not been rerun after it.

A subsequent full race run on a fresh preserved fixture failed the same
controlled precondition after 304.731s (0/2 controlled jobs running, 2 tagged
connections blocked). The all-state ordering fence alone did not fix it: the
focused race test remained red at 16.273s. Inspection of River v0.49.0's insert
path showed why: a nonzero `InsertOpts.ScheduledAt` makes a newly inserted job
`scheduled` even when that timestamp is already in the past. The test now
inserts both controlled rows without `ScheduledAt` and asserts River reports
them as `available`; in the same transaction it updates only those two new
rows' `scheduled_at` values to one second before the all-state ordering fence.
It asserts exactly two rows were updated before commit. This leaves historical
backlog rows and domain scheduling policy untouched, while keeping the lock
precondition independent of a scheduler promotion race. Timeout diagnostics
include the controlled row IDs and room ID. The focused race test then passed in
7.674s on the same preserved loaded UTF-8 PostgreSQL 16 fixture at 55463:
`GOCACHE=/tmp/undergrid-gocache GOMODCACHE=/tmp/undergrid-gomod UNDERGRID_TEST_DATABASE_URL='postgres://nmcdc@127.0.0.1:55463/postgres?sslmode=disable' /tmp/undergrid-toolchain/go/bin/go test -race ./internal/network -run '^TestCommittedDiscoverySurvivesBlockedRecruitment$' -count=1 -timeout=2m`.
No existing queue state was changed or cleared. The focused result does not
replace the pending complete network race gate.
The combined focused race selector, which also includes the unchanged
`TestWorkerShutdownCancelsBlockedTransactionAndPreservesQueuedWork`, passed on
the same loaded fixture in both supported DSN forms: URL DSN in 13.577s and
keyword DSN in 15.137s. Both used the exact selector
`^(TestCommittedDiscoverySurvivesBlockedRecruitment|TestWorkerShutdownCancelsBlockedTransactionAndPreservesQueuedWork)$`.

## Quality and recovery

Matching and recruitment retain their domain transactions and independent effects; queue routing changes no identity, rights or candidate policy. The existing matching implementation is shared by foreground and repair paths. A transaction-scoped advisory lock serializes refresh coalescing across processes, and a queued-row lock prevents a worker consuming its wakeup before the originating command commits. Running, delayed or failed refreshes cannot absorb a new immediate signal. Retry deduplication remains in existing domain persistence; no command executes agent content. Legacy jobs retain compatibility through an explicit queue check rather than rewriting stored jobs.

Code review identified and corrected the legacy-job regression; final read-only review found no substantive blocker. This proves the observed scheduling seam, not provider qualification, host resource qualification, arbitrary-cohort throughput or complete implementation. Financial databases, credentials and deployment identities are unchanged. No deployment occurred.

## Existing-watcher latency follow-up

The later exact release gate exposed a real existing-watch latency miss even with recruitment isolated: a committed candidate reached its earlier watcher after7.130s. The old global-refresh lane was the remaining serial dependency. Current candidate-directed bounded refresh adds a dedicated two-slot `intention_watch_refresh` queue, an independent one-slot `intention_watch_fanout` queue, eight queue fetchers and a16-connection pool. Modern coalesced refresh is catchup without globally locking/resetting all watches; legacy default jobs retain their old effects. See `docs/evidence/committed-watch-refresh.md` for the exact-candidate five-second outage/lock/recovery evidence and its qualification limits. Earlier timings above remain scoped to their historical implementations.
