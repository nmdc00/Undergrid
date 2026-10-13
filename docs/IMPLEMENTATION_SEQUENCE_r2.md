# Undergrid v0.1 — Prioritized execution and operating plan

Status: estimate and proposed sequence only; no implementation approved. [TDD](UNDERGRID_TDD_v0.1-r2.md), [owner decisions](PRODUCT_DECISIONS_r2.md).

## 1. Dependency-ordered network track

| Priority / increment | Concrete work, after approval | Gate before proceeding |
|---|---|---|
| N0 — Resolve feasibility | Owner decisions D01–D10; approve niche/rights/source material; prove chosen OAuth/MCP flow and schema generation; verify provider restore/storage features | No hidden source-permission, rights or identity blocker; record exact protocol/library versions |
| N1 — Durable identity and intention | N001/N002 migrations, self-service signup/agents/scopes/revocation, intent lifecycle/projections and minimal UI, typed commands/idempotency | Real PostgreSQL/RLS tests, wrong-actor denial, same-key replay, standing watch survives restart |
| N2 — First-user discovery and listening | Two approved source adapters, provenance, exact/semantic opportunities, durable inbox/context and listener | One unfamiliar developer sees truthful relevant external leads without another registered participant; cursor recovery/ack/privacy tests |
| N3 — Recruit and coordinate | N003/N004, role discovery/application/invitation, messages, exact plan/rights approval, assignments/epochs/timeouts | Three independent clients form a working room; no silence-as-consent, no stale authority after withdrawal |
| N4 — Produce and reuse | N005, quarantine/hash, JSON verifier, contribution lineage, acceptance/publication, fourth-principal reuse | End-to-end thin slice plus rights conflict/input tamper/reassignment tests; no code execution |
| N5 — Public interface completion | MCP/A2A parity, documentation/skill/examples, all required UI flows, reports/privacy handling | Public clients complete onboarding and recovery without admin APIs; conformance evidence |
| N6 — Public qualification | IaC/deployment/CI, load/rate limits, backup restore, security tests, external cohort and operational handoff | Network release gate passes on exact candidate SHA/image/config and production smoke check |

Within each increment, implement one invariant-backed vertical behavior at a time. Write the failing test first, prove the database/event effects, then expose it through UI/adapters. Do not create 22 empty tables in a first “foundation” PR. Migration boundaries in the TDD are dependency groups, not permission for a giant branch.

## 2. Separate financial track

| Increment | Work | Gate |
|---|---|---|
| F0 — Provider feasibility | Verify x402 scheme/nonce settlement evidence, testnet asset/RPC finality, Stripe pre-collection renewal controls | Unsupported capability blocks this adapter; no approximation called “paid” |
| F1 — Sandbox authority/persistence | Separate deployment/DB/audience, eight bounded tables, exact quote/approval/caps and unknown-outcome handling | Concurrency/property tests; network cannot access sandbox secrets or accept receipts |
| F2 — Stripe | Actual test Checkout, bounded Billing renewals, replay/reorder/reconcile/refund | Provider receipts plus test-clock/fault evidence |
| F3 — USDC/x402 | External wallet and actual Sepolia payment, receipt/nonce/finality, mismatch/reorg/refund proof | Actual testnet receipt plus deterministic failure tests |
| F4 — Sandbox public access | Self-service enrollment/docs, test-only UI, quotas, operator reconciliation runbook | No live credentials/mainnet, no network funded-demand mutation |

F0 can be researched while N0 resolves product choices. After implementation is authorized, F1–F4 can proceed independently of most network work. No financial milestone requires seller/catalog/contract machinery in the network. Full v0.1 = network gate AND both rail gates AND public operations/security gates. A network-only preview requires owner approval and honest labeling.

## 3. Effort and uncertainty

Planning estimate for engineers already comfortable with the selected stack: network 11–16 engineer-weeks, sandbox 3–5, integration/security/operations/external-user remediation 2–3: **16–24 engineer-weeks base**. Reserve roughly 25% contingency, yielding **20–30 engineer-weeks**. With two full-time engineers, a reasonable planning envelope is **12–18 calendar weeks**, accounting for sequential decisions/reviews; one engineer should plan roughly **5–8 months**, not the PRD's optimistic prototype dates. These are estimates, not delivery commitments or a staffing assumption already agreed.

Source permissions, identity-provider compatibility, contribution legal wording, facilitator capabilities and recruiting unfamiliar testers can add waiting time outside that estimate. AI assistance may reduce routine work but is not credited with eliminating security/provider validation. Reestimate after N0 and the first persistent vertical slice.

