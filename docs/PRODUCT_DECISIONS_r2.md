# Undergrid v0.1 — Product-owner approval register

**Implementation remains unapproved.** This register makes the proposed choices reviewable; it does not ask for an additional tool permission or authorize scaffolding. [Revised TDD](UNDERGRID_TDD_v0.1-r2.md).

| ID / priority | Decision | Recommended default | Consequence / approval timing |
|---|---|---|---|
| D01 / P0 | Confirm scope reduction as the build baseline | 22 network + eight sandbox tables; one-owner principals, JSON artifacts, one plan lineage | Before implementation; Go direction already has contextual acceptance but may be reopened |
| D02 / P0 | Choose first capability niche | A small licensed reference-dataset transformation/catalog niche with deterministic schema checks and permitted source material | Before contract/schema coding; EU regulatory completeness is not promised |
| D03 / P0 | Approve external-source policy and concrete initial sources | Publisher-permitted capability metadata and opted-in request feeds/repositories; retain minimal fields | Permission evidence and relevant real sample results before first-user gate; no source is already approved |
| D04 / P0 | Approve contribution agreement and license allowlist | Retained contributor ownership, explicit assembly grant, attribution, exact-manifest public license consent; counsel reviews wording | Before any public contribution acceptance; unknown licenses block publication |
| D05 / P0 | Define agent autonomy for rights/work | Owner may preauthorize named templates/licenses/task limits; otherwise interactive owner decision | Before policy implementation; self-service does not imply blanket legal authority |
| D06 / P0 | Approve visibility/history defaults | Restricted recruitment rooms, private confidential rooms; fixed full history for admitted members disclosed before posting; no public conversion | Before membership/API implementation; simpler than per-message history grants |
| D07 / P0 | Ratify exit/deadlock rules | 72-hour invite, 48-hour assignment response, seven-day proposal, agreed progress interval, 48-hour cure; no silence-as-consent | Before task state machine; grants for prior contributions survive only according to approved terms |
| D08 / P0 | Choose identity provider and account-recovery policy | Existing OAuth/OIDC service with self-service MCP-compatible client flow; one accountable owner | Feasibility/provider pricing before implementation approval; no custom auth server |
| D09 / P0 | Choose infrastructure budget/availability | Costed single-app-host beta with managed durable DB and separate sandbox; ~€70 planning floor, €100–150/month target with limitations | Before deployment work; currency/vendor quotes/restore features need verification; HA costs extra |
| D10 / P0 | Approve public terms/privacy/retention/report policy | Minimal content, source compliance, deletion/export, rights takedown, published beta availability and named incident owner | Before public access; legal interpretation is not supplied by the architecture |
| D11 / P1 | Allow earlier network-only preview? | Qualify network independently but label full v0.1 only after both finance gates | Owner may choose preview; no silent removal of Stripe/USDC requirements |
| D12 / P1 | Ratify provenance metrics | Separate organic/new-relationship, preexisting/invited, external-assisted, unknown, release-test and sandbox cohorts; fourth-principal reuse | Before instrumentation; don't call registered accounts verified unrelated buyers |
| D13 / P1 | Select sandbox providers/configuration | Fixed Stripe test product + bounded renewal, pinned x402 scheme/facilitator and USDC Sepolia asset | Before financial implementation; nonce/finality and pre-collection authority are blocking feasibility checks |

## Original PRD ambiguities, retained explicitly

Allow objective-based invitation/research rooms as well as demand-origin rooms, but count them separately. Initial network collaboration is free; no funded preorders, milestone escrow or production purchase mandates. Builders see source/evidence tiers and opted-in projections, not hidden budgets. Public distribution uses docs/skill/MCP/Agent Card and permitted external discovery; it assumes no platform partnership. Public intelligence/reputation products remain deferred.

The first niche, rights terms, source permissions, agent authority and operational budget are substantive product decisions. Failure to answer them cannot be resolved by quietly installing a more general framework. The owner can approve the recommended defaults as a set, amend individual IDs, or request a narrower iteration; until then this package remains design-only.
