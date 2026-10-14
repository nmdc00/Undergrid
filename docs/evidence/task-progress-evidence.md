# Evidence-bearing task progress

## Requirement and contract

This slice implements TDD r2 §§7.2–7.3 and the task timeout rule in §7. The existing `POST /v1/tasks/{id}/progress` body remains accepted as a heartbeat for wire compatibility. It may record an `in_progress` transition, but it never renews `progress_deadline` or clears `cure_deadline`; a heartbeat while `at_risk` leaves the task at risk.

New API contract: an assignee may upload an artifact with `purpose: "progress"` (artifact purpose defaults to `output` for old clients). At upload time, the domain transaction binds a sorted, unique audience of current accepted non-observer required contributors and one named acceptance-principal reviewer; both reporter and reviewer must be in the audience. The immutable artifact audience is persisted while the upload is still quarantined, so pending metadata, evaluations, and content are private from room members outside it. A later report must repeat the exact audience and reviewer stored on the artifact. Sealing/evaluation events and report submission/review events carry only audience identifiers and are delivered only to that audience; inbox reads recheck current membership and audience. The server computes and stores the report SHA-256. Audience RLS is restrictive so additive artifact-read policies, including the private input-grant policy, cannot widen progress access. Ordinary output artifact response and room event behavior remain unchanged.

The named reviewer uses `POST /v1/tasks/{id}/progress-evidence-review` with `decision` and the exact `report_sha256`. Review requires the current accepted plan, claim epoch, membership, acceptance-principal designation, `tasks:accept` scope and effective rights policy. Rejecting records the decision but changes no task clock. Accepting a new evidence item sets the task `in_progress`, credits its evidence pointer, advances the progress deadline by the task's configured interval and clears an unexpired at-risk cure window. It does not accept the final artifact or publish it. Replays and identical evidence for the same task/epoch/artifact/evaluation/report hash cannot obtain a second extension.

These deadlines are persisted intervals, not a universal promise that a reviewer will act before a deadline. A passing fixed verifier, a report, or a review does not prove factual truth, completeness, legal ownership, or fitness.

## Runtime and persistence

- `POST /v1/tasks/{id}/progress`, evidence submission/review, and task reads enter through the authenticated HTTP transaction, shared plan/task authorization, and domain commands in `internal/network/tasks.go` and `internal/network/task_progress_evidence.go`.
- Progress artifacts use the existing immutable artifact generation and worker evaluation path. `internal/network/artifacts.go` keeps `output` as the default and prevents `progress` artifacts from entering final acceptance or publication.
- Migration `016_task_progress_evidence.sql` adds immutable artifact purpose/audience/reviewer bindings, normalized evidence with exact task/plan/artifact/evaluation/epoch/report/audience/reviewer links, a task's credited-evidence pointer, and a progress-purpose approval reference. Its restrictive audience policy also constrains additive artifact-read policies. The approval constraint extension preserves the existing finite purposes and, when migration 015 columns exist, dynamically extends the current subject predicate so `assembly_grant` and `evaluation_grant` stay intact.
- Events and task/evidence state commit with their transitions. Reviewer/task locks serialize competing reviews; expired cure rejection rolls back the approval and evidence decision.

## Behavioral evidence

The focused HTTP/PostgreSQL lifecycle test runs a real fixed-verifier worker and asserts:

- A bare heartbeat leaves both clocks unchanged; on an at-risk task it preserves the state and original cure deadline.
- A current room member outside the audience receives 404 for progress artifact metadata, evaluations, content, and reports both before and after evaluation. The stored artifact audience is asserted byte-for-byte as a sorted set with its named reviewer; a report cannot substitute a different audience. A wrong artifact is not found, a wrong evaluation and stale epoch are rejected, an invalid audience is denied, and an audience member who is not the named reviewer cannot decide.
- The durable fanout worker delivers sealed/evaluated artifact and submitted/accepted/rejected report events to named recipients and creates no inbox row for an outsider. The ordinary output publication lifecycle still passes, and output upload responses do not gain progress-only audience fields.
- A verified progress artifact remains separate from output submission, output acceptance and publication. A rejected report leaves the credited pointer and progress deadline unchanged.
- An expired cure review creates no approval; two concurrent accepts credit exactly one review and one deadline extension.
- A fresh service handler recovers the exact credited evidence pointer and persisted deadline. Direct PostgreSQL assertions confirm exact artifact, evaluation, report hash and claim-epoch links.

