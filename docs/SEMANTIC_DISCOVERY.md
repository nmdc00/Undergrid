# Locked vertical: semantic demand discovery

Assigned from release135b3fe under full r2 implementation authorization. Owning requirements: INT-02/03/04, TDD §§3.1/4/5.4/9 and threat-model disclosure/tenant boundaries. Branch `feat/v0.1-semantic-discovery`, isolated worktree `/tmp/undergrid-worktrees/semantic/feat-v0.1-semantic-discovery`. Network migration033 is reserved; inactivity owns030/031/032. No semantic migration has been written or applied at this design checkpoint.

## Observable contract

A persisted active intention can discover a paraphrased publicly approved intention through real PostgreSQL full-text and pgvector cosine retrieval, receive ranked constraint-grounded evidence and a durable deduped notification. Lexical overlap is not required; lexical overlap alone cannot establish semantic compatibility. High similarity never overrides incompatible or unknown hard constraints. Current disclosure, principal/agent status, immutable revisions and withdrawal/expiry are rechecked before writes and reads.

Matching uses bounded top-k full-text and actual pgvector nearest-neighbor candidate sets, a versioned deterministic ranking rule, explicit bounded user thresholds and recorded model/input/projection hashes. Cosine is similarity, not a confidence probability. Pairwise evidence persists; compatible groups are bounded computed, principal-deduped views that check every pair and the common license intersection, never transitive stored clusters or account-independence claims. Existing configured-off exact behavior must remain honestly labeled exact.

The hard-constraint evaluator remains deterministic and separately testable: output/input schema family, required fields, license options, declared coverage/exclusions, freshness and delivery deadlines. Undeclared required hard evidence yields clarification. Registered shared demand combines requirements only where an intersection/union is explicitly feasible; semantic similarity supplies no delivery, licensing or factual assurance. External publisher/capability observations retain their separate source/fit qualification and are not registered participants.

## Projection and model boundary

The default embedding input is only an approved redacted-public summary. Never automatically send private objectives, budgets, private preferences or room/artifact content. Private/restricted owners may explicitly select a separate bounded query projection and acknowledge the exact configured embedding-profile hash only when the operator configuration explicitly permits that class of projection. No private source becomes a discoverable candidate. Matching policy/profile metadata must be available to authorized clients so this consent is reviewable; changing a provider/model does not silently reuse an old private disclosure grant.

One immutable embedding/model/input evidence tuple belongs to each immutable intention revision, following r2's table budget. A narrow write-once worker update may fill pending embedding fields; it cannot change the document/public projection or overwrite completed evidence. A model/profile change requires a new revision rather than overwriting history. Model identity is a configured/publisher assertion until real provider/model qualification occurs.

A trusted operator file selects an HTTPS endpoint, provider/model/revision, dimensions, permission evidence and disclosure policy. There is no chosen production provider or implicit provider authority. The bounded HTTP adapter's pinned contract sends selected text only, refuses redirects, validates TLS, model/revision/input hash, dimensions, finite nonzero vectors and bounded responses. API keys are separate operator secrets, never participant/network credentials. Test-only loopback/custom CA configuration is explicit and rejected in production.

Embedding network calls happen outside database transactions/authority locks. The worker reads only selected projection data, releases the read transaction, calls the configured endpoint, then reacquires current principal/agent/intention authority and conditionally persists one result for the exact still-current eligible revision. Retry/restart dedupes; malformed/model-mismatched responses and revoked/changed inputs cannot publish evidence or opportunities. Queue repair/continuation and committed candidate-to-watcher refresh must cover semantic matches without weakening the existing watcher latency/shutdown gates.

## Behavioral proof before implementation

The initial real-HTTP/worker red will publish two public paraphrases with no shared words, configure a controlled HTTPS embedding endpoint, run the existing worker through actual separate restricted API/worker LOGINs and require a semantic opportunity. The endpoint fixture supplies controlled vectors solely to prove numeric retrieval/runtime wiring; it is not real-model relevance or provider verification. Further tests must cover full-text-only candidate inclusion, cosine ranking/thresholds, highest-similarity hard conflict, unknown evidence, private-data non-disclosure and explicit private projection consent, revision/revocation races, immutable writes, invalid/duplicate responses, retries/recovery, principal dedupe/nontransitive groups, protocol/schema parity and the unchanged exact/watch contracts.

