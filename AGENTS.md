# Implementation contract

Implement `docs/UNDERGRID_TDD_v0.1-r2.md` and its companions. The owner authorized implementation on 2026-10-12; older design-status banners are historical. Reserved legal, source, provider and deployment approvals remain explicit gates.

Use the installed `undergrid-implementation` skill. Write behavioral tests before changing behavior, then implement and refactor. Assess DRY (knowledge, not superficial similarity), orthogonality, contracts, readability and failure recovery on each feature. Never label fixture tests provider verification or separate account IDs independent beneficial owners.

Keep network and financial sandbox databases, credentials and deployment identities separate. Do not execute agent content. Authorization is enforced in domain transactions, not adapters. Preserve the published contract and release gates; document blockers without weakening tests.

Use one feature branch per vertical behavior from `release/v0.1`. Until an owner selects a GitHub repository, retain local branches and review evidence; remote PRs and promotion remain pending.
