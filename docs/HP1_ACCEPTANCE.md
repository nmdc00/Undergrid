# HP1 live acceptance ledger

This ledger tracks the five steps in `HAPPY_PATH_EXECUTION.md`. Implementation,
local proof, release integration and external qualification are separate facts.
The complete local journey passed on 24 October at tested candidate
`037410e2ade0099099ba418386e816ac1bc3ded1`, integrating release `6b7fa91`
(PR96). All five steps are implemented and continuously proven. The new acceptance
changes remain pending reviewed PR merge; HP1 is not yet declared locally closed.
External qualification remains unproven.

| Step and exact acceptance criterion | Implemented | Local continuous proof | Merged | External qualification |
| --- | --- | --- | --- | --- |
| 1. Previously unregistered owner signs in, registers, receives one scoped agent credential and creates an explicitly private structured intention with active watch and visible expiry. | Product routes/UI and real live harness implemented. | PASS: same actual Chromium/OIDC/API/PostgreSQL journey; scoped bearer read and reload. | Product and initial harness merged (PR94); complete-path extension pending. | Controlled issuer does not qualify the chosen provider or unfamiliar-owner usability. |
| 2. Owner inspects permitted lead provenance and declared fit, or an explicit honest gap without false independence/provider claims. | Lead browsing, declared constraint report and honest gap implemented. | PASS: initial empty gap, controlled populated capability, loaded revision-one mismatch and later revision-two alignment. | Product merged; complete-path proof pending acceptance PR merge. | Real approved capability/request material and useful unfamiliar-owner outcome remain unproven. |
| 3. Owner declares only part of the need satisfied and sees the exact unresolved requirements preserved under an active watch. | Explicit owner partial-satisfaction command, immutable audit/revision and visible remainder merged in PR96. | PASS: unchecked fields → owner selects `source_id` → revision two retains `source_url`; constraints/watch/expiry preserved, reload and scoped bearer read confirm persistence. | Product merged (PR96); complete-path proof pending acceptance PR merge. | Owner declaration does not qualify publisher claims, legal rights or real usefulness. |
| 4. A later permitted source update produces one durable opportunity and one notification; owner returns to inbox. | Source-refresh/notification and owner inbox implemented. | PASS: actual HTTPS GET and workers create exactly one opportunity/event/inbox for each source/intention input pair; version-two result uses revision-two remainder. | Product and initial harness merged; full extension pending acceptance PR merge. | Controlled publisher is a local fixture, not source approval or relevance qualification. |
| 5. Owner reviews result, acknowledges receipt and dismisses; reconnect and worker restart do not renotify unchanged evidence, but genuinely changed evidence may generate a new suggestion. | Owner inbox/dismissal implemented. | PASS: exact revision-two/version-two discovery ack/dismiss, actual stop/restart and completed downstream matching preserve counts/receipt timestamp; version three yields a different suggestion. | Product and initial harness merged; full extension pending acceptance PR merge. | No public qualification follows from local success. |

## Harness boundaries

`internal/network/hp1_live_journey_test.go` starts an actual HTTPS console/API,
production session/login middleware, separately restricted database LOGIN and a
controlled signed OIDC issuer. The issuer binds one-use codes to redirect, S256
verifier and nonce. Chromium follows real redirects; the product validates token
signatures, encrypted cookies and CSRF. Browser certificate verification is relaxed
only for the temporary local TLS fixture. No browser API/auth route is intercepted.

`web/e2e-live/hp1.spec.ts` drives the built console. Product principal, agent,
credential and intention rows must originate from browser/API commands. Setup may
create schema and restricted database roles in a dedicated empty network database;
it must never insert product rows to complete the journey. Read-only SQL provides
an independent final-state oracle. The first milestone asserted setup,
honest empty-source gap, bearer credential reads and browser reload persistence.