## Local pgvector qualification

The host PG16.11 lacks installed pgvector/server headers and root-owned install directories cannot be written by this account. The pinned Ubuntu `postgresql-16-pgvector`0.6.0-1 package was downloaded (175000 bytes; SHA256 `38a62e892ba162570125ee5ba3041a79a50ee272c68c79817dee22f7a3b5817e`) and extracted under `/tmp/undergrid-pgvector-fixture/prefix`. Matching installed PostgreSQL16.11 server/initdb/pg_ctl binaries were copied into that user-owned layout; unchanged standard shared/library files are symlinks, with the actual packaged vector files added. No system package or retained postmaster was changed.

A separate UTF8/C.UTF-8 network-only fixture `/tmp/undergrid-pg-semantic-55497/{data,socket}` runs port55497. Actual `pg_config` SHAREDIR reports the relocated prefix. In dedicated `vector_qualification` database, normal `CREATE EXTENSION vector` succeeded at0.6.0 and real SQL cosine returned0.800000011920929 for `[1,0,0]` vs `[0.8,0.6,0]`. The application `postgres` database has not had the vector extension or033 manually installed. Application qualification must use normal unmodified migrations; no fake extension catalog/ledger or unpackaged SQL is allowed. Existing94/95/96 fixtures and credentials remain separate.

