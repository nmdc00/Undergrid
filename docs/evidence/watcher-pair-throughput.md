# Drain obsolete durable watcher pairs without owner waits

TDD §§5/6 retains the five-second committed-candidate-to-existing-watcher assertion and twelve-second shutdown assertion. The recruitment integration check exposed a separate pair-queue throughput defect; this branch changes no quotas, schema, projection rules or financial identities.

## Observed failures

The retained55463 recruitment/worker race gate failed in83.047s on the existing-watcher assertion. At completion189 older pair jobs were still available. Both bounded pages of the new candidate's fanout had completed around13:38:21, while the new candidate's pair jobs had never been attempted. Attempts were instead consuming older synthetic withdrawn watches from the deterministic pagination fixture. All rows/jobs were preserved; queues were not paused.

Reducing only the pair queue fetch cooldown from100ms to1ms, retaining two worker slots and100ms idle polling, was insufficient: the unchanged watcher case failed in11.701s (`/tmp/undergrid-watcher-throughput-backlog.log`), leaving105 older available pairs. Per-job completion took80–170ms. A later combined run with the denial-only precheck below still failed the watcher assertion in12.28s, with63 available pairs left, while obsolete no-op tests passed (`/tmp/undergrid-watcher-throughput-precheck.log`). These runs used basec621ed9, whose worker still included the old unrestricted recruitment broadcast; concurrent source validation also generated host filesystem/WAL load. Neither possibility is claimed as a proven sole cause. An integrated quiet-host timing check including bounded recruitment is required before declaring the five-second contract restored.

## Behavior and authorization boundary

Pair execution now performs one read-only, denial-only eligibility query before opening the original authority transaction: the watcher must currently be active, monitoring-enabled and unexpired, with active owner and agent, and the candidate must currently satisfy `public_intents()` eligibility. A false result completes the obsolete durable job without waiting on unrelated owner locks. No queue row is manually deleted, discarded or reprioritized.

A true result still runs `scanIntentCandidate` unchanged, including its principal→agent→intention locking,100ms bounded lock wait, current authorization/projection checks and atomic opportunity/event/inbox effects. The preliminary read cannot grant authority or override revocation. A concurrently withdrawn/revoked candidate remains rejected by the existing transaction. Mutable false states do not reserve or cancel future work: published revisions, activation and renewal retain their command-specific enqueue paths, and periodic due repair remains the catchup path. In particular, resuming a saved watch must restore its requested monitoring state; the separate watch-resume feature owns that correction.

`TestObsoleteWatchPairDoesNotWaitOnOwner` uses actual HTTP-created intentions and a real held principal row. It separately withdraws the watcher through its versioned public command or creates a private candidate. Both obsolete pair executions must return successfully without owner-lock snooze and without opportunities. Before the precheck, both cases failed, returning `JobSnoozeError:250ms`; aggregate4.743s (`/tmp/undergrid-watcher-obsolete-red.log`). Afterward both passed with race in2.82s inside the otherwise failing retained gate. This proves obsolete-pair lock avoidance; it does not prove the broader queue SLA.

## Quality and current status

The existing locked scanner remains the only effect-bearing authority/matching implementation. Repeating current-state checks at the early denial boundary is defensive validation, not a second grant policy. Work concurrency remains two pair slots. Fast refill under an active backlog does not change100ms idle polling or produce unbounded effects. Connections close, errors propagate and the same durable retry/snooze path handles current eligible pairs. Source jobs, migrations, stored timestamps and retained fixture history are unchanged.

Root owns integration with the bounded-recruitment/source/watch-resume verification branch and the decisive retained55463 timing gate after competing test processes end. This evidence intentionally records the failed old-base gates. No passing integrated release, main/develop promotion, deployment or full-TDD completion is claimed.
