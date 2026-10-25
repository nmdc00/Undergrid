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

## First implementation task: discovery capacity

Owner: `partial_resume_oct24`. New branch `feat/v0.1-discovery-capacity`, from
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

- Intention publication: the shared `IntentInput` and owner intention command can
  accept `redacted_public` with `public_summary`, but `App.tsx` currently offers
  private/restricted creation and preserves visibility during revision. Explicit
  owner selection/review of a public projection is missing; do not bypass it with
  fixture SQL or relabel a hidden API call as completed browser publication.
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
ledger records exact source, commands, retained databases and fixture limitations;
public projection and the full collaboration journey remain unproven.

## Parallel owner behavior: explicit intention publication

Owner: `live_resume_oct24`. One new branch
`feat/v0.1-owner-intention-publication`, from release `316fd4e`, isolated at
`/tmp/undergrid-worktrees/owner-intention-publication/feat-v0.1-owner-intention-publication`.
This closes the audited HP2 step-one owner publication control independently of
the capacity implementation. The locked capacity task and its test lease remain
unchanged. Owning requirements: HP2 step 1, TDD §§5.1/5.4 and existing selected
public projection/privacy contracts. Owning evidence:
`docs/OWNER_INTENTION_PUBLICATION.md` plus current `HP2_ACCEPTANCE.md` status.

Observable behavior: owner create/revise controls explicitly select
`redacted_public`, enter a bounded public summary and review the actual allowlisted
summary/output/hard-requirements projection before publication. Private remains
the creation default; editing a private objective or expiry must not publish it.
The private objective, budget, preferences and server-owned partial audit are
never included in the public preview or foreign projection. Existing stored
intention/history remain private when their current visibility is private.

Runtime entrypoints remain the existing owner `POST /v1/owner/intents` and versioned
`PATCH /v1/owner/intents/{id}` commands, followed by normal authenticated foreign
`GET /v1/intents/{id}`. Reuse closed `IntentInput` and current domain projection,
authority, revision/idempotency/event/inbox/queue behavior. Do not add DTO fields,
provider/consent policy, a separate publication table or adapter-only authority.
Public-to-private revision must revoke the current foreign read immediately;
immutable historical revisions are not a grant to keep reading old public data.

Acceptance and required proof:

- Author and run focused UI behavioral RED before publication behavior changes.
  Prove explicit visibility/summary/projection review, private defaults, create
  and revise payloads, unchanged private fields and no automatic publication on
  unrelated edits. Preserve exact retry keys after malformed/unknown responses
  and ignore stale actor/intention responses using the existing request contract.
- Actual multi-owner browser/API/restricted-PostgreSQL lifecycle: A explicitly
  publishes through the console, B's normally issued scoped credential sees only
  the current allowlisted projection, A revises back to private, B receives 404.
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
