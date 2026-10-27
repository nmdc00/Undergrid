# HP2 execution contract

Active after HP1 local closure in PR98, release `c0087c0`. HP1 external
qualifications remain open. This executes the approved TDD and companions; it
does not reduce HP2 to the code already present on parked branches.

## Journey and dependencies

Two buyers and a builder discover a current compatible collaboration, explicitly
join a reviewed room, exchange messages, agree on the exact plan/contribution
terms, separately accept assignments and recover their current obligations.
Three fixture accounts prove authority separation, not independent beneficial
ownership or a real external cohort.

Recruitment roles currently belong to rooms. Therefore the executable order is:
current demand match/group → reviewed room creation with attributable origin and
role → attributable recruitment → explicit admission of the other buyer/builder
→ messages → exact plan and terms decisions → assignment acceptance → reconnect.
This corrects the dependency order in the high-level happy-path map without
dropping capability discovery, opt-in recruitment or any consent boundary.

Existing semantic and capacity work remains preserved. The parked capacity branch
includes an unmerged semantic dependency and must not be merged wholesale. First
extract its independent queue behavior onto the current release. Reconcile and
review semantic discovery only after that prerequisite is proven and merged.
That prerequisite is now complete in PR101, release `5ea78d1`; its capacity,
retained-state and HP1 proofs remain in `DISCOVERY_CAPACITY.md`. The current semantic task is
[`SEMANTIC_INTEGRATION.md`](SEMANTIC_INTEGRATION.md), owned by `semantic_oct26`
and reviewed by `review_oct26`. It has current combined domain, UI, static,
capacity and baseline browser proof, pending final feature review/merge. Owner
publication and builder opt-in prerequisites are reviewed and merged in PR103
and PR106; their independent evidence remains in their owning documents.

## First implementation task: discovery capacity

Owner: `capacity_oct25` (resuming `partial_resume_oct24`). Branch `feat/v0.1-discovery-capacity`, from
current `release/v0.1`, isolated worktree
`/tmp/undergrid-worktrees/hp2-capacity/feat-v0.1-discovery-capacity`.
Owning requirements: HP2 step 1, TDD §5.4 and the existing five-second committed
discovery worker contract. Owning evidence: `docs/DISCOVERY_CAPACITY.md` (create
with actual RED/GREEN/runtime results before the feature PR merges).

Runtime entrypoints are normal intention creation/revision, durable scan/fanout
jobs and `RunWorker` through `runWorkerWithSources`. Expected effects are timely
`last_scan`, one currently eligible opportunity/event/inbox per matching input,
and retained accepted queue work. No semantic provider or new migration is needed.

Acceptance:

- Reproduce on current release before changing behavior, using real restricted
  API/worker PostgreSQL logins and normal HTTP intention commands.
- Under a blocked legacy backlog and an aged fresh backlog surviving restart,
  retain the existing five-second initial-scan and result bounds and twelve-second
  worker shutdown bound. Preserve every accepted job; do not delete, cancel or
  reset work to make the test pass.
- Separate fresh direct scan, watcher fanout and watcher-pair capacity. Move only
  eligible aged, unstarted, nonunique jobs to their corresponding retained lanes
  in bounded 256-row `SKIP LOCKED` batches. Update queue ownership only; preserve
  IDs, arguments, scheduling, attempts, errors, priority and maximum attempts.
- Prove exclusion of running, fresh, unique-key and unrelated jobs, bounded batch
  behavior, actual runtime maintenance, retained descendant routing and recovery
  after authority locks are released. Distinguish normal worker progress from
  maintenance mutation in the preservation oracle. Require bounded progress for
  known eligible unattempted retained work; future-backoff retries must retain
  their schedule rather than being reset to manufacture a five-second recovery.
- Propagate the actual River job queue to continuation/pair routing so retained
  work cannot refill fresh lanes indefinitely.
