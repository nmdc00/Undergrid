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
