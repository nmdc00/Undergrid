# Private artifact assembly and evaluation grants

Implements TDD r2 §§8.1–8.2 through authenticated HTTP domain transactions and restricted PostgreSQL roles. A submitted private artifact can be shared with one named agent/principal for assembly or evaluation against one accepted target task, plan hash and claim epoch. All source required-rights principals must grant the same finite purpose. Neither source publication nor source room admission is required. Output publication continues to require its own exact acceptance and all publication consents.

## Contract and entrypoints

- `POST /v1/artifacts/{id}/input-grants`, scope `artifacts:grant`: exact artifact SHA, manifest hash, source plan/epoch, target task/plan/epoch, recipient agent/principal, purpose, template/hash, grant-policy hash, license/schema, expiry (at most 30 days), and an explicit withdrawal-survival boolean.
- `GET /v1/artifact-input-grants/{id}`: grantor or named recipient can recover the finite agreement.
- `POST /v1/artifact-input-grants/{id}/revoke`: grantor can stop future use, including after departure. Revoked rows cannot reopen.
- Private artifact metadata, content and evaluation reads require `purpose=assembly|evaluation&task_id={id}` and the complete effective grant set. Cross-room responses omit source room, plan and task context. Source coordination/transcripts remain inaccessible.
- Existing finalize/verification/publication commands consume these grants. Normalized input edges pin the exact chosen approval IDs and aggregate scope hashes; later grants cannot silently replace consumed provenance.
- River delivers a grant reference only to its named recipient. `/v1/agent-context` reconstructs currently effective grants after restart.

Template grant purposes and withdrawal survival are explicit operator configuration, hashed into both accepted plans. Adding configuration to an old template cannot extend an already accepted agreement. Survival after departure is recognized only when both the accepted finite agreement and individual grant explicitly establish it. Owner revocation, owner-policy change/expiry, grant expiry, stale epochs, changed configuration or missing required grants fail closed. This does not establish legal irrevocability or ownership.

## Behavioral evidence

Tests use authenticated HTTP, actual fixed verifier/River workers, normalized rows and genuine nonsuperuser/non-BYPASSRLS login sessions. PostgreSQL fixtures are child-owned network databases on ports 55442, 55448 and 55449; financial fixtures are untouched. Exact final migration 015 was installed through the application migrator on the fresh UTF8 cluster at 55449. A fresh database on an existing cluster is insufficient because migration 001 creates cluster-wide roles; that fixture attempt failed before feature behavior and was replaced with a separate cluster.

Observed red cases included missing artifact grant scope/routes, missing restart grant discovery, and a content read returning 200 after its authority lock outlasted owner-policy expiry. The latter exposed transaction-start `now()` in the existing policy helper; grant validity now also checks `clock_timestamp()` after the wait. A normalized scope assertion additionally exposed Go nanosecond expiry versus PostgreSQL microsecond storage: the immutable scope is now canonicalized at the stored precision before hashing. Both regressions pass on the final fresh migration.

Coverage proves unpublished private input → named collaborator sealed output → actual persisted verification; outsider/wrong audience/purpose/hash/template/license/epoch denial; source coordination isolation; explicit survival versus absent survival after withdrawal; revoke/replay denial; exact publication consent with a separate publisher; expiry/configuration/policy changes; durable recipient recovery; and authority-lock revocation/read/seal races using an independent PostgreSQL observer and exact blocking PID. Input lineage preserves attribution and immutable selected grant IDs.

Final validation:

- Final private-grant HTTP/worker/denial/recovery/race suite: `go test -race ./internal/network -run TestPrivateArtifact -count=1`, **PASS**, 113.509s, child-owned fresh-migration cluster 55449.
- Exact final raw migration with private assembly/verification and blocked owner-expiry regression: **PASS**, 23.664s, cluster 55449.
- `go vet ./...`, `go build ./...`, `git diff --check`: **PASS**.
- Broad network/verification/verifier race run on accumulated cluster 55448: verification and verifier packages **PASS**; network run **FAIL**, 285.413s, solely existing `TestCommittedIntentionRunsThroughDurableWorker` opportunity deadline (20.82s). Grant tests passed in that run, which preceded the expiry-precision correction. A focused worker rerun on 55449 is reported separately below; the broad failure is not represented as a full-suite pass.
- Existing committed-intention durable-worker and shutdown cancellation/queued-work regressions on 55449: `go test -race ./internal/network -run 'TestCommittedIntentionRunsThroughDurableWorker|TestWorkerShutdown' -count=1`, **PASS**, 21.159s. The accumulated-fixture broad-run timeout remains reported above; it did not reproduce in this focused rerun.

## Pragmatic quality assessment

- **DRY knowledge:** `valid_input_grants` owns the database validity predicate; RLS, capability reads and provenance proofs reuse it. Runtime template consent remains checked in the domain, where current configuration is available. No shared financial repository was introduced.
- **Orthogonality:** the finite grant extends existing approval persistence without changing admission, plan approval, publication consent or spending authority. Cross-room capability functions expose only target descriptors and existing lineage metadata; they do not grant room membership.
- **Contracts:** typed request fields and immutable normalized scope columns identify exact bytes, audience, purpose, plans and epochs. Scope hashes are distinct from legal ownership evidence; withdrawal survival is mandatory input rather than a default.
- **Readability:** grant parsing, creation/revocation, effective-read proof and recovery are separated by responsibility. The SQL predicate is necessarily explicit because each dimension is authorization bearing.
- **Recovery:** approval, event and River enqueue commit together; retries reuse immutable agreements, revocation cannot reopen them, recipient snapshots rebuild current state, and publication revalidates pinned lineage grants. Sorted authority locks serialize rights changes with content reads and sealing; expiry is rechecked using actual time after waits.

## Remaining qualification gates

Artifact storage and verifier isolation still use the preceding development-only implementation. Production object-provider integration and qualified hard isolation remain separate work; these tests are not provider verification. Test template IDs and account IDs establish neither approved legal terms nor independent beneficial owners. Agent-supplied artifact bytes are data and are never executed. No external provider credentials, deployment or protected-branch promotion occurred in this feature.

## Final release reconciliation and complete gate

Reconciled with release `9d12540` (fairness PR#26 plus controlled-test fixture corrections #29/#30) at implementation/integration SHA `04e7477a2a7a14d8ed4301d61ad192fd7c86e9af`. Complete network race and coverage gate passed on preserved, loaded UTF-8 PostgreSQL port55463 in583.538s: `go test -race -coverprofile=/tmp/undergrid-grants-04e7477.cover ./internal/network -count=1 -timeout=20m -json`. The network package measured74.1% statement coverage. This number excludes later unmerged progress, owner recovery, verifier isolation, reports, sharing and OAuth additions and all financial packages; it is not a product-completion metric.

Earlier complete gates failed only controlled fairness setup:636.248s on port55449 and304.731s on initially fresh port55463. The subsequent fixture corrections assert both specific recruitment IDs start available and are blocked before new HTTP intentions commit; they preserve all old backlog rows and timing bounds. River scheduled-state insertion behavior, the unsuccessful intermediate correction, and URL/keyword proof are recorded in discovery-queue-fairness.md. The successful complete run reused port55463 without resetting data or queues. Migrations001–015 were applied unmodified in this cluster before the rerun; no migration016 is claimed. The final evidence commit changes only this document.
