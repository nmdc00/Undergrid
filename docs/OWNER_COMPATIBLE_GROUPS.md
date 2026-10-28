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

At the initial contract checkpoint no behavior had been imported or tested.
The completed implementation and current proof are recorded below. Room PR112 is merged; its post-merge results will be recorded
from root's authoritative checks. HP2 remains open beyond this bounded viewer.


## Implemented behavior and current proof — 28 October 2026

The owner explicitly opens bounded compatible views from the current monitored
intention. Views remain separate; declared licenses grant no rights and principal
IDs do not establish independent ownership. Responses are validated before display,
including closed shapes, finite cardinalities, member identity/source inclusion,
nullable constraints, Unicode label bounds, uniqueness and RFC3339 delivery values.
Calendar, clock and zone components are checked without JavaScript date normalization;
original fractions/offsets and valid leap-second notation remain unchanged.
Source-version/session changes invalidate pending responses. Content is inert text.
No server grouping, authority policy, database migration or financial code changed.

### Test-first evidence and preserved failed attempts

- `fa36d62`: actual missing-control App RED, one failure/25 skipped in 2.90s;
  `/tmp/undergrid-owner-groups-fa36d62-app-red-oct27.log`.
- Recovered component/mount: 18 selected cases passed in 4.24s. Current-schema
  malformed constraints then produced ten actual behavioral failures in 11.66s;
  `/tmp/undergrid-owner-groups-canonical-red-expanded-oct27.log`.
- Corrected parser/revision guards passed 30 selected cases in 5.44s, then all
  54 App/component cases passed in 23.34s. The initial revision-test attempt used
  an unavailable matcher; its two failures are fixture errors, not behavioral RED.
- Impossible February 30 produced behavioral RED (2.32s), followed by the calendar
  correction. The full console suite on `365b8a9` passed 197 tests/20 files in
  70.81s; `/tmp/undergrid-owner-groups-final-ui-oct27.log`.
- Hour 24 produced a separate behavioral RED (2.31s). Published leap-second
  positives then reproduced two failures before the complete finite timestamp
  correction. Final component cases on `f4828c5` passed 41 tests in 5.52s; the
  unchanged App cases were skipped in this focused command. The earlier full UI
  run and focused final parser proof are separate evidence, not a claimed rerun.
- The first production build failed on test-fixture TypeScript types. Correcting
  those types changed no behavior/assertion; the final coupled strict TypeScript,
  Vite and provenance-manifest command passed (Vite 1.45s). Initial wrong-directory/
  no-test selections and matcher/type failures remain indexed separately and are
  never relabeled as behavioral RED or successful artifact proof.

### Actual three-owner browser

Frozen `f4828c54221b3c7b62e2bf0010fcbf3f9c9631c0` passed
`TestHP2LiveOwnerInspectsNontransitiveGroupsAndCurrentRevision`: test 38.28s,
race package 39.308s. Normal signed OIDC owners and scoped agents created A's
private watched intention and B/C's explicitly reviewed public intentions.
A declares CC0/ODC alternatives, B only CC0 and C only ODC. Actual restricted
workers discover the evidence. The owner sees exactly A/B and A/C two-member
views, never a transitive A/B/C view. Revising A through the console to CC0 clears
the obsolete C view; only A/B remains after refresh and reload. Independent SQL
checks four immutable intention revisions, current source revision two, the exact
license sets and completed scan jobs. The shared harness verifies zero rooms,
memberships, plans, tasks or votes. No product rows were seeded by SQL.

- Log: `/tmp/undergrid-owner-groups-f4828c5-browser-oct27.jsonl`.
- Genuine network database: `hp1_run_e7205ec9962044e6935af84d6becfc06`, port 55502.
- Artifacts: `/tmp/undergrid-hp2-browser-4229265430`.
- Source SHA256: `d4bcec38d6cf66b9aa96a418013614ae2cd0de9f0d8ff9541a779ba612ac2bb2`.
- JavaScript SHA256: `839278f7861bb4d0c9f443c7552747fdde8d4e4d87a3639916cce947d1adb034`.

