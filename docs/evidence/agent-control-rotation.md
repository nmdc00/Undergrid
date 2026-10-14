# Agent signing control and recoverable credential rotation — ID-02 / TDD §4

Branch `feat/v0.1-agent-control-rotation`, from `release/v0.1` efafe57.
Scope: ordinary owner signing-key possession proof and scoped API credential
rotation/recovery. Endpoint probing, OAuth-grant issuance, owner OIDC-recovery
identity transfer and beneficial-owner/provider qualification are separate work.
No external endpoint is contacted by this feature.

## Observable release contract and entrypoints

1. Verified interactive owners can challenge only their own active agent. POST
   `/v1/owner/agents/{id}/control-challenge` requires CSRF, reviewed `If-Match` and
   `Idempotency-Key`. The five-minute Ed25519 challenge binds purpose, network
   audience, principal/agent, exact public key, random nonce, expiry and version.
2. POST `.../control-proof` verifies the exact UTF-8 challenge/signature; wrong
   signer, consumed/replaced/expired challenge and authority-version changes
   fail closed. One current pending challenge slot is consumed atomically, the
   current key is stored and the proof event survives restart. Signing possession
   does not prove endpoint control, legal ownership or independent beneficial ownership.
3. Owner POST `/v1/owner/credentials/{id}/rotate` atomically revokes the old
   credential and issues an equal-or-narrower scoped replacement. Foreign owner,
   inactive authority, stale version, escalation and missing issuance config are
   denied without changing the old live credential. Concurrent different-key
   rotations yield one replacement and one rejected attempt.
4. Owner POST `/v1/owner/agents/{id}/recover-credential` checks agent version and
   explicit desired scopes/policy, revokes all its live credentials and creates
   one replacement. The console requests only intention/inbox discovery scopes.
5. Same-key response-loss/restart replay reconstructs a live authorized token for
   a bounded 15-minute window from an independent network HMAC issuance key and
   fresh persisted random nonce plus immutable credential/agent/principal,
   generation, scopes, audience and expiry. Command results contain reference
   metadata only, never a plaintext bearer. Revoked/superseded/expired/mismatched
   key or policy authority cannot regenerate a token.
6. Existing `ug_` bearer authentication, quoted numeric ETags and owner response
   fields remain. Legacy random credentials continue authenticating by digest;
   explicit rotation moves them onto recoverable issuance. Initial agent creation
   retains its earlier non-idempotent behavior; this feature does not claim its
   unknown response is recovered by a rotation replay.

API startup wires `ConfigureCredentialIssuance` from dedicated
`UNDERGRID_CREDENTIAL_ISSUANCE_KEY` and `..._KEY_ID`; missing/invalid keys and
session/cursor reuse are rejected, with no default. The worker requires no key.
Migration014 adds current agent version/key/pending challenge/proof slots and
credential generation/nonce/key-ID/replay-expiry/predecessor metadata. A trigger
rejects mutation of issuance metadata and reversal/replacement of revocation.
No new application table or financial-sandbox coupling is introduced.

## Test-first observations

- Focused HTTP proof/rotation tests initially reached missing routes and returned
  404. After implementation, real PostgreSQL proof/rotation/restart/replay/
  recovery tests became green. No mock persistence fallback is available.
- The challenge signature is independently computed with Go's Ed25519 private
  test key over the returned exact bytes. Wrong signer, expiry, replacement,
  single consumption and cross-owner denial are exercised. The domain does not
  execute agent material.
- A later adversarial test signed a challenge, changed agent authority through
  recovery, then supplied the newer ETag: it returned 200 before the fix.
  Comparing the signed principal/agent/purpose/audience/version with current
  transaction authority now rejects that stale proof. Cached challenge replay
  also requires the exact current challenge version.
- Rotation independently queries normalized command result JSON to assert no
  bearer is stored, checks original token denial and restored new-token access,
  restarts the service and verifies exact token replay, then recovers access and
  verifies superseded replay fails. Concurrent equal-version requests return
  one201/one409 and normalized live credential count remains one.
- Missing-key rollback, scope escalation and database attempts to mutate immutable
  scope metadata are denied. Encrypted browser-cookie runtime test additionally
  exercises wrong-origin/missing-CSRF/foreign-owner rotation and exact response
  replay through a distinct LOGIN NOSUPERUSER NOBYPASSRLS DB role with app membership.