- Preserve HP1's concrete source-refresher seam, six-hour production scheduling,
  authorization and bounded shutdown. Current release has fourteen queues and
  ten connections of headroom; three added queues require seventeen/twenty-seven,
  not the parked semantic prototype's twenty-two/thirty-two. Prefer one small
  queue configuration as the source of both queue registration and pool sizing.

Assess DRY knowledge ownership, queue/domain orthogonality, transaction/resource
ownership, understandable recovery and actual failure behavior. Run affected
worker/discovery/HP1 regressions, static checks and console checks as appropriate.
Never substitute an easier match or a wider timeout for the acceptance outcome.

## Parallel acceptance and review

`live_resume_oct24` owns `test/v0.1-hp2-live-journey` at
`/tmp/undergrid-worktrees/hp2-live/test-v0.1-hp2-live-journey`, from release,
initially recording `docs/HP2_ACCEPTANCE.md` and actual missing
entrypoints. Author the production-like multi-owner browser/API/worker journey
without seeding product principals, opportunities, admissions, votes or tasks.
Use trusted controlled provider/policy fixtures only where explicitly labeled;
they do not establish real provider, legal or external-user qualifications.

`review_resume_oct24` reviews the capacity extraction and actual proof read-only,
including preservation, current eligibility, restart and pool/queue coupling.
Root integrates reviewed feature PRs into `release/v0.1` in dependency order.
The acceptance branch must integrate reviewed product changes before its full
journey is claimed proven. One agent owns each coding worktree.

One named owner holds the costly-test lease at a time. The next gate starts only
after the previous process is authoritatively terminal and the lease is released.
Poll the same live handle; an observation timeout is not permission to restart.
No hosted GitHub CI is required; local checks remain required.

## Remaining HP2 behaviors, not yet completed

After capacity: reconcile current semantic/profile/hard-constraint/group behavior
and its preserved tests against release, including current dismissal hashes and
the HP1 partial-remainder contract. Semantic dismissal needs a forward migration;
never edit frozen 033–037/040/041. Prove partial revisions do not silently renew
revision/profile-bound private-query disclosure consent. Test exact discovery
latency with a slow semantic provider: dedicated queues alone do not prove
isolation while direct scans can call the provider synchronously.
Prove visible unknown-hard clarification and
registered complementary capability discovery. Complete real owner controls
needed to carry match provenance and recruitment roles into room creation, builder
profile/invitation opt-in, bounded plan creation/revision and application
review/offer. An invitation-based main journey must not erase the separately
required application workflow. Do not assume backend commands alone complete
those user flows. Audit the actual missing controls before defining
each subsequent feature branch and acceptance criterion.

Current-authority reads/replays, exact admission, plan/rights decisions and separate
assignment acceptance remain mandatory. Receipt, chat and semantic similarity
grant no participation, rights or spending. Preserve decline, expiry, revocation,
concurrent-policy and no-duplicate recovery guards. No arbitrary agent content is
executed. Network and financial identities/databases remain separate.

HP2 is locally closed only when every mapped step has implemented entrypoints,
one complete actual journey, adjacent negative/recovery evidence and reviewed
release integration. Real independently operated participants and protocol
clients, provider/source/legal approvals and deployment remain external gates.
Full Undergrid v0.1 completion requires the other paths and all release gates too.

## Audited owner entrypoints and missing controls

The first capacity task above remains unchanged. The acceptance audit at release
`c0087c0` additionally records these concrete owner-control dependencies:

- Intention publication: the audited release offered only private/restricted
  creation. This feature implements and locally proves explicit public creation,
  fixed-audience summary review and withdrawal, merged in PR103 (`b0d49c7`); see
  `OWNER_INTENTION_PUBLICATION.md`. True same-ID disclosure conversion remains
  unsupported by the published domain contract, not hidden behind fixture SQL.
