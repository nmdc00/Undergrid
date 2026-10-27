# Owner compatible-group inspection

## Locked contract — 27 October 2026

HP2 step 1, INT-02 and TDD r2 §5 require an owner to inspect compatible views
without assuming transitive fit. This feature branch is
`feat/v0.1-owner-compatible-groups`, isolated at
`/tmp/undergrid-worktrees/owner-compatible-groups/feat-v0.1-owner-compatible-groups`,
from reviewed release `e045f36` (room-origin/recruitment PR112). Its feature PR
targets `release/v0.1`. The owning implementation and evidence artifact is this
file; update the TDD checkpoint and HP2 execution/acceptance map before merge.

The old parked viewer is a recovery source, not a merge dependency. Recover only
the five-file delta `aa73e0c..8f03ecd`: component, component tests, App mount,
App tests and owning evidence. Import App behavioral tests first and run against
the current application before any component/mount implementation. That RED must
fail at the missing visible control, not a missing import or broken fixture.
Historical proof from that old branch is not current-release validation.

### Runtime and authority contract

Existing owner `GET /v1/owner/intents/{id}/compatible-groups` resolves the owned
intention and active attribution agent in `ownerIntents`, then invokes shared
`compatibleGroups`. The same domain read serves agent requests. It requires a
current active, unexpired, monitored source intention. Preserve this watch gate:
PR112's different shared-demand room-origin eligibility when watch is disabled
does not broaden group disclosure.

The canonical 160-schema contract supplies at most 10 groups, derived from at
most 20 candidates, with 2–21 members each, current source inclusion, per-member
intention/principal IDs and declared hard constraints, common declared licenses,
exact or pairwise-semantic comparison mode, `common_control_unknown` identity
assurance and bounded/nonexhaustive limitations. The server, not the viewer,
owns pairwise comparisons, group feasibility, current revision/projection/profile
checks and principal deduplication. No provider request is needed merely to read.
The read creates no group graph, membership, invitation, assignment, approval,
rights, spending or room. Keep network and financial identities separate.

### Acceptance and required proof

1. An owner reviewing an active monitored intention explicitly loads or refreshes
   groups. Separate views stay separate. Show actual member constraints and common
   declared license options; distinguish exact from pairwise-semantic evidence.
   Disclose bounds, truncation, unknown common control and that license options
   grant no rights. Empty results honestly describe a discovery gap.
2. Validate the finite response at the UI boundary: closed shapes, bounds, supported
   modes, source inclusion, unique member/principal IDs and structural constraints.
   Foreign private fields, unsupported assurances or malformed results fail closed.
   Render content as inert text; execute no agent instructions or HTML.
3. Clear prior views during refresh/error/authority loss. Source intention/revision,
   owner session or unmount invalidates outstanding responses; stale successes and
   401/403/404 responses cannot affect a new context. Session expiry invokes the
   existing sign-out path. Paused/watch-disabled sources do not mount the viewer.
4. Prove the real browser with normal owner login, agent/intention creation and
   actual restricted PostgreSQL workers. Three controlled participants with A/B
   and A/C compatibility but B/C incompatible declared license sets must produce
   separate source-containing views, never an inferred three-member group.
   Use read-only SQL/actual API inspection as independent evidence; no seeded
   product intentions/opportunities/groups complete the journey. Controlled
   identities/vectors do not establish independent owners or provider qualification.
5. Run focused UI, TypeScript/build/provenance, actual browser and relevant current
   group/authority domain regressions plus canonical contract checks. Retain exact
   sources, terminal results, databases/artifacts and failed gates. Review DRY,
   orthogonality, contracts, readability and recovery before final review/merge.

No behavior has been imported and no current test has run at this contract
checkpoint. Root currently holds the costly-test lease; this worker will wait
for explicit handoff. Room PR112 is merged; its post-merge results will be recorded
from root's authoritative checks. HP2 remains open beyond this bounded viewer.
