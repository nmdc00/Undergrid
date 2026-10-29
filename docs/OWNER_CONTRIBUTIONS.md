# Owner contributions contract

HP3 steps1–3, TDD §§7–9: the currently assigned owner reviews exact task/plan/epoch,
selects supported JSON bytes and an explicit output/progress purpose, uploads to
quarantine and finalizes server-observed immutable bytes/hashes with the exact
lineage/rights manifest. Fixed evaluation and substantive progress review remain
separate from output acceptance and publication consent. No agent content is run.

Active branch `feat/v0.1-owner-contributions`, based on release `21a905a`.
One scoped vertical covers owner submission/recovery/readback; subsequent consent/
publication/reuse controls remain distinct HP3 work. Backend, canonical API, owner
UI and actual browser/worker proof are required before reviewed release merge.

Owner sessions must not borrow or fabricate a bearer credential. Retain legacy
credential-bound agent uploads and add an explicit owner-session upload mode:
current verified owner, owned active assignee, current membership/plan/epoch/policy
and bounded short-lived capability authorize owner PUT. A capability alone cannot
authenticate an owner upload. Mode/generation/digest/expiry are immutable. Preserve
canonical plan/artifact authority lock order and domain-owned transitions, fixed
size limits, immutable finalization, audience rules and exact response recovery.

Initial runtime entrypoint: owner POST
`/v1/owner/agents/{agent}/tasks/{task}/artifact-uploads` with reviewed task If-Match,
claim_epoch/media_type/size_bytes and optional progress audience/reviewer. Resolve
task/plan/room context on the server; authority stays in the existing domain.
Owner-scoped bytes/finalization/read/evaluation routes and actual interfaces must
follow before this feature is complete. A pending upload is not a delivered or
verified contribution. External storage/verifier/legal qualification remains open.

## Quarantine authentication checkpoint — 29 October 2026

Test-only `2a96217` creates a real normally accepted artifact-task fixture with
separate restricted API/worker identities. Missing owner creation returns404
(RED2.674s). Implementation `5c30beb` creates/replays one explicitly owner-session
quarantine generation, with NULL upload_credential_id and correct owned assignee/
principal, while the task remains accepted and existing6 plan/rights votes are
unchanged. First positive test passes2.933s.

Forward migration051 retains legacy credential mode and its nonnull credential
binding, permits NULL only for explicit owner_session mode, and freezes mode,
credential, generation, digest and expiry. Applied migration SHA256 is
`7b2b053a33e8a0676bf449166764c89c5994e4b83c71c025726ae51d981c23f6`;
do not edit this applied file. Later changes require a forward migration.
The domain creates owner-specific capability URLs and separates creation recovery
namespaces between owner and credential channels, including deliberate reuse of
the same actor/key/canonical body. Owner If-Match guards new owner allocations;
legacy agent command shape/behavior remains unchanged. Task/plan/room IDs are
resolved from the owned-task path, never accepted from an owner payload.

First3-test guard attempt fails8.791s because the test's agent replay omitted the
original explicit purpose=output; the domain correctly returns409 for changed body.
That failure is retained. Corrected tests make owner/agent canonical bodies and
keys identical and require separate generations plus unchanged agent receipt;
3 groups/4 subcases pass8.440s. The owner capability fails401 through the anonymous
credential upload route. Missing/stale task version, stale epoch and foreign owner
allocate no rows. A further test revokes the selected agent's actual network
credential: bearer auth fails401 while current owner session creation still
succeeds without borrowing any credential. Attempts to alter upload bindings
must fail PostgreSQL23514, not merely any SQL error. The stricter binding test
passes2.898s; final4-top-level/4-subcase aggregate passes11.464s.

Quality assessment: owner authentication is an internal server-resolved identity
mode, not client input or a new policy engine. Existing domain authority/size/
purpose/audience/quota/epoch checks and canonical plan locking own transitions;
the adapter only resolves context and translates a bounded command. Existing
credential capabilities retain their own auth path. No financial identity,
arbitrary executor, provider approval or external verification is introduced.

This is only creation/recovery/authentication proof. Owner byte-transfer,
finalization/readback/evaluation routes, canonical API, UI, all remaining
concurrency/revocation/expiry guards, actual browser/worker acceptance and review/
merge remain required. The owner upload URL is intentionally not claimed usable
until its scoped byte route is implemented and proven. HP3 stays open; branch
work is local and unmerged.

