# Discovery queue isolation

Owning requirement: TDD §12.2 operational fairness and §5 durable discovery. This slice implements foreground discovery isolation from recruitment; it does not qualify all public admission, load or dead-letter requirements.

Committed intentions enqueue a cause-specific discovery job and a coalesced watcher refresh atomically with their domain transaction. Two dedicated discovery slots cannot be consumed by recruitment. One separate repair slot refreshes existing watches and performs periodic repair. Recruitment has its own periodic repair on the existing default queue. The River pool has capacity for six queue fetchers plus completion and maintenance. Previously persisted default-queue scan jobs preserve their original watcher sweep and recruitment effects during upgrades.

## Behavioral evidence

- `TestCommittedDiscoverySurvivesBlockedRecruitment` first observes two running recruitment jobs and two tagged worker connections blocked transitively by a controlled principal lock, then commits matching intentions through HTTP and requires discovery within five seconds. Before routing isolation it failed with unrelated recruitment consuming discovery capacity (11.994s including cleanup). Earlier setup failures were fixture mistakes and are not behavioral red evidence. Its running-job count is global; the tagged connection/blocker observations supply the causal link, without directly correlating River rows to backend PIDs.
- `TestCommittedDiscoveryRefreshesExistingWatcher` proves a previously scanned watch discovers a subsequently committed candidate through the actual worker.
- `TestFailedDiscoveryRefreshDoesNotAbsorbNewCommit` persists a delayed retryable refresh and requires an immediately available follow-up after a new command. The initial coalescer failed (0.496s); available-only coalescing corrected it. This is a persisted retry-state component scenario, not a claim that a worker failure was induced.
- `TestLegacyDefaultDiscoveryJobPreservesWatcherAndRecruitment` persists a release-shaped default-queue job, invokes its registered worker entrypoint, and observes both old-watch and recruitment output. The cause-only implementation failed with an empty watch (1.165s); compatibility routing corrected it. This test invokes the worker directly; the other discovery tests and unchanged durable-worker test execute River dispatch.

Combined focused race tests, including the unchanged durable discovery and shutdown assertions, passed in 39.918s on network-only UTF-8 PostgreSQL port 55453. Command: `go test -race ./internal/network -run 'TestCommittedDiscovery|TestFailedDiscoveryRefresh|TestLegacyDefaultDiscovery|TestCommittedIntentionRunsThroughDurableWorker|TestWorkerShutdown' -count=1 -timeout=2m`. `go vet ./...` passed. Full network race and build results are recorded with the feature handoff before merge.

## Quality and recovery

Matching and recruitment retain their domain transactions and independent effects; queue routing changes no identity, rights or candidate policy. The existing matching implementation is shared by foreground and repair paths. A transaction-scoped advisory lock serializes refresh coalescing across processes, and a queued-row lock prevents a worker consuming its wakeup before the originating command commits. Running, delayed or failed refreshes cannot absorb a new immediate signal. Retry deduplication remains in existing domain persistence; no command executes agent content. Legacy jobs retain compatibility through an explicit queue check rather than rewriting stored jobs.

Code review identified and corrected the legacy-job regression; final read-only review found no substantive blocker. This proves the observed scheduling seam, not provider qualification, host resource qualification, arbitrary-cohort throughput or complete implementation. Financial databases, credentials and deployment identities are unchanged. No deployment occurred.
