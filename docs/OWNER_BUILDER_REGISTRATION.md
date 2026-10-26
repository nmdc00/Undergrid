# Owner-reviewed builder registration

Status on 26 October 2026: implemented and locally proven on this feature branch;
independent final review and merge to `release/v0.1` pending. Full HP2 remains open.
Owning contract: [HP2 execution](HP2_EXECUTION.md), locked task `04f91de`, TDD r2
§6 and §9 primary builder scenario. This is a write/read owner behavior, not a
public directory, existing-profile editor or complete recruitment/admission UI.

## Behavior and boundaries

The ordinary console registration explicitly sends the existing four-field
`AgentProfile`: public declaration, foreign invitations, recruitment and declared
capabilities. All flags default false, capabilities empty. The bounded console
choice is `source_catalog_v1`; the domain's existing capability acceptance is not
narrowed. Review of the exact profile is required even for private defaults;
changing a profile field clears review. Invitation and recruitment flags are
independent. Public declaration currently enables recruitment eligibility rather
than creating a publicly readable profile webpage. Declarations verify neither
expertise nor beneficial ownership.

`POST /v1/agents` remains the existing owner-session/CSRF, idempotent domain command.
No mutation route, database migration, scope grant, admission or policy exception
was added. Inputs freeze while submission is pending. Credential response
validation, including the credential ID, happens before the existing retry-key
manager clears uncertainty; identical malformed/lost-response retries preserve
key/body. A generation check discards a late result after owner-session expiry.
The credential remains an in-memory, dismissible one-time display.

Owner-only `GET /v1/agents` now acknowledges the saved profile. Its separate closed
`SavedAgentProfile` response schema requires all three booleans and a string
array; the optional/nullable creation schema is unchanged. Legacy missing fields
and null capabilities normalize to false/empty at read time without updating the
stored row. The console displays this saved acknowledgement after creation and
reload, and reports missing/malformed acknowledgement without inventing consent.
No foreign profile/credential read or profile-edit endpoint was added.

## Test-first evidence and preserved failures

Tests first: `02143f7`, then `b30c7a2` adds opt-in characterizations. No product
behavior changed before the following terminal REDs:

- `npm test -- --maxWorkers=1 src/OwnerBuilderRegistration.test.tsx`: **4 failed,
  14.46 seconds**, missing review/controls; log
  `/tmp/undergrid-builder-red-b30c7a2-ui-20261026.log`. The wrapper printed the
  failed aggregate with `tail`; its shell exit zero is not a passing test result.
- `go test -json -race ./internal/network -run '^TestOwnerAgentListAcknowledgesDeclaredAndLegacyProfiles$' -count=1`:
  **FAIL 1.009 seconds**, actual owner list lacks profile acknowledgement; log
  `/tmp/undergrid-builder-red-b30c7a2-domain-20261026.jsonl`, retained network DB
  `undergrid_builder_red_20261026` on 55500, genuinely copied from the normal
  migrated empty `undergrid_hp1_template` (through 041).

Product implementation `2bfbf83`; restricted-login test correction `8b081c2`:

- Focused four UI cases: **PASS 3.13 seconds**;
  `/tmp/undergrid-builder-green-focused-ui-20261026.log`.
- Original domain aggregate **FAILED 2.399 seconds**: profile acknowledgement
  passed; all five new opt-in subcases hit an oracle typo, `count(i.id)` although
  inbox has no `id`. This is a fixture failure, not a product regression/RED:
  `/tmp/undergrid-builder-green-2bfbf83-domain-20261026.jsonl`.
- Corrected oracle uses `count(i.event_id)` and existing restricted app/worker
  LOGIN fixtures, retaining original assertions. **PASS 4.514 seconds, two
  top-level cases plus five subcases**;
  `/tmp/undergrid-builder-green-restricted-domain-20261026.jsonl`, DB
  `undergrid_builder_green_20261026`. Owner presence/count, legacy read status,
  stored-row immutability and foreign profile/credential isolation are explicit.

## Frozen release integration proof

Runtime/test source `77283502059f39f2975fc76b56579607a93bd2a9` includes the
reviewed release task-contract merge. Later changes strengthen only the offered-
state test oracle, clarify comments and document evidence; product code is frozen.
All costly gates ran sequentially under the root's explicit lease:

| Gate | Actual result | Retained log |
| --- | --- | --- |
| Focused builder UI, including late owner expiry | 5 passed, 3.51s | `/tmp/undergrid-builder-7728350-focused-ui-20261026.log` |
| Full console UI | 125 passed / 16 files, 42.34s | `/tmp/undergrid-builder-7728350-full-ui-20261026.log` |
| Coupled TypeScript/Vite and source/artifact manifest | PASS, Vite 1.39s | `/tmp/undergrid-builder-7728350-build-manifest-20261026.log` |
| Actual builder browser/API/worker/PostgreSQL lifecycle | PASS 37.539s package; 36.51s Go case; 18.1s scenario / 32.6s runner | `/tmp/undergrid-builder-7728350-browser-20261026.jsonl` |
| Retained full HP1 partial remainder and HP2 publication/withdrawal browsers | Both passed, 54.676s package | `/tmp/undergrid-builder-7728350-retained-browser-20261026.jsonl` |
| Owner issuance/replay/concurrency/policy, credential isolation, recruitment current authority and actual HTTP contract | 8 top-level + 7 subcases passed, 25.867s | `/tmp/undergrid-builder-7728350-retained-domain-20261026.jsonl` |
| `npm run contract:check` | PASS, 145 schemas, generated parity | `/tmp/undergrid-builder-7728350-contracts-20261026.log` |
| `go vet -tags=hp1_live,hp2_live ./...` | PASS exit 0 | `/tmp/undergrid-builder-7728350-vet-20261026.log` |
| `go build ./...` | PASS exit 0 | `/tmp/undergrid-builder-7728350-go-build-20261026.log` |

