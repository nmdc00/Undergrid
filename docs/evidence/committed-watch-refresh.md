# Prompt refresh of an already scanned intention watch

TDD §§5.4/6; implementation `cd02dd5`, no schema migration. This is a network worker scheduling correction, not a new semantic matching/provider qualification claim.

## Observed defect and behavior

The full release7b645fb network race gate failed1047.626s on the previously scanned watcher assertion. Persisted opportunity creation was7.130s after the later candidate's creation, exceeding the existing five-second contract. Root's focused retained-fixture reproduction also failed15.689s. The prior foreground job scanned the newly committed source only; previously scanned watchers depended on a global repair lane that serially visited up to1000 watches. Its coalesced global timestamp reset could also couple unrelated row locks.

Committed intention changes now atomically enqueue a candidate refresh in addition to the original cause-specific discovery and coalesced repair jobs. A dedicated `intention_watch_refresh` queue has two slots. A fanout job reads at most100 current watchers, compares each with the current approved public candidate through the existing single `CompareDemand` policy, and transactionally inserts only compatible pair jobs plus a durable cursor continuation. Pair jobs have priority1; fanout/continuation jobs priority2. There is no synchronous request scan and no private candidate projection is published.

Each pair reuses the existing scan transaction, restricted to its exact candidate. Current source principal, agent, active watch, expiry, revision and candidate public eligibility are rechecked. Owner locks retain principal→agent→intention order. Urgent lock waits are bounded to100ms; lock contention snoozes the durable job250ms without exhausting normal failure attempts. A short wait behind ordinary HTTP reads avoids polling starvation; long unrelated owner holds cannot occupy an urgent worker indefinitely. The pair does not advance a full-scan clock, because matching one candidate does not prove all candidates were scanned.

Modern coalesced refresh remains catchup over independently committed watches, without globally resetting/locking every watch row. Periodic due repair stays independent. Previously persisted default-queue jobs retain their original full watcher-refresh/sweep/recruitment effects. Running/delayed/failed refresh jobs still cannot absorb a new committed command's immediate signal. River now serves seven queues with a16-connection maintenance/fetch pool; the foreground discovery and repair/recruitment isolation remains intact.

## Behavioral proof

The strengthened `TestCommittedDiscoveryRefreshesExistingWatcher` temporarily pauses the repair queue and restores its original pause state afterward. No jobs, backlog, revisions or migration ledger are deleted/reset. The test first proves both existing watches were scanned. One watch is private and its owner row is held in a separate transaction. The exact newly committed public candidate must appear to the unrelated old public watcher within the original five seconds despite the unavailable repair lane and held other owner. The private watch must never become a public candidate. The exact opportunity has one persisted opportunity/event/inbox effect. The locked watch retains durable pair work; releasing the lock must produce its authorized exact match and complete that pair within five seconds. Worker shutdown retains the original12-second bound.

Before implementation, the controlled repair-outage oracle failed10.959s because the five-second exact candidate match never arrived. An intermediate NOWAIT approach exposed a separate recovery defect: frequent authorized HTTP polling could repeatedly win the owner lock, and the locked pair missed the five-second recovery gate. Bounded100ms lock waits corrected this; the expanded retained-fixture test passed9.301s. An initial refactor incorrectly broadened a joined row lock to the immutable revision table; the worker privilege failure was corrected to the original `FOR UPDATE OF i` scope before final validation.

At exact implementationcd02dd5, fresh whole network fixture55488 (`/tmp/undergrid-pg-watch-refresh-55488/data`, socket sibling) applied normal raw001–021 plus River0.49.0 on PostgreSQL16.11 UTF8/C.UTF-8. No omitted role creation, SQL replay or ledger edits. The app/worker group roles remain NOLOGIN/nonprivileged; fixture migration login is the bootstrap owner, so this does not qualify production identity separation. Seven affected discovery/retry/legacy/shutdown race tests passed48.927s:

`go test -race ./internal/network -run 'TestCommittedDiscovery|TestLegacyDefaultDiscovery|TestFailedDiscoveryRefresh|TestDiscoveryRefreshCoalesces|TestWorkerShutdown|TestCommittedIntentionRuns' -count=1 -timeout=5m`

Standing public demand, immutable hard constraints and restricted sharing/privacy regressions also passed7.202s on55488. Network/command `go vet`, command build and diff checks passed. The same seven-case gate on preserved loaded55463 is pending; its result is required before this branch is reported ready.

## Pragmatic assessment and limits

The compatibility rule and normalized opportunity/notification transaction are shared rather than copied into a new matcher. Fanout is a bounded, read-only eligibility filter; effect-bearing pairs independently recheck current state under the existing owner locks. Cursor continuation and child enqueue commit together, while unique opportunity input hashes dedupe retries and notification effects. Job payloads carry IDs/cursors, not agent executable content. Connections, transactions and rows have explicit cleanup; unexpected database errors propagate rather than silently dropping work.

This proves the selected bounded scheduling, lock-recovery and privacy seams on fresh/retained local fixtures. It does not prove arbitrary-load SLA, external provider/source qualification, beneficial-owner independence, full TDD completion, main/develop promotion or production deployment. No financial database, credential or deployment identity is changed.