- `/tmp/undergrid-owner-contributions-create-red-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-create-green-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-create-guards-oct29.jsonl` (changed-body test failure)
- `/tmp/undergrid-owner-contributions-create-guards-corrected-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-authentication-final-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-binding-constraints-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-create-final-oct29.jsonl`
- Retained normally migrated network database `owner_contributions_oct29`, port55502


## Bytes, immutable finalization and evaluation checkpoint — 29 October 2026

Test-only `4c043e9` requires the owner byte/finalization lifecycle and fails405
on the missing PUT route. Implementation `8fd1a8e` maps owned artifact commands
into the existing authoritative artifact domain. Scoped owner PUT requires both
current owner authority and the exact unexpired capability; a capability alone
never authenticates an owner. Finalization, metadata, evaluations and content use
the existing domain and audience rules. Owner and credential submission modes
remain mutually exclusive. Canonical artifact locks precede principal locks;
current contribution authority is checked before idempotent finalization recovery.
No publication or acceptance action is exposed by these owner routes.

Owner bytes/finalization/readback first passes3.109s. Separate producer/mode/token
checks pass with the output lifecycle in6.667s. Actual restricted fixed-worker
output and private progress evaluation pass9.376s: the exact uploaded SHA and
pinned executable SHA are observed, progress keeps the task accepted with its
clocks unchanged, excluded peers cannot read metadata/evaluations/content, and
no progress credit, review decision or consent is fabricated. The executable is
explicitly development_process_not_qualified; this is not provider or isolation
qualification.

The real60-second capability-expiry test passes62.979s without rewriting immutable
expiry: changed bytes are denied401 after expiry, stored bytes remain unchanged,
and the authorized owner may finalize already uploaded bytes. The integrated
owner checks plus legacy publication lifecycle and canonical task policy-wait
regression pass9 top-level/9 subcases in33.669s.

Additional characterization proves policy withdrawal using the normal policy API
blocks issued-capability PUT and both new finalization and retained receipt recovery
with403. Already sealed generations reject PUT409 before withdrawal. Denials leave
quarantine/sealed state and zero/one evaluation job unchanged. Both subcases pass
6.209s. These checks characterize existing authority behavior; no artificial RED
or production change is claimed for them.

Quality assessment: authentication adapters resolve identity/context and preserve
raw bounded bytes; shared artifact transitions retain rights, dependencies, epoch,
current plan and immutable manifest ownership. Purpose-specific audience and input
grants are retained rather than widened for reviewers. No financial identities
or content execution are introduced. Full concurrency/revocation review, canonical
API schemas, reload discovery/recovery, owner UI, actual cookie/CSRF/browser proof
and reviewed merge remain open. This is an unmerged backend checkpoint, not HP3
closure.

- `/tmp/undergrid-owner-contributions-bytes-red-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-bytes-first-green-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-bytes-guards-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-fixed-evaluation-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-expiry-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-bytes-integrated-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-policy-recovery-oct29.jsonl`

The prior empty vet log lacked a retained exit status after context recovery.
A fresh `go vet ./...` on `dd8d4c5` exits0; log:
`/tmp/undergrid-owner-contributions-dd8d4c5-vet-oct29.log`.

## Returning-owner discovery — 29 October 2026

Test-only `ecd0ba7` requires GET
`/v1/owner/agents/{agent}/tasks/{task}/artifacts`; it fails404 in3.093s.
The owner adapter now resolves the current owned active actor; the listing requires
that actor to be the task assignee and currently authorized in its room. Existing
artifact RLS continues to govern visibility. Results use existing immutable-context
projections; no capability token, credential or renewed expiry is returned.
Keyset pagination returns at most100 entries ordered by artifact UUID and a
`next_after_id` continuation. UUID ordering is stable discovery, not chronology;
concurrent new uploads can require refreshing from the first page. This is not a
snapshot/history-consistency claim.