The repeatable environment is `UNDERGRID_HP1_ADMIN_DATABASE_URL` naming the
isolated network PostgreSQL administrative database. The harness normally migrates
an empty `undergrid_hp1_template`, verifies it has no product owners, closes its
connections and creates one genuine PostgreSQL template copy per attempt. It
retains successful and failed run databases as evidence. This schema/role/database
setup does not create product users or complete a product step. Alternatively,
`UNDERGRID_HP1_DATABASE_URL` may name an explicitly prepared empty per-run database.
Existing Node/Chromium caches and built `web/dist` assets are also required.
The opt-in `hp1_live` Go build tag keeps the empty-population/browser acceptance
environment separate from ordinary network regression databases. Execute its
selector explicitly; excluding it from ordinary tests is not evidence of a pass.
Go source, commands, actual output/artifact paths and exact results follow. The
initial setup milestone alone did not prove the full path. The later complete
journey and current integrated guards are recorded at the end of this ledger.

Later source changes will be controlled at the publisher boundary, ingested through
the actual source refresher and durable workers. No manual SQL source observations,
notification rows, receipt or dismissal records may stand in for product actions.
The final proof must include current actor/cursor/revision/source freshness,
private-text non-disclosure, stale/withdrawn lead denials and receipt/dismissal not
granting tasks, rights or spending. Source/provider/legal/deployment approvals stay
explicit external gates.

## Actual first-milestone evidence — 23 October 2026

The first attempt could not start cached Chromium because its loader could not
find five system libraries. No product action ran; this is fixture failure, not
behavioral RED (`/tmp/undergrid-hp1-owner-setup-live55500.jsonl`). Existing local
libraries at `/tmp/undergrid-browser-libs/root/usr/lib/x86_64-linux-gnu` resolved
all missing `ldd` entries; no package/browser download or global mutation occurred.
The second attempt signed in, registered one owner and issued one agent credential,
then failed an overstrict Playwright label selector. The accessibility snapshot
showed the private visibility combobox correctly; using its actual role/name fixed
the selector. No product implementation changed. Its log is
`/tmp/undergrid-hp1-owner-setup-live-attempt2-55500.jsonl`.

The third attempt passed: one Chromium test in 7.5 seconds (runner 10.9 seconds),
one Go/PostgreSQL race test in 12.62 seconds (package 13.642 seconds), zero failed
or skipped tests. It followed real browser OIDC redirects, owner registration,
scoped agent issuance, private structured watch creation, explicit empty-source
gap, public bearer credential read and reload. Independent SQL observed exactly
one principal, credential and intention, with private visibility and an active
watch. No worker was needed for this initial empty-source milestone, and no
ongoing delivery or source usefulness is inferred.

- Source: product release `852c9c3` plus the committed acceptance harness; subsequent proof-document edits do not alter runtime source.
- Build: `npm run build`, log `/tmp/undergrid-hp1-setup-build.log`, PASS.
- Runtime command: `go test -tags=hp1_live -race -json -count=1 -timeout=3m ./internal/network -run '^TestHP1LiveOwnerSetupAndHonestGap$'` with the isolated 55500 admin URL, cached Chromium and `LD_LIBRARY_PATH` above.
- Runtime log: `/tmp/undergrid-hp1-owner-setup-live-attempt3-55500.jsonl`.
- Retained successful database: `hp1_run_fa4a398fabfb4413a88a7f92f0c7e058` on isolated network port 55500.
- Retained browser artifacts: `/tmp/undergrid-hp1-browser-20834695`; failures and their traces/screenshots remain separately retained at `/tmp/undergrid-hp1-browser-251832591` and `/tmp/undergrid-hp1-browser-2147630659`.
- Static check: `go vet -tags=hp1_live ./internal/network`, log `/tmp/undergrid-hp1-owner-setup-live-vet.log`, PASS.

The two failed attempts' databases are retained, not reset or deleted. Genuine
PostgreSQL template copies made the successful retry start with no product owners.
No mock HTTP owner/API response, manually seeded product row, session bypass or
administrative action completed an owner step. The controlled OIDC issuer and
empty-source gap do not qualify any real identity provider, source, license,
publisher claim, deployment or unfamiliar-owner useful result within ten minutes.

Quality assessment: production identity/session/domain code owns authorization;
the browser owns actions, PostgreSQL supplies an independent final-state oracle,
and fixture schema/role/database setup remains outside the product journey.
Cached external dependencies are explicit and resources close on failure. The
separate tagged gate avoids pretending the empty-population browser environment
is available in ordinary retained network regression databases. At this initial
milestone, full-path proof and integration were still pending.