- Builder opt-in: owner `POST /v1/agents` accepts `AgentProfile` with public,
  invitation, recruitment and declared capability fields. The registration form
  sends name/scopes only. Real owner controls must supply explicit bounded opt-ins;
  a fixture agent name such as “builder C” is not a capability or consent record.
- Attributable room creation: owner `POST /v1/owner/agents/{agent}/swarms` already
  accepts `origin_opportunity_id` and `recruitment_roles`, but `CreateSwarm.tsx`
  omits origin and always sends an empty role list. Link the actual selected
  current opportunity and reviewed role into this command; recruitment requires
  an active nonprivate room and cannot precede room creation.
- Plans: shared `POST /v1/swarms/{id}/plans` proposes a bounded immutable plan or
  next revision using `If-Match` and `parent_version`. Owner routes currently list
  room plans, read exact plan/terms and post exact approvals; no owner proposal or
  revision control exists. Creation/revision must not imply any participant vote
  or assignment acceptance.
- Applications: shared `/applications` creates `applied`; coordinator invitation
  with `application_id` creates `offered`; the target independently accepts.
  Owner participation currently exposes offered/accepted items and its invitation
  input has no application reference. Add actual reviewed application/offer
  controls and evidence separately from invitation-only primary acceptance.

`docs/HP2_ACCEPTANCE.md` maps the actual routes, effects, authority/recovery
requirements and missing behaviors. Its scoped three-account setup foundation passed an actual browser/API/PostgreSQL
gate after the capacity worker released its test lease and root granted one. The
ledger records exact source, commands, retained databases and fixture limitations.
Public projection was unproven at that foundation checkpoint; the publication
feature below now has actual local proof and is merged in PR103 (`b0d49c7`). The full collaboration
journey remains unproven.

## Parallel owner behavior: explicit intention publication

Owner: `publication_oct25` (continuing the preserved branch). One branch
`feat/v0.1-owner-intention-publication`, from release `316fd4e`, isolated at
`/tmp/undergrid-worktrees/owner-intention-publication/feat-v0.1-owner-intention-publication`.
This closes the audited HP2 step-one owner publication control independently of
the capacity implementation. The locked capacity task and its test lease remain
unchanged. Owning requirements: HP2 step 1, TDD §§5.1/5.4 and existing selected
public projection/privacy contracts. Owning evidence:
`docs/OWNER_INTENTION_PUBLICATION.md` plus current `HP2_ACCEPTANCE.md` status.

Observable behavior: owner creation explicitly selects `redacted_public`, enters
a bounded public summary and reviews the actual allowlisted summary/output/hard-
requirements projection. Existing public intentions can revise their selected
summary after another review, while preserving their fixed audience. Private remains
the creation default; editing a private objective or expiry must not publish it.
The private objective, budget, preferences and server-owned partial audit are
never included in the public preview or foreign projection. Existing stored
intention/history remain private when their current visibility is private.

Runtime entrypoints remain the existing owner `POST /v1/owner/intents` and versioned
`PATCH /v1/owner/intents/{id}` commands, followed by normal authenticated foreign
`GET /v1/intents/{id}`. Reuse closed `IntentInput` and current domain projection,
authority, revision/idempotency/event/inbox/queue behavior. Do not add DTO fields,
provider/consent policy, a separate publication table or adapter-only authority.
The original task incorrectly assumed public-to-private revision was already
supported. The [published revision contract](evidence/intention-revisions.md)
deliberately rejects audience changes with `visibility_change_requires_grant`.
Keep that guard unchanged. Explicit existing withdrawal stops current public
discovery, disables the watch and denies foreign reads while retaining the
`redacted_public` visibility, revision head and immutable history. It is not a
private conversion or deletion. True consented disclosure transitions remain
separately unimplemented; do not implement them through generic revision UI.

Acceptance and required proof:

- Author and run focused UI behavioral RED before publication behavior changes.
  Prove explicit visibility/summary/projection review, private defaults, create
  and revise payloads, unchanged private fields and no automatic publication on
  unrelated edits. Preserve exact retry keys after malformed/unknown responses
  and ignore stale actor/intention responses using the existing request contract.
