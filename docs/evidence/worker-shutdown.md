# Bounded worker shutdown

Scope: runtime failure recovery, TDD r2 §§9 and 12. Worker shutdown must cancel unfinished domain work, retain committed intentions and queue entries, and finish recording job state before its pool closes. This changes no payment/provider path.

Observed on develop `89c94fa`: the coverage/race suite returned `context deadline exceeded` from RunWorker's shutdown and logged a job completer using a closed pool. The ordinary committed-intention lifecycle failed the same way in three repetitions on the accumulated disposable fixture (291 intentions / 587 queued jobs at inspection). No production runtime was exercised.

Deterministic red: a real PostgreSQL ACCESS EXCLUSIVE lock on principals blocked a running worker domain transaction. `TestWorkerShutdownCancelsBlockedTransactionAndPreservesQueuedWork` failed after 10.50s with the same shutdown deadline. Successful request completion alone had not proved drain completion.

Fix: configure River 0.49.0 SoftStopTimeout to five seconds, retaining the outer ten-second shutdown deadline. Jobs can finish normally; after the grace window River cancels remaining job contexts and completes queue bookkeeping before RunWorker closes the pool. The existing transaction-per-intention design rolls back interrupted work while preserving acknowledged writes and deduplicated repair/retry behavior. No increased timeout, skipped test or swallowed error is used.

Green: ordinary committed-intention plus blocked-transaction lifecycles passed three repetitions under the race detector (80.829s total). The stronger final regression records first-class running job IDs, verifies none remains running after shutdown, verifies the acknowledged intention remains active at version one, and verifies its committed queue entry is retained. The full configured all-package race/coverage suite then passed (network 52.728s, sandbox 3.665s); vet/build/diff checks also passed.

Quality assessment: the queue library owns graceful cancellation rather than an added generic shutdown framework. The grace window is strictly inside the existing outer bound; error propagation and pool ownership remain explicit. The test drives actual PostgreSQL locks and normalized intention/job persistence, not mocked Stop calls. Test data stays separate from provider evidence. Database outage reconciliation and deployment qualification remain external operational work; no public release completeness or deployment is claimed.
