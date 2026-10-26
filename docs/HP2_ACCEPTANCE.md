# HP2 acceptance ledger

Scope: `HAPPY_PATH_EXECUTION.md` HP2 and the capacity-first dependency contract in
`HP2_EXECUTION.md` (task-map commit `47771f5`). This branch starts from release
`c0087c0`, after HP1 local closure. HP1 external qualifications remain open.
Implementation, continuous local proof, reviewed merge and external qualification
are separate facts. **HP2 is not locally proven or closed.** The scoped three-owner setup foundation passed on 25 October; it does not
complete discovery or collaboration. The bounded owner publication/revision/withdrawal control has actual local proof and merged in PR103 (`b0d49c7`). Reviewed builder registration is locally proven and merged in PR106 (`f5e5960`). Exact evidence
is recorded at the end of this ledger.

Three controlled accounts A/B/C test separate current authorities. They do not
prove independent beneficial owners, externally operated participants, a useful
real cohort or provider/legal qualification.

## Executable order

Recruitment roles belong to rooms, so the actual dependency order is: A/B demand
match/group → reviewed restricted room with current opportunity origin and role
→ C receives recruitment → B/C explicitly accept admission → persistent messages
→ exact plan and contribution-terms decisions → separate assignment acceptance
→ reconnect/restart recovery. The application → reviewed offer → acceptance path
is a separately required workflow, even if the main journey uses invitations.

## Current step and evidence matrix

| Step / acceptance outcome | Actual entrypoints and dependencies | Implemented and missing | Continuous proof / merge status | External qualification |
| --- | --- | --- | --- | --- |
| 0. A/B/C sign in separately, register owners and scoped agents; A/B create private watched intentions; separately create explicitly public intentions with reviewed selected projections. | Production OIDC/session/CSRF; `POST /v1/agents`; owner `POST /v1/owner/intents`; normal bearer `GET /v1/me` and `/v1/intents/{id}`. | Basic private account/agent/intention creation merged. Explicit redacted-public creation, reviewed same-audience summary revision and withdrawal merged in PR103. Reviewed builder capability, invitation and recruitment opt-ins are implemented, locally proven and merged in PR106. | PASS: actual three-context browser/API/PostgreSQL setup and private watch ownership boundary at `d8ffa39`; this bounded foundation is reviewed and merged in PR100. The publication→summary revision→withdraw lifecycle passed on `d3d5370`; see `OWNER_INTENTION_PUBLICATION.md` for exact proof and corrected fixed-audience contract. Builder console registration/reload plus real worker recruitment passed on `7728350`; see `OWNER_BUILDER_REGISTRATION.md`. No browser product rows are seeded. | Controlled identities are fixture accounts only. |
| 1. A/B inspect current hard/semantic matches, explanatory unknown-hard gaps and compatible groups without assuming transitivity. | Intention create/revise → durable scan/fanout/pair workers; agent/owner intention opportunities. Parked semantic branch adds current profile-bound vectors and `GET /v1/intents/{id}/compatible-groups` / owner equivalent; dependent group viewer remains parked. | Release exact-demand path exists. Independent fresh capacity now has current-release RED/GREEN, retained-runtime and HP1/foundation regression proof; it is reviewed and merged in PR101/release `5ea78d1` (see `DISCOVERY_CAPACITY.md`). Semantic/profile/hard-constraint/group integration is implemented with current combined local proof on `feat/v0.1-semantic-integration`, with capacity/browser regressions also proven on `5a36e5a`, pending final review and merge. Visible unknown-hard clarification and selected group UI remain missing. | Current semantic integration `d01aedd`: 36 top-level/40 subcases PASS 94.503s; preserve the earlier failed aggregate and scoped remediation in `SEMANTIC_INTEGRATION.md`. Five-second fresh-watch/result bounds are unchanged. No continuous HP2 proof. | Controlled vectors do not qualify model relevance/provider operation. |
| 2. Discover registered complementary capability; create a reviewed restricted room carrying the current match origin and opt-in recruitment role; C receives attributable recruitment. | Agent `POST /v1/swarms`; owner `POST /v1/owner/agents/{agent}/swarms`; `origin_opportunity_id`, `recruitment_roles`; normal recruitment worker; agent `GET /v1/opportunities`. | Domain room/recruitment commands exist; registered complementary capability discovery remains missing. Owner room UI always sends empty roles and no origin. Reviewed owner agent profile controls and saved acknowledgement are locally proven and merged in PR106. | Missing user controls must be implemented and integrated before the full journey. Direct API fixture setup must not be mislabeled as completed owner controls. | Separate account IDs and declared capabilities do not establish independence or verified competence. |
| 3a. A invites B/C; each reviews full-history disclosure and independently accepts or declines; messages persist. | Agent invitations, invitation read, membership response and messages; owner `/v1/owner/agents/{agent}/swarms/{room}/invitations`, `/invitations/{id}`, `/swarms/{room}/membership-response`, `/context`, `/messages`; `GET .../participation`. | Merged room creation, invitation, explicit response, owner room context and persistent messages. Must connect actual current discovery/recruitment to this lifecycle and retain opt-in checks. | Existing focused evidence is not a continuous three-owner journey. New main journey must prove reviewed room policy, pending vs accepted state and cross-owner message recovery. | Controlled participants are not the required external cohort. |
| 3b. C applies to an available role; coordinator reviews that exact application and explicitly offers admission; C separately accepts. | Agent `POST /v1/swarms/{id}/applications`, then coordinator invitations with `application_id`, then membership response. | Agent domain transition exists. Owner participation currently lists offered/accepted states, not application review; owner invitation DTO omits `application_id`. User-facing application review/offer remains missing. | Separate required lifecycle: apply does not admit, offer does not admit, decline/expiry/revocation/retry do not fabricate acceptance. It cannot disappear because the primary journey chooses invitations. | No external operation qualified. |
| 4. A creates/revises a bounded plan; all required principals review the exact plan hash and enabled contribution terms and explicitly vote. | Agent `POST /v1/swarms/{id}/plans` with current room `If-Match` and `parent_version`; `GET /v1/plans/{id}` and approvals. Owner routes currently list/read plans and post approvals. | Shared plan proposal/revision and exact votes exist; owner plan creation/revision controls are missing. Owner exact review/approval is merged. Normal enabled template/license policy and current room contributors remain prerequisites. | Full journey must create and revise through actual controls, with no SQL plan/vote seed or copied hidden fixture result. Required principal votes and rights records stay distinct from assignments. | Local test-only templates do not approve legal terms. |
| 5. Each assignee separately accepts responsibility; only then can exact current plan become accepted; obligations recover after reconnect/restart. | Agent task read/assignment response and `GET /v1/agent-context`; owner plan assignments review and task assignment response; owner participation/context/plan review. | Shared acceptance and owner assignment controls merged. Need full accepted-head/task/approval agreement with the intended three-party plan and dependency behavior. | Reconnect must rebuild current obligations without relying on stale browser state; retry must preserve one effect and exact reviewed hash/version. Current policy/member/agent authority must be rechecked. | Local receipt/acceptance proof is not independent third-party use or funding. |

