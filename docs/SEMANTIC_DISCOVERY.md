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

## Owner create-query console checkpoint

The create-intention console now offers an optional explicit policy review. It
fetches current owner-authorized matching metadata, validates provider/model and
exact profile/privacy hashes, displays the selected provider and policy evidence,
and accepts a separately entered query with an unchecked disclosure consent.
Editing the query clears consent. Required form controls block an incomplete
selection; removing it allows an ordinary private intention with no query grant.
The private objective never populates this external query automatically. Successful
creation clears the query component and consent. Disabled/malformed/unavailable
policy cannot grant processing, and unmounted requests cannot update the view.
API/domain validation still decides current-profile authority at commit.

Actual App behavioral RED: absent policy-review control
(`/tmp/undergrid-private-query-app-red.log`). The earlier component import
failure (`/tmp/undergrid-private-query-ui-red.log`) is scaffolding failure, not
a behavioral RED. Focused App/component tests passed23 cases7.01s, then the
full UI suite with the no-consent submission/removal oracle passed59 tests
across8 files18.20s (`/tmp/undergrid-private-query-full-ui.log`). TypeScript and
production build passed (`/tmp/undergrid-private-query-ui-build.log`).

Quality: one component owns review/disclosure controls; generated wire types
define the grant; existing shared request code owns cookies/CSRF/retry identity.
No provider secret or bearer credential is introduced. Existing revision-edit
query review/preservation/removal controls remain pending; this checkpoint
completes create-flow controls only and does not complete the discovery vertical.

## Owner revision-query console checkpoint

Revision editing now displays an existing selected query and requires a conscious
choice before save: review the current provider profile and explicitly consent
for the new revision (with the existing or edited selected text), or remove the
query. An unrelated objective edit cannot silently omit existing disclosure
selection. Failed/disabled policy review leaves that decision required; explicit
removal remains available. Canceling/unmounting invalidates pending policy reads.
The shared component keeps new-query consent unchecked and clears it after edits.
Current API/domain profile checks remain authoritative, including stale-profile
rejection after the policy read. No private objective auto-populates the query.

Actual App RED reproduced both keep/remove cases sending a revision before any
query decision (`/tmp/undergrid-private-query-revision-red.log`,2 failed cases).
Focused initial correction passed26 tests7.89s. Further behavioral review
reproduced whitespace-only and1200-byte UTF-8 selection silently passing native
form validation when the required consent checkbox was disabled
(`/tmp/undergrid-private-query-input-validity-red.log`,2 failed cases; also
preserved in the intermediate full UI log). The query field now owns custom
validity matching the shared Unicode/byte predicate, so invalid selections cannot
be submitted as an implicit removal. Final full UI suite passed65 tests across
8 files19.16s (`/tmp/undergrid-private-query-revision-final-ui.log`); TypeScript
and production build passed (`/tmp/undergrid-private-query-revision-final-build.log`).

Quality review: create/edit share the same policy/consent component and generated
wire grant; existing owner versioned PATCH retains ETag/CSRF/retry behavior. The
new revision gets an explicit grant choice rather than a carried checkbox. The
UTF-8 check is a user-facing guard; server validation remains independent. This
closes the recorded create/edit console gap locally, not compatible groups,
complementary discovery, provider qualification or full-release integration.

## Explicit refresh isolation checkpoint

Configured global refresh now uses the same independently durable revision/profile
repair coordinator as due sweeps, while including not-yet-due active watches.
The shared projection policy selects priority2 for selected semantic queries
in both due repair and explicit refresh; exact-only watches retain priority4
background work. A newly persisted observation promotes its exact revision/profile
repair wakeup to priority1 in the same transaction as vector/ready-work persistence. Both use the same
two-worker lane and duplicate/retry protection. No provider I/O occurs in the
coordinator. Configured-off exact refresh remains on its existing path.

