# External opportunity notifications

Requirements: TDD §§5.2–5.4, INT-04. Implemented on
`feat/v0.1-external-opportunity-notifications`; release integration, actual
provider/source qualification and deployment remain pending.

Permissioned source refresh now wakes active intention watches. A relevant
publisher claim creates an `external_request` or `external_capability`
opportunity, with identity and hard-constraint fit explicitly unknown. It is not
a participant, verified capability, independent buyer or funded demand. The
server never follows advertised links, contacts publishers or runs their software.

## Runtime and recovery

Successful controlled source refresh atomically commits a source/snapshot-bound
queue wakeup. Bounded keyset fanout reads at most 100 current watches, selects
matching watches in one local query, and produces independent intention/revision
jobs. Query selection and the text predicate are shared with the matching
transaction; current authority is still checked there. The continuation cursor
advances over all scanned watches, including unmatched ones.
Identical active snapshots/jobs coalesce, including running jobs; completed jobs
do not absorb later repair. Changed versions, content or approval configuration
have distinct snapshot identities. Stale queued snapshots/revisions decline
work. Fresh source fanout/matching retain reserved capacity alongside existing
repair queues; confirmed matching continuations also have independent ready
capacity, so they cannot wait behind their own initial fanout. Retained legacy
jobs and data are preserved.

Each matching transaction reads at most 50 relevant observations. A full page
atomically enqueues a durable observation-ID continuation carrying the source
snapshot and intention revision. Normal watch scans also reconcile external matches. Current approved title and
summary text are compared locally using PostgreSQL full-text search. Private
objectives/queries are never sent to the publisher. The opportunity fingerprint
binds the intention revision, observation/version, content and approval
configuration. Opportunity, event and inbox publication share one transaction;
unchanged refreshes cannot duplicate user notifications.

Listings, selected reads, inbox and agent context recheck current principal,
agent, intention/watch, expiry, exact revision, source permission/freshness,
observation content and approval configuration. Public lead detail reuses the
established permission-filtered projection; raw config, private approval bases
and unapproved publisher fields stay outside envelopes. The closed schema keeps
external kind/origin consistent and rejects invented verified hard fit.

Detailed projections require effective `intents:read`; an inbox-only credential
gets an empty external-opportunity array in recovery context and cannot list
opportunity details. An authenticated owner session receives only this read
capability after the owner wrapper proves current ownership of the intention and
agent. Foreign owners remain denied; no credential or unrelated authority widens.

## Verification

The actual lifecycle oracle uses distinct genuine nonsuperuser/non-bypass
PostgreSQL LOGINs for API and worker, controlled HTTPS publisher responses and
real domain persistence. The isolated database on network fixture 96 is a genuine
PostgreSQL clone of the normally applied 001–032 schema/ledger. Migration 038 was
then normally applied; its frozen SHA256 is
`74badea1612955915fc2fc3e70b6bd3df73759cbc8b72a4395eb85ac187e99d8`.
Append-only migration 039 closes SQL CHECK NULL semantics by explicitly requiring
a revision for every external observation opportunity. Its frozen SHA256 is
`335f8f14998b4a90675da37240a35c305670d189ed7a1978f419b370890ba707`.
The genuine worker SQL-role oracle always rolls back its malformed insert and
requires SQLSTATE 23514 with the exact new constraint; it is not an HTTP exploit.

| Evidence | Result | Log in `/tmp` |
| --- | --- | --- |
| Missing behavior through HTTPS refresh/watch | RED 3.755s: no opportunity | `undergrid-external-opportunity-red-baseline96.log` |
| Inbox-only context scope leak | RED 1.730s: details disclosed despite listing 403 | `undergrid-external-opportunity-scope-second-red96.log` |
| Owner session scope regression | RED 5.077s: authorized owner listing empty | `undergrid-external-opportunity-owner-red96.log` |
| Candidate truncation with actual 55-item HTTPS feed | RED 10.805s: exactly 50 opportunities, tail absent | `undergrid-external-opportunity-tail-red96.log` and `undergrid-external-opportunity-tail-red-persistence96.log` |
| Missing revision at the worker SQL boundary | RED 0.803s: malformed insert accepted and rolled back | `undergrid-external-opportunity-null-revision-red96.log` |
| Full lifecycle and queued tail continuation with race detector, migrations 038–039 and ready lane | 2 cases PASS, 18.637s; zero failures/skips | `undergrid-external-opportunity-ready-039-green96.jsonl` |
| Final source/external/inbox/discovery/worker race cluster | 41 cases PASS, 89.981s; zero failures/skips | `undergrid-external-opportunity-final-affected41-race96.jsonl` |
| Retained-cohort source fanout regression | 39 pass / 1 fail, 94.081s: exact job waited 11.288s behind irrelevant watches | `undergrid-external-opportunity-final-affected40-race96.jsonl` |
| Accepted uppercase source ID | RED 0.802s: no matching opportunity | `undergrid-external-opportunity-uppercase-red96.log` |
| Prefilter and uppercase fixes with complete lifecycle/tail | 3 cases PASS, 24.680s; zero failures/skips | `undergrid-external-opportunity-prefilter-uppercase-green96.jsonl` |
| Final network static analysis | PASS | `undergrid-external-opportunity-final-vet.log` |
| OpenAPI lint, all 133 schemas and generated-type parity | PASS | `undergrid-external-opportunity-final-contract.log` |
| TypeScript and production bundle | PASS | `undergrid-external-opportunity-web-build.log` |

The full lifecycle checks atomic opportunity/event/inbox output, unchanged
snapshot/job deduplication, new-version replacement, old-version 404, current
selected reads and recovery context, authorized owner/foreign owner boundaries,
inbox-only scope denial, pause/watch/expiry revocation, source permission and
freshness removal, a real revision PATCH, and publisher withdrawal. It manually
invokes controlled `SourceRefresher.RefreshDue`, then requires the normal
`RunWorker` to complete the exact intention job and publish the replacement
within the unchanged eight-second delivery wait. This does not qualify the
normal worker's publisher HTTP client or any actual provider.

Earlier invocation failures included an already-existing cluster-role fixture,
a missing mandatory River uniqueness state, counting `intent.created` as an
external notification, a malformed schema-test URN and a token lacking its
required prefix. These are fixture/integration errors, not additional behavior
REDs. Retained shared-lane backlog also exposed real delivery starvation; queues
were isolated and identities deduplicated without deleting jobs or extending
the deadline. The tail oracle independently selects the greatest persisted observation UUID
from an actual 55-item feed, then requires its opportunity/event/inbox and a
completed snapshot/revision/keyset job through the normal worker within the
unchanged eight-second wait. It reads that opportunity and finds its notification
through actual inbox cursor pages. The final 41-case affected race suite and static analysis passed with migration
039, the ready lane, the same-policy watch prefilter and source-ID normalization.
All unchanged eight-second delivery waits and retained jobs remain. Release
integration still needs validation against its exact merge SHA.

## Quality and limits

DRY: existing permission-filtered public projection and atomic domain notification
are reused. Orthogonality: publisher I/O completes before the notification
transaction; financial databases/credentials are untouched. Contracts/recovery:
current read authorization is independent of historical notification facts,
output deduplicates transactionally and individual watch failures retry
independently. No generic matching framework was introduced.

Local text relevance is limited. External semantic ranking, measured production
relevance/load, real approved-source qualification, independent ownership and
rights clearance remain open requirements. No fixture result proves them.
