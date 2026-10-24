# HP1 live acceptance ledger

This ledger tracks the five steps in `HAPPY_PATH_EXECUTION.md`. Implementation,
local proof, release integration and external qualification are separate facts.
The current branch starts from release `852c9c3`. No complete HP1 journey has run.

| Step and exact acceptance criterion | Implemented | Local continuous proof | Merged | External qualification |
| --- | --- | --- | --- | --- |
| 1. Previously unregistered owner signs in, registers, receives one scoped agent credential and creates an explicitly private structured intention with active watch and visible expiry. | Existing product routes/UI; live harness implemented. | PASS: real Chromium/OIDC/API/PostgreSQL setup, bearer read and reload on 23 October; exact evidence below. No mocked owner/API responses. | Product behavior in release; harness pending. | Actual chosen identity provider and unfamiliar-owner usability not qualified by the controlled issuer. |
| 2. Owner inspects permitted lead provenance and declared fit, or an explicit honest gap without false independence/provider claims. | Lead browsing, declared constraint report and empty-result message exist. | PASS: honest gap with no configured source in the same setup journey. Controlled populated-source/declared-fit proof still pending. | Existing product behavior in release; continuous harness pending. | Real approved capability/request material and useful first-owner outcome remain unproven. |
| 3. Owner declares only part of the need satisfied and sees the exact unresolved requirements preserved under an active watch. | Missing partial-satisfaction command, persistence and owner controls at this branch baseline. Terminal `satisfied` stops monitoring and does not fulfill this step. | Pending implementation and behavioral red/green proof. | Pending. | Local owner declaration cannot qualify publisher claims, legal rights or real usefulness. |
| 4. A later permitted source update produces one durable opportunity and one notification; owner returns to inbox. | Source-refresh/notification and owner inbox product paths exist. | PASS: continuous controlled-source browser/worker milestone at `fef4e96`; actual publisher GETs and normalized notification effects recorded below. | Pipeline and owner inbox merged; this acceptance milestone pending review/integration. | Controlled HTTPS publisher is explicitly a local fixture, not source approval or relevance qualification. |
| 5. Owner reviews result, acknowledges receipt and dismisses; reconnect and worker restart do not renotify unchanged evidence, but genuinely changed evidence may generate a new suggestion. | Owner inbox/dismissal product paths exist. | PASS: browser receipt/dismissal/reconnect, actual worker restart and unchanged/changed-source effects at `fef4e96`; exact discovery receipt timestamp stable. | Inbox/dismissal merged; this acceptance milestone pending review/integration. | No public qualification inferred from local success. |

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
an independent final-state oracle. The first milestone currently asserts setup,
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
passing first milestone does not close the full HP1 path: partial satisfaction,
populated-source discovery, worker notifications and receipt/dismissal/restart
remain unproven in a continuous journey.

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
is available in ordinary retained network regression databases. Full-path proof
and integration remain pending the missing steps in the ledger.

## Source/notification/receipt extension — locally proven, integration pending

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

HP1 is **not locally closed**. Step three still lacks the owner partial-satisfaction
and visible unresolved-requirement action in this milestone. The final continuous
journey must integrate that real action, validate the remaining requirement pins
and account separately for opportunities generated by the changed intention
revision. Controlled local issuer/source/scheduling fixtures do not qualify actual
identity providers, sources, legal permissions, public deployment or the real
unfamiliar-owner useful-result gate.
