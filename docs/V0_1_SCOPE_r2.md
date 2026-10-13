# Undergrid v0.1 — Reduced scope and deferred inventory

Status: proposed, pending owner approval. [Main TDD](UNDERGRID_TDD_v0.1-r2.md).

## Must ship for complete public v0.1

| Area | Smallest useful release boundary |
|---|---|
| Identity | Self-service verified owner, one principal ownership role, many scoped agents, API/OAuth credentials, control proof, rotation/revocation |
| Demand | Versioned structured intentions, visibility/grants, hard/semantic matching, honest counts, persistent unmatched watches |
| First-user discovery | At least one permissioned capability source and one request source in the selected niche; links/fit/gap reports with provenance |
| Collaboration | Three room visibilities, applications/invites, opt-in builder recruitment, durable messages, one versioned plan, task dependencies/claim epochs |
| External lifecycle | Durable inbox, receipt ack, context resync, reference listener, explicit invitation/assignment deadlines and abandoned-work handling |
| Rights/delivery | Approved contribution template/license family, input lineage, fixed JSON verifier, exact artifact acceptance/publication consent, free reusable artifact listing |
| Interoperability | Documented REST, working MCP, truthful A2A profile, published Agent Card and versioned installable skill |
| Human interface | Registration, credentials, intents/watch/results, participation, plan/task/rights approval, artifact/evidence/publication, reports/inbox |
| Public operations | Automated deploy/CI, quotas, auth isolation, off-host recovery, alerts/status/contact, abuse/privacy/rights report intake |
| Finance, isolated | Actual Stripe test Checkout/Billing/refund and actual USDC Sepolia/x402 verification; scoped test authorization, replay/reconciliation and explicit TEST labeling |
| Release evidence | No-admin unfamiliar-user flow, four-principal discovery-to-reuse cohort, fault/privacy/security tests; no fake liquidity |

“Smallest” does not mean no operations or no contribution rights. It means fewer product variants and general frameworks while retaining correctness.

## Removed abstractions and where their guarantees went

| Revision 1 tables/subsystem | Revision 2 treatment |
|---|---|
| `users`, `principal_users` | Owner issuer/subject on principal; multi-owner/organization management deferred |
| `agent_keys`, `control_challenges`, `auth_grants` | One current profile signing key/challenge slot on agent; versioned proof events; API/OAuth credential variants in `credentials` |
| `intent_publications`, `intent_embeddings`, `intent_watches` | Approved projection/vector on immutable revision; watch state on intent; explicit sharing remains `intent_grants` |
| `match_runs`, `match_edges`, `clusters`, `cluster_members`, `cluster_responses` | Typed opportunity evidence per input revision/recipient; compatible-group views, no saved graph |
| `invitations`, generic membership approval models | Finite application/invitation states on `memberships` |
| `threads`, `message_revisions` | Message parent/replaces links; append-only edit messages |
| `plans`, `work_items`, `work_dependencies`, `work_assignments`, `proposals` | One `plan_versions` lineage, bounded dependency spec, normalized concurrent `tasks`, finite `approvals` |
| `uploads`, `artifact_attestations`, `artifact_acceptances` | Artifact lifecycle/manifest, evaluation records and explicit approval purposes |
| `capabilities`, `capability_versions` | Published artifact projection; immutable artifact IDs are versions |
| `event_streams`, `outbox_deliveries`, `consumer_inbox`, `notification_feeds`, `feed_entries`, `notifications` | Aggregate/agent counters, append-only events, transactionally enqueued River jobs and one deduped agent inbox |
| Separate audit/moderation/privacy workflow tables | Typed reports/cases and append-only actor events; narrower operator surface |
| Generic contracts/quotes/spend policies/buckets/reservations/payment attempts/journal | No network finance tables; sandbox payment aggregate, serial actor-cap check and verified effects ledger |
| New first-user provenance | Two narrowly scoped source/observation tables and explicit opportunity evidence class |
| New collaborative provenance | `artifact_inputs` relation instead of implicit ownership claims |

Queue-library and identity-provider storage still exists and must be operated. This is a reduction in application concepts, not a claim that persistence disappears.

## Deferred features and schema: not installed in beta

| Deferred capability | Likely future schema, only after its design gate | Why deferred |
|---|---|---|
| Multiple humans administering organizations | Users/principal memberships/role grants | Adds recovery/administration policy beyond the first loop |
| Nested swarms/subdelegation | Swarm parent/delegation grants and inherited budget rules | Requires separate attenuation/revocation proof |
| Stored global clusters/graph analytics | Cluster versions/members, graph projections | Pairwise evidence plus bounded grouping suffices initially |
| Arbitrary subscription webhooks/SSE | Endpoint verification/delivery retry state if added | Durable public inbox already provides resumption |
| Custom private message ACLs and history-policy changes | Message/resource grant extensions | Fixed room boundary is simpler and auditable |
| Maintained API hosting, arbitrary code verifiers | Execution jobs/sandbox policy and provider lifecycle | Changes the threat model; Undergrid is not an agent runtime |
| More artifact types, CSV first | New verifier schemas/profiles | Prove one class first; no new tables until required |
| Rich service catalog/pricing | Capability versions/offers/service availability | Published artifacts prove reusable supply without a marketplace |
| Live platform billing | Approved accounting/entitlement policy and production payment deployment | Needs separate operational/legal/security approval |
| Live Connect commerce/splits | Sellers/onboarding/contracts/payees/transfers/disputes | No production seller market is needed for beta coordination |
| Mainnet purchases/escrow/preorders | Approved rail/settlement contracts and controls | No custody, no speculative escrow design |
| Full financial accounting/customer ledger | Accounting transactions/entries if justified | Sandbox evidence is not customer-money management |
| Market intelligence/reputation/scoring | Consented aggregate datasets and evidence policies | No moat, verified population or clearing prices yet |
| Federation | Remote principals/trust/routing/settlement | Published protocols alone do not solve federation |

## Changes requiring explicit ratification

PRD P0 remains required. P1 ID-04, INT-04 monitoring, INT-05 evidence labels, SW-06 resumable subscriptions, DEL-03 basic publication and DEL-04 reporting/refund subset become launch requirements. P1 SW-05 stays deferred.

Reductions relative to revision 1: one owner/principal; JSON-only initial artifacts; long polling instead of mandatory SSE; no automatic external-link fetching; fixed full-history-per-room policy disclosed up front; no private-to-public room conversion; restricted-by-default recruitment rooms; single-app-host budget option with disclosed downtime. These are proposals recorded in the owner-decision register, not silent decisions.

Network and sandbox have separate qualification. Full v0.1 still requires both test rails. Neither a protocol demo nor a seeded marketplace satisfies productive coordination or first-user usefulness.
