# Authenticated private report intake evidence

This branch implements a bounded authenticated intake slice for DEL-04. It accepts `abuse`, `bug`, `privacy` and `rights` reports from an agent credential with `reports:write`, binds the reporter principal and agent from that credential, and requires an explicit `Idempotency-Key`. Subjects and optional evidence references are allowlisted typed UUIDs stored opaquely; the service does not resolve or fetch them. Statements are limited to 4,000 UTF-8 bytes and the general request decoder caps bodies at 64 KiB. Attachments and client-supplied reporter identities are rejected. Event facts contain only the report kind and pending state. A reporter with `reports:read` can retrieve their principal's case; another principal gets 404. The app role has no update/delete permission, and the report state remains pending in this slice.

Admission is capped at 30 reports per principal per minute. Authenticated requests serialize on the principal authority row, so sibling agent credentials share the same cap. An internal triage target defaults to 72 hours, can be configured from one hour to 30 days, and is neither returned to reporters nor described as an SLA. This slice does not include anonymous intake, operator/staff review, moderation decisions, sanctions, refunds, deletion/hold policy, attachments, or autonomous arbitration. It does not complete DEL-04 or qualify the public reporting safety gate.

## Red/green and database evidence

The behavioral suite was first run before the handler and scope entries existed. It failed as expected: `reports:read`/`reports:write` were rejected as `scope_not_authorized`, so the authenticated route contract could not pass. After implementation, the focused race run passed:

```text
GOCACHE=/tmp/undergrid-gocache GOMODCACHE=/tmp/undergrid-gomod \
UNDERGRID_TEST_DATABASE_URL=postgres://nmcdc@127.0.0.1:55462/undergrid_report_intake?sslmode=disable \
  /tmp/undergrid-toolchain/go/bin/go test -race ./internal/network \
  -run '^(TestAuthenticatedReportReceiptIsBoundAndPrivate|TestReportIntakeBoundsAndScopeSeparation|TestReportIntakeRateCeilingIsPrincipalScoped)$' -count=1
ok   undergrid/internal/network  5.115s
```

The first cluster was freshly initialized as a UTF8 PostgreSQL 16.11 network-only fixture on port 55462. The test fixture invoked the unmodified application `network.Migrate`; its recorded ledger is `001` through `014`, followed by `019_private_report_intake.sql` and the River migration marker. Migrations `015`–`018` are absent from this feature branch's selected base and remain for integration; this is not evidence of a combined `001`–`019` migration chain. The preserved fixture had 192 test reports at final inspection, accumulated over repeated focused runs; it was not reset to hide prior activity.

Database inspection showed `undergrid_app` and `undergrid_worker` are NOLOGIN, NOSUPERUSER and NOBYPASSRLS; `undergrid_report_api_runner` is LOGIN, NOSUPERUSER and NOBYPASSRLS. The HTTP test runs as that restricted login and executes under `SET LOCAL ROLE undergrid_app`. The test also manually set a foreign principal context under the restricted login and confirmed a direct `reports` read returned zero rows. The installed policies allow SELECT only for `actor_principal()` and INSERT only when both principal and agent match the actor. The app grant is SELECT/INSERT only. Tests assert credential-derived reporter binding, private 404 behavior, request-scope separation, no reporter/narrative/subject/evidence reference in event facts, safe unknown-reference acceptance, attachment and identity-spoof rejection, bounded input, idempotent replay and changed-body conflict, handler restart recovery, and the principal-shared rate boundary.

The strengthened race test first commits 29 reports, then concurrently submits from two distinct agent credentials belonging to the same principal, using distinct idempotency keys. It asserts exactly one 201 receipt and one 429 denial, exactly 30 durable report rows and 30 `report.submitted` events. This is a characterization of the existing principal-row lock and rate admission behavior; it produced no implementation change. `subject_id` is now also explicitly checked absent from the event facts.

The required-cgroup verifier-isolation qualification is outside this branch's report-intake scope. No full network cohort, public launch qualification, legal/retention decision, moderator workflow, or release approval is claimed.

## Static checks

After the final handler/configuration changes (implementation unchanged by the later test-only hardening):

```text
GOCACHE=/tmp/undergrid-gocache GOMODCACHE=/tmp/undergrid-gomod /tmp/undergrid-toolchain/go/bin/go vet ./...   PASS
GOCACHE=/tmp/undergrid-gocache GOMODCACHE=/tmp/undergrid-gomod /tmp/undergrid-toolchain/go/bin/go build ./...  PASS
git diff --check                              PASS
```

After the test-only hardening, `git diff --check` passed again. `go vet` and `go build` were not repeated because no implementation code changed.