Actual new oracle on retained98 moves its three controlled intentions' scheduler
timestamps one day ahead, then requires refresh replay deduplication, separate
revision wakeups and healthy completion/opportunity despite one rejected query.
RED0.969s reproduced the serial provider error
(`/tmp/undergrid-semantic-refresh-red98.log`). Initial durable scheduling and
then universal refresh priority still failed the unchanged eight-second healthy
recovery deadline (logs `semantic-refresh-green98.jsonl` and
`semantic-refresh-priority-green98.jsonl` under `/tmp/undergrid-`). A four-worker
trial also failed and was reverted (`semantic-refresh-four-workers98.jsonl`).
Queue inspection showed refresh flooding the foreground with exact-only watches.
Using the existing selectedProjection policy for priority, with two workers,
passed the focused oracle7.12s / package8.156s
(`/tmp/undergrid-semantic-refresh-selected-priority98.jsonl`).
Vet passed; broader affected gate is pending in
`/tmp/undergrid-semantic-refresh-affected98.jsonl`.

No queue/history deletion, migration change or deadline extension occurred.
The scheduler uses shared domain projection eligibility rather than duplicating
disclosure rules. Priority is local scheduling metadata and does not change job
identity, authorization or provider verification. Larger-load fairness, atomic
in-flight provider claims, groups/complementary discovery and full release gates
remain open.

The first broader affected gate failed62.763s with two distinct findings
(`/tmp/undergrid-semantic-refresh-affected98.jsonl`): scheduled semantic work
still had background priority and missed its unchanged deadline; the legacy
recruitment oracle also depended on retained eligible accounts and emitted only
three offers. The source now applies shared projection priority to due repair
as well. The legacy fixture explicitly creates five eligible foreign recipients
instead of relying on unrelated retained accounts; its five offer/event/inbox
assertions remain unchanged. This is a test setup correction, not evidence of
a provider or beneficial-owner independence defect. Final affected rerun is
`/tmp/undergrid-semantic-refresh-final-affected98.jsonl`; vet passes.

The next broader run still failed70.750s on scheduled-repair latency
(`/tmp/undergrid-semantic-refresh-final-affected98.jsonl`); the corrected legacy
fixture passed. Retained fixture inspection found92 active public watches,
including39 completed vectors from older immutable profiles. Equal selected-query
priority was insufficient. Ready observation persistence now atomically promotes
its existing pending/scheduled/retryable repair job to priority1, while pending
selected queries use priority2 and exact-only watches4. No provider authority or
job identity changes. Both unchanged recovery oracles then passed package9.396s
(`/tmp/undergrid-semantic-refresh-ready-promotion98.jsonl`; explicit refresh2.55s).
Vet passed. The final broader gate is
`/tmp/undergrid-semantic-refresh-ready-final98.jsonl`: PASS61.986s,
19 top-level tests, zero failures/skips. This supersedes earlier pending
affected-gate statements; scheduled repair passed2.76s in the combined run.

Already-observed vectors scheduled by later repair, as well as very large queue
and model-cutover workloads, still require additional fairness qualification;
this checkpoint specifically proves promotion when the observation completes
after repair scheduling. No passing whole-feature/release claim follows.

## Observation-before-scheduling recovery checkpoint

The lifecycle oracle now stops the actual worker after healthy completion,
requests another explicit refresh, and inspects the newly durable current
revision/profile repair jobs. The coordinator must make zero provider calls
and assign priority1 to already-observed vectors. Actual RED3.067s found zero
ready-priority jobs (`/tmp/undergrid-semantic-already-ready-red98.log`).

The coordinator now reads exact-profile vector readiness with the immutable
revision and selects priority1 immediately when shared projection policy permits
it. Pending selected queries remain2 and exact-only watches4. A revision FOR SHARE
lock spans selection and insertion, so a concurrent observation either precedes
the readiness read or waits until insertion and promotes the job in its own
transaction. No transaction includes provider I/O. Both lifecycle oracles passed
package9.907s (`/tmp/undergrid-semantic-already-ready-green98.jsonl`), including
actual post-stop rescheduling; no queue deletion or deadline change.
Vet passed. Affected semantic/private-consent gate passed20.583s,
11 top-level tests with zero failures/skips
(`/tmp/undergrid-semantic-already-ready-affected98.jsonl`).

