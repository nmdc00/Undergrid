# HP2 continuous acceptance contract

**Current status: HP2 is locally closed.** [PR124](https://github.com/nmdc00/Undergrid/pull/124)
merges the reviewed continuous proof at release
`cd2c298308dbab0cb5746e34631852735b64dd42`. The full journey passes again on that exact merged release
in38.701s. Implementation, local runtime proof and reviewed integration are
complete; external qualification remains open. HP3 is the next active local path.


Active branch `test/v0.1-hp2-continuous`, based on release `cddab23`.
HP1 remains locally closed. This artifact owns continuous HP2 proof, not a new
product feature or external qualification.

The same controlled buyer A, buyer B and builder C must register through normal
owner/session flows, discover current demand and declared builder capability,
review a restricted room/role, recover a committed room/application offer, admit
participants separately, exchange persistent messages, propose a bounded plan
with dependencies, record each participant's exact plan/rights decisions and
separate assignee responsibility, revise without corrupting the accepted head,
and reconnect/recover across a real worker restart. Adjacent decline, stale/current
authority, expiry and no-duplicate effects must remain covered by named guards.
No SQL mutation or hidden administrator action may complete a product step.
Read-only fixture checkpoints and actual worker lifecycle control are permitted.

The existing plan browser proof uses explicit API room/invitation setup and is
retained as a focused regression. Extract its already proven policy/plan journey
into a shared browser helper before composing continuous owner room/recruitment/
application steps. Passing the retained case alone does not close HP2.

Completion requires exact source/build provenance, real isolated PostgreSQL and
restricted workers, independent normalized state/effect oracles, final review,
release integration and current authority/recovery gates. Controlled identities,
local fixed terms and provider fixtures never establish independent beneficial
owners, legal approval or provider/deployment qualification. All external gates
remain separately open. No closure is claimed yet.

## Initial composition evidence — 28 October 2026

The extracted existing plan scenario passes49.381s on `afa5825`, retaining exact
votes/assignment activation/revision/restart checks. The first continuous browser
on `e6c2dc9` passes37.2s: same accounts go from current demand match through owner
room creation/recovery, worker recruitment, C application/recovered offer/separate
admission, B explicit invitation/separate admission, persistent message, exact
plan/rights votes, separate task acceptance, plan revision and worker restart/
obligation recovery. The aggregate39.675s fails afterward: an application-only
global offered-event assertion sees2 because B's separate normal invitation is
also present. The corrected oracle counts application-origin offers explicitly;
the recruitment fixture's expected final plan/task/approval counts are separately
updated to the full journey's exact2/4/12 rather than the earlier0/0/0. This is a
fixture composition correction, not a product change or weakened authority gate.

- `/tmp/undergrid-hp2-continuous-plan-extraction-regression-oct28.jsonl`
- `/tmp/undergrid-hp2-continuous-first-oct28.jsonl` (browser pass, aggregate failure)
- First continuous database `hp1_run_431ffac946ff49748eab9bf59dfb902b`
- First continuous browser artifacts `/tmp/undergrid-hp2-browser-1396905735`

Final aggregate, negative guards and reviewed release integration remain required.
HP2 is not locally closed yet. External qualification remains separately open.

## Corrected continuous lifecycle checkpoint

On frozen `e2301bf`, the actual browser and final normalized aggregate both pass
43.172s (`/tmp/undergrid-hp2-continuous-corrected-oct28.jsonl`), with source-identical
production build in `/tmp/undergrid-hp2-continuous-corrected-build-oct28.log` and
retained database `hp1_run_f9921f91a0c84c41b341528d947cbebf`. The preceding aggregate
failure remains retained. Explicit compatible-group and registered-capability UI
inspection are now added before room creation to cover those discovery-review
steps within the continuous journey, rather than relying only on their separate
feature proofs. The expanded journey, guard aggregate and reviewed release
integration remain pending; no HP2 local closure is claimed yet.

## Expanded discovery run and retryable contention

On `b908abd`, explicit current compatible-group and registered C-capability UI
inspection both complete, then the offer receipt retry encounters real NOWAIT
principal contention (`55P03`); the server returns500/retryable without a second
offer. The aggregate fails24.447s at the overly strict first-replay201 assertion.
The failed log/database/artifacts remain retained. The browser now performs at
most3 explicit visible UI recovery attempts, requires every body/key to equal the
original committed offer, permits only500 with retryable=true before eventual201,
and retains the independent final exactly-one application-offer event oracle.
No server lock, deadline, authorization or success/effect gate is weakened; the
case exercises the product's already implemented uncertain-command retry behavior.

- `/tmp/undergrid-hp2-continuous-discovery-oct28.jsonl` (retained failure)
- Database `hp1_run_12bf8f68c06442b69bda1c0d860cd5e9`
- Browser artifacts `/tmp/undergrid-hp2-browser-3113558250`

## Expanded continuous journey PASS — 28 October 2026

Frozen `9f5bcdc` passes the expanded actual Chromium/session/API/isolated restricted-
PostgreSQL/worker journey and independent final normalized aggregate in41.867s.
A real55P03/retryable failure occurs and the visible exact-key/body recovery
succeeds; final SQL still finds exactly one application-origin offer. Product
source and production JavaScript remain identical to reviewed release `2bcd7df`;
this branch changes acceptance harnesses and documentation only.

| HP2 outcome | Continuous proving actions/oracles |
| --- | --- |
| Current demand and compatible group | B normally publishes a reviewed intention; A's private watch finds the current declared match; A inspects the exact compatible group containing A/B. |
| Appropriate declared builder | C normally publishes its opt-in declared profile; A inspects C's registered capability and principal reference; the actual worker emits the bounded room-role recruitment opportunity. |
| Reviewed room and separate admission | A creates/recover-retries the selected-match restricted room; C reviews/applies/reloads; A reviews immutable claims and recovers its offer; C separately accepts. B separately opts in, receives an explicit owner invitation and accepts. History is denied before C's admission. |
| Persistent communication | A posts through owner UI; B reloads and reads the same message. Messages grant no plan votes or assignments. |
| Exact plan/rights decisions | All3 review/hash-check and separately vote on each immutable plan; independent checkpoint shows exact per-purpose vote counts and no premature activation. |
| Separate task responsibility | C separately accepts both dependent assignments; accepted plan head changes only afterward. A later revision preserves the original accepted head until new decisions and assignments complete. |
| Restart/recovery and no duplicates | Actual worker stop/start completes its normal startup deadline job; all accounts reload accepted state; agent context preserves current immutable plan hash/task claim epochs; committed room/offer retries retain exact bodies/keys and single normalized effects. |

- `/tmp/undergrid-hp2-continuous-recovery-oct28.jsonl` (PASS41.867s)
- `/tmp/undergrid-hp2-continuous-recovery-build-oct28.log` (coupled source/build manifest)
- Retained isolated network database: hp1_run_c677c997e952467e9aa741aa63675339 on port 55502
- Retained HP2 browser artifacts: /tmp/undergrid-hp2-browser-540688694

The focused negative guard aggregate, final scoped review and release integration
remain required before HP2 local closure. Historical failures above remain
retained. Controlled users, test contribution templates and fixed local/provider
fixtures are not external qualification. HP3 has not started.

## Current negative gates and preservation review — 28 October 2026

The focused negative aggregate passes20.209s on fresh normal-migrated
`hp2_continuous_guards_oct28`. Named tests cover invitation lifecycle, application
expiry, exact reviewed offer/recovery, target principal/agent contention rollback,
assignment decline/expiry/atomic fanout failure, accepted commitments during
rejected/expired revisions, canonical-wait current authority and peer policy
revocation, and nontransitive/group-wide license assumptions.

Exact log: `/tmp/undergrid-hp2-continuous-negative-guards-oct28.jsonl`.

Review found this branch initially replaced the earlier acceptance ledger; the
complete prior ledger is restored below as historical evidence from `cddab23`.
Its status statements are historical and superseded by the current checkpoints
above. No prior runtime databases, artifacts or logs are discarded. Scoped review,
static checks and release integration remain required before local closure.

## Reproduction and final scoped review

The passing continuous runtime SHA is `9f5bcdccd6044f46ae9c1c8145491d99da3e061c`.
Frontend source SHA256 is
`0ec62e536b8de9a486874199af45e91ef92374b74bce72855c08cc361e822d32`;
built JavaScript SHA256 is
`608485f6455ae143536b0a00693f026296d85291e33ca146fd7bc9855ade670b`.
The fixture creates a fresh database from the genuine normally migrated template
and applies migrations normally. It does not reset product data or fake ledgers.

Run from the acceptance worktree, first `web`, then repository root:

```sh
# In web; recording inseparably rebuilds TypeScript/Vite and pins HEAD/source/assets.
PATH=/home/nmcdc/.nvm/versions/node/v24.15.0/bin:$PATH node scripts/record-live-build.mjs
# In repository root; leave source and HEAD frozen until this returns terminal.
PATH=/home/nmcdc/.nvm/versions/node/v24.15.0/bin:$PATH \
LD_LIBRARY_PATH=/tmp/undergrid-browser-libs/root/usr/lib/x86_64-linux-gnu \
GOCACHE=/tmp/undergrid-gocache GOMODCACHE=/tmp/undergrid-gomod \
GOTMPDIR=/dev/shm/undergrid-gotmp \
UNDERGRID_HP1_ADMIN_DATABASE_URL='postgres://nmcdc@localhost:55502/postgres?host=/tmp/undergrid-pg-semantic-55502/socket&sslmode=disable' \
/tmp/undergrid-toolchain/go/bin/go test -tags='hp1_live hp2_live' ./internal/network \
-run '^TestHP2LiveContinuousDiscoveryAdmissionPlanAndRecovery$' -count=1 -json
```

Negative gate uses normal migration on fresh `hp2_continuous_guards_oct28` with
`UNDERGRID_TEST_DATABASE_URL` at the same network socket/port, normal Go cache/temp
settings above and:

```sh
go test ./internal/network -count=1 -json -run '^Test(OwnerAssignmentDeclineExpiryAndAtomicFanoutFailure|OwnerPlanUnacceptedRevisionPreservesAcceptedCommitments|OwnerPlanFinalApprovalSerializesPeerPolicyRevocation|OwnerPlanAuthorityRecheckedAfterCanonicalWait|OwnerAssignmentAuthorityRecheckedAfterCanonicalWait|ApplicationDeadlineCannotAdmitAfterExpiry|OwnerParticipationInvitationLifecycleWithoutAgentCredential|CompatibleGroupsRejectTransitiveAndGlobalLicenseAssumptions|OwnerApplicationOfferPinsReviewsAndRecoversExactly|ApplicationOfferTargetContentionRollsBackAndRecoversExactly)$'
```

All10 top-level/14-subcase negatives pass20.209s. Tagged `go vet -tags='hp1_live
hp2_live' ./...` and `go build ./...` pass; exact logs are
`/tmp/undergrid-hp2-continuous-vet-oct28.log` and
`/tmp/undergrid-hp2-continuous-go-build-oct28.log`.

Root scoped review confirms: production files/migrations are unchanged; extracted
policy/plan steps retain all original independent hash/vote/task/revision/restart
assertions and their original focused browser case passes49.381s; new continuous
setup uses only normal owner mutations and agent reads; fixture endpoints are
read-only checkpoints or actual worker lifecycle controls, not product authority;
only explicitly retryable500 responses can precede bounded exact-command recovery;
all successful states/effects remain required. Prior ledger evidence is preserved.
No material in-scope finding remains after restoring it. This is root review, not
a claim of fresh independent worker review; established swarm workers remain
quota-unavailable. No replacement agent/model was launched to evade that limit.
Reviewed merge and exact-release checks still gate local HP2 closure.

## Earlier acceptance ledger retained from release cddab23

The following dated checkpoints and initial matrix preserve earlier evidence and
missing-step history; they do not override the current runtime proof above.

# Historical HP2 acceptance ledger

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
| 1. A/B inspect current hard/semantic matches, explanatory unknown-hard gaps and compatible groups without assuming transitivity. | Intention create/revise → durable scan/fanout/pair workers; agent/owner intention opportunities. Merged PR107 adds current profile-bound vectors and `GET /v1/intents/{id}/compatible-groups` / owner equivalent; dependent owner group viewer is active on `feat/v0.1-owner-compatible-groups`, unmerged. | Release exact-demand path exists. Independent fresh capacity now has current-release RED/GREEN, retained-runtime and HP1/foundation regression proof; it is reviewed and merged in PR101/release `5ea78d1` (see `DISCOVERY_CAPACITY.md`). Semantic/profile/hard-constraint/group-read integration is reviewed and merged in PR107/release `3bb46eb`, with combined domain, capacity and baseline browser proof on `5a36e5a`. Visible unknown-hard clarification remains missing. The owner group viewer is reviewed and merged in PR114/release `bc6ddda`, proven on `f4828c5`; see `OWNER_COMPATIBLE_GROUPS.md`. | Merged semantic integration proof `d01aedd`: 36 top-level/40 subcases PASS 94.503s; preserve the earlier failed aggregate and scoped remediation in `SEMANTIC_INTEGRATION.md`. Five-second fresh-watch/result bounds are unchanged. No continuous HP2 proof. | Controlled vectors do not qualify model relevance/provider operation. |
| 2. Discover registered complementary capability; create a reviewed restricted room carrying the current match origin and opt-in recruitment role; C receives attributable recruitment. | Agent `POST /v1/swarms`; owner `POST /v1/owner/agents/{agent}/swarms`; `origin_opportunity_id`, `recruitment_roles`; normal recruitment worker; agent `GET /v1/opportunities`. | Owner match selection, exact origin/role/full-history review, frozen command snapshots and shared current-authority checks are implemented and locally proven on the room-origin feature branch. Registered complementary capability discovery remains missing. Reviewed owner agent profile controls and saved acknowledgement are merged in PR106. | Actual selected-match → reviewed restricted room → lost committed response/exact recovery → restricted-worker C recruitment passed on `857e07e` (race package 20.111s). Final room/retained authority gates passed 17 top-level/36 subcases in 83.752s on test-only `2950d02`; full UI 163 cases and retained owner-plan browser also pass. Reviewed and merged in PR112/release `e045f36`; exact-release UI12/build and restricted two-top-level/two-subcase checks pass; see `OWNER_ROOM_ORIGIN_RECRUITMENT.md`. This bounded proof is not the full HP2 journey. | Separate account IDs and declared capabilities do not establish independence or verified competence. |
| 3a. A invites B/C; each reviews full-history disclosure and independently accepts or declines; messages persist. | Agent invitations, invitation read, membership response and messages; owner `/v1/owner/agents/{agent}/swarms/{room}/invitations`, `/invitations/{id}`, `/swarms/{room}/membership-response`, `/context`, `/messages`; `GET .../participation`. | Merged room creation, invitation, explicit response, owner room context and persistent messages. Must connect actual current discovery/recruitment to this lifecycle and retain opt-in checks. | Existing focused evidence is not a continuous three-owner journey. New main journey must prove reviewed room policy, pending vs accepted state and cross-owner message recovery. | Controlled participants are not the required external cohort. |
| 3b. C applies to an available role; coordinator reviews that exact application and explicitly offers admission; C separately accepts. | Agent `POST /v1/swarms/{id}/applications`, then coordinator invitations with `application_id`, then membership response. | Agent domain transition exists. Owner participation currently lists offered/accepted states, not application review; owner invitation DTO omits `application_id`. User-facing application review/offer remains missing. | Separate required lifecycle: apply does not admit, offer does not admit, decline/expiry/revocation/retry do not fabricate acceptance. It cannot disappear because the primary journey chooses invitations. | No external operation qualified. |
| 4. A creates/revises a bounded plan; all required principals review the exact plan hash and enabled contribution terms and explicitly vote. | Agent `POST /v1/swarms/{id}/plans`; owner `GET .../plan-context` and `POST /v1/owner/agents/{agent}/swarms/{id}/plans`; existing owner exact plan review/approval and separate assignment controls. | Owner structured multi-task DAG proposal/revision and current participant checks are reviewed and merged in PR110/release `2099a76`. | Continuous two-plan owner lifecycle passed on frozen `9040113`: normal three-owner setup/admission, exact individual votes, separate assignments, prior-head preservation through pending decisions, immutable revision activation and actual worker restart/reconnect. Restricted reject/expiry accepted-head guards and actual wire-contract samples pass separately. See `OWNER_PLAN_PROPOSAL.md`; retained domain guards passed 21 top-level/16 subcases in 57.479s; canonical schemas/parity and tagged vet/build pass. Post-merge release UI/build and restricted lifecycle/contract checks pass; see the owning evidence. API room/invitation setup is explicitly fixture support; the owner room-origin journey is proven separately in PR112, and full HP2 remains open. | Local test-only templates do not approve legal terms; distinct accounts do not establish beneficial-owner independence. |
| 5. Each assignee separately accepts responsibility; only then can exact current plan become accepted; obligations recover after reconnect/restart. | Agent task read/assignment response and `GET /v1/agent-context`; owner plan assignments review and task assignment response; owner participation/context/plan review. | Shared acceptance and owner assignment controls merged; PR110 proves accepted-head/task/exact-vote agreement for three controlled owners and a dependent two-task initial plan plus immutable replacement. Integration with all remaining HP2 steps is still required. | PR110 actual browser reload/restart reconstructs both current accepted assignments and exact plan hash/claim epochs; independent SQL confirms the accepted head. Retained authority, retry and dependency guards pass. This bounded plan journey does not prove discovery-to-collaboration continuity. | Local receipt/acceptance proof is not independent third-party use or funding. |

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

## Reviewed room origin and role local proof — 27 October 2026

The feature `feat/v0.1-owner-room-origin-recruitment` normally integrates plan
PR110/release `2099a76` and documentation checkpoint `054a269`. It closes the
missing owner controls for supplied match provenance and room recruitment roles;
all four origin kinds have current-authority/hash/profile/source guards before
new writes and retained recovery. The actual room browser uses normal owner
controls and real restricted workers, without product SQL seeds. Its source,
exact body/key recovery, saved origin/roles, one room/creator receipt and absence
of candidate/builder admission are independently proven. Shared-demand monitoring
off remains a positive allowed control. Receipt/recruitment do not grant rights.

See `OWNER_ROOM_ORIGIN_RECRUITMENT.md` for the 163-case UI gate, actual room and
retained two-plan browsers, restricted 17-top-level/36-subcase race aggregate,
160-schema/parity/static gates, frozen 045 and preserved selector/55P03 fixture
failures. Final review and feature merge are complete in PR112/release `e045f36`.
Post-merge UI12 passed 8.73s, coupled Vite/manifest 1.44s preserves source/asset
hashes, and restricted current-recovery/watch-off checks passed two top-level/two
subcases in 6.000s on `room_release_e045f36_oct27`. Exact root logs are in the
owning evidence. This proof does not complete complementary capability discovery,
the separately merged group viewer (PR114), unknown-hard clarification,
remaining typed requirements, application review/admission or the continuous HP2
journey. External qualification remains separate.


## Owner group-viewer local proof — 28 October 2026

On `f4828c5`, normal three-owner browser/API/restricted-worker discovery yields
separate A/B and A/C views with incompatible B/C license choices; source revision
removes C and reload preserves only A/B. The race package passed 39.308s. SQL
independently proves four immutable revisions/current source r2 and completed
scans; no rooms/memberships/plans/tasks/votes result. Source/session/revision races,
closed response shapes, nullable bounds, inert content and canonical timestamp
validation have focused behavioral evidence. Seven domain groups/two subcases
passed 23.115s. The earlier 197-case full UI and final 41-case parser check are
separate recorded gates. Retained room and plan browsers passed together in
72.192s; canonical 160-schema/parity, tagged vet and build passed. Exact source,
commands, retained databases/artifacts, failure classification and log hashes are
in `OWNER_COMPATIBLE_GROUPS.md`. PR114 is reviewed and merged at release `bc6ddda`. This is not
continuous HP2 or external/provider/independent-owner qualification.

## Exact merged-release closure — 28 October 2026

PR124 merges reviewed acceptance head `8dc82b6` into `release/v0.1` at `cd2c298`.
The exact release coupled TypeScript/Vite build passes with the same production
source/asset hashes as the feature proof; the entire expanded continuous browser/
API/PostgreSQL/worker aggregate then passes38.701s. No narrowed smoke case stands
in for the final journey. The10-top-level/14-subcase decline/expiry/current-authority/
rollback gate passes20.209s before merge. Root review and preservation fixes are
recorded above; no new independent worker review is claimed.

- `/tmp/undergrid-hp2-continuous-release-build-oct28.log`
- `/tmp/undergrid-hp2-continuous-release-cd2c298-oct28.jsonl` (PASS38.701s)
- Retained isolated network database: hp1_run_f59d898163ee43eeae7c285c87b8da16 on port 55502
- Retained HP2 browser artifacts: /tmp/undergrid-hp2-browser-3880072985

All5 happy-path steps and adjacent authorization/recovery guards now have named
runtime proof and reviewed integration. HP2 is locally closed under the map's
implemented/proven/merged definition. Public qualification still requires externally
operated participants and independent REST/MCP/A2A clients with real outcomes,
plus applicable source/provider/legal/deployment gates. Controlled account IDs
and local test terms do not satisfy those gates. Main/develop remains0/1, release
integration PR62 remains open; no promotion or production deployment occurred.
