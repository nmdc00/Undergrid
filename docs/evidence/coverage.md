# Local Go statement coverage — 2026-10-13

Passing measurement after the worker-shutdown fix in PR #8, on source from `fix/v0.1-worker-shutdown` (implementation commit f6ac304). Run the complete suite with both real disposable PostgreSQL databases and a separate restricted financial test login. `CI=true` requires fixture configuration; GitHub-hosted CI remains disabled.

Command: `go test -race -count=1 -timeout=10m -coverprofile=/tmp/undergrid-coverage-fixed.out ./...`.

| Package | Covered statements | Coverage |
|---|---:|---:|
| Network domain/HTTP/worker | 1248 / 1752 | 71.2% |
| Sandbox domain/HTTP/reconciliation | 377 / 524 | 71.9% |
| Identity/OIDC/sessions | 67 / 112 | 59.8% |
| Fixed verification library | 86 / 111 | 77.5% |
| Sandbox command | 18 / 67 | 26.9% |
| Network command | 0 / 92 | 0.0% |
| Verifier command | 0 / 30 | 0.0% |
| **Total, weighted by statements** | **1796 / 2688** | **66.8%** |

This measures Go statements, not branches, SQL/RLS policies, provider compatibility or completed product requirements. Real HTTP/PostgreSQL tests separately assert current credential authority, deduplication, privacy, durable replay, concurrent caps/quotas, plan/hash/rights/assignment state and shutdown/retry retention. Race tests passed on all packages. The verifier executable is exercised by subprocess acceptance tests, but those tests build the child without Go coverage instrumentation, so command entrypoint counters remain zero even when that process is tested.

Highest-value test gaps: successful network/sandbox command startup and configuration, Session/Logout endpoints (0%), long-poll timeout/cancellation/revocation paths (waitInbox 16.7%), plan rejection/supersession/current-policy edge paths (maybeAcceptPlan 47.4%), sandbox GetPolicy (0%), and additional malformed/error recovery cases. No arbitrary percentage threshold replaces the named lifecycle contracts. Provider receipts, external cohort, OS isolation and restore/security qualification remain separate unproven release gates.

The initial coverage run failed the worker shutdown lifecycle and is not presented as a green result. The new deterministic blocked-transaction regression reproduced the failure before fixing it; the successful worker control plus blocked case passed three repeated race runs, then the complete configured coverage suite passed (network 52.728s, sandbox 3.665s). A final measurement on the promotion candidate will be recorded with the promotion PR before merging main.
