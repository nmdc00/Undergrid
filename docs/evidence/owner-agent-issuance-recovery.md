# Recoverable initial owner agent issuance — ID-01 / TDD §§4, 9

Branch `feat/v0.1-owner-agent-issuance-recovery`, from `release/v0.1` f1409cf.
This slice fixes response loss on the first owner-created agent and credential.
It adds no owner identity qualification, endpoint verification or beneficial-
ownership inference.

## Observable behavior

Verified owner `POST /v1/agents` requires a stable `Idempotency-Key`. A retry for
the same owner principal, operation, key and canonical JSON body returns the same
agent, credential and `ug_` bearer after reauthorizing the current owner, active
agent and current scope policy. A changed body with the same key returns `409`.
The principal lock serializes same-key requests and quota admission. Agent,
recoverable credential, issuance event, optional recruiter job and reference-only
command result commit together. Replays do not add agents, credentials, events or
jobs. A committed response can be reconstructed after service restart for the
existing 15-minute replay window; revoked, superseded, expired or key-ID-mismatched
credentials fail closed.

Credential generation reuses `issueRecoverableCredential` and
`regenerateCredential` from credential rotation. The command result stores only
credential/agent references and metadata; the bearer is derived from the
dedicated network HMAC key, random persisted nonce and immutable issuance fields.
No production default was added. Existing `ug_` authentication and response
fields remain intact.

Migration017 adds a nullable owner principal subject and partial owner uniqueness
to the existing idempotency table, keeping agent command keys on their current
agent subject. Its RLS policy allows owner rows only with a verified owner
transaction, matching principal and no agent actor; bearer-authenticated commands
cannot claim that subject. No parallel command table was introduced.

The browser keeps the pending `/v1/agents` command key in memory for an unchanged
form retry after response loss. Editing the form clears that pending key; success
clears it as well. The token remains a one-time memory-only display. A browser
reload discards the key and token; the owner reviews the agent list and uses
explicit credential recovery when the replay window or key is no longer available.

## Behavioral proof

- Red: before implementation, the real HTTP test sent POST `/v1/agents` without
  `Idempotency-Key`; it got `201` and an unrecoverable random token instead of
  the required `400` key error.
- Real HTTP service and restricted `undergrid_app` PostgreSQL role: required key,
  missing-key denial, same-key response replay, changed-body `409`, owner
  separation, bearer-without-owner-context denial, restart replay and revocation.
- Parallel same-key HTTP requests produce one new agent and credential, one issue
  event and stable token; normalized row and River-job counts verify atomicity.
- Persisted idempotency JSON does not contain the bearer. A replay at agent quota
  returns the committed result without a second admission. Recruit work is enqueued
  once for a recruitment-enabled profile.
- Encrypted owner browser-session fixture verifies CSRF/origin handling and retry
  through the restricted DB identity. It does not verify an external OIDC provider.
- Frontend request tests simulate a lost response, retry unchanged input with the
  same command key, and change the form to establish a new key. The token is never
  written to browser storage.

Fixture owner authentication is a test boundary only. These tests do not establish
provider compatibility, independent beneficial ownership, legal approval, public
deployment, financial sandbox authority or production credential configuration.

## Quality assessment

DRY: the common idempotency request fingerprint and body restoration are shared;
owner and agent command subject lookups/inserts remain separate because their
authorization and RLS subjects differ. The established credential issuance owner
is reused rather than duplicating HMAC, nonce or regeneration logic.

Orthogonality: owner identity, network issuance key, session/cursor key and
financial sandbox credentials remain separate. Authorization is checked in the
domain transaction; the UI only retains a retry key and displays the returned
one-time token.

Contracts and recovery: the principal row lock covers quota, inserts, events,
recruitment enqueue and result ref. Transaction rollback leaves no partial agent
or credential when issuance is unconfigured. Replay checks active ownership,
agent, scope policy, revocation, expiry, immutable digest and current key ID.

Readability and effects: one narrow owner idempotency helper differs from the
existing agent helper only in command subject and If-Match semantics; no generic
command engine or new result store was added. SQL uses the existing table and
restricted role, and the result remains bearer-free.

## Validation status

Migration017 applied from a fresh UTF8 PostgreSQL 16 cluster on port 55451 using
the real restricted app role. Focused network tests passed for owner creation,
quota replay and the existing unconfigured-rotation rollback. The owner-creation
tests passed under `go test -race`, including same-key parallel requests and an
RLS read proving an agent context cannot see the owner command result. The
encrypted-cookie/CSRF runtime test passed through the restricted DB login.

`go build ./...`, `go vet ./...`, and `git diff --check` passed. Frontend Vitest
passed 9 tests and the strict TypeScript/Vite build passed. Chromium passed all 3
production-bundle browser tests; the owner creation flow deliberately aborts the
first POST after submission, then retries the unchanged form and asserts the
same CSRF-protected `Idempotency-Key`.

The full network package ran on a fresh UTF8 cluster but did not pass in that
run: `TestCommittedIntentionRunsThroughDurableWorker` failed to produce its
opportunity before the test deadline (322 seconds for the package run, with
worker queue shutdown/transaction-delay logs). A separate initial fixture
expectation assumed missing issuance configuration after the common test fixture
began configuring its required explicit test key; that assertion is now set up
with an absent key and its rollback case passes. The durable-worker failure is
tracked separately as a queue liveness/fairness issue and is not hidden by a
copied schema, shorter assertion or weakened test.

No external provider verification, public deployment, remote PR, merge or live
call is part of this branch task. Root owns final integration review and any
release-level regression rerun.
