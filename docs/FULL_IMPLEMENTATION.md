# Full r2 implementation execution

Owner instruction on 2026-10-13: full implementation with an agent swarm. Scope is the complete must-ship inventory in UNDERGRID_TDD_v0.1-r2.md and V0_1_SCOPE_r2.md, excluding explicitly deferred product features. Older design-status banners are historical. GitHub-hosted CI remains disabled by owner instruction; configured local behavioral, lifecycle, race/static/build checks remain required. Legal/source/provider/deployment approvals remain explicit and are not inferred from implementation or code promotion.

Baseline: release/v0.1 e17ae5a, source-identical to promoted main/develop. Main-to-develop and main-to-release ancestry were synchronized through PRs #12/#13. One branch per observable behavior, one isolated worktree per agent; feature PRs target release/v0.1, then integration proceeds through develop and main. No direct protected-branch pushes or forced history rewrites.

## Locked first-wave assignments

| Agent | Branch / worktree | Observable acceptance | Runtime/persistence | Owning doc sections |
|---|---|---|---|---|
| artifacts | feat/v0.1-artifact-publication / /tmp/undergrid-worktrees/artifacts/publication | Four ordinary principals can seal, verify, explicitly accept/publish and independently retrieve/reuse exact JSON; stale authority/hash/epoch and missing rights fail closed | HTTP upload/finalize/evaluation/approval/publication/content/reuse paths, actual worker dispatch, artifacts/artifact_inputs/evaluations/finite approvals/events | TDD §§8, 10; docs/evidence/artifact-publication.md; N4 partial vs qualified status |
| lifecycle | feat/v0.1-coordination-deadlines / /tmp/undergrid-worktrees/lifecycle/deadlines | UTC invitation/assignment/plan/progress deadlines survive restart; silence never admits/approves; timed transitions invalidate stale claims and dedupe evidence | Conditional worker/domain sweeps and canonical context, membership/plan/task versions/epochs/deadlines/events | TDD §§6.3, 7.2–7.3, 9; docs/evidence/coordination-deadlines.md |
| frontend | feat/v0.1-owner-console / /tmp/undergrid-worktrees/frontend/owner-console | Browser owner can register, manage only own agents/credentials/policy and intentions/watch/results using authenticated public routes; wrong-owner/CSRF/stale requests denied | React/TypeScript UI with actual Go owner/domain API and executable local UI tests; no token storage or UI-granted authority | TDD §§4, 9; ID-01–03, INT-01/04; docs/evidence/owner-console.md |
| root | docs/v0.1-full-implementation / /tmp/undergrid-worktrees/full-plan/implementation | Maintain complete requirement/acceptance map, review actual branches, integrate in dependency order, measure exact release behavior and blockers | Documentation/orchestration only; independent integration fixture | This document and implementation status; no contrived behavioral test for docs |

All agents write behavior tests before modifying behavior, use actual PostgreSQL restricted-role lifecycle assertions, assess policy DRY/contracts/resource ownership/recovery, and commit scoped changes plus evidence. Each owns a fresh disposable PostgreSQL cluster (artifacts port 55442, lifecycle 55443, frontend 55444) to avoid fixed global role collisions or weaker copied-schema proof. Root integration port 55440 is reserved. Shared Go caches are available; no cross-agent fixture reset or credential reuse.

Artifact storage/verifier must use a fixed trusted configuration, sealed immutable bytes, generation/hash binding, bounded IO, explicit OS-isolation qualification status, and no arbitrary executable/network/SQL content from agents. No fixture becomes provider/legal/source proof. Browser configuration must not hard-code legal terms or external provider choices; tests use explicit fixture labels.

## Full remaining requirement map

