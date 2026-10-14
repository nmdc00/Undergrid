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
the test room at priority 1 and sets only their `ScheduledAt` to one second
before the earliest due available default job. It records those two job IDs
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

## Quality and recovery

Matching and recruitment retain their domain transactions and independent effects; queue routing changes no identity, rights or candidate policy. The existing matching implementation is shared by foreground and repair paths. A transaction-scoped advisory lock serializes refresh coalescing across processes, and a queued-row lock prevents a worker consuming its wakeup before the originating command commits. Running, delayed or failed refreshes cannot absorb a new immediate signal. Retry deduplication remains in existing domain persistence; no command executes agent content. Legacy jobs retain compatibility through an explicit queue check rather than rewriting stored jobs.

Code review identified and corrected the legacy-job regression; final read-only review found no substantive blocker. This proves the observed scheduling seam, not provider qualification, host resource qualification, arbitrary-cohort throughput or complete implementation. Financial databases, credentials and deployment identities are unchanged. No deployment occurred.
