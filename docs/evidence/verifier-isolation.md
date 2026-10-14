# Bubblewrap verifier process isolation

Scope: TDD r2 §§8.3 and 12, DEL-02. This change adds a separately configured Linux verifier runner to the existing development artifact adapter. It does not qualify the local filesystem object store for production, attest a container image, or complete provider/source/legal approval gates.

## Runtime contract

`UNDERGRID_ISOLATED_VERIFIER_CONFIG_FILE` selects `IsolatedVerifierConfig`; it takes precedence over the older `UNDERGRID_LOCAL_ARTIFACT_CONFIG_FILE`. Configuration pins the verifier, root-owned `bwrap` and `prlimit` binaries by SHA-256 and names a service-owned delegated cgroup-v2 root. The verifier must be a static executable. The isolated setter clears any earlier local-process runtime before validation and leaves artifact processing unavailable if any requirement fails. Production continues to reject the local artifact adapter.

For each job the parent reopens, hashes and copies all three trusted executables into sealed Linux memfds. It passes only those fixed descriptors to `prlimit`/`bwrap`; only the sealed verifier is mounted read-only as `/verifier`. The job arrives on stdin as the already sealed artifact JSON and manifest. No submitted path, program, environment, argument, URL, or schema selects process behavior. The verifier closes inherited descriptors before reading job bytes.

Bubblewrap requires user, PID, IPC, UTS, cgroup and network namespaces, disables nested user namespaces, starts a new session, drops every capability, and uses `--die-with-parent`. Its filesystem contains fresh `/proc` and `/dev`, a 16 MiB private `/tmp`, a separate 16 MiB private working tmpfs, and the single read-only verifier file. It receives a clean environment with only the operator digest attestation and trusted executable hash.

The kernel places the launcher into a new cgroup v2 at process creation (`clone3` `CLONE_INTO_CGROUP`), before `prlimit` or the verifier can run. Qualification requires delegated, enabled `cpu`, `memory`, and `pids` controllers and `cgroup.kill`. Each job sets `cpu.max=100000 100000`, `memory.max=268435456`, `memory.swap.max=0`, and `pids.max=32`. `prlimit` adds a 30-second CPU ceiling, 64 KiB file-size ceiling, 16-process ceiling and 32 open-file ceiling. The parent applies a 30-second wall deadline, kills the cgroup on cancellation, and bounds stdout and stderr separately to 64 KiB. Cgroup cleanup waits for an empty group and fails the job if cleanup cannot be confirmed. Unsupported clone3, namespace setup, cgroup controls, memfd sealing, or runner hash changes fail closed.

Evidence added by the parent after successful execution records `runtime_isolation`, the pinned bubblewrap and prlimit hashes, and the enforced limit profile. The operator's configured `image_sha256` remains distinct and is labeled `operator_configured_digest_attestation_not_runtime_attestation`; this process runner does not inspect a container image. These facts are stored both in immutable evaluation JSON and first-class `evaluations` columns (migration 018). The evaluation HTTP read path checks JSON/column parity when a raw field exists and projects normalized legacy labels for older rows without rewriting their evidence, so later runtime configuration cannot relabel historical evaluations.

## Evidence and current gate

- `TestBubblewrapProcessBoundaryWithoutQualification` builds a trusted test probe and launches it under actual bubblewrap namespaces. The probe confirms that `/etc/passwd` and `/root` are absent, the inherited `HOME`/database environment is unavailable, a live parent-loopback listener cannot be reached, `/tmp` is bounded, capabilities are zero, and launcher descriptors are closed before job bytes are read. This proves those process/filesystem/network behaviors only; it does not qualify resource limits.
- `TestVerifierAndLauncherBytesAreSealedBeforeExecution` proves the copied executable cannot be written after sealing. `TestSealedVerifierExecutesAndReportsItsCopiedBinaryHash` launches the real fixed verifier from that sealed copy and checks the exact executable and input hashes.
- `TestIsolatedVerifierConfigurationFailsClosedWithoutDelegatedCgroups` confirms that absent host delegation removes a preconfigured development-process runner instead of falling back.
- `TestArtifactPublicationLifecycleThroughHTTPAndExecutableWorker` runs the real development worker against PostgreSQL and checks the durable evaluation through HTTP and normalized columns, alongside acceptance/publication authority. The persisted mode remains `development_process_not_qualified`; the operator digest is not represented as measured image identity.
- `TestBubblewrapVerifierIsolationEnforcesFilesystemNetworkAndLimits` and `TestBubblewrapEvaluationEvidencePersistsThroughWorkerAndHTTP` are the required successful cgroup qualification and worker lifecycle contracts. They explicitly skip as unavailable in an ordinary development run when cgroup delegation is absent. Setting `UNDERGRID_REQUIRE_VERIFIER_ISOLATION=1` turns that condition into a test failure for qualification CI/release checks.

