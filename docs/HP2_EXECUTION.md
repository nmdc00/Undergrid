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
current `release/v0.1`, isolated worktree under `/tmp/undergrid-worktrees`.
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

`live_resume_oct24` owns a separate `test/v0.1-hp2-live-journey` branch/worktree
from release, initially recording `docs/HP2_ACCEPTANCE.md` and actual missing
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
