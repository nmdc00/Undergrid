# Current semantic projection authority

Current integration `feat/v0.1-semantic-integration` additionally binds private
processing permission to immutable server-owned revision/profile/query-hash
metadata. Explicit validated create/full revision commands mint that binding;
partial fulfillment carries the historical binding without renewing it. Legacy
unbound completed observations remain retained but cannot authorize current
semantic reads, groups or receipt recovery. See `SEMANTIC_INTEGRATION.md`.
The focused provider fixtures now call the actual dedicated embedding worker;
local scans do not perform provider I/O. An unreadable optional semantic policy
keeps embedding work retryable and authorizes no derived semantic result, while
valid exact/external scans and generic reads continue. Initial invalid operator
configuration at worker startup remains unchanged. The historical branch/test
record below is preserved; current combined proof is 36 top-level/40 subcases
PASS at `d01aedd`, 94.503s, with broader release integration still pending.

Scoped correction to TDD §5.4's operator-controlled disclosure boundary. An embedding worker's startup client is not continuing permission to send public or privately selected text. The observation path checks the current trusted operator file immediately before provider I/O and again before the write-once observation update. The current profile must equal the worker's pinned startup profile, and the current projection class/private profile acknowledgment must remain eligible. Removing or replacing the profile declines old observations; an unreadable or malformed configured file returns a retryable policy-unavailable error without disclosing text or publishing a new observation. Existing completed evidence remains immutable history.

Branch `feat/v0.1-semantic-profile-authority` was cut from release `d1e866d` and explicitly depends on semantic checkpoint `9b59521`. It changes only the shared projection authority check, its two observation callers, focused runtime tests, an existing withdrawal fixture's trusted config setup and this document. No migration, discovery identity, opportunity read behavior, financial database, provider selection or deployment is introduced.

## Actual test-first evidence

Two real PostgreSQL race tests contain six controlled TLS cases: operator configuration removed, replaced or public processing disabled before an old worker's provider call, and the same changes while that call is held in flight. API and worker use separately verified LOGIN NOSUPERUSER NOBYPASSRLS identities. The fixture sends only its approved public summary; its private objective is never a permitted endpoint input. It verifies provider POST counts, durable embedding count and opportunity count, then withdraws its own test intention through the versioned API without deleting retained observations or queues.

Actual RED on the dependency source failed all six cases in 3.212 seconds (`/tmp/undergrid-semantic-public-authority-red98.jsonl`). Each pre-I/O case incorrectly sent one POST and persisted one embedding; each in-flight revocation also persisted an embedding. Unique fixture projections yielded no opportunities, so the demonstrated defect is unauthorized public disclosure/durable observation, not a claim of an observed duplicate match or a model relevance result. There were no fixture or compilation errors in that RED.

The minimal correction replaces the private-only freshness check with one current-profile/projection check for both disclosure classes at the existing two boundaries. The existing withdrawal-during-provider-I/O fixture previously constructed only an in-memory client; it now also records the matching temporary trusted operator file. Its original withdrawal, timing, embedding and opportunity assertions remain unchanged. This fixture setup correction is distinct from the six behavioral failures above.

Final affected PostgreSQL race validation passed on 2026-10-23: four top-level tests and all six authority subcases, zero failures or skips, package elapsed 13.126 seconds (`/tmp/undergrid-semantic-profile-authority-green98.jsonl`). The same selector included actual withdrawal during provider I/O (4.45 seconds) and the existing pinned private-consent/owner-only lifecycle (4.82 seconds). Sequential `go vet ./internal/network` passed (`/tmp/undergrid-semantic-profile-authority-vet.log`). Source remained frozen throughout both checks. This local fixture does not qualify any actual provider, model relevance, legal permission, deployment or the full semantic discovery vertical.

## Quality and remaining boundaries

DRY: one current projection eligibility method owns public/private observation permission at both boundaries; the existing selected-projection policy owns class and profile-bound private acknowledgment. Orthogonality: operator policy, provider transport and transaction-owned observation persistence remain separate responsibilities. Provider I/O stays outside principal/agent/intention locks. Failure recovery: temporary policy-read failures retain retryability, while removed or replaced authority declines stale work; a late valid provider response cannot independently grant authority to persist. Immutable completed observations are preserved.

Discovery evidence identity across model changes, current-profile filtering of direct opportunity reads and stale workers scanning already retained embeddings remain separate integration concerns owned by the semantic discovery branch. This correction does not claim to solve them. Its PR/release integration remains pending the semantic dependency. A trusted-file change cannot atomically cancel a POST that has already begun; the second boundary prevents that late response from becoming a newly authorized durable observation.

## Already captured observations and stale matching workers

The public disclosure check alone did not cover already embedded intentions: the observation method correctly returns without a new provider call, but a long-lived worker could still use its old profile to publish new semantic matches. A separate actual PostgreSQL RED exercised operator profile removal, replacement and public processing disablement after two real controlled captures and before the old worker's scan. All three cases wrongly committed one opportunity, one `opportunity.available` event and one inbox item; package elapsed 2.670 seconds (`/tmp/undergrid-semantic-stale-worker-write-red98.jsonl`). There were no extra provider calls or fixture failures.

The scanner now derives one local current-profile eligibility value after its transaction-owned source authority/document checks and uses it for both semantic candidate retrieval and semantic compatibility fallback. It never mutates the shared service provider. Withdrawn authority uses the existing exact comparison path; unreadable or malformed current policy remains retryable. Completed observations remain immutable and are not deleted. This correction affects new writes; current-profile opportunity read filtering and profile-specific evidence identity remain owned by the semantic dependency branch.

Final affected race validation passed six top-level tests, including all nine current-authority subcases, with zero failures or skips, package elapsed 13.263 seconds (`/tmp/undergrid-semantic-stale-worker-write-green98.jsonl`). The selector retained private-consent isolation, withdrawal during provider I/O and hard-constraint-before-candidate-limit retrieval. That last fixture's old synthetic in-memory profile was replaced with a matching trusted operator file, so it continues to exercise semantic retrieval instead of silently taking exact fallback. Sequential `go vet ./internal/network` passed (`/tmp/undergrid-semantic-stale-worker-write-vet.log`). No actual provider qualification or deployment is claimed.
