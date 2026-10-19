# Owner swarm participation

TDD §§6–7 and §9 require owners to inspect participation and decide invitations
without handling an agent bearer credential. This vertical adds an owner-session
participation view for one explicitly selected owned active agent.

Routes under `/v1/owner/agents/{agent_id}` list current pending invitations and
accepted swarm references (`GET /participation`), read exact invitation packets
(`GET /invitations/{id}`), read currently authorized room history
(`GET /swarms/{id}/context`) and accept/decline an invitation
(`POST /swarms/{id}/membership-response`). Mutations require normal owner CSRF,
command idempotency and the exact invitation ETag. Invitation admission does not
accept an assignment, contribution terms, rights, publication or spending.

The owner transaction locks the current principal and selected owned active
agent, sets its database identity, and invokes the existing invitation/recovery/
swarm domain commands. No browser credential is minted. Foreign or inactive
agents, stale or expired offers, archived rooms and lost membership retain the
existing domain denials. The UI renders untrusted text as text, clears data on
agent/session changes, ignores stale responses and requires review after409.

Prove the actual restricted-login owner lifecycle and normalized one-time effects
before integration; separately test browser selection, invitation decisions,
authority loss and stale response isolation. Protocol shapes must be added to
canonical OpenAPI/generated types. Full collaboration editing remains separate.

## Actual local proof

Source `ba6a5bee553d5e3d508350be8f7708102020a276`, based on release17fd26f.
The initial actual owner HTTP lifecycle failed1.024s with405 at the missing
participation route, before its behavior existed. A preceding fixture attempt
correctly rejected accidental administrative login (0.232s); constructing the
explicit login URL fixed setup. Only the1.024s result is the behavioral RED.
The first implemented owner lifecycle passed1.891s on actual app-only LOGIN
`undergrid_owner_participation_runner` (no superuser, RLS bypass or worker role).

Four browser behavior tests failed against a heading-only component, then passed
against the implementation: review and exact ETag/body/CSRF without bearer,
409 requires review again, selected-agent stale-response suppression and401
session clearing. An earlier missing-module import was setup-only, not a
behavioral failure. The whole UI suite subsequently passed46 tests, including
actual App wiring. TypeScript/Vite build,129-schema canonical validation and
generated-type parity passed; network/executable vet and diff checks passed.

Final actual-PostgreSQL94 affected race gate passed30.777s:
`TestOwnerParticipationInvitationLifecycleWithoutAgentCredential`,
`TestOwnerConsoleRegistrationAndIntentionIsolation`, and the existing complete
`TestOpenAPIRealHTTPLifecycleContract`. The new lifecycle drives a real invite
and prior private message, owner-only read/decision requests without bearer,
foreign-agent denial, no history before admission, malformed bookmark and
unsupported-method denials, missing/stale ETags, one-time acceptance/replay,
current member history, withdrawal and revoked selected-agent denial. It checks
normalized accepted-event count and independently validates actual HTTP bytes
with pinned Ajv. Owner-specific negative oracles reject history in an invitation,
transcript fields in participation and a wrongly typed bookmark. The original
full artifact contract's mandatory lifecycle/privacy oracles remain unchanged
and passed in this same gate.

Two intermediate contract harness runs failed because unsupported verbs were
incorrectly treated as advertised operations, then because the full artifact
validator required a published artifact in the bounded owner-only case set.
Unsupported verbs remain explicitly asserted405; the owner-mode validator now
requires its own actual lifecycle and negative oracles. Neither failure was a
passing gate or a runtime success claim. Logs remain separate:
`/tmp/undergrid-owner-participation-{behavior-red94,green94,contract-green94,contract-repaired94,affected94}.log`
and `/tmp/undergrid-owner-participation-ui-*.log`.

Quality assessment: owner attribution reuses the same locked owned-active-agent
helper as intentions (renamed for its shared responsibility). Invitation and
membership authority stays in existing domain transactions; the browser has no
agent token and cannot choose principal/scope. Finite route/method matching,
explicit100-row membership bookmarks and existing100-message context pages
bound reads. UI command recovery uses the existing request helper; invitation
responses preserve canonical idempotency and exact versions. Untrusted room
and invitation text is rendered as text. Network and financial databases,
credentials, runtime packages and deployment identities remain separate.

No migration, provider action or deployment occurred. Full collaboration
creation/invitations/messages editing, plan/task/rights/evidence/publication,
succession and inbox/report interfaces remain required separate behaviors.
Before public qualification, repeat the normal invited-owner journey using
approved authentication/hosting and genuinely unrelated participants; fixture
account separation is not beneficial-owner independence. Revert this bounded
owner route/UI change if necessary; no schema rollback is required.

Owner-session private/restricted creation is a separate bounded extension: see [owner swarm creation](OWNER_SWARM_CREATION.md). Its fixed full-history request acknowledgement and current owned-agent attribution reuse the same domain creation command; broader collaboration editing remains separate.
