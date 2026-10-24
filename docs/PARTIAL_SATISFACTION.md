# HP1 partial satisfaction

Implemented on `feat/v0.1-intention-partial-satisfaction`, reconciled with release
`723527b`. The feature implements HP1 step 3/TDD §5.3 for the existing bounded
JSON dataset required-field model. HTTP/SDK/domain and focused UI proof are
recorded below; the complete browser journey, feature merge and external
qualification remain pending. HP1 is not locally closed by this feature alone.

The new owner/agent command explicitly records an owner-declared proper subset
of current required fields as fulfilled, creates an immutable revision containing
the remaining fields and keeps the current watch active. All other input
constraints are preserved. The audit is server-owned/private and bounded to the
last action, with original/result revision links; later generic revisions carry
it unchanged as historical provenance. It is absent from public/granted views.
Current actor/source authority and original immutable report pins precede receipt
recovery. Exact retries survive their own revision but not unrelated later heads.

## Runtime contract

The public agent route is `POST /v1/intents/{id}/partial-satisfaction`; the
owner-session route is `POST /v1/owner/intents/{id}/partial-satisfaction`.
REST, the finite `intention_partial_satisfaction` MCP tool and the same A2A command
use one authorization-bearing domain transaction. Owner sessions attribute the
decision to the intention's current active agent without minting a bearer token.

Both commands require `If-Match` and `Idempotency-Key`. Their closed request body
contains `lead_id`, `source_version`, `projection_hash`, `report_hash`,
`intention_revision` and `fulfilled_required_fields`. The fit response includes
the intention version/ETag from the same transaction as its immutable revision
and selected source report; the UI must not mix that report with a later GET.

The selected fields must be a nonempty unique proper subset of the current
required fields. The server computes the remainder in existing field order.
Empty/unknown/duplicate selections, removing every required field, ambiguous
historical field sets, paused/terminal/expired/unwatched intentions, stale pins
and foreign or revoked actors do not create revisions. Only a currently
permitted capability lead is eligible; an external request is not capability
evidence. The owner declaration never upgrades publisher, identity, license,
rights, payment, participation or verification assurance.

One successful command commits a new immutable document/hash/public projection,
intention head/version, `intent.partially_satisfied` event, inbox entry and
discovery queue work. The watch remains active. All other document inputs,
including licenses, output, budget, preferences, visibility and expiry, are
preserved. The source and observation stay locked through the decision commit;
contention is a retryable unknown result with no partial effects.

Current actor/source eligibility precedes receipt lookup. An exact retry uses
the original immutable requirements and command fingerprint, even though the
first result has advanced its own intention revision. Recovery is allowed only
while the current head is that command's result and the source projection still
matches. A subsequent revision or changed/withdrawn source cannot recover an
obsolete result. The bounded private audit links to earlier immutable revisions
rather than growing a copied history array.

## Checkpoint evidence

- Actual restricted-login owner HTTP RED: 405, 0.973s;
  `/tmp/undergrid-partial-satisfaction-red96.log`.
- Initial remainder/active-watch/exact-retry GREEN: 1.326s;
  `/tmp/undergrid-partial-satisfaction-first-green96.log`.
- Five top-level adverse cases PASS, 10.030s;
  `/tmp/undergrid-partial-satisfaction-adverse96.jsonl`. Covers unchanged source
  recheck, changed claim/permission/actor, private projections, forged metadata,
  historical audit and initial enqueue-failure rollback.
- UI scaffold RED: three behavioral failures, 12.41s;
  `/tmp/undergrid-partial-satisfaction-ui-red.log`.
- Actual MCP/A2A recovery RED: command/retry committed once, but task recovery
  lacked its live resource snapshot, 2.626s;
  `/tmp/undergrid-partial-satisfaction-a2a-recovery-red96.log`. Narrow new-operation
  resource reference correction followed below.