## Source/notification/receipt extension — historical PR94 milestone

The branch now retains the original empty-source milestone as a separate test and
adds `TestHP1LiveOwnerSourceNotificationReceiptDismissalAndRestart`. The extension
starts from its own empty PostgreSQL copy and drives the same browser setup,
controlled publisher ingestion and declared-fit review, owner inbox receipt,
dismissal, reconnect and a genuinely changed source version. Partial satisfaction
is explicitly still missing; omitting that action does not close HP1.

The public `RunWorker` keeps normal source-file configuration and delegates its
unchanged worker body to a private concrete-source helper. This lets the fixture
route only external HTTPS transport to its controlled publisher; ordinary durable
source coordinators, source approval checks, notification workers and domain
transactions still execute. Before extraction, three actual source-fairness and
shutdown tests passed in 9.708 seconds on retained network 96
(`/tmp/undergrid-hp1-worker-before-extraction96.jsonl`). The reconciled original
live milestone passed package 14.108 seconds
(`/tmp/undergrid-hp1-owner-setup-reconciled55500.jsonl`). These characterize the
previous source, not the unrun extension.

The restart controller cancels and awaits worker termination before creating a
new worker on the same database. It waits for that new worker to complete the
unchanged source refresh and the snapshot-bound external matching descendants,
with no relevant pending/running or failed/cancelled descendants, then checks exactly one opportunity/event/discovery-inbox
row before allowing publisher version two, and checks the exact dismissed
opportunity's acknowledged event timestamp. It stops and awaits the final worker
before independent final SQL counts. Receipt assertions exclude `intent.created`;
there must be exactly one acknowledged `opportunity.available` event. No product
row is inserted, deleted, rescheduled or reset by the controller.

Build provenance is recorded by `node scripts/record-live-build.mjs`, which
executes the actual `npm run build` with production `NODE_ENV` before recording. Its manifest binds the repository head, all `src` and
`public` files, HTML, package/lock/compiler/Vite configuration, Vite environment
files and the recorder itself, plus SHA-256 for every built artifact. The live
harness checks that manifest before opening Chromium and logs the verified
hashes. Run this coupled build/recorder after each frontend reconciliation. The current
Vite configuration has no local imports outside these inputs. This check is not
proof that arbitrary future build configurations have no other inputs.

The first frozen extension gate at `85bb84d` passed the original milestone but
failed the new watch scenario before its first opportunity. Read-only inspection
of its retained database showed one ingested version-one observation, but its
title omitted two words present in the intention's full-text AND query. The
fixture query/title mismatch explained the observed zero matches. Review also
identified a separate scheduling gap that would prevent later replay/update
fetches: `RefreshSource` is an ordinary guarded refresh, not a forced fetch, and
its nil result may mean no HTTP request occurred. Both are fixture defects, not
evidence of a product defect. Actual output is retained in
`/tmp/undergrid-hp1-watch-live-85bb84d-55500.jsonl` (package 46.691 seconds), with
failed watch database `hp1_run_337b6d6cf413444ab65e1a1f058349c3` and browser artifacts
`/tmp/undergrid-hp1-browser-4005800177`.

The corrected fixture accelerates only its concrete refresher's successful refresh
interval to 100 milliseconds. The production default remains six hours, including
zero-value constructors; a real PostgreSQL lifecycle assertion checks exactly
21,600 seconds between success and next refresh. Due checks, approvals, snapshot
identity and persisted timestamps still use the actual database clock. Publisher
changes wait for normal due eligibility, with no manual SQL scheduling mutation.
Restart waits for due eligibility and lets the new worker's actual startup source
coordinator perform the unchanged fetch and downstream matching. This explicitly
simulates accelerated scheduling; it does not qualify a real six-hour operational
watch or a real source. The corrected extension passed as recorded below.

## Corrected continuous watch milestone — actual PASS

