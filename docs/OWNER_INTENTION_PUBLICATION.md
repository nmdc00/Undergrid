# Owner intention publication — implementation checkpoint

Branch: `feat/v0.1-owner-intention-publication`, from release `316fd4e`.
This feature is unmerged. HP2 and real provider/user qualification remain open.

## Implemented, not yet release-qualified

The owner creation form offers an explicit redacted-public selection, bounded
public summary and a review of the selected summary/output/hard-requirements
projection. A shared control receives none of the private objective, budget,
preferences or server-owned audit. Safe server identifiers/version/status remain
part of actual foreign API reads.

Create and revision drafts have independent visibility/review state. Review is
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

The corrected opt-in browser/API/PostgreSQL lifecycle is authored but unexecuted.
It observes public creation, same-audience summary revision, the unchanged 409
audience guard, explicit withdrawal and foreign 404 with retained history. The
final normalized oracle expects two A revisions, version three after withdrawal,
one revision event, one withdrawal event and unchanged non-summary fields.
No real lifecycle, provider result, deployment or independent beneficial ownership
is claimed by these UI mocks.
Full UI/build/static, current authority/private-audit regressions and independent
review remain pending. Source and financial identities/databases stay separate.