This closes the previously recorded observation-before-scheduling priority gap.
The share-lock ordering is an implementation argument supported by affected
race tests, not a claim of exhaustive schedule/model-cutover qualification.
Full load, in-flight provider deduplication, groups, complementary discovery and
release/provider qualification remain open.

## Bounded exact compatible-group checkpoint (2026-10-20)

Owner/agent GET intentions/{id}/compatible-groups plus the shared finite
intention_compatible_groups MCP/A2A operation derive bounded groups from current
revision-bound opportunities and current safe public projections. Each group
contains the current owned active monitored source, checks every member pair's
exact declared outcome and hard constraints, intersects license options across
all members, and counts a principal once. At most20 candidate records and10
deterministic greedy views are returned; this is explicitly nonexhaustive,
not a persisted/transitive cluster graph or evidence of beneficial-owner
independence. Public candidates are revalidated at read time. No private query,
objective or budget enters the group projection; members expose IDs and declared
constraints only. An owner can inspect only their own source through the same
owner-session transaction/agent attribution boundary as opportunity reads.

Initial HTTP missing-route RED1.531s is retained at
/tmp/undergrid-compatible-groups-http-red98.log. Initial policy test compilation
failed on absent types/functions (scaffolding, not behavioral RED); the policy
oracles then passed. Initial actual endpoint slice passed2.274s. Extended
protocol test first failed setup because retained98 postmaster was stopped after
host interruption; normal pg_ctl restart recovered the same database and logs,
without data/queue deletion. The next run completed runtime assertions but the
validator's default artifact fixture requirement failed; a dedicated group mode
now retains generic request/response validation and adds independent private-
field, false independence, empty-license and false-semantic-mode denials.

Final real restricted-login race slice passed15.429s
(/tmp/undergrid-compatible-groups-protocol-final98.log), including group policy,
principal dedupe, global license versus pairwise overlap, REST/owner/MCP/A2A
reads, immediate withdrawal and foreign-owner404. Canonical OpenAPI/generated
types and142 schemas passed before the final dedicated validator correction;
actual HTTP schemas and negative oracles pass in the final race test.

The response honestly reports exact_declared_outcome and
semantic_cross_member_evidence_pending. This checkpoint does NOT finish computed
semantic groups: current public pair-score projection, current-profile numeric
checks between candidates, richer group/runtime/privacy/load qualification and
owner group display remain pending. Network037 is reserved for the narrow public
pair-score helper;038 is independently reserved/applied on the external-
notification fixture. This feature remains unmerged.

### Independent review corrections and final exact-group proof

Read-only swarm review found a real global-feasibility defect: input families
{A,B},{B,C},{A,C} pass every pair but have no common group input. A new actual
policy RED failed0.018s (`/tmp/undergrid-groups-input-alternatives-red.log`).
The group builder now accumulates typed CombinedConstraints through the existing
domain evaluator before accepting each member; global input alternatives and
other typed constraint rules share their policy owner. License intersection
remains explicit for legacy exact inputs whose public compatibility shape omits
CombinedConstraints. The policy GREEN passed1.028s.

Review also identified the upcoming external-notification integration seam:
those opportunities have observation IDs rather than candidate_intent_id.
The existing extraction loop was characterized/extracted, then an actual mixed
opportunity oracle reproduced its nil-to-string panic
(`/tmp/undergrid-groups-external-member-red.log`). Group extraction now accepts
only shared_demand rows with valid registered candidate IDs. External requests
and capabilities never become group members or counted registered demand.
The final independent policy slice passed1.027s
(`/tmp/undergrid-groups-policy-final.log`).

Final affected real HTTP/protocol/policy race suite passed **5 top-level cases**,
zero failures/skips, package15.558s
(`/tmp/undergrid-compatible-groups-final-reviewed98.jsonl`). Vet passed
(`/tmp/undergrid-groups-reviewed-vet.log`). This includes actual REST, current
owner-session, MCP and A2A reads and runtime Ajv privacy/evidence denials. The
independent review otherwise found no material authority/public-projection leak.
Exact-group code is a checkpoint of the still-unmerged full discovery branch;
semantic cross-member evidence, group display and broad qualification remain
explicitly pending as described above.