On this host, `/sys/fs/cgroup` is not a service-owned delegated root and the `cpu`, `memory`, and `pids` subtree controllers are not enabled. The successful hard-limit runner and isolated worker evaluation lifecycle therefore remain unproven here. The required qualification command must fail on this host; fixture cgroup files or a manually launched bubblewrap probe cannot satisfy it. Do not promote this branch or describe verifier isolation as qualified until the required-mode tests pass on a host with real delegated cgroup-v2 enforcement.

Image/provider identity, S3 storage, production deployment, external ownership/rights truth, and source/provider permissions remain separate gates. Test probes and synthetic artifacts are not provider, owner, beneficial-owner, or image attestations.

## Review and verification record

- Red: the new legacy-evidence lifecycle assertion returned HTTP 500 because the old read path demanded newly added labels inside preexisting raw JSON.
- Green: `TestArtifactPublicationLifecycleThroughHTTPAndExecutableWorker` passed through the PostgreSQL worker and HTTP read path after the read path began projecting normalized legacy labels without updating raw evidence. The test compares the stored JSON before and after the GET byte-for-byte.
- Red/green: the same lifecycle removes a runtime label from a modern row; the permissive projection returned 200, then the stricter per-evaluation parity check returned 500. It restores the fixture bytes before exercising the legacy projection.
- Red: `TestMalformedIsolatedEnvironmentConfigClearsEarlierLocalRunner` showed that malformed higher-priority isolated configuration returned an error but retained an earlier local runner.
- Green: the environment loader now clears the prior runner as soon as isolated configuration is selected; the regression passes.
- Green: the actual bubblewrap namespace probe, sealed executable immutability and execution, and fail-closed cgroup configuration checks passed on the available host. The probe is trusted compiled fixture code and establishes only the observed OS boundary.
- Required gate: running `TestBubblewrapVerifierIsolationEnforcesFilesystemNetworkAndLimits` with `UNDERGRID_REQUIRE_VERIFIER_ISOLATION=1` fails as required. `/sys/fs/cgroup` is root-owned mode 0755; its `cpu`, `memory`, and `pids` controllers exist but `cgroup.subtree_control` is empty. The test cannot qualify kernel-enforced resource limits here.
- Race check: the focused worker lifecycle, namespace probe, sealed executable checks, and both fail-closed configuration regressions passed with `go test -race`.
- Static/build: `go vet ./internal/network ./internal/verification ./cmd/undergrid-verifier`, `go build ./cmd/...`, `gofmt -l` on tracked and added Go files, and `git diff --check` passed.

## Old-to-new database migration proof

On 2026-10-14, a separate fresh UTF-8, network-only PostgreSQL cluster was initialized at port 55456. The clean release worktree at `4df9cfd6b701c6cff7608a460e93220addb89360` ran its unmodified `TestArtifactPublicationLifecycleThroughHTTPAndExecutableWorker` with `go test -race` (16.481 seconds). Its application `Migrate` recorded all 14 migrations present at that release (`001`–`014`), then the real worker/API lifecycle wrote one evaluation at `2026-10-14T11:06:50.550942963Z`. The evaluation row was `c88990da-55b5-4b2e-b0c9-effefcf70cd5`; its raw `evidence::text` MD5 was `9373d7f5798529b8054ec9286739f105` (543 bytes), with no `runtime_isolation` field.

The same untouched database then ran the feature worktree's `TestArtifactPublicationLifecycleThroughHTTPAndExecutableWorker` with `go test -race` (6.739 seconds). The fixture called the application `Migrate`, which applied the unmodified migration 018 after the recorded `001`–`014` migrations. The old row kept the same raw evidence MD5 and length, and the raw field remained absent. Its new columns received `legacy_unqualified_evidence`, `not_recorded_for_legacy_evaluation`, and `legacy_configured_digest_unverified`. The focused test also exercised a new worker evaluation and verified that modern HTTP evidence matched its normalized columns before the test separately exercised legacy projection. Both API and worker login-role assertions passed; the recorded `undergrid_artifact_api_runner` and `undergrid_artifact_worker_runner` roles had `rolsuper=false` and `rolbypassrls=false` before and after the upgrade.