- Actual multi-owner browser/API/restricted-PostgreSQL lifecycle: A explicitly
  publishes through the console, B's normally issued scoped credential sees only
  the current allowlisted projection, A revises the public summary at the same
  audience, then explicitly withdraws; B receives 404. Observe the updated public
  projection before withdrawal and retained revision/history afterward. A direct
  attempted audience change still returns the existing 409.
  No product SQL seeds or hidden public revision may complete a browser step.
- Observe immutable revision/head changes and the existing normalized effects.
  Run relevant partial-remainder/audit privacy, current actor/revision and owner
  authorization regressions. Current public or private read policy remains a
  domain responsibility; do not widen it merely to make a UI test pass.
- Update feature evidence and HP2 ledger as implemented, locally proven, merged
  or pending accurately before the PR merges. Controlled identities and source
  fixtures do not establish independent beneficial owners or provider relevance.

Review actual DRY knowledge ownership (the server remains the public projection
contract), orthogonality, bounded readable inputs, retry/error resource ownership
and private failure behavior. All Go/Node gates require a root-granted serial
lease; authoring this contract grants none. Full HP2 discovery, recruitment,
room/plan/application flows and external qualification remain open.

## Locked task: owner-reviewed builder registration — 25 October 2026

Owner: `builder_oct26`, resuming the preserved `publication_oct25` task, independent of the semantic integration worker.
Branch `feat/v0.1-owner-builder-registration`, based on reviewed release
`b0d49c7770b007126eb17316f5da5f074708e4bc`, isolated at
`/tmp/undergrid-worktrees/builder-registration/feat-v0.1-owner-builder-registration`.
This contract precedes tests and behavior changes. It implements the builder C
onboarding prerequisite in HP2 acceptance step zero and the TDD r2 builder
recruitment scenario (§ coordination/recruitment and primary network scenario),
not the complete discovery-to-agreed-plan journey.

Behavior and runtime boundary:

- Use existing owner-session/CSRF `POST /v1/agents` and its closed AgentProfile:
  `public`, `accept_invitations`, `recruitment`, `capabilities`. Keep private/
  unlisted defaults, separate opt-ins and known bounded `source_catalog_v1`
  selection. Require review of the exact selected profile snapshot; changing
  that snapshot invalidates review. Claims are declared, not verified expertise.
- Add owner-only saved-profile acknowledgement to existing `GET /v1/agents`
  and its canonical/generated closed response. Normalize legacy missing/empty
  profiles to explicit false flags and an empty capabilities list. Do not widen
  the creation schema, add a profile PATCH or invent a public profile directory.
  The public declaration currently enables recruitment eligibility, not a public
  webpage. Existing-agent profile editing/revocation remains unsupported.
- Preserve current scope/policy/resource admission, active owner/agent authority,
  credential issuance/recovery, RLS and transaction boundaries. Publishing a
  declared capability or opting in grants no admission, rights, assignment or
  spending authority. Invitation eligibility and recruitment are orthogonal:
  invitation-off may still recruit but rejects foreign unsolicited offers.

Test-first acceptance and evidence:

1. Author and run focused UI RED for missing controls/review/defaults and real
   PostgreSQL owner-read/domain RED before changing behavior. Gate execution is
   serialized by root; no Node/Go slot is granted at contract time.
2. Prove exact creation body, review invalidation and owner refresh of the actual
   saved normalized profile. Preserve same key/body on malformed/lost creation
   responses, one agent/credential effect, and stale actor/draft safety. A success
   claim must follow a valid acknowledged response, not only an outgoing payload.