Exact runtime source `fef4e96` passed both tagged live tests on 24 October 2026:
original setup/gap 10.34 seconds, continuous source/watch/receipt/dismissal/restart
17.26 seconds, package 28.627 seconds, zero failures or skips. The controlled
publisher observed three actual GETs. The first suggestion produced exactly one
opportunity, `opportunity.available` event and discovery inbox row. The owner
acknowledged that precise event and dismissed the suggestion. After cancellation
and awaited worker termination, a new worker fetched unchanged source evidence
and completed its snapshot-bound matching descendants before the controller
verified the same one/one/one counts and unchanged receipt timestamp. Version two
then produced a different suggestion. The final worker terminated before SQL
confirmed exactly two opportunities/events/discovery inbox rows, one dismissal,
one acknowledged discovery event and zero room/task/approval effects. No financial
runtime, credential or database participated.

- Default production schedule/source/worker characterization: four actual race tests, package 9.302 seconds, zero failures/skips; `/tmp/undergrid-hp1-source-default-interval-fef4e9696.jsonl`. The SQL assertion observes the normal six-hour interval exactly.
- Coupled frontend build/provenance: `/tmp/undergrid-hp1-watch-fef4e96-build-manifest.log`, PASS, with exact head/source/artifact SHA-256 values. The manifest check ran inside both live tests before Chromium.
- Live runtime: `/tmp/undergrid-hp1-watch-live-fef4e96-55500.jsonl`, PASS.
- Retained successful watch database: `hp1_run_40ee30266d6f4cf8a835838532f6ca81`, isolated network port 55500.
- Browser artifacts: `/tmp/undergrid-hp1-browser-1838448042`.
- Static validation: `go vet -tags=hp1_live ./internal/network`, PASS; `/tmp/undergrid-hp1-watch-fef4e96-vet.log`.

The branch subsequently merged release `7fdae92` documentation. Its diff from the
tested runtime changes only documentation, so the bounded runtime evidence above
remains exact for the implementation. A future live gate must rebuild its manifest
to bind the new repository head. Build provenance binds declared inputs and actual
artifacts, not a reproducible attestation of all installed dependency bytes.

This PR94 milestone did not close HP1: it omitted partial satisfaction and the
visible unresolved-requirement action. The later complete journey below integrates
that real action, validates remaining requirements and accounts separately for
opportunities generated by the changed intention revision. Controlled local issuer/source/scheduling fixtures do not qualify actual
identity providers, sources, legal permissions, public deployment or the real
unfamiliar-owner useful-result gate.

## Full partial-remainder extension — scenario and current result

The separate `test/v0.1-hp1-partial-live` branch extends the proven milestone with
`TestHP1LiveOwnerPartialRemainderNotificationReceiptDismissalAndRestart`.
The reviewed partial-satisfaction product feature was integrated through release
`6b7fa91` before testing. The exact candidate and actual complete-path result are
recorded below; the acceptance changes still await reviewed PR integration.

The owner console currently creates two required fields, `source_id` and
`source_url`. This journey uses that real browser control, with no hidden
intention revision or SQL product seed. The owner explicitly declares only
`source_id` fulfilled and sees `source_url` retained under the same private active
watch, output, licenses, preferences, budget and expiry. Reload and the normally
issued scoped bearer read must show that same persisted remainder at revision two.
The initial loaded fit report must show a declared mismatch; the later controlled
capability declares the remaining field and must show declared alignment, with
identity and rights still unverified.

A changed intention revision is genuinely new matching input. The SQL oracle
therefore separately requires exactly one opportunity/event/inbox for each pair:
revision one/source version one; revision two/source version one; revision
two/source version two; revision two/source version three. The browser settles
each current result before the next change. It acknowledges and dismisses the
revision-two/version-two result, then requires actual worker stop, due source
refresh, a new worker, completed revision-two snapshot matching, unchanged
normalized counts and the precise discovery receipt timestamp before publishing
version three. The final worker stops before independent SQL checks. Exactly one
partial-satisfaction event, one dismissal and one acknowledged discovery event
are expected; rooms, tasks and approvals remain absent.

This remains controlled local OIDC/HTTPS/scheduling acceptance. Actual provider,
permitted real capability/request source, legal and unfamiliar-owner qualification
remain external gates. The passing proof does not close HP1 before its reviewed
acceptance PR is merged.


## Complete local journey and current guards — 24 October 2026

Exact tested code: `037410e2ade0099099ba418386e816ac1bc3ded1`. The initial
coupled-build attempt failed before product execution because this new worktree
had no `node_modules` (`/tmp/undergrid-hp1-full-037410e-build.log`). Linking the
existing unchanged local dependency cache resolved that environment failure;
no dependency installation or download occurred.