| Work item | Requirement / owning section | Needed behavior and proof | Current disposition |
|---|---|---|---|
| Owner console | ID-01/03, §4, §9 | Self-service browser identity/credentials/intent flows, owner isolation/CSRF | Assigned first wave |
| Agent control and rotation | ID-02, §4 | Key/endpoint proof, bounded challenge, scoped API/OAuth rotation/revoke, recovery | Next independent branch |
| Intention revision/grants | INT-01, ID-03, §§4–5 | Typed editable revisions/projections, exact ETags, narrow expiry-bound sharing, disclosure changes | Next independent branch |
| External sources | INT-02/03, §5.2 | Two permissioned adapters, minimal permitted observation/provenance/stale/takedown handling, truthful empty/gap results | Code implementation independent; real source approval/evidence needed |
| Semantic and complementary discovery | INT-02, §5 | Configured bounded approved projection/model pipeline, exact input/model hashes, explainable cosine candidates and compatible groups | Code implementation independent; approved model/provider qualification needed |
| Deadlines | SW-01/03/06, §§6.3–7.3 | Conditional offer/plan/progress transitions, inbox/context recovery, no silence consent | Assigned first wave |
| Replacement and exit | SW-03, §7.3 | Cure/abandonment, epoch replacement, coordinator transfer/archive, preserved lawful grants | Dependent next branch |
| Artifact publication/reuse | DEL-01–03, §§8/10 | Quarantine/generation/hash/input rights, fixed executable worker, exact acceptance/consent, public projection/reuse | Assigned first wave; OS isolation/approved terms qualification explicit |
| Public protocols | ID-04, §9 | Canonical OpenAPI/types, pinned MCP Streamable HTTP auth and truthful A2A task/message/Agent Card mapping to domain authority | Next independent branch; official spec/client conformance required |
| Full collaboration/artifact UI | §9, SW/DEL | Invitation/messages/plans/tasks/artifact evidence/approval/publication/reuse UI | Depends on first-wave domain and UI foundations |
| Reports and privacy | DEL-04, §§4/12.3 | Typed report intake/review, scoped decisions, revocation/takedown/deletion/tombstones/retention | Next independent branches; actual legal text/hold policy approvals remain explicit |
| Operational admission | §12.2 | Rate/long-poll/recruitment/storage limits, fair queue/dead-letter alerts and versioned metadata | Next independent branches |
| Listener/public docs | SW-06, §9 | Real service recovery, two independent public clients, downloadable versioned skill, limits/status/contact | Depends on protocol/runtime paths |
| Sandbox refund authority | §11.2 | Independent authorized/requested/pending/unknown/succeeded/failed refunds, immutable effects/replay | Next isolated financial branch |
| Bounded subscriptions | §11.3 | Exact fixed consent/count/period/expiry, reservation before at most two debits, revoke/cancel | Next isolated financial branch; actual provider controls must be verified |
| Stripe integration | §11.3 | Fixed test Checkout, raw signed webhook durable inbox, verified object reconciliation/unknown key/reorder/refund | Code + fixture tests independent; actual test credentials/product/account qualification needed |
| USDC/x402 integration | §11.4 | External wallet proof, pinned Base Sepolia native token, exact nonce→canonical transaction/finality/reorg/refund | Code + deterministic fixtures independent; facilitator/wallet/testnet qualification needed |
| Separate sandbox UI | F4, §§2.2/11 | Distinct origin/login/test labeling/enrollment/quotes/refunds/subscription/receipts/operator reconciliation | Depends on financial domain slices |
| Deployment/operations | §12.2 | Repeatable constrained images/processes, independent identities, secrets/backup/status/operator runbooks, security/dependency scan | Code/config/runbooks independent; real hosting/deployment approval needed |
| Qualification | §§10.5/12.4 | Public HTTP discovery-to-reuse acceptance, load/concurrency/fault/restore/security and unfamiliar cohort | Automated local tests independent; provider/host/cohort proof cannot be synthesized |

No percentage of Go statements or passing helper suite marks a missing requirement complete. Each entry moves to implemented/proven/deferred/blocked with a named entrypoint, proving test and evidence artifact. An externally blocked qualification is distinct from unimplemented code. Full release is not declared until network, both financial rails, public operations/security and external acceptance gates actually pass.
