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
