# Evidence-bearing task progress

## Requirement and contract

This slice implements TDD r2 §§7.2–7.3 and the task timeout rule in §7. The existing `POST /v1/tasks/{id}/progress` body remains accepted as a heartbeat for wire compatibility. It may record an `in_progress` transition, but it never renews `progress_deadline` or clears `cure_deadline`; a heartbeat while `at_risk` leaves the task at risk.

New API contract: an assignee may upload an artifact with `purpose: "progress"` (artifact purpose defaults to `output` for old clients). Sealing and fixed evaluation of a progress artifact do not submit the task. The assignee posts a report tied to that exact artifact SHA-256, a passing evaluation, current accepted plan hash, current claim epoch, a finite audience of current named plan contributors, and one named acceptance-principal reviewer. The server computes and stores the report SHA-256. The reporter and reviewer must be in the audience. Report reads are limited to that audience and current room members; other current room members receive 404.

The named reviewer uses `POST /v1/tasks/{id}/progress-evidence-review` with `decision` and the exact `report_sha256`. Review requires the current accepted plan, claim epoch, membership, acceptance-principal designation, `tasks:accept` scope and effective rights policy. Rejecting records the decision but changes no task clock. Accepting a new evidence item sets the task `in_progress`, credits its evidence pointer, advances the progress deadline by the task's configured interval and clears an unexpired at-risk cure window. It does not accept the final artifact or publish it. Replays and identical evidence for the same task/epoch/artifact/evaluation/report hash cannot obtain a second extension.

These deadlines are persisted intervals, not a universal promise that a reviewer will act before a deadline. A passing fixed verifier, a report, or a review does not prove factual truth, completeness, legal ownership, or fitness.

## Runtime and persistence

- `POST /v1/tasks/{id}/progress`, evidence submission/review, and task reads enter through the authenticated HTTP transaction, shared plan/task authorization, and domain commands in `internal/network/tasks.go` and `internal/network/task_progress_evidence.go`.
- Progress artifacts use the existing immutable artifact generation and worker evaluation path. `internal/network/artifacts.go` keeps `output` as the default and prevents `progress` artifacts from entering final acceptance or publication.
- Migration `016_task_progress_evidence.sql` adds the immutable artifact purpose, normalized evidence with exact task/plan/artifact/evaluation/epoch/report/audience/reviewer links, a task's credited-evidence pointer, and a progress-purpose approval reference. Audience RLS hides report contents. The approval constraint extension preserves the existing finite purposes and, when migration 015 columns exist, dynamically extends the current subject predicate so `assembly_grant` and `evaluation_grant` stay intact.
- Events and task/evidence state commit with their transitions. Reviewer/task locks serialize competing reviews; expired cure rejection rolls back the approval and evidence decision.

## Behavioral evidence

The focused HTTP/PostgreSQL lifecycle test runs a real fixed-verifier worker and asserts:

- A bare heartbeat leaves both clocks unchanged; on an at-risk task it preserves the state and original cure deadline.
- A wrong artifact is not found, a wrong evaluation and stale epoch are rejected, an invalid audience is denied, an audience member who is not the named reviewer cannot decide, and a current room member outside the audience cannot read the report.
- A verified progress artifact remains separate from output submission, output acceptance and publication. A rejected report leaves the credited pointer and progress deadline unchanged.
- An expired cure review creates no approval; two concurrent accepts credit exactly one review and one deadline extension.
- A fresh service handler recovers the exact credited evidence pointer and persisted deadline. Direct PostgreSQL assertions confirm exact artifact, evaluation, report hash and claim-epoch links.

The red test first reproduced the existing heartbeat deadline bypass through the real endpoint and database. The evidence endpoint red then returned 404 after the actual artifact worker had recorded a passing fixed evaluation. Focused green command:

```sh
UNDERGRID_TEST_DATABASE_URL='postgres://nmcdc@127.0.0.1:55450/undergrid_network_progress?sslmode=disable' \
GOCACHE=/tmp/undergrid-gocache GOMODCACHE=/tmp/undergrid-gomod \
/tmp/undergrid-toolchain/go/bin/go test ./internal/network \
-run '^TestTaskProgress(HeartbeatDoesNotExtendDeadline|EvidenceReviewLifecycle)$|^TestProgressDeadlineCureAndExpiredPlan$' -count=1
```

Result: passed on the isolated UTF-8 PostgreSQL 16 network fixture at port 55450. The database and credentials are separate from the financial sandbox. `go vet ./...`, `go build ./...`, and `git diff --check` passed. The full `go test -race ./internal/network -count=1` run failed only at the existing `TestCommittedIntentionRunsThroughDurableWorker` opportunity timeout. That test passes in isolation under race (11.20s). The same full-suite failure was independently reproduced by the parent on the integrated grants/control/deadline baseline, which is investigating a separate recruitment-queue fairness defect; this feature branch does not weaken or mask it. A full non-race package rerun was stopped after that reproducible baseline failure was reported. Full package release proof remains pending the queue-fairness fix and rerun.

## Quality review and limits

DRY: task epoch/window/rights/dependency policy now has one domain validator used by heartbeat and evidence submission. Similar but separate review and final-output approval rules stay distinct. Orthogonality: verifier execution is reused through the existing fixed worker, while progress evidence has its own normalized subject and never enters output acceptance or publication. Contracts: exact plan/artifact/evaluation/report hashes and claim epoch are checked under plan/task locks; principal authorization stays in the transaction. Failure recovery: evidence, approval, deadline and event updates commit or roll back together, and a restart reads the normalized task pointer. Readability: route names and `purpose` distinguish progress evidence from final output. The audience and report are bounded; no generic workflow engine or provider abstraction was added.

Fixture terms and artifacts are release-test evidence only. The verifier proves only fixed structural checks, not real-world quality. No provider verification, legal approval, deployment, CI, or production lifecycle was performed. Production owner/acceptor policy and independently provisioned network credentials remain governed by existing gates.
