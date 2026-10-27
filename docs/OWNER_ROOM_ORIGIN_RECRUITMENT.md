# Owner room origin and recruitment roles

Status: implemented and locally proven on `feat/v0.1-owner-room-origin-recruitment`;
final independent review/feature PR merge pending. HP2 steps 2–3; SW-01/02/06;
TDD §§6/9. This is a bounded journey, not full HP2 closure or external qualification.
The branch normally integrates plan PR110/release `2099a76` and documentation
checkpoint `054a269`. Its locked task contract is in `HP2_EXECUTION.md`.

## Behavior and authority contract

An owner selects a current result belonging to the responsible agent, reviews its
origin together with the exact objective, private/restricted visibility, full
history disclosure and zero-to-ten supported recruitment roles, then creates a
room. Owner and agent commands share the domain transaction. Provenance grants
no admission to either room, plan consent, rights, assignment or spending.

Supplied origins must remain current before a new write **and** before recovery
of an existing receipt. Current identical commands recover the same room; changed
commands conflict. Ineligible origins deny access without new effects. An
objective-only command remains valid. Current shared demand does not require
monitoring; external evidence retains its monitoring and source-permission gates.
Semantic evidence remains bound to current profile, projection and processing
consent. Recruitment binds the current room, coordinator, role and recipient
profile. Current authority stays locked until commit or rollback.

Frozen migrations 040–045 are not editable. Migration 045 adds the reviewed
narrow boolean authority-lock helper required by restricted RLS; it discloses
no foreign private rows and adds no broad table grants.

## Behavioral tests and actual focused evidence

All tests below target actual runtime entrypoints. Frozen test-only `d8f7f5e`
ran before product changes. Restricted HTTP RED failed in 41.708 seconds: six
top-level tests and 33 subcases failed because stale origins still created rooms
or recovered retained receipts; two positive top-level controls passed. All
external/semantic fixtures reached their genuine current positive preconditions.
Whole-console RED failed all three cases in 24.84 seconds because match selection
was absent. Original logs remain:

- `/tmp/undergrid-room-origin-d8f7f5e-http-red-20261027.jsonl`
- `/tmp/undergrid-room-origin-d8f7f5e-ui-red-20261027.log`

Product `c8e2b2e` passed eight top-level HTTP tests and 34 subcases in 36.665
seconds, including an added bounded coordinator-cutover contention case. Ten
console/component tests passed in 7.41 seconds. Logs:

- `/tmp/undergrid-room-origin-c8e2b2e-http-green1-20261027.jsonl`
- `/tmp/undergrid-room-origin-c8e2b2e-ui-green1-20261027.log`

Both HTTP runs used retained database `room_origin_red_20261027` on isolated
network PostgreSQL port 55502, cloned normally from `undergrid_hp1_template`.
Restricted API/worker roles enforce actual RLS. No product/evidence reset occurred.
After root and independent SQL review, normal migration applied 045 at
`2026-10-27 15:40:10.911945 UTC`; its now-frozen SHA256 is
`3a3bd9eba721f88a0e0bdbfc6a35b33d80a6cbc8f33c8ce5b601acbebe2bbb7f`.
The first SQL review caught coordinator identity being derived before locking
its room. The correction pins the room before deriving current authority and
adds real bounded contention plus successor-revocation evidence. No applied
migration changed.

Release policy correction `862063d` subsequently merged normally as `6c5cbe4`.
Additional pending/context UI guards passed in the focused 12-case gate on
`df07bf9` (8.87 seconds), followed by strict TypeScript success. Costly gates remain
serialized. Go temporary compiler artifacts use `/dev/shm/undergrid-gotmp`; no
user databases or evidence are deleted to make gates pass.

| Test | Runtime contract |
| --- | --- |
| `TestRoomOriginRejectsRetainedIneligibleSharedEvidenceBeforeCreation` | Owner and agent commands deny dismissed, withdrawn, revised, expired, paused and revoked/suspended evidence; room, membership, event, inbox and job counts stay unchanged. |
| `TestRoomOriginCurrentEligibilityPrecedesRetainedResponseRecovery` | Current identical commands recover; candidate withdrawal denies the same retained key. |
| `TestRoomOriginVisibilityDoesNotInventSharedMonitoringRequirement` | Visible current shared demand creates with monitoring disabled; saved origin and roles agree, only creator is admitted. |
| `TestRoomOriginForeignAndObjectiveOnlyControlsRemainDistinct` | Foreign provenance is denied; objective-only retry remains duplicate-free. |
| `TestRoomOriginExternalKindsRequireCurrentSourceBeforeReceiptRecovery` | Both actual source adapters refresh controlled HTTPS data and restricted workers publish opportunities; removal of source permission denies old and new keys. Fixture data is not source qualification. |
| `TestRoomOriginRecruitmentRequiresCurrentRoomAndRecipientProfile` | Restricted recruitment publishes a real opportunity; archived/private room, revoked coordinator, recipient optout and changed role deny creation/recovery without source admission. |
| `TestRoomOriginSemanticEvidenceRemainsBoundToCurrentProcessingProfile` | Actual controlled embeddings produce semantic evidence for public and explicitly consented private sources; operator profile cutover denies creation/recovery. Fixture vectors are not provider qualification. |
| `TestRoomOriginCurrentForeignAuthorityMustRemainLockedThroughCreationAndReplay` | Uncommitted foreign revocation yields retryable rollback for new and retained commands; same-key recovery succeeds after rollback; changed command conflicts. |
| `OwnerRoomOriginRecruitment.test.tsx` | Whole-console actual selection, reviewed payload/roles, draft invalidation and malformed-response exact-key recovery. |

