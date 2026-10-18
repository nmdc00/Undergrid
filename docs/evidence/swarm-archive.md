# Explicit terminal swarm archive evidence

Requirement: TDD §7.3 explicit coordinator archival and continued authorized history. This feature does not implement automatic inactivity warning or archival.

The restricted-login HTTP route test was authored before behavior and failed for the missing route (2.388s, `/tmp/undergrid-archive-route-red94.log`). The first implemented lifecycle run failed in 55.953s because the preissued-upload oracle reused an already sealed generation; its upload capability correctly returned 401 before the archive guard. The fixture now issues a separate still-quarantined generation before archive, preserving the required 409 guard assertion rather than weakening runtime behavior.

Runtime owns archive through the shared domain command used by REST, MCP and A2A. Current actor authority is locked before the room, then coordinator membership, explicit scope, reviewed room ETag and terminal state are checked. The transition and durable room event commit together. Idempotent replay does not append a second transition. Membership exit preserves terminal state while removing current private-history access. The existing room authority gate remains the sole active-room write rule for plans, task progress, artifacts and new input grants; fixed verification already acknowledged before archive can complete without conferring acceptance or publication. Existing public licensed bytes and attributed reuse remain available.

No migration was necessary, and no applied migration bytes were changed. Network and financial databases, credentials and roles remain separate.

Quality assessment: authorization belongs to the shared domain transaction, not transport adapters. Factoring membership lookup from active-room authority permits safe archived-member exit without duplicating the room policy. The command reuses existing idempotency, version and event contracts. Fixed verification and canonical acceptance remain distinct responsibilities. Real restricted application and worker roles are used for lifecycle proofs; fixtures establish no provider or independent-owner qualification.
