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