A first two-generation lifecycle passes. The strengthened test creates101 normal
owner generations through the HTTP API, observes100 plus1 without duplication,
then an empty terminal page. Invalid cursors fail400 and foreign owner selection
fails404. This passes10.060s. Forward migration052 adds only the task/agent/id
lookup index; its applied SHA256 is
`e5cdbb60e9b07abcf75422ed7685ff1a342b592950ca98ef79cdf431a386545a`.
Do not edit the applied migration. No policy or existing051 is changed.

This supplies metadata discovery only. An expired upload capability is not renewed,
and the UI still needs an explicit recovery decision for incomplete quarantine
bytes. Exact finalization and verification/readback remain separate actions.
The complete owner browser journey and reviewed merge remain pending.

- `/tmp/undergrid-owner-contributions-reload-red-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-reload-green-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-reload-pagination-oct29.jsonl`

The follow-up restricted-role HTTP gate passes4 top-level/6 subcases in14.882s:
selected coordinator cannot enumerate another assignee's generations; a controlled
membership withdrawal removes returning-owner discovery. Existing owner creation
version/epoch/foreign-owner guards, bytes/finalization and policy-before-receipt
checks remain green. Log:
`/tmp/undergrid-owner-contributions-reload-guards-oct29.jsonl`.

Fresh `go vet ./...` for the discovery implementation exits0;
`/tmp/undergrid-owner-contributions-discovery-vet-oct29.log` retains the static gate.
Review tradeoff: discovery is bounded to100 projections and uses existing artifact
projection knowledge rather than a competing wire model. It performs up to100
bounded projection lookups after closing the ID query; reducing that round-trip
cost may be warranted if measured runtime exceeds the journey budget. No broader
repository abstraction or policy duplication is introduced.

## Owner controls and published contract checkpoint — 29 October 2026

Contract-only test `3360fdc` records actual owner creation, raw PUT, immutable
finalization/recovery, metadata, evaluations and owned-task discovery. It fails on
an unadvertised owner creation operation (4.347s). The canonical OpenAPI document
now advertises those owner-session paths, including conjunctive owner-session and
upload capability authentication on PUT. Owner input excludes server-resolved
room/plan/task IDs; its receipt explicitly names owner_session. Finite schemas
reject credential mode, fabricated consent, oversized data, capability disclosure
in metadata and more than100 discovery entries. The first actual HTTP gate passes
4.553s; its stronger output/private-progress creation and restricted fixed-worker
evaluation extension passes6.232s. The contract lint,178 schema compilation and
regenerated TypeScript contract check pass. Legacy credential schemas are retained.

Browser trust-boundary tests first fail3 positive cases at explicit unimplemented
contract boundaries; all14 then pass. Control tests initially fail all9 cases on
the missing UI. Implemented controls then pass23 focused cases, including reviewed
raw-byte transfer, exact finalization retry without repeating sealed PUT, denied
context cleanup, explicit returning-owner selection of already uploaded bytes,
and session/agent switch cleanup. Named fixed-evidence presentation has its own
missing-control and missing-contract RED, then passes with exact immutable SHA/
manifest bindings and explicit development_process_not_qualified display.

A private-progress case exposes the server's sorted audience versus selection
order. It fails before normalization; sorting the reviewed command audience and
restricting reviewer choices to designated acceptance principals fixes it. The
four-file focused UI gate passes49 cases7.330s. These UI tests use a deliberately
fixed digest oracle; only the actual Chromium journey may prove file hashing.

The assignment screen now exposes task contribution controls for accepted plans,
including submitted tasks so returning owners can discover immutable results.
Review fetches the current plan, assignments, owner and bounded task generations.
JSON data is read only as inert bytes, bounded to10MiB, SHA256 hashed and reviewed
alongside a finite64KiB lineage/attribution manifest. Explicit purpose/audience
review precedes creation. Byte transfer rejects any unexpected origin, agent,
artifact path, fragment, query field, token shape or size before attaching cookie/
CSRF authority. Raw bytes are never JSON-reencoded for transfer.

A private staged command retains reviewed body/version/epoch and the original
idempotency key for uncertain creation/finalization retries. Confirmed PUT moves
to finalization; a lost finalize response never repeats PUT against sealed bytes.
Reload deliberately discards local files/capabilities and uses metadata discovery.
The owner may select a current pending generation, reselect original data/manifest
and seal already uploaded bytes. Missing/mismatched bytes fail in the domain;
expired capabilities are never renewed. An explicit new-upload action preserves
the old generation and quota rather than silently replacing it. No browser
credential/private file storage, financial coupling, output acceptance, progress
credit/review or publication consent is introduced.