## Pairwise semantic group work in progress (20 October)

The next group oracle traverses actual restricted API/worker logins and the
controlled trusted HTTPS embedding adapter. Four nonlexical public summaries have
numeric vectors where the source matches two peers but those peers do not match
each other; a fourth member matches both. The existing exact-only endpoint
returned zero instead of two three-member views (3.502s,
`/tmp/undergrid-semantic-groups-red98.log`). These are controlled numeric fixtures,
not model relevance or provider verification.

The pending implementation evaluates each pair independently and still requires
typed hard-constraint aggregation and a common license intersection. Network037
adds a bounded public pair-score projection: at most20 explicitly named current
public active intentions, live principals/agents and one exact profile; no raw
vector, private document or participant preference crosses the helper. Current
source-to-candidate opportunity proof and both current public projection hashes
bind the computed view. A changed operator profile cannot reuse old source
proof. Exact groups retain their exact label; semantic views have a distinct
`pairwise_semantic_and_hard_constraints` contract value. Identity remains
`common_control_unknown`; views grant no membership or assignment authority.

Independent review identified and corrected the closed-schema enum and potential
mixed-revision projection evidence before qualification. The first green attempt
found SQL parameter/column ambiguity (migration transaction rolled back); the
second applied normal037, passed original pairwise group assertions, then exposed
an invalid revision-PATCH test fixture (400). The fixture is corrected to use the
published revision envelope; final lifecycle/schema/vet qualification remains
pending. Applied037 is frozen at
`3fb1eb8d5064b9a95726bbab5873c0a4f2c8b812c7ef3ceb446f9ddbcc6141a8`.
The behavior is uncommitted/unmerged and does not complete the semantic vertical.


Final corrected scoped qualification passed six top-level PostgreSQL/race cases,
zero failures/skips,20.779s (`/tmp/undergrid-semantic-groups-final98.jsonl`),
including the actual semantic HTTP/schema oracle, nontransitivity, changed
candidate revision and changed operator profile. `go vet ./internal/network`
passed (`/tmp/undergrid-semantic-groups-final-vet.log`); canonical lint,142 schemas
and generated-type parity passed
(`/tmp/undergrid-semantic-groups-final-contract.log`). Subsequent route-description
wording was reconciled with the already-tested semantic mode; no behavior changed.
Quality review corrected duplicated evidence assumptions by binding the narrow
public score projection to both loaded public hashes. Candidate preference
thresholds remain private; only the requesting source threshold is evaluated.
Group UI, complementary capability discovery and full release/provider/load
qualification remain open. This checkpoint does not claim complete TDD delivery.

After reconciling release `d1e866d`, the group lifecycle also characterizes an
explicit profile-consented private source query. It receives current public group
views without publishing its objective/selected text; a foreign owner is denied,
and the private intention never becomes another owner's public group candidate.
The combined exact integration slice passed ten top-level race tests, zero
failures/skips,25.027s (`/tmp/undergrid-semantic-private-groups-release98.jsonl`),
including owner plan review and message lifecycle/rollback alongside groups.
This does not prove absence of the separately reproduced owner final-vote policy
race; its dedicated fix remains a required integration dependency.

## Remaining code audit, 20 October evening

A fresh independent code/requirement audit leaves these behaviors open before a
full semantic/discovery-vertical claim. None is replaced by provider qualification:

- Root is adding an actual public model-cutover regression: repeated identical
  approved documents currently hash opportunities without the profile identity.
  The assertion later reproduced the failure in4.864s; the corrected six-case race slice passed19.776s.
- A separate dependent feature is testing current public-profile authority before
  HTTPS provider I/O and again before persistence. The existing current-policy
  helper checks private consent but immediately accepts public inputs.
- Ordinary opportunity reads bind stored evidence to current vectors but still
  need current operator-profile eligibility, as group reads already enforce.
- The published dismissed state has no authorized owner/agent mutation entrypoint.
  Worker uniqueness is not a substitute for an explicit dismissal contract.