3. Through ordinary owner registration and normal API/worker recruitment, prove
   selected declared capability eligibility and independent opt-in flags. Test
   unlisted/recruitment-off/mismatched capability negatives, current active
   authority, owner-only list separation and no foreign credential/profile leak.
   Existing foreign invitation commands must produce exactly offered invitations
   only when allowed; opting in alone creates no membership. No automatic accepted
   membership, rights approval, task or financial effect is permitted.
4. Normal-agent room setup needed to exercise recruitment is explicit fixture/API
   support, not evidence that missing owner recruitment-role/room controls ship.
   Do not seed product rows to complete the user journey or call fixture accounts
   independent beneficial owners. Preserve recruitment budget/current-authority
   guards and network/financial separation.
5. Capture exact RED/GREEN/build/runtime source, logs and retained databases;
   assess DRY knowledge, orthogonality, contracts, readability and failure recovery.
   Independent review and scoped PR to `release/v0.1` precede integration. Full HP2,
   public directory, existing-profile mutation and external qualifications stay
   open unless independently implemented and proven.

## Next task contract: reviewed room origin and recruitment roles — 26 October 2026

Status: locked for implementation after semantic PR107 and prerequisite checkpoint
PR108, source release `6a93b05`. Implementer `semantic_oct26`; independent review
`review_oct26`; root integrates the reviewed feature. Branch
`feat/v0.1-owner-room-origin-recruitment`, isolated worktree
`/tmp/undergrid-worktrees/room-origin/feat-v0.1-owner-room-origin-recruitment`.
This assignment grants test authoring, not a costly test lease. The older planning
and static-audit checkpoint below remains history; PR107 now makes shared-demand
reads compare current input/profile/projection hashes before publication.

All four existing supplied origin kinds must be validated: `shared_demand`,
`external_request`, `external_capability`, `builder_recruitment`. The UI can start
from current owned-intention results; it needs no group viewer or new generic
read route. A selected origin pins the actual owned recipient agent. Selection,
agent/session/context or command-field changes invalidate review; accepted newer
drafts cannot be erased by older completions. Command fields freeze while their
snapshot is pending, and generation checks reject stale results.

Origin eligibility is checked in the shared domain transaction **before retained
idempotent response recovery**, not only before a new insert. Unchanged current
origin plus identical body/key recovers the same room; changed input conflicts.
Stale, dismissed, foreign, revoked, expired or changed source/candidate/hash/profile
origins deny recovery and produce no new effects. Objective-only commands retain
their existing creation/retry behavior.

Do not reuse migration040/043 dismissal eligibility wholesale: shared-demand
visible/current provenance does not require an active watch, although dismissal
and computed-group contracts do. Include a positive otherwise-current shared
origin with monitoring disabled. External kinds retain the existing monitored,
current permitted-source/observation/revision contract. Recruitment origins bind
the active public/restricted source room/coordinator and current recipient role,
profile and recruitment hash; receipt is neither admission nor rights consent.

Forward migration **045 is reserved for this feature**, after the plan feature's
frozen044. A narrow boolean/current-eligibility NOWAIT authority-lock helper is
permitted only where actual restricted application RLS prevents safely locking
foreign principal/agent/intention/source rows. It may not expose private foreign
rows, broaden table grants or change frozen040–044. Keep payload/current-evidence
validation in the shared domain, not owner adapters.

Review exactly the selected origin, objective, private/restricted visibility,
full-history disclosure and bounded zero-to-ten existing supported recruitment
roles. The existing created id/version/visibility/history response is sufficient;
no new DTO/read route is authorized merely for convenience. Verify persisted
origin/roles independently in the runtime proof. The parked group viewer at
`8f03ecd` remains preserved as a separate five-file feature delta.

