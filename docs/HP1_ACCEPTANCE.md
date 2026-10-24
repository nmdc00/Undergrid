# HP1 live acceptance ledger

This ledger tracks the five steps in `HAPPY_PATH_EXECUTION.md`. Implementation,
local proof, release integration and external qualification are separate facts.
The current branch starts from release `852c9c3`. No complete HP1 journey has run.

| Step and exact acceptance criterion | Implemented | Local continuous proof | Merged | External qualification |
| --- | --- | --- | --- | --- |
| 1. Previously unregistered owner signs in, registers, receives one scoped agent credential and creates an explicitly private structured intention with active watch and visible expiry. | Existing product routes/UI; live harness implemented. | PASS: real Chromium/OIDC/API/PostgreSQL setup, bearer read and reload on 23 October; exact evidence below. No mocked owner/API responses. | Product behavior in release; harness pending. | Actual chosen identity provider and unfamiliar-owner usability not qualified by the controlled issuer. |
| 2. Owner inspects permitted lead provenance and declared fit, or an explicit honest gap without false independence/provider claims. | Lead browsing, declared constraint report and empty-result message exist. | PASS: honest gap with no configured source in the same setup journey. Controlled populated-source/declared-fit proof still pending. | Existing product behavior in release; continuous harness pending. | Real approved capability/request material and useful first-owner outcome remain unproven. |
| 3. Owner declares only part of the need satisfied and sees the exact unresolved requirements preserved under an active watch. | Missing partial-satisfaction command, persistence and owner controls at this branch baseline. Terminal `satisfied` stops monitoring and does not fulfill this step. | Pending implementation and behavioral red/green proof. | Pending. | Local owner declaration cannot qualify publisher claims, legal rights or real usefulness. |
| 4. A later permitted source update produces one durable opportunity and one notification; owner returns to inbox. | Source-refresh/notification product pipeline exists; owner inbox feature work ongoing. | Pending one continuous browser/worker journey. Existing focused fixtures are not substituted for this proof. | Pipeline merged; owner inbox integration pending. | Controlled HTTPS publisher is explicitly a local fixture, not source approval or relevance qualification. |
| 5. Owner reviews result, acknowledges receipt and dismisses; reconnect and worker restart do not renotify unchanged evidence, but genuinely changed evidence may generate a new suggestion. | Owner inbox/dismissal feature work ongoing. | Pending browser commands plus independently observed persistence/restart effects. | Pending feature integration and acceptance proof. | No public qualification inferred from local success. |

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

## Source/notification/receipt extension — authored, execution pending

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