- Six top-level domain/protocol cases PASS, 10.485s;
  `/tmp/undergrid-partial-satisfaction-protocol-green96.jsonl`. This includes
  actual SDK MCP→A2A exact retry/live resource recovery, inbox-sequence rollback
  and faulting the later fanout insert after the first queue insert.
- Reviewer reproducer: four well-pinned responses missing owner-consumed fields
  and a same-revision/lead changed-version late response failed, with three other
  tests passing, 27.97s; `/tmp/undergrid-partial-owned-response-red-oct24.log`.
  The corrected parser validates the complete consumed intention before the
  retry key is cleared; all eight focused UI cases PASS, 2.02s;
  `/tmp/undergrid-partial-owned-response-green-oct24.log`.
- Strict-shape reviewer reproducer: a pinned response with array visibility
  failed as expected, 2.35s; `/tmp/undergrid-partial-visibility-shape-red-oct24.log`.
  Explicit string validation then passed all nine focused cases, 2.00s;
  `/tmp/undergrid-partial-visibility-shape-green-oct24.log`.
- Reconciled runtime `1837c74`: **30 top-level and 20 subcase actual PostgreSQL
  race checks PASS**, zero failures/skips, package 76.233s;
  `/tmp/undergrid-partial-final-1837c74-race96.jsonl`. This includes all eight
  partial-satisfaction cases plus immutable revision/renewal/satisfaction,
  owner console, fit, grants, inbox and A2A current-authority regressions. The
  in-flight oracle pauses the actual immutable-revision insert after source
  authorization, proves source revocation cannot acquire its update lock until
  commit, then commits revocation and denies retained-result recovery.
- `go vet ./internal/network`: PASS;
  `/tmp/undergrid-partial-final-1837c74-vet.log`.
- Complete owner-console UI suite: **110 tests across 14 files PASS**, 29.70s;
  `/tmp/undergrid-partial-final-1837c74-full-ui.log`.
- Canonical OpenAPI lint, **144 compiled schemas**, generated TypeScript parity:
  PASS; `/tmp/undergrid-partial-final-1837c74-contract.log`.
- TypeScript and production Vite build: PASS, Vite 1.28s;
  `/tmp/undergrid-partial-final-1837c74-build.log`.

These final gates ran against runtime `1837c74`, reconciled with release
`723527b`. Subsequent edits reconcile documentation only. Restricted application
LOGINs and controlled source fixtures are used by the PostgreSQL cases; browser
unit tests use controlled HTTP responses and are not the full live journey.

Migration 041 was normally applied on isolated network fixture 96. Frozen SHA256:
`611a2ee732e5ecb3e0a1bc848ce20647e8bad36f0511febe5d189e6682abb206`.
It locks current source/observation eligibility via a narrow boolean helper;
no direct private source-table access or configuration output is granted.

Canonical request/private audit schemas, owner controls, UI/build validation and
generated parity are implemented and locally proven as scoped above. Feature
merge and the complete scheduler/browser HP1 walkthrough remain pending. Real
source qualification, useful-result evidence and deployment are separate gates.

## Quality assessment and limits

REST/owner/MCP/A2A reuse one domain transition; the adapters do not grant authority.
Generic and partial revisions share their actual immutable document/head and
event/inbox/queue persistence seams. Financial stores, credentials and identities
are untouched. The small private audit record links existing immutable history
instead of adding a table or copying an unbounded audit chain. Generic request
DTOs remain closed and cannot forge it; public/granted projections allowlist
their existing fields instead of serializing that audit.

Source/observation locks span the commit boundary, with bounded NOWAIT contention
and exact same-key recovery. Failure tests independently observe normalized
heads, revisions, counters, events, inbox, both queue effects and receipts. The
UI holds all mutation pins together, leaves every field initially unselected,
rejects malformed committed responses before releasing the retry key and ignores
responses from stale review/session identities. A receipt is not evidence of
provider verification, rights or completion of other requirements.
