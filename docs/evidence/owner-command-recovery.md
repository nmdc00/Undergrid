# Owner policy and credential command recovery

TDD §§10.1/10.3. Feature branch `feat/v0.1-owner-policy-command-recovery`, based on releasea9505e4. Remote PR, integration and promotion pending.

Policy POST and owner credential revocation now require Idempotency-Key and retain principal-scoped command receipts in the existing idempotency aggregate. The verified-owner transaction already locks the active principal. Receipt persistence commits with policy/version/event or revocation/event; failures roll back together. No new schema or adapter-level authority was introduced. Replay returns the original status/result even after another policy change; it does not undo that newer policy. Current verified active owner authority is checked before receipt access.

Owner command identity now includes the supplied If-Match, as agent commands already do. A reused key with changed body or precondition conflicts. Commands without version requirements use an empty precondition; this change adds no version requirement to credential revocation. Browser owner mutation retries retain the original key/body/precondition after transport or response-body uncertainty, using the existing bounded in-memory recovery map. Keys are not persisted as browser credentials. The canonical OpenAPI and generated TypeScript declare the newly enforced key requirements and recovery semantics.

## Evidence

Actual HTTP/PostgreSQL behavioral reds on preserved network55486: policy committed once then exact retry returned409 version_conflict; revocation succeeded but had zero command receipts (combined1.202s). Corrected focused policy/revocation/ownership race tests passed2.647s. Additional six-way same-key policy replay, invalid-command rollback and suspended-owner denial passed2.909s: exactly one receipt and one policy event. Policy replay after a later policy change returns its original result; changed body/precondition returns409. Revocation retry retains one receipt and one revocation event.

The two browser regressions failed against the original browser transport because retry generated different keys (6 prior tests passed). Corrected browser suite passed19 tests. Canonical contract lint, all117 independent JSON Schema component compilation, deterministic generated-type check, TypeScript checking and Vite build passed. Existing positive policy fixtures now supply explicit unique keys; no authorization or version assertion was weakened.

The broader owner/policy/credential/OAuth/OpenAPI/command/control network race gate passed67.973s on retained55486; final network/command vet and diff checks passed. An earlier command incorrectly named nonexistent cmd/undergrid-network and is not a passing command-package gate; actual cmd/undergrid race subsequently passed1.027s, with network vet/build passing.

## Quality and limits

Policy and revocation reuse the existing transactional owner receipt implementation, rather than duplicating recovery rules in HTTP/UI. Body and precondition fingerprints are shared with agent commands; independent database assertions check actual effects. Financial databases/credentials, policy authority and OAuth issuer verification stay separate. No migration bytes changed. Command-reference retention/tombstones, bounded cursor gaps, external identity-provider interoperability, release-wide qualification and deployment remain subsequent requirements. This is focused recovery evidence, not a passing full release gate.
