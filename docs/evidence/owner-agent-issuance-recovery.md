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

Integration note: this local branch now includes private artifact grants
migration 015 and task progress migration 016 before owner issuance migration
017. Owner `artifacts:grant` remains in centralized `validateAgentScopes` with
the other protected scopes and still requires current owner coordination policy.
The integration regression uses the real owner HTTP path to prove issuance is
denied before policy configuration and succeeds afterward with the persisted
credential scope exactly `artifacts:grant`. Existing grant scenario fixtures now
send stable owner idempotency keys, preserving migration 017's required-key
contract.

## Validation status

Migration017 applied from a fresh UTF8 PostgreSQL 16 cluster on port 55451 using
the real restricted app role. Focused network tests passed for owner creation,
quota replay and the existing unconfigured-rotation rollback. The owner-creation
tests passed under `go test -race`, including same-key parallel requests and an
RLS read proving an agent context cannot see the owner command result. The
encrypted-cookie/CSRF runtime test passed through the restricted DB login. A
focused owner replay test also proves that an expired 15-minute replay window and
a changed issuance key ID return `credential_replay_unavailable` without adding
agents, credentials, issue events or idempotency results. The combined focused
suite ran from raw migrations 001–017 on fresh UTF8 PostgreSQL 16 at port 55461;
under `-race` it passed owner creation/key replay, protected grant-scope policy,
progress heartbeat/evidence-review lifecycle, and explicit grant scope,
configuration, prior-agreement, expiry and concurrent revocation cases.

`go build ./...`, `go vet ./...`, and `git diff --check` passed. Frontend Vitest
passed 13 tests and the strict TypeScript/Vite build passed. Chromium passed all 3
production-bundle browser tests, including the owner creation flow that aborts
the first POST after submission, then retries the unchanged form and asserts the
same CSRF-protected `Idempotency-Key`.

Follow-up correction validation: the frontend red test initially showed that a
`201` whose response body stream failed was converted to `{}` and resolved. The
new malformed-201 regression also went red because `{ "id": "agent" }` resolved
successfully before the missing token was rendered by the caller. The
owner-create key is retained until the response has nonempty `id` and `token`
fields; malformed success JSON and unexpected `204` responses fail while
preserving same-key recovery. A definitive 4xx releases the key. Body-loss,
malformed-success, 204 and 4xx cases pass. Full Vitest now passes 13 tests and
`npm run build` passes. The focused real-PostgreSQL
`TestOwnerAgentCreation` tests pass normally and under `-race` on a fresh UTF-8
PostgreSQL 16 cluster at port 55457, including replay-window and issuance-key-ID
denials with no additional persisted effects. `go vet ./...` and `go build ./...`
pass. The owner Playwright rerun passes all 3 cases when Chromium is launched
with `LD_LIBRARY_PATH=/tmp/undergrid-browser-libs/root/usr/lib/x86_64-linux-gnu`;
no system libraries were installed.

An earlier full network package run on a fresh UTF8 cluster did not pass:
`TestCommittedIntentionRunsThroughDurableWorker` failed to produce its
opportunity before the test deadline (322 seconds for the package run, with
worker queue shutdown/transaction-delay logs). A separate initial fixture
expectation assumed missing issuance configuration after the common test fixture
began configuring its required explicit test key; that assertion was reset to
an absent key and its rollback case passed. The durable-worker timeout was
subsequently corrected through discovery-fairness PR #26 and follow-up recovery
changes in PRs #29 and #30. The latest full network race on integrated release
014+015 passed in 583.538 seconds (74.1%). That run does not prove the full
network suite with progress migration 016 or owner-issuance migration 017.

No external provider verification, public deployment, remote PR, merge or live
call is part of this branch task. Root owns final integration review and any
release-level regression rerun.

## Progress/grants reconciliation and exact-source validation

The final progress branch, including the progress-artifact input-grant audience
correction, was merged locally into this owner retry branch at source SHA
`fff38655ab8a76c1203554e5f5e56d6e24b89fbe`. No merge conflicts occurred. The
progress-artifact rule remains intact: input-grant recipients must already be
inside the immutable artifact audience and be current accepted source-swarm
members; separate source-rights approval does not widen the audience. The
owner's idempotency result remains protected from agent RLS reads, and retries
still require a matching request fingerprint, owner, issuance-key ID and replay
window.

The combined owner/progress/grant race-enabled selection passed against a new
raw-migration PostgreSQL fixture on port 55472 in 77.106 seconds:

```sh
UNDERGRID_TEST_DATABASE_URL='postgres://nmcdc@127.0.0.1:55472/undergrid_owner_progress_grants?sslmode=disable' \
GOCACHE=/tmp/undergrid-gocache GOMODCACHE=/tmp/undergrid-gomod \
/tmp/undergrid-toolchain/go/bin/go test -race ./internal/network \
-run '^(TestOwnerAgentIssuanceRequiresPolicyForProtectedArtifactGrantScope|TestOwnerAgentCreationResponseLossReplay|TestOwnerAgentCreationWithoutIssuanceKeyRollsBack|TestOwnerAgentCreationReplayWindowAndKeyIDFailClosed|TestOwnerAgentCreationConcurrentSameKey|TestOwnerCanReviewAndRevokeOnlyOwnCredentials|TestProgressArtifactInputGrantsRespectImmutableAudience|TestTaskProgressEvidenceReviewLifecycle|TestPrivateArtifactNamedGrantsExposeOnlyInputAndPermitAssembly|TestPrivateArtifactGrantRevocationSerializesWithReadAndSeal|TestArtifactPublicationLifecycleThroughHTTPAndExecutableWorker|TestArtifactPublicationSerializesWithRightsPolicyRevocation)$' -count=1
```

The application migrator created the database from source migrations 001–017
in order, then recorded River readiness; no SQL was manually replayed and no
ledger was edited. PostgreSQL reports version 16.11, UTF8, and `C.UTF-8` database
collation/character type. `undergrid_app` and `undergrid_worker` are
`NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS`. Migration 016 has SHA-256
`e7c439471609dbff95f9d5f4bae48e5bda4abf43a0f90d09cc24265c71f1377b`; migration
017 has SHA-256
`9828ddeae1accf601d67123e23e5a80da6a5c5abcdc77ff6bb37e448df1e199e`.
`go vet ./internal/network`, `go build ./...`, and `git diff --check` passed on
the same source tree. This focused result is not a full network race or combined
release gate, and does not establish cgroup, provider, beneficial-owner, or
production qualification.