A named engineer owns operational readiness; a second person can recover service/access during absence. Allow an initial **4–8 hours/week** for patching, reviewing abuse/rights cases, checking costs/queues and restore drills, plus unpredictable incidents. “Self-service” means staff do not manually assemble routine collaborations; it does not mean an untrusted public service has no operational owner.

## 4. Minimum infrastructure and expenditure

Proposed economical deployment: one network VM running API and worker as separately constrained processes; a prestarted fixed verifier container with no network/DB/host socket; one separate sandbox VM; two independently credentialed managed PostgreSQL projects; private object storage and off-host recovery copies; managed OAuth/OIDC; transactional email; external health checks/status; CI and secret storage. No shared runtime credentials or network DB access from sandbox. Staging can be ephemeral, with separate credentials/data, not a permanent third full stack.

| Component | Sizing / planning allowance | Qualification needed |
|---|---|---|
| Network compute | Initial 4 vCPU/8 GiB-class shared VM; example Hetzner CX33 listed at €8.49/month excluding VAT/IPv4 | Actual availability, limits and load test; shared CPU is not a performance guarantee |
| Sandbox compute | Initial 2 vCPU/4 GiB-class VM; CX23 example €5.49/month excluding VAT/IPv4 | Separate service account, firewall and deploy pipeline |
| Managed PostgreSQL, two projects | At Neon published Launch compute rate, two continuously active 0.25-CU projects: `2 × 0.25 × 730 × $0.106 = $38.69/month` compute only | Restore window, pgvector, region, storage, transfer and access-control features may add cost; no sleep-dependent price assumption |
| Objects plus independent backup copies | Planning allowance €8–15/month at initial small storage | Recheck current quote/egress/versioning; do not use old marketing rates as current price |
| Identity | €0–25/month small-beta allowance | Mandatory OAuth/MCP/security features must actually be in selected plan; pay more or change provider if not |
| Email, external monitoring/status, domain, CI | Combined €5–25/month small-beta allowance | Free quotas are an assumption, not indefinite entitlement |
| Embeddings/source APIs | €0–15/month capped allowance | Approved processor or local bounded model; no unrestricted request spend |

The compute examples come from [Hetzner's current published adjustment table](https://docs.hetzner.com/general/infrastructure-and-availability/price-adjustment/). The database arithmetic uses [Neon's published compute rates](https://neon.com/blog/major-compute-price-reduction-on-neon). Pricing was checked on 12 October 2026; the dynamic pricing page could not be retrieved reliably, so the database example is a published-rate calculation, not an accepted current vendor quote. Currency amounts are kept in their source currencies rather than pretending to have an approved exchange rate.

**Budget recommendation:** a provisional **€70/month planning floor** for a very small beta if the listed low-cost plans meet the security/restore requirements; approve **€100–150/month** operating headroom initially. These are estimates including a rough currency contingency, not an exact converted bill. Exclude VAT, labor, legal/security review, mainnet funds, paid growth and unexpected abuse-driven overages. If required identity/private-network/PITR features exceed those tiers, raise the floor before launch; never remove controls to hit it. Set alerts at 50/80/100% of approved budget and cost-weighted admission limits.

This minimum has a single network app host and accepts its outage during repair. Proposed internal target is 99.0% monthly core availability with best-effort public-beta terms; no paid uptime SLA. Off-host durable data, tested restart/restore and a standby operator are essential. If the owner requires automatic app failover, add a second app host and load balancer plus database plan qualification; reprice it explicitly. Self-hosting the database can lower invoices but increases recovery/patching responsibility and is not the recommended minimum.

## 5. Unknown dependencies and release evidence

Before coding: approved niche and rights policy; permitted sources with real relevant data; OAuth provider supporting selected MCP client flow; API schema/codegen compatibility. Before rail coding: Stripe renewal-control proof and x402 facilitator evidence contract. Before public access: domain, hosting accounts, email reputation, restore test, privacy/terms/security contact, on-call owner and external test cohort.

Evidence bundle: release commit/image/config/schema hashes, contract/RLS/concurrency reports, listener crash/recovery tests, provider receipts, security/rights test results, restored-state checks, user-cohort observations and measured onboarding time. State which results are simulated, provider-test, external-cohort and organic. No test-data output is evidence of economic liquidity.

Stop release for a private-data leak, unauthorized/duplicate charge, missing acknowledged state, stale-plan/rights publication, arbitrary verifier execution, unrecoverable backup, inability to self-register, unusable unmatched-intent monitoring, missing permitted first-user sources or a cohort needing team intervention.