## Final local journey and retained gates — 27 October 2026

Runtime integration `c95c0b9` preserves both the room-origin and plan-proposer
preconditions before retained receipt recovery. Independent review checked the
merge and the shared evidence refactor. Full UI passed **163 tests / 19 files in
98.75 seconds**. Source/build provenance is:

- Source SHA256: `24844a3dc491229b91f640116c4e08ba68bedb831e138fc32e363ba2e478203d`.
- Production JavaScript: `assets/index-BDFu98Hd.js`, SHA256
  `b9b495a2895be8cc6475e37b9e734f488e5416bf1f7d8521a2f77683941daa35`.
- Coupled TypeScript/Vite/manifest ran on `c95c0b9` (Vite 1.50 seconds),
  selector-only `857e07e` (1.38 seconds) and test-only `2950d02` (1.41 seconds).
  Source and asset hashes are identical. Browser harnesses independently checked
  the exact corresponding HEAD/source/artifact manifest before running.

`TestHP2LiveOwnerSelectedMatchRoomAndRecruitment` passed on `857e07e`:
Playwright case **15.8 seconds**, Playwright aggregate **17.3 seconds**, Go case
**19.07 seconds**, race package **20.111 seconds**. Three normal controlled OIDC
owners register scoped agents through the console. B explicitly publishes a
reviewed summary; A selects the actual current exact match and reviews its pinned
responsible agent, room objective, restricted visibility, full history and role.
Chromium forwards the real creation command to a successful server commit, then
deliberately loses its response. The unchanged form recovers the **same body,
Idempotency-Key and room ID**. A genuine restricted `RunWorker` publishes C's
recruitment opportunity. Independent final SQL proves the saved A→B opportunity
origin, exact role, one room, one creator membership and one creation receipt;
recruitment opportunity/event/inbox are each exactly one. C remains unadmitted,
foreign invitations remain disabled and C's private room context is denied.
There are no browser product SQL seeds and no independent-owner/provider claim.

Retained room database: `hp1_run_3752d4f7a28847cfab1d2240b6b182ba`, network port
55502. Artifacts: `/tmp/undergrid-hp2-browser-1706078624`.

The same final restricted race selector passed **17 top-level tests + 36 subcases
in 83.752 seconds** on `2950d02`, database `room_origin_final2_20261027`. It covers
all eight room groups / 34 room subcases plus exact/external/recruitment dismissal,
semantic dismissal/restart/profile cutover and focused plan/partial-satisfaction
authority/rollback guards. A retained actual owner-plan browser on this same HEAD
proves the shared harness/merge preserved two immutable plan activations,
individual decisions and restart recovery: Playwright case 26.8 seconds,
aggregate 28.4 seconds, Go case 30.10 seconds, race package **31.143 seconds**.
Its database is `hp1_run_da54780b36da41fe91bd404e470027b9`; artifacts are
`/tmp/undergrid-hp2-browser-2759741456`.

Canonical lint, **160 JSON Schema components**, generated wire parity, all-package
vet with `hp1_live,hp2_live` tags and `go build ./...` all passed. After these gates,
normal merge `c0a57ec` adds only the four documentation files from release
`054a269`; subsequent evidence edits are also documentation only. Runtime source
and frozen migrations have not changed since their proving gates.

Final command families (run sequentially from the feature worktree; npm/recorder
commands run from `web`):

```sh
npm test -- --reporter=verbose
node scripts/record-live-build.mjs
go test -race -json -count=1 -timeout=240s -tags=hp1_live,hp2_live ./internal/network -run '^TestHP2LiveOwnerSelectedMatchRoomAndRecruitment$'
go test -race -json -count=1 -timeout=240s ./internal/network -run '^(TestRoomOrigin.*|TestOpportunityDismissal.*|TestSemanticSuggestionDismissal.*|TestSemanticDismissal.*|TestPlanProposalReplayRejectsCurrentObserver|TestOwnerPlanProposalCreatesImmutableDependentTasksWithoutVotes|TestPartialSatisfactionCurrentSourceAndActorRequiredForRetry|TestPartialSatisfactionNOWAITAndEnqueueFailureRollBackAllEffects)$'
go test -race -json -count=1 -timeout=240s -tags=hp1_live,hp2_live ./internal/network -run '^TestHP2LiveOwnerPlanProposalRevisionAndRecovery$'
npm run contract:check
go vet -tags=hp1_live,hp2_live ./...
go build ./...
```