The continuous owner browser/worker extension has been written but is not yet
proven at this checkpoint. Full UI, final build/type checks, browser evidence,
remaining concurrency/failure review and reviewed integration remain required.
The branch is still local and unmerged; HP3 remains open.

- `/tmp/undergrid-owner-contributions-contract-red-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-contract-green-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-contract-worker-oct29.jsonl`
- `/tmp/undergrid-owner-contributions-contract-check-oct29.log`
- `/tmp/undergrid-owner-contributions-ui-contract-red-oct29.log`
- `/tmp/undergrid-owner-contributions-ui-contract-green-oct29.log`
- `/tmp/undergrid-owner-contributions-controls-red-oct29.log`
- `/tmp/undergrid-owner-contributions-controls-first-green-oct29.log`
- `/tmp/undergrid-owner-contributions-evaluation-contract-red-oct29.log`
- `/tmp/undergrid-owner-contributions-evidence-controls-red-oct29.log`
- `/tmp/undergrid-owner-contributions-progress-controls-red-oct29.log`
- `/tmp/undergrid-owner-contributions-controls-final-oct29.log`

The complete console suite passes327 cases across27 files in90.47s; log:
`/tmp/undergrid-owner-contributions-full-ui-oct29.log`. The assignment-screen
integration is included. The continuous browser harness now extends the actual
same-owner HP2 journey; its runtime result remains pending until executed on a
frozen source/build manifest.

First actual continuous browser attempt on `a12f299` fails at96.224s after normal
HP2 acceptance/task activity and before any artifact allocation. All contribution
review reads return200. Retained database is
`hp1_run_48b4899e85454e059c5c3afb857bbb51`; runtime directory
`/tmp/undergrid-hp3-contribution-runtime-709708451`; browser artifacts
`/tmp/undergrid-hp2-browser-3595687162`; full log
`/tmp/undergrid-owner-contributions-a12f299-browser-oct29.jsonl`.

The terminal trace waits on an exact Playwright label selector for the purpose
select. A separate real Chromium reproduction with the same wrapped label/options
returns exact-label count0 versus exact-combobox-role count1. The harness now uses
stable accessible combobox roles for purpose/reviewer. The stable accessible-role
unit test also passes. This is a harness selector correction, not a product RED or
an excuse to weaken authority/timeout checks. Rebuild/rerun remains required.
The initial build recorder invocation used repository cwd instead of web cwd and
failed before building; the corrected coupled production build passed with
source `d0df2f13241f2ba6c0d57db0042002adda7b98830f99d4d76c0c18a0fbc4a53e`
and JS `220289735080b09b350ef898f430588b4c053d0a45835b83284b225fc05619cd`.

## Actual continuous contribution journey — 29 October 2026

Frozen `b2963d555a36b0f36351f8b02aa7f7d10ee778c1` passes the actual same-owner
HP2→task execution→private progress→output contribution journey in50.480s.
The only correction after the initial browser failure is the harness combobox
selector; production JS remains byte-identical. Coupled build source SHA256:
`fea2b9fb557a35d357611017da84573f769ba658f0904d3e6344811012cd89c1`;
production JS SHA256:
`220289735080b09b350ef898f430588b4c053d0a45835b83284b225fc05619cd`.

Chromium hashes the actual selected inert JSON file and sends exact bytes with
owner cookies/CSRF. The owner reviews a private audience and designated reviewer,
seals progress, deliberately loses a committed finalization response and recovers
with the identical body/key. The excluded peer receives404 for metadata,
evaluations and content. Fixed evidence binds exact bytes and the manifest,
explicitly displays development_process_not_qualified, and never credits progress
or changes its clocks. The same owner uploads task output, loses the request before
finalization, reloads, discovers the pending generation through normal controls,
reselects original data/manifest and seals existing bytes without a new generation
or capability renewal. Both artifacts verify through the actual pinned executable
and restricted River worker; evidence/readback survives normal worker restart.
Browser localStorage/sessionStorage remain empty.