Primary extension semantics are documented in the [pinned pgvector0.6.0 README](https://github.com/pgvector/pgvector/blob/v0.6.0/README.md): exact nearest neighbors and cosine operators plus hybrid PostgreSQL full-text search. Real model/provider configuration, source permissions/relevance, performance qualification, legal disclosure policy and deployment remain explicit external gates. This checkpoint establishes a concrete task and toolchain, not implemented semantic matching or full §5.4 completion.

## Actual missing-behavior checkpoint

The initial real restricted-login worker test ran on isolated97 against release
source135b3fe and failed24.841s: no semantic opportunity for the nonlexical
paraphrase and no configured endpoint inputs. Log
`/tmp/undergrid-semantic-primary-red97.log`. This is a missing-runtime-behavior
RED, not a vector/provider/relevance pass. The assigned agent then stopped at a
usage limit; root preserved this test-only checkpoint before implementation.
Inactivity migration032 was subsequently reserved/applied for proven privileged
helper hardening, so this feature owns033. No semantic SQL has been applied.

## Current public pipeline checkpoint

Public projection embedding, immutable evidence, actual full-text/pgvector
retrieval and current-projection HTTP reads now have scoped local proof on
source5bcd158. See [actual evidence and preserved failures](evidence/semantic-discovery.md).
Migrations033 and034 are applied/frozen on qualified97 only;034 is the narrow
public metadata/RLS correction. Earlier “not written/applied” statements describe
the initial design checkpoint. Private query consent, user thresholds, richer
typed constraints, compatible groups and broad qualification remain open. This
branch is unmerged and does not complete §5.4 or the full TDD.

A further application-role insertion RED led to normal migration035's narrow
revision-column INSERT grant. Applied033/034 remain unchanged;035 is frozen and
the latest14-case race slice passes. Provider observations remain worker-only.
The complete remaining feature list above is unchanged.

## Typed constraints and constraint-first retrieval checkpoint

Typed coverage, freshness, delivery and input alternatives now have scoped actual
HTTP/contract and PostgreSQL lifecycle proof. Hard eligibility precedes all three
retrieval limits; a 25-candidate saturation regression proves incompatible demands
do not hide an eligible exact match. See the evidence checkpoints for actual REDs,
restricted-role race results, retained-queue failure and scan-cost tradeoff.
Private selected-query consent, user thresholds, compatible groups, complementary
discovery and broad queue/load/provider qualification remain unfinished. This
feature branch remains unmerged.

## User threshold checkpoint

Optional per-intention semantic cutoffs0–1 now have actual restricted-role worker,
HTTP/Ajv, audit, privacy, revision and owner-console proof. Empty/default selection
uses the operator cutoff; explicit choices may be stricter or looser. New semantic
evidence names the effective cutoff. See the evidence checkpoint for measured
RED/green results and the controlled-vector/provider distinction. Private query
consent, computed groups, complementary discovery and broader queue/load/provider
qualification remain unfinished; user thresholds are no longer an open runtime
implementation item.

## Private query consent checkpoint

The domain/API/worker path now supports explicit selected private query text,
reviewable exact-profile consent, operator class gating, current-policy checks,
write-once database binding and owner-only public-material matches. Actual
restricted-role PostgreSQL/HTTP and blocked-provider revocation tests pass; see
[evidence](evidence/semantic-discovery.md) for the missing-route/storage/fanout/
old-worker REDs and the final scoped gate. Migration036 is applied/frozen on98;
033–035 are unchanged. Private-query console controls remain pending.

The retained-queue primary worker deadline still fails. Both vectors were ready
but the post-embedding wakeup remained behind old queued work; recovery/fairness
must be proven without deleting retained jobs or extending the deadline. Computed
groups, complementary discovery and full release/provider qualification remain
open. This checkpoint does not establish full §5.4 completion.

## Reserved fresh and ready-work capacity

The unchanged retained98 primary worker failure now has a scoped before/after
proof:13.332s failure before isolation,5.322s pass afterwards. Fresh configured
embedding jobs and committed ready-source/fanout/pair work have separate bounded
capacity. Fanout checks actual current vector/profile/cutoff readiness before
queueing semantic pairs. The affected race gate passes17 top-level tests24.035s,
including private consent, typed constraints and held-provider recovery. No
retained queue was deleted and the match deadline was not increased. See the
evidence for the separate held-provider characterization and setup failures.

The earlier statement that the primary retained-queue deadline still fails is
historical at its recorded checkpoint. Serial repair error isolation, broader
queue/load/provider qualification, private query controls, computed groups and
complementary discovery remain incomplete; this is still an unmerged feature.

## Independent semantic repair checkpoint

Configured `ScanDueIntentions` now commits up to1000 per-intention durable
repair jobs without provider I/O. Job identity includes intention revision and
profile; pending/running/retryable work is excluded before the batch limit and
River uniqueness also closes concurrent coordinator insertion races. A later
revision has a separate wakeup. A dedicated two-worker repair queue keeps
provider failures separate from fresh embeddings and ready matching. Existing
configured-off exact sweeps retain their behavior. Each repair reuses current
domain eligibility and observation guards; stale revision/profile jobs finish
without starting work. No new table or migration is needed.

Actual restricted-role HTTP/PostgreSQL RED0.951s reproduced one provider
rejection aborting the coordinator (`/tmp/undergrid-semantic-repair-red98.log`).
The first implementation then failed the unchanged eight-second recovery
deadline behind retained jobs (`/tmp/undergrid-semantic-repair-green98.log`).
Reducing active fetch cooldown, while retaining two effect slots and100ms idle
polling, passed6.829s (`/tmp/undergrid-semantic-repair-green-throughput98.log`).
The oracle checks no coordinator HTTP calls, durable replay deduplication,
distinct revision wakeups and completed healthy repairs/opportunity despite a
controlled rejected projection. No retained queue/data was deleted. Vet passed;
the affected race gate passed13 top-level tests23.078s with zero failures/skips
(`/tmp/undergrid-semantic-repair-affected98.jsonl`).

Quality review: repair scheduling owns queue effects, while existing domain
commands own authority/matching and provider observation policy. Retry state
remains durable; failure affects one job. The bounded batch avoids excluding
later due intentions behind already active repairs. This is local fixture
evidence, not provider relevance or full load qualification. The legacy explicit
refresh sweep and in-flight provider request deduplication still need review.
Private-query UI, groups, complementary discovery and release gates remain open.
