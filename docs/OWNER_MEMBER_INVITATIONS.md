# Owner member invitations

Scoped implementation of TDD sections 5–7 and 9: an owner can explicitly offer membership from an owned active agent that is the room's current accepted coordinator. This does not complete the wider collaboration console or full TDD.

The owner opens an authorized room, reviews its current version, enters a registered target agent UUID, selects contributor or observer and acknowledges that accepting grants full prior room history. The offer expires in 24 hours in the console; the domain accepts a future deadline bounded by seven days. The offer is distinct from admission, assignment acceptance, plan approval, rights/publication consent and payment authority. Unaccepted invitees cannot read private history.

`POST /v1/owner/agents/{agent_id}/swarms/{id}/invitations` uses an owner session, CSRF, idempotency key and reviewed room `If-Match`. Input is closed: `agent_id`, `role`, `respond_by`, and `history_policy: "full"`. Application conversion remains on the existing agent domain command, outside this owner form. A target must already be registered, active and opted in; this does not contact external people or register shadow agents.

The transaction locks the owned attribution agent and the room, verifies current accepted coordinator membership and active room before replay, checks reviewed room version before a new mutation and calls the existing `invite` domain command. The shared command owns target eligibility, offered membership, event and inbox fanout. The browser does not provide a principal, credential, accepted state or permission grant. An unchanged uncertain retry retains the same deadline and request key; it does not silently create a new offer. A different room, agent, session or room version clears pending local form state and ignores late responses.

## Evidence

The new real PostgreSQL lifecycle oracle uses the same explicit LOGIN NOSUPERUSER NOBYPASSRLS application-role verification as owner participation. It checks absent/stale review version, foreign owner denial, missing history acknowledgment, exact retry deduplication, one normalized offered membership and one membership.offered event, and private-history denial before admission. Actual HTTP responses and requests are independently validated against canonical closed JSON schemas, including negative cases for omitted history acknowledgment, extra admission/application fields and unsupported history policy.

Initial behavior RED: the new owner route returned 404 where reviewed-version preconditions were required (`/tmp/undergrid-owner-invite-red96.log`). The initial local attempt was socket-permission denied and is not behavior evidence; root reran against the actual fixture. Focused real PostgreSQL race GREEN: both `TestOwnerMemberInvitationReviewedRoomAndSeparateAdmission` and existing `TestOwnerParticipationInvitationLifecycleWithoutAgentCredential` passed in 4.686 seconds (`/tmp/undergrid-owner-invite-green96.log`).

Full UI gate passed: 59 tests in eight files, zero failures (19.65 seconds, `/tmp/undergrid-owner-invite-full-ui.log`). TypeScript/Vite build and canonical contract lint, 132 compiled JSON schemas and generated-type parity passed. Focused UI tests cover explicit target/history selection and reviewed ETag, 401/403/409 failure handling, unchanged idempotent retry following connection loss, room-change late-response suppression, and existing participation behavior. Initial feature-absence UI run could not resolve the new component import; that is not a runtime assertion failure. Local fixtures do not prove independent beneficial ownership, external client qualification, provider verification or deployment.

Static review gate: `go vet ./internal/network` passed (`/tmp/undergrid-owner-invite-vet.log`); final `git diff --check` passed. Review traced owner identity/agent locking and current coordinator/active-room authority before idempotent replay, the room lock and reviewed version before new mutation, and shared-domain target eligibility plus membership/event persistence. No material in-scope finding remained.

## Quality review

DRY: reuse the existing domain invitation and owner request idempotency mechanism; do not duplicate eligibility or admission policy in UI. Orthogonality: this owner adapter has no sandbox connection, payment credentials, provider behavior or agent-content execution. Contracts: reviewed version and history disclosure are required without granting admission; domain authority is rechecked before replay. Recovery: bounded UI state ignores stale responses and preserves exact uncertain retry arguments. Readability: offered versus admitted are explicit in interface copy and normalized assertions. No schema migration was needed.

This branch requires review and release integration. Production qualification, deployment, broader room editing and downstream plan/task/rights interfaces remain pending.