The red tests first reproduced the existing heartbeat deadline bypass through the real endpoint and database. Before this privacy correction, a room member outside the intended report audience could read a pending progress artifact's metadata and evaluations (200), then metadata, evaluations, and content after sealing (all 200), and received artifact sealed/evaluated plus report submitted/rejected events. After correction, the focused lifecycle checks pass against a fresh UTF-8 PostgreSQL 16 cluster on port 55454; this branch's migration ledger confirms `016_task_progress_evidence.sql` applied. The test executes the durable fanout worker synchronously to avoid conflating privacy assertions with the independently tracked queue-fairness defect:

```sh
UNDERGRID_TEST_DATABASE_URL='postgres://nmcdc@127.0.0.1:55454/undergrid_network_progress_privacy?sslmode=disable' \
GOCACHE=/tmp/undergrid-gocache GOMODCACHE=/tmp/undergrid-gomod \
/tmp/undergrid-toolchain/go/bin/go test ./internal/network \
-run '^TestTaskProgress(HeartbeatDoesNotExtendDeadline|EvidenceReviewLifecycle)$|^TestArtifactPublicationLifecycleThroughHTTPAndExecutableWorker$' -count=1
```

Result: focused progress heartbeat/evidence plus ordinary output publication passed on the isolated fixture; the progress lifecycle passed under `go test -race`. `go vet ./...`, `go build ./...`, and `git diff --check` passed. Port 55450's pre-existing data was preserved; the isolated test cluster is owned at port 55454. The broad `go test ./internal/network` and full race package gate remain pending the separately tracked durable-worker opportunity timeout; the parent is fixing queue fairness, and this feature does not weaken or mask that failure. The timeout was independently reproduced on clean and loaded fixtures. No provider qualification is claimed.

## Combined grants/progress migration and runtime check

After private-artifact-grant migration 015 was integrated ahead of progress migration 016, the combined branch at `c7302a8b0ec1fb04503eec32b04dee5886b8ac42` passed a focused race-enabled validation on a new UTF-8 PostgreSQL 16 cluster at port 55459 (`undergrid_integrated`). The test selector was `^TestTaskProgressEvidenceReviewLifecycle$|^TestPrivateArtifact.*$|^TestArtifactPublicationLifecycleThroughHTTPAndExecutableWorker$`; it passed in 232.714 seconds. The tests invoke the unmodified application migrator. Its ledger records migrations 001 through 016 in order, followed by River readiness.

The combined runtime persisted three private-input lineage edges, each with pinned grant approval IDs; one progress artifact with its reviewer in the recorded audience; 45 grant approvals; and two progress reviews. The audience-restrictive artifact policy and scoped-input-read policy coexist. The approval-purpose check includes both grant purposes and `progress`, and the approval-subject check preserves both grant subjects while adding the progress subject. API and worker test login roles were `NOSUPERUSER NOBYPASSRLS`; the `undergrid_app` and `undergrid_worker` roles are non-login roles without superuser or RLS-bypass privileges. `go vet ./...`, `go build ./...`, and `git diff --check` passed on the same combined branch.

This is focused combined-migration and lifecycle evidence only. The full network suite remains pending its separately tracked run; required cgroup-isolation qualification, production storage/provider qualification, and production lifecycle qualification are not established by this test.

## Quality review and limits

DRY: task epoch/window/rights/dependency policy now has one domain validator used by heartbeat and evidence submission; audience normalization is shared between upload and report commands. Similar but separate review and final-output approval rules stay distinct. Orthogonality: verifier execution is reused through the existing fixed worker, while progress evidence has its own normalized subject and never enters output acceptance or publication. Contracts: exact plan/artifact/evaluation/report hashes, sorted audience/reviewer, and claim epoch are checked under plan/task locks; principal authorization stays in the transaction. Failure recovery: evidence, approval, deadline and audience-scoped event updates commit or roll back together, and a restart reads the normalized task pointer. The restrictive RLS guard keeps future/additive permissive artifact grants from bypassing the progress audience. Readability: route names and `purpose` distinguish progress evidence from final output. The audience and report are bounded; no generic workflow engine or provider abstraction was added.

Fixture terms and artifacts are release-test evidence only. The verifier proves only fixed structural checks, not real-world quality. No provider verification, legal approval, deployment, CI, or production lifecycle was performed. Production owner/acceptor policy and independently provisioned network credentials remain governed by existing gates.