The database recorded 14 migration files after the release run and 15 after the feature run: `001`–`014` plus `018_verifier_isolation_evidence.sql`. Files 015–017 are reserved for sibling features and are absent from these two worktrees. No migration SQL was copied, edited, or omitted in either application migration run. The 55456 fixture remains available for integration review.

Commands used, with both invocations run against `postgres://nmcdc@127.0.0.1:55456/undergrid_test?sslmode=disable`:

```sh
GOCACHE=/tmp/undergrid-gocache GOMODCACHE=/tmp/undergrid-gomod UNDERGRID_TEST_DATABASE_URL='postgres://nmcdc@127.0.0.1:55456/undergrid_test?sslmode=disable' /tmp/undergrid-toolchain/go/bin/go test -race ./internal/network -run '^TestArtifactPublicationLifecycleThroughHTTPAndExecutableWorker$' -count=1 -timeout=6m
```

The first invocation ran from the clean release worktree; the second ran from this feature worktree. This proves the old-to-new migration preserved an existing raw evaluation and that the new lifecycle uses non-superuser, non-BYPASSRLS API/worker logins. It does not qualify the bubblewrap cgroup resource gate or production object storage.

## Combined sibling migrations and runtime validation

After local integration of private-artifact grants, task-progress evidence, and owner issuance recovery, the merged verifier branch ran focused race validation on a fresh UTF-8 network-only PostgreSQL 16 cluster at port 55464 (`undergrid_test`). The tests invoke the unmodified application `network.Migrate`. The recorded migration ledger contains every network migration `001`–`018` in order, including sibling migrations `015`–`017` and verifier migration `018`; no migration SQL was copied, skipped, or reordered. `pg_database` reported the database encoding as UTF8.

The selector exercised the ordinary artifact publication lifecycle through the real worker and HTTP APIs, progress-artifact audience and reviewer privacy, private input-grant reads and exact rights, serialized revocation, owner issuance replay/concurrency/protected-scope policy, and verifier configuration, sealed-executable, and namespace regressions. It passed under `go test -race` in 68.606 seconds. The publication lifecycle checked modern evidence against normalized evaluation columns and exercised legacy-label projection after removing new fields from the raw JSON; the raw JSON bytes were unchanged by the HTTP read. The real worker recorded `development_process_not_qualified`, kept the operator-configured image digest distinct from runtime image identity, and persisted matching normalized columns. The same fixture contained one legacy-default evaluation row, one progress artifact, two task-progress evidence rows, and 18 grant approvals after the focused tests.

Database inspection confirmed `undergrid_app` and `undergrid_worker` are NOLOGIN, NOSUPERUSER, and NOBYPASSRLS. The separately used `undergrid_artifact_api_runner` and `undergrid_artifact_worker_runner` are LOGIN, NOSUPERUSER, and NOBYPASSRLS. The focused API/worker lifecycle tests connect through these restricted login roles and switch only to their corresponding app/worker role. Progress audience and grant tests passed against the combined 015→016 schema, including the restrictive audience policy, grant read policy, and approval constraint extensions.

The required hard-cgroup qualification was run separately with `UNDERGRID_REQUIRE_VERIFIER_ISOLATION=1`; it failed as required in 0.382 seconds with `required isolated verifier qualification unavailable: delegated cgroup root must be owned by the service and not group/world writable`. No cgroup or host-global policy was changed. This host therefore still has no successful kernel-enforced resource-limit qualification. The focused run does not replace the broader network/fairness suite or establish production storage, provider, legal, or deployment qualification.

Commands:

```sh
GOCACHE=/tmp/undergrid-gocache GOMODCACHE=/tmp/undergrid-gomod \
UNDERGRID_TEST_DATABASE_URL='postgres://nmcdc@127.0.0.1:55464/undergrid_test?sslmode=disable' \
/tmp/undergrid-toolchain/go/bin/go test -race ./internal/network \
-run '^(TestArtifactPublicationLifecycleThroughHTTPAndExecutableWorker|TestTaskProgressEvidenceReviewLifecycle|TestPrivateArtifactNamedGrantsExposeOnlyInputAndPermitAssembly|TestPrivateArtifactGrantRejectsWrongExactScopes|TestPrivateArtifactDerivedPublicationStillNeedsAllExactRightsConsent|TestPrivateArtifactGrantRevocationSerializesWithReadAndSeal|TestOwnerAgentIssuanceRequiresPolicyForProtectedArtifactGrantScope|TestOwnerAgentCreationResponseLossReplay|TestOwnerAgentCreationWithoutIssuanceKeyRollsBack|TestOwnerAgentCreationReplayWindowAndKeyIDFailClosed|TestOwnerAgentCreationConcurrentSameKey|TestMalformedIsolatedEnvironmentConfigClearsEarlierLocalRunner|TestIsolatedVerifierConfigurationFailsClosedWithoutDelegatedCgroups|TestBubblewrapProcessBoundaryWithoutQualification|TestVerifierAndLauncherBytesAreSealedBeforeExecution|TestSealedVerifierExecutesAndReportsItsCopiedBinaryHash)$' \
-count=1 -timeout=12m

GOCACHE=/tmp/undergrid-gocache GOMODCACHE=/tmp/undergrid-gomod \
UNDERGRID_REQUIRE_VERIFIER_ISOLATION=1 \
/tmp/undergrid-toolchain/go/bin/go test ./internal/network \
-run '^TestBubblewrapVerifierIsolationEnforcesFilesystemNetworkAndLimits$' -count=1
```

Quality review: pinned executable validation and process/cgroup lifecycle now live in `verifier_runner.go`, separate from local artifact storage. Configuration, worker writes, normalized evidence, and the HTTP projection retain concrete contracts. The runner closes sealed files and cgroup descriptors, kills the whole group on cancellation, waits for an empty cgroup before removal, and turns cleanup uncertainty into a failed job. No agent-controlled process attributes enter the launch. The remaining resource-limit uncertainty is an explicit host qualification gate, not a fixture claim.

## Final release reconciliation proof (2026-10-14)

Implementation/merged source `5ae3269cf4eaa436d6fe68a422f04ed80ced3bfd` includes release `06fc773` and final progress/owner migrations. The existing interrupted-run fixture at `/tmp/undergrid-pg-verifier-55474` was inspected before reuse; its ledger already contained unmodified raw network `001`–`018` with one original migration-transaction timestamp, plus the River marker. This invocation is a preserved-fixture regression, not a newly initialized zero-state proof. No migration replay, omitted statements, ledger edits, or host cgroup changes occurred. The earlier fresh 55464 and authentic old-to-new 55456 records remain separate historical evidence.

The focused actual-worker/HTTP publication lifecycle, modern/legacy normalized evaluation parity, final progress input-grant audience regression, progress review lifecycle, private grant assembly/rights/revocation, owner issuance policy/replay/concurrency, real namespace probe, sealed executable checks, and fail-closed configuration tests passed together with `go test -race` in **102.963 seconds** against database `postgres` at 55474. The database is UTF8 with C collation/ctype. `undergrid_app` and `undergrid_worker` remain NOLOGIN/NOSUPERUSER/NOBYPASSRLS; the distinct artifact API/worker login roles are LOGIN/NOSUPERUSER/NOBYPASSRLS.

Exact migration SHA-256:

- 016: `e7c439471609dbff95f9d5f4bae48e5bda4abf43a0f90d09cc24265c71f1377b`
- 017: `9828ddeae1accf601d67123e23e5a80da6a5c5abcdc77ff6bb37e448df1e199e`
- 018: `3c873b2a27a3bfc83669e73deee326992110a619650dfb5320d3003b4be7d537`
- Unchanged separate financial 008: `5fe8b8871772fa9d0a99ccd4ae61c4ca2187b5ddf8ab1e2a0acb207695f0cfe4`

`go vet ./internal/network ./internal/verification ./cmd/undergrid-verifier`, `go build ./cmd/...`, and `git diff --check` passed. The standalone isolated-worker positive lifecycle explicitly skipped (10.973 seconds) because delegated controllers are unavailable; this is **not** positive isolated-worker evidence. Required-mode `TestBubblewrapVerifierIsolationEnforcesFilesystemNetworkAndLimits` failed in 1.244 seconds with the service-owned delegated-root requirement. Kernel resource isolation therefore remains unqualified. Production storage, image, provider, legal and deployment gates remain open.