- A controllable private issuance clock characterizes the existing 15-minute replay
  expiry without rewriting immutable metadata. Key-ID/secret rollover deny bearer
  regeneration while previously issued digest authentication remains active.
  Configuration rejects an implicit/missing/wrong-length/cursor-reused key and
  copies supplied key material rather than retaining a caller-mutable slice.
- Two authority UI tests first failed on an empty component, then proved reviewed
  credential/scopes/ETag and exact challenge/signature bytes. The built console
  never asks for a private key; new tokens use the existing memory-only dismissible
  display and are never saved in local/session storage.

## Validation evidence and limits

Fresh UTF8 PostgreSQL `/tmp/undergrid-pg-control-fresh`, exclusively-owned port
55444, applied final migration014. The previous owner-console fixture was stopped
and its data preserved. Focused initial four domain race tests passed. After the
signed-version fix, root's initial full-access run passed the five domain lifecycle
tests and restricted-browser runtime proof: network4.475s, identity2.422s. This older
child session still blocks local sockets, so root runs these authorized checks
without additional approval. Final seven domain/configuration plus restricted-browser runtime tests also
passed from root's full-access session88324: network7.871s, identity2.697s.
These include bounded expiry and key-ID/secret mismatch.

Frontend `npm test` passes8 tests, strict TS/Vite production build passes, and
Chromium passes3 built-bundle tests including rotation/exact signing proof,
If-Match/CSRF, memory-only credential display and explicit unverified endpoint/
unknown ownership labels. Browser API wire fixtures are labeled; actual domain/
RLS/cookies use the real Go/PostgreSQL tests. No external provider or public host
acceptance is claimed. Go vet, formatting and diff checks pass for touched paths.
Dependencies/lockfile are unchanged from the audited owner-console tree. The Go API binary also builds successfully with the dedicated issuance startup wiring.

An attempted full network/identity suite on the accumulated fixture failed existing
`TestCommittedIntentionRunsThroughDurableWorker` after its opportunity deadline;
identity runtime tests passed. Repeated prior worker fixtures left a loaded queue.
This is not hidden by weakening the test or declaring the full suite green. Root
must validate the exact combined release on a fresh integration fixture and also
retain loaded-queue/fairness qualification as a separate operational requirement.

## Pragmatic quality review

DRY: registration, rotation, recovery and replay share domain scope/policy validation.
The issuance derivation has one owner; cached responses cannot bypass live target/
policy authorization. Orthogonality: owner UI/session authentication, network key
issuance and financial sandbox use separate credentials/configuration/authority.
Contracts: principal/agent locks serialize versioned transitions, immutable digest/
metadata and terminal revocation survive replay, and signed bytes carry their own
version rather than trusting a newer request ETag. Readability: credential issuance,
control proof and presentational authority UI have distinct files and named
operations. Recovery: missing key rolls back revocation; same-key replay needs the
same protected key and live target; explicit owner recovery handles unavailable or
superseded results instead of resurrecting old credentials. Effects: connections/
rows/transactions close and no bearer enters persisted command results or logs.

Concrete fixes found: quoted numeric ETags remain the existing wire contract;
raw signing-version binding closes a reproduced stale-authority path; the local
node_modules symlink is ignored without adding dependency artifacts. A flaky DOM
`datetime-local` typing sequence now waits for policy hydration and dispatches its
real change event, preserving the same explicit UTC payload/ETag assertion.

## Runbook, release and retrospective

See `credential-issuance-runbook.md` for network secret ownership,15-minute drained
rollover, lost/compromised-key recovery and immutable retention. Post-deploy proof
requires ordinary owner challenge/rotation/replay/revoke and nonsecret normalized
checks from that runbook; a health response is insufficient. Root owns feature PR,
exact release-SHA integration/promotion gates and deployment approval.

Applied skills: undergrid-implementation, runtime-lifecycle-testing,
release-contract-validation and session-process-retrospective. Learned/test gap:
a freshly supplied If-Match can conceal an older signed statement; both signed
version and current transaction version require assertions. Institutionalize that
pattern in future proof/consent lifecycle tests, not a new generic framework.
Automation/release gap: clean migration proof and populated-queue runtime proof
must remain distinct so an empty fixture never masks fairness/backlog defects.
Skill guidance should require naming the credential replay secret owner and
redaction boundary; mandatory repo tests, not a skill claim, enforce no persisted
plaintext bearer. Historical provider/code-fixture distinctions remain intact.
