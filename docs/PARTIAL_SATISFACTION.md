# HP1 partial satisfaction

Work in progress on `feat/v0.1-intention-partial-satisfaction`, based on release
`81e4730`. This closes HP1 step 3/TDD §5.3 for the existing bounded JSON dataset
required-field model. It does not claim full HP1 completion or provider verification.

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

Migration 041 was normally applied on isolated network fixture 96. Frozen SHA256:
`611a2ee732e5ecb3e0a1bc848ce20647e8bad36f0511febe5d189e6682abb206`.
It locks current source/observation eligibility via a narrow boolean helper;
no direct private source-table access or configuration output is granted.

Canonical request/private audit schemas and owner controls are authored. Final
adverse/race/static checks, complete UI/build validation, generated parity and
review remain pending for the reconciled release candidate. The new controlled
in-flight source-lock test has not yet run. Controlled HTTPS data is fixture
evidence only. Full scheduler/browser HP1 walkthrough, real source qualification
and deployment remain separate gates.
