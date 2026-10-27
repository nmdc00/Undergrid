# Owner plan proposal and immutable revision

This HP2 step 4 feature is reviewed and merged in [PR110](https://github.com/nmdc00/Undergrid/pull/110)
at release `2099a763efb4323562a1b1a2f39544fe191f43a9`. Final feature head
`bebf1bc` contains only documentation changes after tested runtime/frontend
`9040113`. That runtime already integrates the reviewed policy-draft prerequisite
PR109/release `862063d`. The owner plan lifecycle browser, retained domain/static/contract gates
and focused post-merge checks pass. This closes the bounded owner-plan feature,
not the complete HP2 journey. The original task contract is in `HP2_EXECUTION.md`,
“Locked task: owner plan proposal and revision”.

## Implemented behavior

An authenticated owner selects a currently owned active agent, loads the bounded
accepted-member roster and latest plan context, and reviews a structured plan.
The editor supports multiple tasks, a bounded dependency DAG, actual accepted
assignees, required/acceptance/publication principal sets, response/task deadlines,
and enabled contribution-template/license choices. Submitting reaches
`POST /v1/owner/agents/{agent}/swarms/{room}/plans`, which delegates to the same
`swarmCommand`/`proposePlan` domain transaction used by agent commands. The context
read is `GET /v1/owner/agents/{agent}/swarms/{room}/plan-context` and returns no
foreign private profiles or credentials.

Each submission creates a sealed immutable proposed version and offered tasks;
it does not approve terms, accept assignments or replace the accepted head.
Revisions name the latest proposal parent. The previous accepted plan and its
commitments remain until the new plan has its own exact-hash plan/rights votes
and separate assignment acceptances. Account/principal attribution does not prove
independent beneficial ownership.

Current contributor and room authority are checked before retained idempotent
response recovery. A committed same-key/body/original-ETag receipt remains
recoverable after its own room/head advance; a new proposal revalidates selected
participants. Migration 044 supplies the narrow room-scoped participant-activity
lock check under restricted application privileges, including an active eligible
membership witness for each selected principal. It is frozen after normal apply:
`c87465968c82555c7aee6bbbe022282a7e92976f3d904a920b2e8615a296117f`.

The console freezes captured inputs during submission, preserves exact retry
payload/key/parent/ETag after uncertain outcomes, rejects stale actor/context
responses and verifies returned sealed contracts. Unchanged timestamps preserve
all original fractional digits; equivalent offsets compare as exact instants,
without accepting a changed submillisecond deadline.

## Evidence and preserved failures

Earlier behavioral failures cover the missing owner context/POST/editor,
contributor-to-observer retained replay, selected inactive identities, nullable
published plan fields, pending draft snapshots and timestamp precision. The
focused restricted-PostgreSQL proposal gate passed six top-level cases and six
subcases in 11.454 seconds. These focused results are not the full journey.

On frozen `26c7a4d`, the full console suite passed 157 tests in 18 files in
61.08 seconds, and TypeScript/Vite plus the coupled provenance manifest passed
in 1.35 seconds. Logs include
`/tmp/undergrid-owner-plan-26c7a4d-full-ui-20261026.log`.
The manifest records source SHA-256
`9df5662362c1b50933a25805672a651183925e9439b0d1d6210ff7e728e0235c`
and built JavaScript SHA-256
`04a0d96f4f98fa9db9f8160f5efd7e1289ebd5a0e87a56391b71b3b981956022`.

The first actual `TestHP2LiveOwnerPlanProposalRevisionAndRecovery` run failed
in 96.172 seconds on 26 October because the filesystem filled. Chromium reported
`ENOSPC: no space left on device, write`; the trace ZIP is truncated. Preserve
that failed aggregate rather than count it as product success:

- Log: `/tmp/undergrid-owner-plan-26c7a4d-live-browser-20261026.jsonl`.
- Genuine run database: `hp1_run_c1da6376cb374685be071b266542e944` on the isolated
  network PostgreSQL fixture at port 55502.
- Artifacts: `/tmp/undergrid-hp2-browser-261375922`.

The authored browser scenario uses normal signed owner login, policy/profile
setup, explicitly labeled API room/invitation setup and owner admission controls.
It then proposes a two-task dependent plan, records three separate exact-hash
votes and two assignment acceptances, revises the plan, checks the old accepted
head after every partial decision, and checks actual worker restart/reconnect.
Read-only SQL checkpoints independently inspect normalized persistence. No SQL
plan, approval or assignment seed completes the browser steps. At that point the authored
scenario remained unproven pending an uninterrupted run with adequate disk space.

The unchanged rerun on 27 October had adequate disk space after moving the
regenerable Go cache and compiler temporary files to shared memory. It failed in
110.481 seconds at the first owner's policy setup, preserving the normal
90-second browser bound. The completed intention POST was followed by a workspace
refresh; the scenario edited policy before that action finished, and the refreshed
record replaced those edits. Native required-field validation then prevented a
policy POST. This also exposed a real existing policy-editor defect: inputs were
editable during the pending refresh although Save was disabled. Its bounded
test-first repair is a separate prerequisite feature, not a silent test-only
workaround. The plan scenario now waits for the completed action and independently
checks persisted policy before proceeding. The separate policy repair is reviewed
and merged in PR109; the integrated corrected browser passes as recorded below.

- Rerun log: `/tmp/undergrid-owner-plan-26c7a4d-live-browser-20261027.jsonl`.
- Retained database: `hp1_run_4b062274e2794dd1885bf6918f5bcec6` on port 55502.
- Retained artifacts: `/tmp/undergrid-hp2-browser-474990043`.

### Current passing lifecycle proof

On frozen runtime/frontend `9040113`, the actual plan browser passed in 24.8
seconds (Playwright aggregate 26.3 seconds; Go race package 29.067 seconds).
It continuously performs both plan activations, independent decisions, per-phase
accepted-head preservation and actual worker restart/reconnect described above.
Final read-only SQL independently verifies 12 exact-hash approval records,
two cancelled prior tasks, two accepted replacement tasks, two proposal events
and the immutable parent lineage. Source/build provenance was checked before
the run, with no source or HEAD mutation during it.

- Log: `/tmp/undergrid-owner-plan-9040113-live-browser-oct27.jsonl`.
- Genuine isolated database: `hp1_run_a608a88393b243eb8d6c8524a27b0598`, port 55502.
- Artifacts: `/tmp/undergrid-hp2-browser-3441837557`.
- Coupled TypeScript/Vite/manifest passed, Vite 1.37 seconds:
  `/tmp/undergrid-owner-plan-9040113-build-oct27.log`.
- Source SHA-256: `cee2140e986e18731b5afb3d98a605e6c4aae4b3de0e2f92a233f8aa2f71f01c`.
- JavaScript SHA-256: `c631b0016566cf03bb85eef8a967f5f020812cf7e30c8528fa2f16649110fe1b`.
- Focused proposal/approval UI: 23 tests passed in 10.99 seconds,
  `/tmp/undergrid-owner-plan-9040113-focused-ui-oct27.log`.
- Integrated policy UI: three tests passed in 1.86 seconds,
  `/tmp/undergrid-owner-plan-policy-integration-web-oct27.log`.

The new restricted-HTTP lifecycle characterization
`TestOwnerPlanUnacceptedRevisionPreservesAcceptedCommitments` independently
creates and accepts an initial owner plan, proposes a replacement and rejects or
expires it. It compares the entire normalized prior plan and task rows before
and after each transition and a denied late vote. Repeated rejection recovery
and repeated sweeps leave one terminal event; the replacement inherits no approval.
The expiry uses the production sweep clock argument without rewriting a stored
deadline, through a separate restricted worker login. The API login retains only
application authority.

`TestOwnerPlanProposalActualHTTPMatchesPublishedContract` validates three actual
owner context/proposal HTTP samples against the canonical schemas, plus independent
forged-principal, string-version and foreign-profile/credential negative oracles.
Together these two top-level tests and two lifecycle subcases passed 5.033 seconds
in `/tmp/undergrid-owner-plan-lifecycle-contract-worker-oct27.jsonl`, using genuine
database `undergrid_plan_lifecycle_oct27`. Preserve the first failed aggregate
`/tmp/undergrid-owner-plan-lifecycle-contract-first-oct27.jsonl`: contract and
rejection passed, while the expiry fixture incorrectly attempted a worker sweep
through the intentionally app-only service. The correction changes only the test's
worker identity, not production grants or API privileges.

## Final retained gates and review

On clean `4390c8d` (only four documentation files differ from tested runtime
`9040113`), the combined retained race gate passed 21 top-level tests and
16 subcases in 57.479 seconds. It covers owner proposal/context/replay, selected
participant activity and contention, exact approval/rights separation, assignment
authority/decline/expiry, canonical-lock waits, rejected/expired revisions and
transitive dependency preservation. Genuine database `undergrid_plan_retained_oct27`
was copied from the preserved current network template, then migrated normally
through frozen 044. No product rows or migration ledger were rewritten.

Exact selector:

```sh
go test -race ./internal/network -run '^Test(OwnerPlan.*|PlanProposal.*|OwnerAssignment.*|ExactPlanApprovalAndIndependentAssignmentAcceptance|PlanDependencyContract|Withdrawal(DurablyBlocksTransitiveDependencies|DependencyEvidenceRecoveryAndReplacement|SerializesWithDownstreamProgress|DependencyBlockingPreservesVerifiedEvidence))$' -count=1 -timeout=5m -json
```

Log: `/tmp/undergrid-owner-plan-4390c8d-retained-domain-oct27.jsonl`.
`npm run contract:check` passed Redocly lint, compilation of 160 canonical
JSON Schema components and generated TypeScript parity; log
`/tmp/undergrid-owner-plan-4390c8d-contract-oct27.log`. All-package
`go vet -tags=hp1_live,hp2_live ./...` and `go build ./...` both exited zero;
logs `/tmp/undergrid-owner-plan-4390c8d-vet-oct27.log` and
`/tmp/undergrid-owner-plan-4390c8d-go-build-oct27.log`.

The current gates used the installed Go toolchain with
`GOCACHE=/tmp/undergrid-gocache`, `GOMODCACHE=/tmp/undergrid-gomod` and
`GOTMPDIR=/dev/shm/undergrid-gotmp`; the cache path is a symlink to shared memory.
Browser runs additionally used the existing Chromium/browser-library fixture and
`UNDERGRID_HP1_ADMIN_DATABASE_URL` for the separate port-55502 network cluster.
No financial database or production identity is involved. A SHA-256/size index
of 12 retained success/failure logs is
`/tmp/undergrid-owner-plan-validation-oct27.json`.

Final independent review approved `bebf1bc`; PR110 is merged to `release/v0.1`.
Documentation-only updates after `4390c8d` do not change the tested runtime.
No deployment or protected-branch promotion is implied by this feature proof.

The feature does not complete room-origin/recruitment controls, missing group
viewer/discovery/editor steps or the continuous HP2 journey. Test-only enabled
terms do not qualify legal terms, a real provider, independent participants or
production deployment. Those explicit gates remain open.

## Post-merge release checks — 27 October 2026

Root validated exact release `2099a763efb4323562a1b1a2f39544fe191f43a9`:

- Proposal/policy UI: 15 tests in two files passed in 10.57 seconds,
  `/tmp/undergrid-plan-release-2099a76-ui-oct27.log`.
- Coupled TypeScript/Vite/build manifest passed; Vite 1.53 seconds,
  `/tmp/undergrid-plan-release-2099a76-build-oct27.log`. Source hash `cee2140e986e18731b5afb3d98a605e6c4aae4b3de0e2f92a233f8aa2f71f01c`
  and JavaScript hash `c631b0016566cf03bb85eef8a967f5f020812cf7e30c8528fa2f16649110fe1b`
  match the previously proven browser runtime.
- Restricted PostgreSQL actual owner proposal/context contract and rejected/expired
  revision lifecycle: two top-level tests and two subcases passed in 4.064 seconds,
  `/tmp/undergrid-plan-release-2099a76-domain-oct27.jsonl`.
  Genuine database `plan_release_2099a76_oct27` applied migration 044 normally.

These integration checks do not substitute for or relabel the complete passing
feature browser. Its retained artifacts and failures remain above. No protected
branch promotion, deployment or external qualification is implied by this merge.
