# Owner intention publication

Branch: `feat/v0.1-owner-intention-publication`, from release `316fd4e`.
Reconciled with reviewed capacity release `5ea78d1`. Implementation and bounded
local lifecycle proof are complete; independent review and merge completed in
PR103 at release `b0d49c7`. HP2 and
real provider/user qualification remain open.

## Implemented, not yet release-qualified

The owner creation form offers an explicit redacted-public selection, bounded
public summary and a review of the selected summary/output/hard-requirements
projection. A shared control receives none of the private objective, budget,
preferences or server-owned audit. Safe server identifiers/version/status remain
part of actual foreign API reads.

The creation audience and fixed-audience revision review have independent state. Review is
bound to the current actor/record and selected projection; summary changes
invalidate it. Publication-related saved responses are validated inside the
existing request callback before its retry key is cleared. Late owner responses
do not refresh a session whose generation has changed. Existing domain commands
still own all authorization, versioning and durable effects.

## Contract finding: visibility changes are not ordinary revisions

The task initially assumed an existing public-to-private revision was supported.
Actual `internal/network/intents.go` rejects every revision visibility change with
`409 visibility_change_requires_grant`. The published revision evidence explicitly
preserves that guard and says consented disclosure transitions are not implemented.
No domain guard, migration, closed DTO or test has been weakened.

Root corrected the task after checking that published contract: the feature is
public creation, a reviewed public-summary revision at the same audience, then
explicit withdrawal. Revision UI now displays the fixed audience rather than
offering unsupported conversion. Withdrawal retains `redacted_public` visibility,
the revision head and immutable public projections, disables the watch and
excludes the current foreign read. It is not a private conversion or deletion.
True consented disclosure transitions remain separately unimplemented.

## Actual evidence so far

- Before behavior changes, six focused UI tests failed at the missing public
  creation option or revision control: 14.34 seconds,
  `/tmp/undergrid-owner-publication-ui-red-20261025.log`. These early failures do
  not individually prove the later retry or stale-response assertions were reached.
- First GREEN attempt passed two tests; four stopped on an ambiguous Responsible
  agent label shared with the room form. This was a test selector issue, retained
  in `/tmp/undergrid-owner-publication-ui-green-attempt1-20261025.log`.
- With that selector scoped to the intention form, six UI tests passed in 7.40
  seconds: `/tmp/undergrid-owner-publication-ui-green-attempt2-20261025.log`.
- Subsequent test-only strengthening removes old public summary from the mock's
  replacement revision and requires acknowledged/refreshed UI state. It has not
  established a visibility conversion. The earlier outgoing-PATCH assertion must not be treated as actual
  successful visibility conversion.

The six-pass provisional mock-only checkpoint above did not establish a supported
audience conversion and must not be used as that evidence. The corrected audience
UI expectation produced two failures and five passes in 19.58 seconds before the
revision control was changed:
`/tmp/undergrid-owner-publication-fixed-audience-red-20261025.log`.

The revised lifecycle also exposed three real client recovery defects before
their fixes: untouched seconds/milliseconds were truncated by the minute editor,
malformed withdrawal success cleared recovery before a detail-render failure,
and a late withdrawal reply refreshed an expired session. The nine-case run had
three failures and six passes (9.65 seconds), with the malformed response's render
error retained: `/tmp/undergrid-publication-recovery-red-20261025.log`.
A separate delayed-withdrawal test proved that closing intention detail could be
undone by the late reply (one failure, 1.99 seconds):
`/tmp/undergrid-publication-closed-intention-red-20261025.log`.

The bounded client corrections preserve the original exact expiry when its
minute-precision input was not changed, validate public withdrawal's complete
consumed result and unchanged revision head before clearing its retry key, and
ignore mutation/refresh results after the view/session epoch changes. Closing
detail does not cancel a domain command; it prevents its late reply from reopening
that view. The server still owns authority and all committed effects.

All ten focused UI cases now pass (8.83 seconds):
`/tmp/undergrid-publication-recovery-green-attempt1-20261025.log`.

The corrected opt-in browser/API/PostgreSQL lifecycle has now passed.
It observes public creation, same-audience summary revision, the unchanged 409
audience guard, explicit withdrawal and foreign 404 with retained history. The
final normalized oracle expects two A revisions, version three after withdrawal,
one revision event, one withdrawal event and unchanged non-summary fields.
The focused UI mocks alone do not prove that lifecycle. The actual runtime
evidence below does not qualify providers, deployment or independent ownership.
Source and financial identities/databases stay separate.

## Final validation and actual lifecycle — 25 October 2026

