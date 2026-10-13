# Stripe bound recovery — pending review

This working branch repairs missed-webhook recovery and object binding in the financial sandbox. It is not merged or provider-qualified.

Five PostgreSQL behavioral regressions failed before implementation: a known closed Checkout session remained pending; an unknown closed session without a redirect URL could not recover; an unrelated canceled PaymentIntent could release a reservation; a canceled refund remained pending; and a valid charge behind an invalid enclosing session lacked an explicit anomaly record.

The implementation polls pending sessions, accepts closed sessions without active redirect URLs, requires the stored Checkout association before cancellation can release a reservation, and treats canceled refunds as terminal failures. Canonical charge retrieval is separated from ledger mutation. Valid financial facts behind mismatched wrappers are persisted atomically with an owner-visible anomaly rather than silently discarded.

The older cancellation fixture had no stored Checkout association. Its assertion now requires rejection; a separate bound-session lifecycle proves the authorized release path.

Recorded checks after shared effect-validation refactoring: financial packages under the race detector passed (sandbox 41.989s; sandbox command 1.109s), `go vet ./...` passed, and `go build ./...` passed. Tests used the restricted financial fixture login against `undergrid_sandbox_stripe_recovery` on port 55440. These are SDK HTTP fixtures and real PostgreSQL lifecycle checks, not Stripe provider verification.

Quality assessment: canonical object validation is distinct from persistence; actor locks and existing reservation contracts remain in place; anomalous monetary facts and their classification share a transaction. Final peer review, exact committed-head checks, and integration checks remain pending.

Remaining financial work includes owner-command idempotency, cancellation after credential revocation, controlled subscription invoice collection, comprehensive unexpected-charge/fee/receipt reconciliation, and Base Sepolia/x402 settlement. No provider credentials, external payments, or deployment were used.