The fresh coupled frontend build and source/artifact manifest passed:
`/tmp/undergrid-hp1-full-037410e-build-attempt2.log`. Source SHA-256 is
`69acf04b7ff9faffb522c235882dad4c621d1d5b263b3838532acf341ac8abff`;
served JavaScript SHA-256 is
`d7501ff1ee666231dc52b63a5d7d73694615e4e2135f36a605204f5b41051985`.
Each live test independently verified the manifest before opening Chromium.

- Live command: `go test -tags=hp1_live -race -json -count=1 -timeout=5m ./internal/network -run '^TestHP1LiveOwner(SetupAndHonestGap|SourceNotificationReceiptDismissalAndRestart|PartialRemainderNotificationReceiptDismissalAndRestart)$'`.
- Live log: `/tmp/undergrid-hp1-full-037410e-55500.jsonl`; **PASS 74.847 seconds, three top-level tests, zero failures/skips**.
- Complete partial-remainder case: **PASS 22.58 seconds**; actual Chromium scenario 19.5 seconds and four actual controlled publisher GETs. The final SQL oracle confirmed exactly the four revision/source pairs described above, each with one opportunity/event/inbox; one partial event, one dismissal, one specific discovery acknowledgement and no room/task/approval effects.
- Complete-case retained database: `hp1_run_0246939ccca14ec99d91b2cb368f781a` on isolated network PostgreSQL port 55500. Browser artifacts: `/tmp/undergrid-hp1-browser-3670241865`.
- Static command: `go vet -tags=hp1_live ./internal/network`; **PASS**, `/tmp/undergrid-hp1-full-037410e-vet.log`.
- Current integrated negative-guard command: `go test -race -json -count=1 -timeout=5m ./internal/network -run '^Test(ExternalLead|OwnerInbox|OpportunityDismissal|InboxReceiptAndRecovery)'` against the retained network96 external baseline with normal migration 041. Log `/tmp/undergrid-hp1-full-037410e-negative96.jsonl`; **PASS 43.410 seconds, eleven top-level tests and eleven subcases, zero failures/skips**.

The negative guard gate, rather than the happy browser journey, proves current
source projection/query-time eligibility, cursor authority/revision binding,
REST/MCP/A2A read boundaries, inbox resource reauthorization and receipt-only
semantics, dismissal atomicity, unchanged-evidence suppression and source
permission changes. Specifically it runs `TestExternalLeadReadCurrentPermissionProjection`,
`TestExternalLeadCursorAuthorityAndRevisionBinding`,
`TestExternalLeadQueryTimeEligibilityAndNoReadEffects`,
`TestExternalLeadSharedMCPAndA2ARead`,
`TestExternalLeadRejectsUnrepresentableAndOversizeRawQuery`,
`TestInboxReceiptAndRecovery`,
`TestOpportunityDismissalHTTPPersistsAndSuppressesUnchangedEvidence`,
`TestOpportunityDismissalExternalKindsAndCurrentSourcePermission`,
`TestOpportunityDismissalRecruitmentDoesNotAdmitOrAcceptRights`,
`TestOwnerInboxReceiptAndBoundCursor` and
`TestOwnerInboxRechecksPrivateResourceAndReceiptRemainsSeparate`.
Partial-specific actor/source pins, explicit proper subsets, response recovery,
concurrent source locks and rollback evidence remain documented separately in
`PARTIAL_SATISFACTION.md`; do not relabel fixture proof provider verification.

Quality assessment: browser and normally issued agent credentials create and read
product data; SQL only observes final normalized effects. The controller changes
publisher input and worker lifecycle, with a private accelerated schedule that
preserves normal due/approval checks. Snapshot/revision-specific completion
barriers prevent unprocessed restart work from masquerading as deduplication.
Per-input accounting distinguishes legitimate revision changes from duplicate
notifications. No arbitrary content execution, rights grant, financial identity
or financial database participates. This local proof remains pending reviewed
acceptance integration and does not qualify real sources/providers, legal rights,
deployment or the unfamiliar-owner useful result within ten minutes.