Product candidate: `9bf10a97fd4799442952c9bb314b5eaa8f684f17`. Final browser
selector candidate: `d3d5370e1628e980cc0bc25d91b5de01cc0adba8`; its only change
is the conditional publication scenario's accessible textbox lookup and form
scope. Product source and served artifact hashes are identical between them.

The first production build on `0fb714a` failed TypeScript before any browser ran:
RTL role options incorrectly used Playwright's `exact`, and the Refresh action
needed to await/discard the applied refresh generation. These were corrected,
without changing matching or domain authority. Its prior 120-test UI pass is an
earlier checkpoint, not the final candidate's proof.

- Final-source full UI: 120 tests in 15 files passed, 48.57 seconds;
  `/tmp/undergrid-publication-final-9bf10a9-full-ui.log`.
- Coupled TypeScript/Vite/provenance build passed on `9bf10a9` (Vite 1.45s) and
  again after the selector change (1.27s):
  `/tmp/undergrid-publication-final-build-attempt2-20261025.log` and
  `/tmp/undergrid-publication-final-d3d5370-build.log`. Source SHA256:
  `ec6158d1f77c6ce869cefbf1449e8d0a9dbc13f18cddc76ae163e89d419af47c`;
  served JS SHA256:
  `6a078bb0b730f4a27353a31d516ab1d127a9f71c504479d7dffe5bb6ebe4f0d0`.
- The initial four-case browser aggregate on `9bf10a9` **failed** (156.175s).
  Its HP1 setup, complete HP1 partial/watch/receipt/restart, and HP2 three-owner
  foundation cases passed (29.11s, 21.33s, 12.09s). The publication case stopped
  before summary revision: exact native-label lookup included the prefilled
  textarea text, while its accessible textbox name was correctly Public summary.
  The retained trace and database show A at version/revision one, public/active,
  with its original summary. No revision or withdrawal proof follows from that
  failed attempt. Log: `/tmp/undergrid-publication-final-9bf10a9-browser55500.jsonl`;
  DB `hp1_run_03a849f2a263487f942b75d195660e50`; artifacts
  `/tmp/undergrid-hp2-browser-2886933092`.
- With only that harness lookup corrected, the publication case passed under
  the unchanged 90-second browser limit: Go case 15.31s/package 16.338s,
  Playwright scenario 12.1s/runner 13.5s;
  `/tmp/undergrid-publication-final-d3d5370-browser55500.jsonl`.
  Retained DB: `hp1_run_ff0364db77a143c69ba839b03cbd59ca`, isolated network55500;
  artifacts `/tmp/undergrid-hp2-browser-4161594796`.
- Tagged `go vet -tags=hp1_live,hp2_live ./...` passed;
  `/tmp/undergrid-publication-final-d3d5370-vet.log`.
- Actual retained-network96 domain/privacy race regressions passed: 16 top-level
  cases and 12 subcases, zero failures/skips, 36.242s;
  `/tmp/undergrid-publication-final-d3d5370-domain96.jsonl`. These include immutable
  visibility/revisions, renewal, restricted grant/revoke races, owner isolation,
  partial audit privacy/response recovery/source locks, cursor authority and
  terminal satisfaction. No financial runtime or database participates.

The browser creates all three controlled accounts, agents and two intentions
through normal production login/session/API paths. A explicitly publishes the
selected fields, B reads exactly the safe projection and server metadata, A
reviews and saves the changed summary, a direct audience-changing PATCH still
returns 409, then A explicitly withdraws and B receives 404. Read-only SQL proves
the withdrawn intention retains public visibility and revision two, advances to
version three, disables its watch, preserves both immutable public projections
and every non-summary document field, and records exactly one revision and one
withdrawal event. It creates no rooms, memberships, plans, rights approvals or
assignments. Product rows are not seeded to complete browser steps.

Quality assessment: one small projection control serves creation and fixed-
audience revision; the server retains the public allowlist and authority. The
preview receives no private fields. Typed current responses are validated before
retry-key release, and per-view/session epochs prevent late replies from restoring
obsolete detail. Immutable history is observed independently rather than erased
to pass tests. No schema, DTO, provider policy or generic visibility guard changed.
The full discovery-to-agreed-plan HP2 journey and external qualifications remain
unproven. Reviewed feature integration completed in PR103 at `b0d49c7`;
post-merge focused UI checks passed (10 tests, 9.53 seconds) and the console
TypeScript/Vite build passed (1.34 seconds). Retained logs:
`/tmp/undergrid-owner-publication-release-b0d49c7-ui.log` and
`/tmp/undergrid-owner-publication-release-b0d49c7-build.log`.