Write actual restricted HTTP and console behavioral REDs before corrections,
then prove an actual normal-owner selected-match → reviewed restricted room with
origin and role → restricted-worker recruitment opportunity/event/inbox journey.
No browser product rows may be seeded and no additional member may be admitted.
All failure, retry, current-authority and unchanged HP1 checks remain mandatory. One later feature branch,
`feat/v0.1-owner-room-origin-recruitment`, must start from the then-current
reviewed `release/v0.1` in its own isolated worktree. Root assigns its implementer
and serial test lease after the current dependencies are ready. This contract is
recorded on `docs/v0.1-room-origin-contract`, based on release `f80460e`, at
`/tmp/undergrid-worktrees/room-origin-contract/docs-v0.1-room-origin-contract`.

Owning requirements: HP2 steps 2–3, SW-01/02/06 and TDD §§6/9, especially the
published `POST /v1/swarms` precondition: “visible current opportunity or explicit
objective.” The reference is provenance; it grants no candidate membership,
rights, assignment acceptance or spending authority. Objective-only creation
without an origin remains valid. Supplying an origin alongside an objective must
not turn a stale or foreign reference into valid provenance.

Static audit at `f80460e` (not a runtime reproduction): `createSwarm` in
`internal/network/swarms.go` checks a supplied origin's UUID and recipient-row
existence only. Opportunity RLS in migration 002 restricts the recipient, but
does not establish current source/candidate eligibility, suggestion state or
input hash. Owner creation delegates to that same domain command. Thus the code
contains no current-evidence guard for a retained stale origin. The check also
sits inside the idempotent callback, so retained command responses bypass it.
No origin-specific runtime test was found by this audit. Existing opportunity
listing alone is insufficient as an authority check: the shared-demand query
checks active/public rows without comparing the stored evidence hash to current
revisions. These findings need actual behavioral RED before implementation.

Runtime entrypoints and expected effects:

- Owner console creation in `web/src/CreateSwarm.tsx` uses existing
  `POST /v1/owner/agents/{agent}/swarms`; scoped agent creation uses
  `POST /v1/swarms`. Both must reach the same transaction-owned validation.
  Select an actual currently accessible opportunity, disclose its provenance and
  review the exact objective, nonpublic visibility, full-history policy and
  bounded recruitment-role specification before submission. Changes invalidate
  review. Keep public transcript consent and its existing shared-command guard
  distinct; do not widen the owner's existing private/restricted choices.
- Preserve existing role limits (at most ten, nonempty key, supported
  `source_catalog_v1` family and bounded description), policy/quota admission and
  owner/agent authorization. Review is a UI safeguard, never domain authority.
- A successful creation persists one swarm with the reviewed origin and roles,
  one accepted creator/coordinator membership, the existing creation event/inbox
  effects and durable recruitment work. Other participants remain unadmitted.
  Private rooms remain ineligible for public recruitment. No new table or
  migration is planned; justify any later necessity from actual implementation
  evidence, while preserving frozen migrations and semantic migration ownership.

Required test-first acceptance and failure recovery:

1. Reproduce stale-origin acceptance through actual restricted PostgreSQL HTTP
   commands before behavior changes. Prove a current eligible origin succeeds;
   reject foreign, dismissed, withdrawn/expired or inaccessible source/candidate
   references and changed revision/evidence bindings. Include current semantic
   profile/projection eligibility after that integration, and current permitted
   external source/observation evidence for supported external kinds. Cover
   current actor deactivation/revocation and concurrent eligibility changes with
   the same relevant authority locks; never obtain private foreign data merely
   to validate provenance. Denial must commit no swarm, membership, event/inbox
   or recruitment-job effects.
2. Test current eligibility before retained-response recovery for commands with
   an origin, as well as current owner/agent authorization. Under unchanged
   authority and input, a lost or malformed response must reuse the same key and
   body, recover the same room and create no duplicate effects. Changed payloads
   conflict. A stale origin must not become usable through a previously retained
   key. Keep objective-only retries working without inventing an origin gate.
3. Write console behavioral RED for missing current-origin/role controls, review
   invalidation, history disclosure and exact outgoing command. Prove stale
   actor/selection responses cannot restore an obsolete review or room, and
   response validation precedes clearing retry identity. Do not silently drop an
   invalid selected origin and submit an objective-only command instead.
