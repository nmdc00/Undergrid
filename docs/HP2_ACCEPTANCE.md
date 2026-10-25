# HP2 acceptance ledger

Scope: `HAPPY_PATH_EXECUTION.md` HP2 and the capacity-first dependency contract in
`HP2_EXECUTION.md` (task-map commit `47771f5`). This branch starts from release
`c0087c0`, after HP1 local closure. HP1 external qualifications remain open.
Implementation, continuous local proof, reviewed merge and external qualification
are separate facts. **HP2 is not locally proven or closed.** No HP2 test has run
on this branch; the foundation below is authored, not passing evidence.

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
| 0. A/B/C sign in separately, register owners and scoped agents; A/B create private watched intentions; later explicitly publish selected projections. | Production OIDC/session/CSRF; `POST /v1/agents`; owner `POST /v1/owner/intents`; normal bearer `GET /v1/me` and `/v1/intents/{id}`. | Basic private account/agent/intention creation merged. The owner intention UI offers private/restricted only and cannot publish `redacted_public` / selected `public_summary`; agent registration form cannot declare builder capability, invitation or recruitment opt-ins. | New real three-context foundation authored, unexecuted. No product rows may be seeded. | Controlled identities are fixture accounts only. |
| 1. A/B inspect current hard/semantic matches, explanatory unknown-hard gaps and compatible groups without assuming transitivity. | Intention create/revise → durable scan/fanout/pair workers; agent/owner intention opportunities. Parked semantic branch adds current profile-bound vectors and `GET /v1/intents/{id}/compatible-groups` / owner equivalent; dependent group viewer remains parked. | Release exact-demand path exists. Fresh capacity still needs current-release RED/GREEN and merge; semantic/profile/hard-constraint/group integration, visible unknown-hard clarification and selected group UI are not yet merged. | First blocked acceptance outcome. Preserve the existing five-second fresh-watch/result bounds; no continuous HP2 proof. | Controlled vectors do not qualify model relevance/provider operation. |
| 2. Discover registered complementary capability; create a reviewed restricted room carrying the current match origin and opt-in recruitment role; C receives attributable recruitment. | Agent `POST /v1/swarms`; owner `POST /v1/owner/agents/{agent}/swarms`; `origin_opportunity_id`, `recruitment_roles`; normal recruitment worker; agent `GET /v1/opportunities`. | Domain room/recruitment commands exist; registered complementary capability discovery remains missing. Owner room UI always sends empty roles and no origin. Owner agent UI sends no profile opt-ins; existing owner agent-creation API accepts a profile. | Missing user controls must be implemented and integrated before the full journey. Direct API fixture setup must not be mislabeled as completed owner controls. | Separate account IDs and declared capabilities do not establish independence or verified competence. |
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

This foundation stops at scoped three-account setup, two private watched intentions and foreign-access denial.
Public projection publishing remains an explicit missing owner control, not a
hidden API mutation used to label that user flow complete. It does **not** skip missing discovery/profile/room/plan
controls by silently seeding their rows or granting consent. Subsequent reviewed
features will extend it into the complete path, plus the distinct application
workflow. No Go/Node gates have run for this branch. Capacity owns the first
costly-test lease; this branch requires a root-granted lease before execution.

All HP2 proof remains pending. Real independently operated participants and
REST/MCP/A2A clients, provider relevance/operation, permitted sources/legal policy,
deployment and all cross-path release gates remain separately unqualified.

Authored opt-in foundation selector:
`go test -tags=hp1_live,hp2_live -race -json -count=1 ./internal/network -run '^TestHP2LiveThreeOwnerScopedSetupFoundation$'`.
The dual tag deliberately reuses HP1's signed issuer/template infrastructure
without making existing HP1 live selectors execute an unfinished HP2 scenario.
The first authorized gate must also rerun the existing HP1 selectors because the
controlled issuer now binds its selected subject into each one-use grant; the
HP1 wrapper keeps its original fixed subject. This remains unexecuted test code.
The separate `e2e-hp2` directory/config shares the established browser/session
fixture policy without routing API/auth requests through mocks.
