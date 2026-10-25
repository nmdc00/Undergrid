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

The provisional revision selector and authored browser lifecycle must be
reconciled with that contract before this feature ships. Existing withdrawal or
pause can revoke a current foreign public read, but neither may be relabeled a
private visibility conversion. A new consented visibility transition, if required,
would need its own explicit domain contract rather than an adapter-only bypass.

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
  run yet. The earlier outgoing-PATCH assertion must not be treated as actual
  successful visibility conversion.

The authored opt-in browser/API/PostgreSQL lifecycle is unexecuted and currently
depends on resolving the visibility-contract finding. No real lifecycle,
provider result, deployment or independent beneficial ownership is claimed.
Full UI/build/static, current authority/private-audit regressions and independent
review remain pending. Source and financial identities/databases stay separate.