Go executable: `/tmp/undergrid-toolchain/go/bin/go`; environment:
`GOTMPDIR=/dev/shm/undergrid-gotmp`, `GOCACHE=/tmp/undergrid-gocache`,
`GOMODCACHE=/tmp/undergrid-gomod`. Browser administration uses
`UNDERGRID_HP1_ADMIN_DATABASE_URL=postgres://nmcdc@localhost:55502/postgres?host=/tmp/undergrid-pg-semantic-55502/socket&sslmode=disable`;
domain gates use the same socket/port with their named database in
`UNDERGRID_TEST_DATABASE_URL`. Chromium is
`/home/nmcdc/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`, with
`LD_LIBRARY_PATH=/tmp/undergrid-browser-libs/root/usr/lib/x86_64-linux-gnu`.
The template is genuinely normally migrated; no migration ledger is fabricated.

Exact final logs:

- `/tmp/undergrid-room-origin-c95c0b9-full-ui-20261027.log`
- `/tmp/undergrid-room-origin-c95c0b9-build-20261027.log`
- `/tmp/undergrid-room-origin-857e07e-build-20261027.log`
- `/tmp/undergrid-room-origin-857e07e-browser-20261027.jsonl`
- `/tmp/undergrid-room-origin-2950d02-build-20261027.log`
- `/tmp/undergrid-room-origin-2950d02-domain-final2-20261027.jsonl`
- `/tmp/undergrid-room-origin-2950d02-plan-browser-20261027.jsonl`
- `/tmp/undergrid-room-origin-2950d02-contract-20261027.log`
- `/tmp/undergrid-room-origin-2950d02-vet-20261027.log`
- `/tmp/undergrid-room-origin-2950d02-go-build-20261027.log`

Machine-readable log/status/SHA256 index:
`/tmp/undergrid-room-origin-validation-20261027.json`.

## Preserved failures and bounded remediation

The first room browser failed **42.076 seconds** on `c95c0b9` because Playwright's
exact label-text selector included select-option text. The retained accessibility
snapshot shows the correctly pinned disabled responsible-agent combobox and
current origin. Test-only `857e07e` uses exact named combobox roles for responsible
agent and visibility; no payload, identity, timing or permission oracle changed.
Original log: `/tmp/undergrid-room-origin-c95c0b9-browser-20261027.jsonl`; database
`hp1_run_aaa4841a62a64413ad70c5a0fd2793fb`; artifacts
`/tmp/undergrid-hp2-browser-2632207546`. One recorder invocation from the repository
root failed before execution; the corrected web-directory coupled build is the
only claimed artifact proof. Its failed log is
`/tmp/undergrid-room-origin-browser-selector-build-failed-cwd-20261027.log`.

The first combined domain aggregate on `857e07e` failed **99.842 seconds**:
16 top-level tests and 36 subcases passed; one semantic lifecycle failed its final
strict dismissal while a real worker held foreign principal authority. The exact
server request `855b4b5d-7284-4a9f-a626-e9e38cc98e71` logged PostgreSQL **55P03**,
not an RLS or evidence-validation failure. The public response correctly remained
500/retryable with rollback. Original log:
`/tmp/undergrid-room-origin-857e07e-domain-final-20261027.jsonl`; original database
`room_origin_final_20261027` remains retained.

Test-only `31fe011` deterministically holds the foreign principal, sends a bounded
actual semantic dismissal and proves prompt 500/retryable, suggested state and
zero dismissal event/receipt. Releasing the lock lets the **same key** succeed and
recover exactly one final event. This characterization passed in **2.423 seconds**;
request `c2c2306d-4c15-493d-ade9-7baf9e6b7056` logged the same 55P03.
Test-only `2950d02` then waits for the existing real worker's bounded shutdown
before the lifecycle's final strict successful post-cutover receipt assertion.
The earlier live-worker/restart phases remain; there is no retry loop, widened
shutdown bound or weakened success/count oracle. The focused lifecycle passed
**12.185 seconds**, then the unchanged full selector passed 83.752 seconds.
Independent review approved this test-only separation of worker discovery and
expected authority contention. Production code did not change.

Remediation logs:
`/tmp/undergrid-room-origin-semantic-contention-repro-20261027.jsonl` and
`/tmp/undergrid-room-origin-2950d02-semantic-lifecycle-focused-20261027.jsonl`.

## Quality assessment and remaining gates

`swarmCommand` owns origin eligibility before idempotent recovery for both owner
and agent adapters. Narrow SQL owns foreign authority locks; shared
`opportunity_evidence.go` owns current demand/semantic and recruitment evidence
knowledge for room creation and dismissal. Their different watch/state contracts
stay separate. UI review and frozen snapshots are presentational safeguards;
they grant no domain authority. Atomic effects, bounded contention and exact
recovery are directly exercised. No financial/provider abstraction or deployment
coupling was added.

Final independent evidence review and feature PR merge to `release/v0.1` remain
pending. HP2 still needs group viewing, complementary registered capability
discovery, unknown-hard clarification, remaining typed requirement controls,
application review/offer and the complete continuous discovery-to-accepted-plan
journey. Separate controlled accounts do not prove independent beneficial owners.
Source/provider/legal/cohort/deployment qualification and release promotion remain
separate open gates.
