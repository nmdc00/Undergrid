# Owner room origin and recruitment roles

Status: implemented with focused local proof; full browser/review/merge pending on `feat/v0.1-owner-room-origin-recruitment`, based on
release `6a93b05`. HP2 steps 2–3; SW-01/02/06; TDD §§6/9. This document does not
claim a passing full browser journey, merge or external qualification.
The locked task contract is in `HP2_EXECUTION.md`.

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

Frozen migrations 040–044 are not editable. Forward 045 is reserved for a narrow
boolean authority-lock helper if restricted RLS requires it, with review before
first application. It must not disclose foreign private rows or broaden grants.

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
Additional pending/context UI guards and the actual selected-match browser mode
are authored at `d99233a`; they still require execution. Costly gates remain
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

## Remaining proof

Run the additional pending/context guards, full UI/build/static/contracts and
applicable retained runtime checks on the integrated final source. Prove the normal-owner selected match → reviewed restricted
room → restricted-worker builder opportunity/event/inbox journey in real Chromium
and PostgreSQL without product SQL seeds. Verify saved origin/roles and explicit
absence of extra admission independently. Record source/build manifest, commands,
logs, retained databases/artifacts and original failures here.

Before review, reconcile this document, `HP2_EXECUTION.md` and `HP2_ACCEPTANCE.md`
with actual evidence. Review DRY ownership, orthogonality, authority locks, retry
and rollback behavior. A scoped reviewed PR targets `release/v0.1`; promotion and
external source/provider/legal/deployment qualification remain separate gates.