### Retained gates on the same frozen candidate

Seven restricted PostgreSQL group test groups plus two subcases passed in
23.115s. Selector:

```sh
go test -race ./internal/network -run '^Test(CompatibleGroups.*|SemanticGroupsRequireEveryCurrentPairAboveSourceThreshold|CurrentReleaseMatchingPolicyAndGroupEntrypoints)$' -count=1
```

The log is `/tmp/undergrid-owner-groups-f4828c5-domain-oct27.jsonl`; fixtures
use real restricted roles and controlled data, not provider qualification.
Canonical lint, 160-schema compilation and generated TypeScript parity passed.
Tagged all-package vet and `go build ./...` exited zero. Their logs are
`/tmp/undergrid-owner-groups-f4828c5-{contract,vet,go-build}-oct28.log`.

Both retained browser modes passed with the exact same candidate/manifest:
room origin/recruitment 37.59s and plan revision/recovery 33.57s; combined race
package 72.192s. Existing per-test browser, discovery and shutdown bounds remain.
Log: `/tmp/undergrid-owner-groups-f4828c5-retained-browsers-oct28.jsonl`.
Databases: `hp1_run_a9ccc802c7174263ac60a0f1cebd29fb` and
`hp1_run_2b2ea0fd700846bfa1a7e12b470b3dc6`. Artifacts:
`/tmp/undergrid-hp2-browser-3883972645` and
`/tmp/undergrid-hp2-browser-1040796996`. Network and financial identities remain
separate. All costly gates ran serially; compiler temporaries used
`/dev/shm/undergrid-gotmp`. No evidence/database cleanup made a gate pass.

A SHA256/size index of all 24 retained feature logs is
`/tmp/undergrid-owner-groups-validation-oct28.json`.

## Quality, review and limits

The viewer owns display/response validation; the existing shared domain owns
feasibility, authority and current evidence. Separate views are not combined in
the UI. No speculative framework or provider/financial dependency was introduced.
Independent review checked the recovered delta, current schema, revision races,
complete timestamp helper and actual browser/SQL proof. Root completed the remaining
retained gates after worker quota exhaustion and reviewed their terminal results.
Final documentation/feature PR integration is complete in PR114/release `bc6ddda`.

This closes local proof for the bounded viewer only. Unknown-hard clarification,
complementary registered capabilities, remaining typed requirement editing,
application review/offer and the continuous HP2 journey remain open. Controlled
accounts/vectors do not qualify beneficial-owner independence, providers, legal
terms, real-source outcomes or deployment. No release promotion is implied.


## Merged release checks — 28 October 2026

PR114 merged final feature head `c56a83c5c66accd569cd45ce84d3bcb68b2e9a2a`
into `bc6dddaeef89757081d7611ab74a8535b629fea7`. Its final delta after tested
runtime `f4828c5` is documentation only; all 24 log hashes were checked before merge.
Root checked the exact merged release:

- Viewer component: 41 tests passed, 14.08s;
  `/tmp/undergrid-groups-release-bc6ddda-ui-oct28.log`.
- Coupled TypeScript/Vite/manifest passed, Vite1.52s; source `d4bcec38...` and
  JavaScript `839278f...` match the full hashes recorded above;
  `/tmp/undergrid-groups-release-bc6ddda-build-oct28.log`.
- Restricted PostgreSQL three top-level cases/two subcases passed, 20.456s;
  `/tmp/undergrid-groups-release-bc6ddda-domain-oct28.jsonl`. Genuine database
  `groups_release_bc6ddda_oct28` was cloned from the preserved normal network
  template. The fixture restarted normally after interruption; no database,
  migration ledger or evidence was reset. This is not production restore proof.

The merged viewer is locally complete. HP2 remains active. Visible unknown-hard
clarification is next, followed by the other explicit remaining outcomes above.
Workers reached their stated quota until 2 November; root completed these
already-authorized gates directly, without replacement workers/model changes.
