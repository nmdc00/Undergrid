# HP1 milestone integration check

Release source: `f8ad17ede69a2ff3751342ae1ec882f68f0b748c`, after reviewed PR94.
Checked on 24 October 2026. This validates the merged source/worker seams and
console build; it does not close HP1 or repeat the complete browser journey.
The browser milestone and its controlled-provider boundaries remain recorded in
[`HP1_ACCEPTANCE.md`](../HP1_ACCEPTANCE.md).

## Actual release checks

The network fixture was the preserved PostgreSQL instance on port 55496, database
`postgres`, with its normal ordered migrations through 040. No financial runtime,
database or credential participated. Both retained network fixture instances were
restarted with their original ports and socket directories after session restart;
no database was reset or product data seeded.

```sh
go test -race -json -count=1 -timeout=3m ./internal/network \
  -run '^Test(SourceRefreshPermittedLifecycle|SourceWorkerSlowSourceDoesNotBlockHealthySource|SourceWorkerCompletedJobsDoNotSuppressRefresh|WorkerShutdownCancelsBlockedTransactionAndPreservesQueuedWork)$'
```

Using the cached Go toolchain and the isolated network fixture URL, all four
top-level tests passed with zero failures or skips, package duration 13.883
seconds. Output: `/tmp/undergrid-release-f8ad17e-source-worker96-attempt2.jsonl`.
The lifecycle assertion observes the default 21,600-second persisted refresh
interval; the worker checks cover source queue progress, repeated refresh and
bounded shutdown preserving queued work.

The earlier invocation's process handle disappeared during session restart and
its log was empty. It has no result and is not counted as a pass or behavioral
failure. The second invocation began only after confirming that no test process
remained and restoring the existing fixture servers.

`npm run build` in the release console passed TypeScript checking and Vite
production bundling (Vite duration 1.70 seconds). Output:
`/tmp/undergrid-release-f8ad17e-web-build.log`. The release worktree remained clean.
This build is not a replacement for the coupled source/artifact manifest check
required by the live browser gate.

## Remaining acceptance

The partial-satisfaction feature is not part of this SHA. Its owner control,
complete response validation, current source authorization, protocol recovery,
constraint-preserving revision and private audit must finish review and merge.
The final continuous HP1 journey must then include that action and prove visible
unmet requirements, later notifications, receipt/dismissal and restart recovery
against the integrated source. External identity/source/legal/deployment and
unfamiliar-owner usefulness qualifications remain separate, unproven gates.