Browser commands (same actual cached environment for both selectors):

```sh
export GOCACHE=/tmp/undergrid-gocache GOMODCACHE=/tmp/undergrid-gomod
export UNDERGRID_HP1_ADMIN_DATABASE_URL='postgres://nmcdc@localhost:55500/postgres?host=/tmp/undergrid-pg-hp1-55500/socket&sslmode=disable'
export UNDERGRID_TEST_NODE=/home/nmcdc/.nvm/versions/node/v24.15.0/bin/node
export UNDERGRID_TEST_CHROMIUM=/home/nmcdc/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome
export LD_LIBRARY_PATH=/tmp/undergrid-browser-libs/root/usr/lib/x86_64-linux-gnu
/tmp/undergrid-toolchain/go/bin/go test -json -race -tags=hp1_live,hp2_live ./internal/network -run '^TestHP2LiveOwnerBuilderRegistrationAndRecruitment$' -count=1
/tmp/undergrid-toolchain/go/bin/go test -json -race -tags=hp1_live,hp2_live ./internal/network -run '^Test(HP1LiveOwnerPartialRemainderNotificationReceiptDismissalAndRestart|HP2LiveOwnerPublicationAndWithdrawal)$' -count=1
```

The coupled build command is `node scripts/record-live-build.mjs` in `web` using
cached Node 24.15.0 on PATH. Source SHA256:
`8ff861a7eb29fcf3566b8286e0eb048d9412204ebf81de6448c6d7f73f9afc2e`.
Actual JavaScript artifact SHA256:
`3ef5c2443b975c975ba0cd52f6af65f1149361f77ab1127dd729b43110fab8f6`.
Both browser runs verified the exact frozen HEAD/source/artifact manifest before
launch. Existing matching package-lock/dependency cache was linked; no dependency
install or provider download occurred.

The retained domain selector was
`^Test(OwnerAgent(IssuanceRequiresPolicyForProtectedArtifactGrantScope|Creation.*)|OwnerCanReviewAndRevokeOnlyOwnCredentials|RecruitmentRechecksCurrentSourceAndRecipient|OpenAPIRealHTTPLifecycleContract)$`
with `go test -json -race ./internal/network -count=1`, database URL targeting the
genuine empty-template copy `undergrid_builder_retained_20261026` on 55500.

The actual new browser uses three normal signed OIDC/session/PKCE owner contexts.
C selects and reviews the profile, registers through console/CSRF, reloads and
reads the saved acknowledgement. A's normally issued, explicitly scoped agent
creates a restricted room and catalog role through the normal API. This is
**fixture API support**, not proof of the still-missing owner room/origin/role
controls. The actual worker runs with a distinct restricted worker LOGIN and
emits C's current attributable recruitment opportunity. Invitation-off rejects
A's unsolicited offer (404); the opportunity grants no room context (404).
Owner lists contain only their own agent. No product SQL seeds complete the
browser steps; role/schema/template setup is infrastructure only.

Read-only SQL independently confirms three principals/agents/credentials, two
private watches, C's exact saved profile, one opportunity/event/inbox triple and
completed durable recruitment jobs. Exactly one room and its accepted creator
membership exist; there is no foreign membership, plan, task or approval. New
run DB: `hp1_run_84e4d715a8aa419abb3b3812c3e4bcc8`; browser artifacts:
`/tmp/undergrid-hp2-browser-3708585751`. Retained regressions used separate DBs
`hp1_run_0887fde434c54d268807a52f4ebff1b3` (HP1; browser artifacts
`/tmp/undergrid-hp1-browser-3382250369`) and
`hp1_run_45cffd03496843ffa152492dc34de55d` (publication; artifacts
`/tmp/undergrid-hp2-browser-3408927404`). No database or accepted work was reset.

The focused domain negatives separately prove unlisted/recruitment-off/mismatched
capability exclusion and invitation eligibility independent of recruitment. They
use normal owner commands and restricted API/worker LOGINS. Explicit legacy `{}`
compatibility data is a labeled domain fixture, not browser journey completion.
Current authority/replay guards remain in the retained domain suite.

## Quality and remaining scope

DRY: the existing transaction owns registration, scopes, policy, issuance and
recruitment eligibility. Generated request/response wire types describe their
different contracts; runtime response validation protects the external boundary.
No second authority engine or duplicated policy state was added. Profile review,
saved acknowledgement and credential recovery remain separate responsibilities.
Busy draft freezing and generation checks make uncertain/stale replies recoverable;
no new browser credential persistence or generic retry abstraction was introduced.

The existing owner-list query owns the normalized four-field projection; it
preserves owner/RLS scope and stored legacy data. Its long existing JSON SQL style
is retained to keep this additive change bounded. Independent normalized SQL,
real restricted roles and browser/API commands prove effects beyond UI mocks.
Read-only independent review found no material code blocker at `7728350`; final
review includes this evidence and the offered-state oracle before merge.

Existing profile mutation/revocation and a public profile directory remain
unsupported. Complementary capability discovery, semantic/groups and visible
unknown-hard results, reviewed room origin/roles, owner plan creation/revision,
application review/offer and the continuous HP2 journey remain open. Controlled
accounts do not prove independent beneficial ownership, externally operated
participants or provider relevance. Legal/source/provider/deployment gates and
other happy paths remain separate; this feature performs no deployment, payment
or arbitrary agent-content execution.
