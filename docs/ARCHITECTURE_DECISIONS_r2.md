# Undergrid v0.1 — Architecture decision log, revision 2

Status: proposed design; implementation not approved. Read with the [revised TDD](UNDERGRID_TDD_v0.1-r2.md). Only ADR-001's stack direction has prior owner acceptance; the new reductions/policies await the [approval register](PRODUCT_DECISIONS_r2.md).

## ADR-001 — Go versus Rails, with selection evidence

Conversation sequence: the architect recommended “Go + PostgreSQL, with a TypeScript operator UI.” The project owner then instructed: “Ok write the TDD with that stack in mind.” This explicitly accepts the recommendation in context. It does not approve scaffolding, and the later generic Rails-default brief does not erase that preceding stack direction. Status: **Go selected for design; implementation approval withheld**. The owner can still change it.

| Consideration for this beta | Go/PostgreSQL | Rails/PostgreSQL |
|---|---|---|
| Implementation effort | Explicit assembly of API validation, identity integration, admin views and jobs; React is a separate UI build | Likely less glue for CRUD, views, migrations and administrative workflows if the team is fluent in Rails |
| Library fit | pgx/sqlc/River and official MCP Go SDK fit explicit SQL and adapters; A2A/x402 exact coverage still needs compatibility tests | Active Record/Active Job/Solid Queue simplify persistence/jobs; official Ruby MCP SDK exists; A2A/x402 integration still needs verification |
| Operational complexity | API/worker binary process modes plus isolated verifier; PostgreSQL/object store/auth remain the real dependencies | API/worker Ruby processes plus verifier; essentially the same infrastructure and operational obligations |
| Concurrency need | Explicit request cancellation and worker concurrency; no measured advantage claimed | Adequate hypothesis for the proposed small beta; database transactions still govern correctness |
| Expected workload | Qualification target 100 principals, 1,000 intents, 10 requests/second, 100 long polls | Same target; no evidence it requires a language change or specialist routing service |
| Domain safety | Types assist local modeling; policy/state/SQL constraints and tests enforce business invariants | Same need for transaction, policy and lifecycle tests; dynamic typing alone does not determine correctness |
| Team knowledge | Not established by this conversation | Not established by this conversation |

Recommendation: retain Go because the owner accepted it, not because Rails cannot support the workload. If that acceptance were absent and the team had equal skills, Rails would deserve a modest implementation-speed preference for this bounded product, especially with server-rendered UI. Switching now is a product/team decision with no existing production-code migration cost. No speculative throughput, cost or “future scale” claim ratifies the stack.

Primary library references: [Rails jobs](https://guides.rubyonrails.org/active_job_basics.html), [MCP Ruby SDK](https://github.com/modelcontextprotocol/ruby-sdk), [MCP Go SDK](https://go.sdk.modelcontextprotocol.io/), [River](https://riverqueue.com/docs/transactional-enqueueing). These demonstrate availability, not a successful compatibility test.

## Remaining architecture decisions

| ADR | Proposed decision | Why / cost / approval condition |
|---|---|---|
| 002 | 22-table network, eight-table separate sandbox | Removes 38 application tables and generic market machinery; don't hide mutable ACLs/money in generic JSON |
| 003 | One application with API/worker modes; isolated fixed verifier | Keep code/operations small while preserving the untrusted-input boundary |
| 004 | Durable inbox long-polling, not mandatory SSE/webhooks | One recovery/ack protocol; external owners schedule their listeners; SSE becomes optional later |
| 005 | Pairwise typed opportunities and derived cluster views | No persistent clustering/graph framework; distinguish demand, complements and recruitment |
| 006 | Two approved read-only external-source adapters | Supplies first-user discovery; permission/provenance/staleness are mandatory; no crawling or autonomous outreach |
| 007 | One principal owner, one plan lineage, bounded JSON artifacts | Removes organization admin, generic proposals and verifier marketplace; explicit owner scope approval needed |
| 008 | Invitations/applications share membership state; tasks hold current assignments/epochs | Small finite state machines; history remains in events, not overwritten authority |
| 009 | Fixed contribution templates and exact-manifest publication approval | Cross-principal rights need explicit terms; legal wording/allowlist pending, not inferred from contribution |
| 010 | Sandbox evidence ledger, not a generic double-entry customer-account model | Records charge/refund/reorg facts accurately without custody/accounts/revenue abstraction; future live accounting is a new design |
| 011 | Independent network and financial tracks, both required for full v0.1 | Finance does not infect network schema/schedule; any earlier network-only preview needs owner decision |
| 012 | One costed app host at minimum, managed durable DB/backups, separate sandbox host | Accepts disclosed app downtime; HA is an explicit budget upgrade, not assumed free infrastructure |
| 013 | No blanket safety claim for external LLM runtimes | Server controls its own authority; external confused-deputy risks remain and are tested/documented |
| 014 | Four-principal reuse acceptance, provenance by stage and cohort | A swarm member downloading their own artifact is not third-party network effect |

Reconsider a decision only with measured pain, a changed product boundary, or an owner decision. Nullable columns, unused adapters and speculative interfaces are not evidence that a deferred feature is delivered.