## Main continuous journey contract

A and B publish compatible, explicitly selected intention projections with complete
initial hard requirements; a third-party embedding/provider fixture may return
controlled numeric evidence through the actual bounded adapter. A/B inspect the
loaded current match and pairwise group evidence. Exact/current revision/profile
pins must be observed; stale proof must not be accepted merely because a score is
high. Unknown hard evidence must be visibly distinct from an ordinary no-match;
that separate result is required, not avoided by choosing complete fixture inputs.

A creates the room from the selected current opportunity, with reviewed restricted
visibility/full-history policy and a `source_catalog_v1` role. C explicitly opts
into the corresponding public capability/recruitment profile, receives the
attributable opportunity and reviews it. B/C are independently offered admission
and explicitly accept. A/B/C exchange persistent untrusted text messages. No
receipt, message, match or recruitment notification grants membership, plan votes,
rights, assignment or spending authority.

A creates a bounded plan and an actual revision with the current parent/room
version. The exact plan includes A/B/C as required contributors, exact acceptance
and publication sets, enabled test-only terms and a supported JSON dataset task
for C. All required owners review the loaded canonical hash and contribution
terms and decide for themselves. C separately reviews and accepts task
responsibility. The acceptance proof must assert plan/rights votes, task
assignment state and canonical accepted room head independently; matching UI text
alone is insufficient. Dependency-blocked offered work must not be mislabeled as
accepted or started merely to make the main path pass.

After browser reconnect and actual worker stop/awaited termination/start, current
room, plan and obligation reads must recover the accepted state. Repeat exact
commands/receipt acknowledgement without duplicate authority effects. Prove a
separate decline/revise/reoffer lifecycle rather than converting rejection into
silent consent. Preserve relevant five-second discovery and bounded shutdown
contracts throughout the complete path.

## Required normalized observations and guards

Read-only SQL is an independent oracle, never a way to complete a product step:

- Principal/agent/credential attribution, exact intention revisions and selected
  public projections; private objective/budget/preferences absent from foreign
  projections and publisher requests.
