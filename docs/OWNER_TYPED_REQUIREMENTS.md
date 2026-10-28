# Owner typed requirement controls

## Contract — 28 October 2026

HP2 step1 / INT-02 / TDD §5: complete the existing creation/revision editor for
freshness, delivery windows and input schema alternatives. Work starts from
reviewed release `334bb99` on `feat/v0.1-owner-typed-requirements`; root continues
while established swarm workers remain quota-limited, without replacement workers
or model switches. Capability discovery is merged in PR118; full HP2 stays open.

The domain already validates and persists these dimensions. Add explicit optional
controls to the shared creation/revision editor, preserving independent coverage,
fields/licenses, public projection review and server authority. Freshness uses
integer seconds1–31536000. Delivery bounds remain exact RFC3339 strings, including
fraction/offset; do not normalize through local datetime or Date serialization.
At least one delivery bound is required; server-time expiry and ordering policy
remain enforced by the existing domain command. Input alternatives are an explicit
nonempty bounded set (up to20 labels), distinct from an absent/unknown declaration.
Unchecking a declaration removes only that dimension. Omitted and nullable legacy
values survive unrelated edits without inventing facts or weakening policy.

Reuse shared declared-constraint validation knowledge at the response boundary,
while preserving nullable fields and exact wire timestamps. The current editor
uses Date.parse independently and accepts some malformed dates/constraint shapes;
behavioral counterexamples must precede correction. Do not turn client validation
into authorization or make group/capability/demand mechanisms share fit semantics.

## Required acceptance before merge

1. Real missing-control component/App behavioral RED, then explicit create/edit/
   remove controls with independent expected callbacks/submitted bodies. Preserve
   unrelated dimensions, nullable/omitted values and immutable public review.
2. Runtime response parser rejects malformed/nonfinite/out-of-range freshness,
   calendar-invalid delivery and malformed input alternatives; preserves valid
   fractional/offset timestamps exactly. Existing group/capability constraints
   retain their contract and generation/session protections.
3. Normal owner browser creation and revision through actual HTTP/restricted PG
   preserve exact declared values, immutable history, current version and reload.
   Removing one dimension leaves the others unchanged. Independently inspect
   normalized revisions; no SQL product seeds complete user steps.
4. Actual server negatives retain existing422/authority/retry/current-source
   behavior. Full/focused UI, canonical/static/build and affected retained runtime
   gates pass before scoped review and merge. Record exact source/artifacts and
   original failures. No migration, provider call or financial identity is needed.

No behavior or proof is claimed by this contract. Applications and complete
continuous HP2 acceptance remain separate required work; external qualification
and reserved legal/source/provider/deployment approvals remain explicit.

## First editor implementation checkpoint — 28 October 2026

Test-only `2b37dae` reaches genuine RED:9 failures/1 passing preservation case
in1.17s. Missing freshness/delivery/input controls fail, and malformed response
freshness/range/calendar/empty/duplicate examples incorrectly pass the old reader.
The shared editor now exposes explicit optional declarations and removes only
the selected dimension. Freshness uses bounded integer seconds; delivery uses
raw timezone-bearing strings with at least one native required bound; input
alternatives require a nonempty input. The existing domain remains authoritative
for all range/order/expiry/label policy and422 behavior.

readIntentionConstraints first invokes shared declared-constraint validation,
then retains nullable fields and exact original timestamp strings. It no longer
independently uses Date.parse for calendar acceptance. First focused gate passes
10 cases in1.95s; existing App plus editor gate passes40 in12.27s.

The first TypeScript gate exposed an existing generator limitation: the delivery
schema's required-bound union widens to unknown. A narrow local editor delivery
model, validated at ingress, fixes the type boundary without changing the wire
schema or bypassing it with any casts. Corrected TypeScript and whitespace checks
pass. This final type/comment-only correction does not alter tested UI behavior.

- `/tmp/undergrid-typed-requirements-initial-red-oct28.log`
- `/tmp/undergrid-typed-requirements-first-green-oct28.log`
- `/tmp/undergrid-typed-requirements-app-first-oct28.log`
- `/tmp/undergrid-typed-requirements-tsc-oct28.log` (generated unknown-type gap)
- `/tmp/undergrid-typed-requirements-tsc-model-oct28.log`

Creation/revision submitted-body integration, actual browser/persistence proof,
complete focused/full regression gates, review and merge remain pending. This is
an implementation checkpoint, not full typed-controls or HP2 closure.

## Actual submitted bodies and owner browser journey — 28 October 2026