4. Run an actual owner browser/API/worker lifecycle after dependencies merge:
   review the current match, create the room with exact origin and role, observe
   persisted provenance and one attributable eligible builder opportunity.
   Recruitment receipt creates no invitation acceptance or membership. Preserve
   separate later invitation/application review and explicit admission steps;
   fixture setup cannot stand in for the missing owner controls.
5. Preserve owner isolation, current room/history ACLs, quota/idempotency guards,
   separate admission/plan/rights/assignment decisions and HP1 recovery. Assess
   DRY ownership of eligibility rules, orthogonality, readable provenance and
   transaction/error ownership. Record actual RED/GREEN, source SHA, commands,
   logs and retained databases in `docs/OWNER_ROOM_ORIGIN_RECRUITMENT.md`; update
   this task's status and `HP2_ACCEPTANCE.md` before independent review and a
   scoped feature PR to `release/v0.1` can merge.

This task does not implement complementary capability discovery, application
review, plan proposal/revision or the full HP2 journey. Account separation and
controlled recruitment fixtures do not qualify independent beneficial owners,
provider relevance, legal consent or deployment. Those gates remain open.


### Builder registration implementation checkpoint — 26 October 2026

The locked builder task above is implemented and locally proven on
`feat/v0.1-owner-builder-registration` (`7728350` frozen runtime/test source),
pending independent final review/merge. Evidence and exact RED/GREEN/retained
runtime gates: [`OWNER_BUILDER_REGISTRATION.md`](OWNER_BUILDER_REGISTRATION.md).
The console reviews explicit independent profile opt-ins and acknowledges the
saved normalized owner-only profile. Ordinary browser registration/reload and
actual restricted-worker recruitment pass; the API-created fixture room is not
completion of the later owner room-origin/role contract. Existing-profile mutation
and public-directory controls remain unsupported. Full HP2 remains open.


## Remaining typed owner requirement controls

The semantic integration's public review and command payload share the actual
required-field/license/coverage draft. Stored freshness/delivery/input-schema
requirements are conserved. Their explicit owner create/edit controls remain an
atomic follow-up before full HP2 closure, with test-first payload/preview/review
invalidation and exact unedited timestamp preservation. No unsupported output
format selector is implied; the current domain supports source-catalog JSON.


## HP2 prerequisite: freeze coordination-policy inputs during workspace refresh

The real owner-plan browser exposed a race after its initial intention POST:
the console allowed policy edits while the resulting workspace refresh was still
busy, then the refreshed policy record replaced that draft. The Save button alone was
disabled. This prerequisite belongs to HP2 steps 1/4 and the existing versioned
owner coordination-policy contract; it grants no plan, rights or spending consent.

Task branch `feat/v0.1-owner-policy-busy-draft`, based on release `6a93b05`, owns
`web/src/OwnerPolicy.tsx` and its behavioral tests. Its isolated worktree is
`/tmp/undergrid-worktrees/owner-policy-busy/feat-v0.1-owner-policy-busy-draft`.
Acceptance: all six policy input kinds remain disabled throughout the parent's
busy workspace action; attempted edits cannot create an apparently editable
draft that a pending refresh discards. Controls reenable after refreshed state
loads and explicit subsequent edits reach the existing save callback. Existing
succession opt-in defaults and retained consent must remain unchanged.

The real runtime entrypoint is the owner-console policy form. Persistence remains
the existing versioned policy command; this fix adds no database effects, grants,
migrations or financial coupling. Behavioral RED precedes the disabled-fieldset
change; focused policy/App regressions and TypeScript/production build are the
local gates. Evidence/status live in
[`OWNER_POLICY_BUSY_DRAFT.md`](OWNER_POLICY_BUSY_DRAFT.md). Independent review and
a feature PR to `release/v0.1` precede integration into the owner-plan browser.
