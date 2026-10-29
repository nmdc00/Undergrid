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