- The hard-constraint evaluator returns needs-clarification correctly, but unknown
  candidates are filtered before any user-visible clarification report. That
  distinction from an ordinary no-match remains a missing runtime result.
- Complementary-capability discovery is still missing beyond builder recruitment
  and the separate selected external publisher-declaration fit report.

The completed group viewer remains on a separate dependent feature branch with
115 UI tests/build/147-schema proof; it is not merged. Real model relevance,
permitted provider/source configuration, representative load/retained queues and
restart/in-flight-call qualification remain separate release evidence. The full
TDD also contains other collaboration, artifact, financial UI and operations work.


### Public model-cutover identity proof

The actual controlled HTTPS/API/worker regression re-revises the same approved
public documents under model revision r2 after r1 observations exist. Before the
fix, all new vectors were current but the group read returned zero groups because
the unchanged opportunity identity collided with old-profile rows
(`/tmp/undergrid-semantic-profile-cutover-red98.jsonl`,4.864s). Semantic opportunity
identity now includes the immutable profile hash, while exact-demand identity is
unchanged. A single Go hash helper owns writer and read parity; the SQL pre-limit
filter explicitly uses the same field order. No raw private revision hash is added
to public output, no migration or provider selection is introduced.

The corrected group/model/private-query integration slice passed six top-level
race cases, zero failures/skips,19.776s
(`/tmp/undergrid-semantic-profile-cutover-green98.jsonl`). Current public-policy
revocation and ordinary opportunity-read eligibility are separate ongoing work;
this proof covers successful cutover and stale group suppression, not those
other contracts. Full affected vet/integration remains required after the pending
profile-authority dependency is reconciled.

### Current operator profile on ordinary discovery reads (23 October)

The actual owner/agent opportunity read returned r1 semantic suggestions after
operator configuration changed to r2, even though compatible-group reads hid them
(`/tmp/undergrid-semantic-current-read-red98.jsonl`,3.405s). Ordinary reads now load
current trusted profile metadata without provider I/O and filter semantic proof
against that profile before the bounded SQL result limit. Both owner and shared
REST/MCP/A2A reads invoke the same service command. Exact-demand reads remain
available when semantic configuration is disabled. Invalid configured policy
returns a retryable unavailable error instead of presenting old semantic proof.

The six-case group/private-query/cutover race slice passed21.577s with no failed
or skipped cases (`/tmp/undergrid-semantic-current-read-green98.jsonl`); vet passed
(`/tmp/undergrid-semantic-current-read-vet.log`). Applied migrations are unchanged.
The guard neither overwrites historic observations nor confuses model similarity
with hard eligibility. A separately reproduced old-worker write/notification gap
is being fixed in the profile-authority branch before integration.

## Combined release integration checkpoint (23 October)

Root reconciled release `ab330e3` (external notification PR88 and owner lock PR87)
and the reviewed public profile-authority fix into `d735921`. The worker retains
all19 independent semantic/external/core queues with29 pooled connections,
preserving ten nonfetcher slots. Owner opportunity reads retain their narrow
intents:read scope and the shared current-profile command; canonical schema maps
were reconciled structurally and types regenerated.

The actual combined race gate ran38 top-level cases:37 passed, one failed, no
skips,94.519s (`/tmp/undergrid-semantic-combined-d73598.jsonl`). The failure is
`TestCommittedDiscoveryRefreshesExistingWatcher` before candidate arrival:
the first two watches did not scan within the unchanged five-second bound.
Actual retained database evidence shows their discovery jobs10136/10138 and
fanout10137/10139 unattempted at the deadline.349 older eligible priority1 jobs
were ahead;58 older jobs progressed during the window,293 remained ahead.
Subsequent suite workers scanned these intentions about26s after creation.
This is FIFO backlog starvation, not an observed profile rejection or pool
exhaustion. Neither jobs nor data were removed and no deadline was extended.

The separate fresh-discovery capacity vertical will prove stable fresh/background
routing and aged pending-work recovery, preserving row identity, arguments,
attempts, ordering evidence and independent retained progress. This feature stays
unmerged until its combined worker gate is green. Passing individual semantic,
private/group, external and owner tests does not satisfy that failed invariant.
