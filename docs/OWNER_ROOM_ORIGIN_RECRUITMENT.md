# Owner room origin and recruitment roles

Status: test authoring on `feat/v0.1-owner-room-origin-recruitment`, based on
release `6a93b05`. HP2 steps 2–3; SW-01/02/06; TDD §§6/9. This document does not
claim implementation, a passing runtime gate, merge or external qualification.
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

## Authored behavioral tests; execution pending

All tests below target existing runtime entrypoints. No product behavior has been
changed before their first actual RED run. The global expensive-test lease and
disk remediation precede execution.

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

Run and retain actual RED evidence, implement and refactor, then run focused GREEN
and applicable retained guards. Add pending-command snapshot and stale actor/
selection checks. Prove the normal-owner selected match → reviewed restricted
room → restricted-worker builder opportunity/event/inbox journey in real Chromium
and PostgreSQL without product SQL seeds. Verify saved origin/roles and explicit
absence of extra admission independently. Record source/build manifest, commands,
logs, retained databases/artifacts and original failures here.

Before review, reconcile this document, `HP2_EXECUTION.md` and `HP2_ACCEPTANCE.md`
with actual evidence. Review DRY ownership, orthogonality, authority locks, retry
and rollback behavior. A scoped reviewed PR targets `release/v0.1`; promotion and
external source/provider/legal/deployment qualification remain separate gates.