- Current source/candidate/profile-bound opportunity identities and exactly one
  event/inbox effect per matching input; retained jobs survive fresh-capacity and
  restart maintenance without fabricated retries or dropped work.
- Room creator/current origin, explicit visibility/history policy and role;
  separate applied/offered/accepted membership rows, attributable decisions and
  persistent message sequences.
- Immutable plan lineage/specification/hash/template hash, exact plan and rights
  approval records/current policy versions, separate assigned-agent response,
  current accepted head and task claim/dependency state.
- No financial runtime/database/credential, public rights grant, publication vote,
  output acceptance or arbitrary execution occurs as a side effect of HP2.

Focused current-authority guards must accompany the happy journey: invalid/stale
cursor/revision/profile/source, incompatible vs unknown constraints, nontransitive
groups, private-query consent pinned to exact revision/profile (including after
HP1 partial revisions), revoked principals/agents/membership/policies, concurrent
final vote/assignment eligibility, expired or declined invitations/applications,
wrong assignee/hash/version, response loss and transactional rollback. Existing
focused tests are leads for selecting the final current integration gate, not a
claim that this unexecuted browser harness proves every adverse condition.

## Foundation boundaries and current execution status

The first bounded foundation uses one actual HTTPS console/API and controlled
OIDC issuer, three isolated Chromium contexts and the real production session,
CSRF and network domain transactions. Test identity selection happens only at the
controlled issuer and is bound into the one-use signed-token grant; it never
forges an owner cookie or `X-Test-Owner` header. The normally issued bearer
credentials must read only their own private records or selected foreign public
projections. Product owner/agent/intention rows originate from browser commands.
Infrastructure may create normal schema/LOGINs and pristine retained PostgreSQL
template clones. Publisher/provider/policy doubles stay explicitly controlled.

The historical foundation checkpoint stopped at scoped three-account setup, two
private watched intentions and foreign-access denial. Public projection publishing
was then a missing owner control. The publication feature now proves separate
explicitly public creation, reviewed same-audience summary revision and withdrawal
(see `OWNER_INTENTION_PUBLICATION.md`), merged in PR103; it does not convert private
IDs to public. Neither proof skips missing discovery/profile/room/plan controls by
silently seeding their rows or granting consent. Subsequent reviewed features
will extend them into the complete path, plus the distinct application workflow. The scoped foundation ran only after root granted a serial test lease, and the
lease was released after all processes terminated. Further execution still
requires a root-granted lease; the capacity implementation remains a dependency
of discovery acceptance.

Full HP2 proof remains pending. Real independently operated participants and
REST/MCP/A2A clients, provider relevance/operation, permitted sources/legal policy,
deployment and all cross-path release gates remain separately unqualified.

Authored opt-in foundation selector:
`go test -tags=hp1_live,hp2_live -race -json -count=1 ./internal/network -run '^TestHP2LiveThreeOwnerScopedSetupFoundation$'`.
The dual tag deliberately reuses HP1's signed issuer/template infrastructure
without making existing HP1 live selectors execute an unfinished HP2 scenario.
The first authorized gate also reran the existing HP1 setup/gap selector because
the controlled issuer now binds its selected subject into each one-use grant; the
HP1 wrapper keeps its original fixed subject. Both cases passed as recorded below.
The separate `e2e-hp2` directory/config shares the established browser/session
fixture policy without routing API/auth requests through mocks.


## Actual scoped foundation evidence — 25 October 2026

Frozen runtime/test source: `d8ffa39208ac45d85b72796ea01686b07c6699ba`.
Only ledger/map documentation changes follow this tested source.

- Coupled actual console build and artifact/source manifest: PASS, Vite 1.50 seconds; `/tmp/undergrid-hp2-foundation-d8ffa39-build.log`. The existing unchanged dependency cache was linked into this worktree; no install/download occurred. Both live tests verified the manifest before Chromium.
- Command: `go test -tags=hp1_live,hp2_live -race -json -count=1 -timeout=5m ./internal/network -run '^Test(HP1LiveOwnerSetupAndHonestGap|HP2LiveThreeOwnerScopedSetupFoundation)$'` with the preserved isolated 55500 network PostgreSQL template-copy environment and cached Chromium/libraries.
- Runtime log: `/tmp/undergrid-hp2-foundation-d8ffa39-55500.jsonl`; **PASS 44.185 seconds, two top-level tests, zero failures/skips**. HP1 setup/gap passed 27.59 seconds; the new HP2 setup foundation passed 15.58 seconds (actual browser scenario 12.4 seconds; Playwright runner 13.8 seconds).
- HP2 retained run database: `hp1_run_24f1cc40edd6465ebfbaebcec563bf92` on network fixture port 55500; the shared template helper retains its historical database-name prefix. Browser artifacts: `/tmp/undergrid-hp2-browser-2247452439`.
- HP1 regression retained database: `hp1_run_d65d964b022747d5baea583f7c24d89d` on the same isolated network server, separately cloned before its browser commands.
- Static command: `go vet -tags=hp1_live,hp2_live ./internal/network`; PASS, `/tmp/undergrid-hp2-foundation-d8ffa39-vet.log`. Formatting and diff checks pass.

