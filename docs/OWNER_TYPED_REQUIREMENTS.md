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
