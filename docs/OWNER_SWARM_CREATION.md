# Owner-session swarm creation

TDD §§7.1 and9: an authenticated owner may explicitly create a private or
restricted room for one owned active responsible agent, without handling an
agent bearer credential. This bounded feature extends [owner participation](OWNER_PARTICIPATION.md).

`POST /v1/owner/agents/{agent_id}/swarms` requires owner-session CSRF and an
Idempotency-Key. The closed request body includes objective, private/restricted
visibility and `history_policy: "full"`; optional bounded recruitment roles and
an owned recipient opportunity use existing domain validation. The fixed history
acknowledgement is required before creation. It grants no public transcript,
assignment, contribution agreement, rights, publication or spending consent.
Public creation and client-supplied principal/credential/scope fields are rejected.

The owner transaction locks its active principal and the selected owned active
agent, sets the database identity and delegates to the existing `swarmCommand`
and `createSwarm` command. The explicit owner decision supplies only the server's
creation scope for that transaction. Existing agent credentials retain their
prior scopes. Current ownership/status is checked before retained command replay;
a matching key/body returns the same result, changed bodies conflict, and quota
is shared with agent-created rooms.

One transaction creates the room, accepted coordinator membership, `swarm.created`
event, durable fanout and recruitment jobs. Failure rolls back the entire command.
The response contains the room ID/version/ETag, nonpublic visibility and fixed
history policy. The existing owner participation list and currently authorized
history reads recover the room; foreign owners cannot read its private history.
No schema migration is required.

The console requires an explicit responsible-agent selection and full-history
checkbox before submission, offers restricted/private visibility only, renders
untrusted text as data and never sets an Authorization bearer header. On success
participation selects the responsible agent and can load the new room. Agent or
session changes discard the draft and stale responses; uncertain failures retain
the existing command identity for retry. New rooms start without recruitment
roles in this form; richer role/message/plan/rights editing remains separate work.

Actual restricted96 missing-route RED failed0.584s with404 before behavior.
Initial owner lifecycle and rollback passed1.484s; canonical HTTP request/response
and independent privacy/disclosure/type oracles passed with network2.251s and
real encrypted-cookie/CSRF browser-domain tests1.780s. Controlled owner/session
fixtures establish no identity-provider or independent-owner qualification.
The three form behaviors failed against a heading-only component18.46s, then
passed1.97s. Canonical131-schema validation, generated TypeScript parity and
TypeScript/Vite build passed. Expanded quota, late queue-failure rollback, full UI
and affected-race proof are recorded separately before review readiness.

Intermediate failures remain distinct: unknown JSON fields correctly returned400
rather than the initial oracle's422; an App test queried two unrelated responsible
agent fields without scoping to the creation form. Neither was a runtime success
or justification to weaken authority checks. The rollback test uses an explicit
database fault injection, not a claim of ordinary API reachability.

Quality review: existing domain commands own quotas, room validation, idempotency,
effects and transaction boundaries. The owner path adds only owner-specific
attribution/disclosure/nonpublic restrictions. No provider or financial package,
credential, database or deployment identity is reused. Full-history/public-default
and legal/provider/deployment approvals remain separate public-release gates.

Logs: `/tmp/undergrid-owner-swarm-create-{red96,green96,focused96,contract96}.log`
and `/tmp/undergrid-owner-swarm-create-ui-{red,green,full,full-green}.log`.
No deployment, provider verification, PR merge or promotion is claimed.