Three additional App integration cases pass3.00s: normal creation submits exact
freshness/delivery/input fields without a bearer; revision removes freshness while
retaining nullable fields, precise timestamp and input alternatives plus If-Match;
editing typed requirements invalidates an earlier public-projection review.
Log: `/tmp/undergrid-typed-app-bodies-oct28.log`.

Frozen runtime/frontend `ee34f63088df3b47d0f982ea6cc5307e9486118e` passes the actual
three-owner Chromium/API/restricted-worker/PostgreSQL aggregate17.092s. A normally
creates private watch-disabled EU coverage with3600-second freshness, precise
fractional/offset delivery bounds and two input alternatives. It edits to7200s,
a new precise deadline and one alternative, reloads, then removes only freshness
through the normal revision form. Reload recovers the unchanged remaining values.
Independent SQL proves five total intention revisions across three controlled
owners, A immutable original constraints plus current revision3/version3, original
fraction/offset values, current deadline/input/coverage and zero opportunities.
Existing generic harness checks prove current owner attribution/watch policy and
zero collaboration effects. Separate accounts are controlled attribution, not
beneficial-owner independence. No SQL product seeds complete browser steps.

- Browser: `/tmp/undergrid-typed-browser-ee34f63-oct28.jsonl`
- Build: `/tmp/undergrid-typed-browser-ee34f63-build-oct28.log`
- Retained DB: `hp1_run_f26f13e631454575a7e3c39b2fc2156c`
- Artifacts: `/tmp/undergrid-hp2-browser-1897977081`
- Source SHA256: `59d801867136129bbd3f1e3a73d7e9eb221cb60c110adad93c8bb5da47c1ef52`
- Production JS SHA256: `9dcd0bc06c49e2f67e68de669a7febd536b06fff07aa03bb6662de2d5c97e918`

Standalone TypeScript, tagged harness compilation and coupled frontend build pass.
Final full UI, retained server/runtime gates, review and merge remain pending.
This feature does not close continuous HP2, applications or external qualification.

## Final local validation checkpoint — 28 October 2026

At frozen runtime/frontend `ee34f63`, full UI passes272 cases across23 files
in91.52s. A fresh normally copied `typed_requirements_final_oct28` fixture proves
the affected server persistence, typed unknown/incompatibility semantics,
malformed-range422 behavior and owner isolation:6 top-level/17 subcases pass4.042s.
Canonical lint, all168 schemas and generated-type parity pass; tagged vet and Go
build pass. No backend behavior, migration or financial/source identity changes.

- `/tmp/undergrid-typed-ee34f63-full-ui-oct28.log`
- `/tmp/undergrid-typed-final-db-oct28.log`
- `/tmp/undergrid-typed-ee34f63-domain-oct28.jsonl`
- `/tmp/undergrid-typed-ee34f63-contract-oct28.log`
- `/tmp/undergrid-typed-ee34f63-vet-oct28.log`
- `/tmp/undergrid-typed-ee34f63-go-build-oct28.log`

Resource maintenance moved only the known regenerable npm content cache from
`/home/nmcdc/.npm/_cacache` to `/dev/shm/undergrid-npm-cacache-oct28`, preserving the
original path as a symlink and recording
`/tmp/undergrid-npm-cache-relocation-oct28.json`. No databases, product blobs,
credentials, worktrees or failure evidence were removed. Final retained browser
regressions, review and merge remain pending; no broader path closure is claimed.

## Reviewed final feature evidence — 28 October 2026

All five retained actual capability/clarification/group/room/plan browser journeys
pass126.135s on the same frozen `ee34f63` source and production build. Per-case
90-second browser, three-minute helper and12-second shutdown bounds are unchanged;
only the aggregate command timeout covers multiple sequential cases.
Log: `/tmp/undergrid-typed-ee34f63-retained-browsers-oct28.jsonl`.
Size/SHA256 evidence index: `/tmp/undergrid-typed-requirements-validation-oct28.json`.

Root reviewed the scoped diff and actual proof. Shared parser knowledge replaces
independent Date.parse calendar interpretation; the editor separately preserves
nullable fields and exact raw timestamp evidence. Distinct optional dimensions
use explicit controls instead of a generic policy engine. Server validation,
authorization, If-Match and recovery semantics remain unchanged, with actual
negative/persistence proof. No migration, new dependency, provider call, financial
connection or public CI is introduced. No fresh independent-agent review is
claimed while established workers remain quota-limited. Changes after `ee34f63`
are documentation only. All local feature gates pass; PR merge and exact-release
checks remain pending. Applications and continuous HP2 acceptance remain open.