Independent final SQL observes2 verified owner-session generations with NULL
credential IDs,1 two-person private progress audience/reviewer,1 output and2
passed development evaluations. Exact normalized/embedded evaluation hashes and
runtime identity agree. The collect task is submitted at version4/epoch1 with
its exact output pointer and no credited progress. No task progress report,
output-acceptance vote or publication vote is created; unverified activity remains
one event. These are controlled release-test owners, not independent beneficial
owners or provider/storage/isolation/legal qualification.

Retained database: `hp1_run_aa8bbccdded94dc993c1e55e5f38a2e2` on55502.
Runtime/verifier/blobs: `/tmp/undergrid-hp3-contribution-runtime-1751914390`.
Browser artifacts: `/tmp/undergrid-hp2-browser-3955274706`.
Logs:
- `/tmp/undergrid-owner-contributions-b2963d5-build-oct29.log`
- `/tmp/undergrid-owner-contributions-b2963d5-browser-oct29.jsonl`

Actual local contribution proof is now present. Remaining targeted transaction/
revocation review, final static/contract gates and reviewed merge are still
required. Substantive progress reporting/review, designated output acceptance,
separate publication and fourth-principal reuse remain later HP3 work.


## Transaction review checkpoint — 29 October 2026

The finalization characterization gate checks incorrect plan/epoch/hash/license,
then injects a controlled event-fanout enqueue failure after seal/task/evaluation
job writes. The entire transaction rolls back: quarantine/no SHA, accepted task,
zero evaluation jobs/sealed events/receipts. Removing the fixture fault and retrying
the exact key/body produces one seal, one evaluation job, one event and one receipt;
the second successful retry returns the identical result.

Four real concurrent canonical-wait cases hold owner authority in a separate PG
transaction, prove the API is blocked in lock_artifact_authorities via
pg_blocking_pids, then commit policy withdrawal, agent revocation, owner suspension
or membership withdrawal before releasing it. The protected finalization denies
with the exact current error and leaves quarantine/job/receipt state unchanged.
The first gate fails only because the test expected404 for a revoked owned agent;
existing lockOwnerAgent explicitly returns403 inactive_agent. The corrected test
requires that exact code (plus owner_policy_denied, registration_required and
not_found for the other cases), rather than accepting any denial. No production
behavior changed to satisfy these characterization checks.

The corrected race gate passes2 top-level/8 subcases in16.139s. Logs:
- `/tmp/undergrid-owner-contributions-transactions-race-oct29.jsonl` (incorrect expected status retained)
- `/tmp/undergrid-owner-contributions-transactions-corrected-race-oct29.jsonl`

Root scoped review: owner adapters resolve identity/context and call the existing
artifact domain; canonical resource locks precede owner locks, current authority
precedes recovery, and channel namespaces prevent credential-generation borrowing.
The UI validates context and destinations before transfer, retains immutable retry
stages, discards late state on identity changes, and renders all content inertly.
Shared domain and projection knowledge is reused; no financial repository or
provider coupling is introduced. Applied051/052 remain frozen. The bounded100
projection lookups and explicit file/manifest re-selection after reload are
recorded implementation tradeoffs, not unqualified resumable storage claims.
Swarm review workers remain quota-unavailable; no fresh independent review is
claimed. Final static/contract gates and reviewed release integration remain open.


Final pre-PR gates: tagged `go vet -tags=hp1_live,hp2_live ./...` exits0;
canonical lint,178 independently compiled schema components and regenerated
TypeScript contract check pass. Logs:
- `/tmp/undergrid-owner-contributions-final-vet-oct29.log`
- `/tmp/undergrid-owner-contributions-final-contract-oct29.log`

This feature's implemented/local-proof scope is owner dataset and private-progress
artifact creation/bytes/sealing/discovery/evaluation UI and exact recovery. Deferred
within HP3: substantive progress report and named review, designated output
acceptance, separate publication/rights consent, private-input grant controls,
fourth-principal reuse and continuous complete HP3 acceptance. External legal,
source/provider, storage/verifier-host and deployment gates remain explicitly open.

`go build ./...` also exits0; log:
`/tmp/undergrid-owner-contributions-final-go-build-oct29.log`.
Frozen migration hashes rechecked unchanged before PR. All changed Go files are
already gofmt-clean; `git diff --check` passes. Root review found no remaining
in-scope authority or recovery blocker. Exact merged-release checks still remain.