The real browser registered three controlled subjects through the signed one-use
OIDC/PKCE flow, created one scoped agent per principal and two private active
watched intentions, reloaded each workspace, and observed HTTP 404 for every
foreign private-intention read using each normally issued bearer credential.
Read-only normalized SQL independently confirmed the exact attribution/counts,
private watch state, and absence of rooms, memberships, plans, approvals and tasks.
No product row was seeded by SQL. The actor named builder C has not acquired any
capability or recruitment consent merely from its fixture name.

Quality assessment: shared controlled issuer code owns nonce/PKCE/token behavior;
subject selection is bound into the one-use grant and never substitutes an app
session. Separate browser contexts own their cookie/session state. The explicit
second tag and scenario directory keep this incomplete HP2 foundation out of
existing HP1 scenario selection. Normal PostgreSQL migrations/role setup and
retained empty-template copies are infrastructure, while all product creation
uses actual browser commands. SQL is a read-only outcome oracle. No financial
identity/database or arbitrary agent-content execution is involved.

At that historical foundation checkpoint, this proved only bounded setup. Later public projection and builder registration evidence supersede those two missing-control observations; full HP2 remains open. The original checkpoint left explicit public projection,
semantic/groups/current discovery capacity, builder opt-ins/complementary
capability, attributable room/role controls, separate application review/offer,
plan creation/revision and the complete collaboration/recovery journey remain
open as mapped above. Full HP2 is not locally closed; public/external/legal/provider
qualifications remain unproven.


## Actual builder registration prerequisite — 26 October 2026

Implemented and locally proven on `feat/v0.1-owner-builder-registration`, pending
independent final review/merge. Frozen product/test source `7728350` passed five
focused UI cases, full console125 tests/16files, strict145-schema/closed HTTP
contracts, tagged all-package vet/build, the actual builder browser and retained
full HP1 partial-remainder plus HP2 publication/withdrawal browsers. Exact logs,
commands, failed aggregates, fixture remediation and retained DBs are recorded in
[owner builder registration evidence](OWNER_BUILDER_REGISTRATION.md).

Through normal OIDC/session/CSRF C explicitly selects and reviews public declared
`source_catalog_v1` capability and recruitment, leaving foreign invitations off.
Reload acknowledges the actual saved four-field profile. A's scoped normal-agent
API room/role fixture causes the actual restricted worker to emit exactly one
C opportunity/event/inbox triple; unsolicited invitation and room context remain
404, with no foreign membership, plan, task or approval. Fixture API room setup
does not complete the missing owner room-origin/recruitment-role controls. Domain
negatives separately preserve unlisted/recruitment-off/mismatched capability
exclusion and invitation eligibility independent of recruitment.

This closes the local builder onboarding prerequisite only after reviewed merge.
It does not prove registered complementary-capability discovery, semantic/groups,
unknown-hard clarification, reviewed room provenance/roles, application review,
plan creation/revision, full HP2 admission/consent/recovery or external independence.


## Typed owner-input boundary still open

Semantic integration supplies reviewed required-field/license/coverage drafts and
preserves all stored freshness, delivery and input-schema requirements. Console
creation/editing controls for those latter dimensions remain a separate HP2
owner feature; do not count them complete from domain DTOs or stored JSON. The
only currently supported output is `json_dataset`/`source_catalog_v1`.


## Current semantic prerequisite regression checkpoint — 26 October

`feat/v0.1-semantic-integration` has 36 top-level/40 subcase combined domain proof
at `d01aedd` and four retained capacity cases, 145 UI tests, current tagged vet/
build and four actual baseline browser cases at `5a36e5a`. See
`SEMANTIC_INTEGRATION.md` for exact timing, manifest, retained databases/artifacts
and preserved failures/remediations. Feature review/merge remains pending.
These baseline journeys do not complete the continuous HP2 collaboration path or
its missing owner/group/clarification/capability controls and qualifications.
