# Bounded manual Stripe TEST renewals

Owning contract: TDD §§11.1–11.3. This feature extends existing owner-only immutable consent and `PrepareRenewal`; it does not create an automatic Stripe subscription. The consent still fixes one operator-selected product, maximum gross amount, credential, period, expiry and at most two renewals. The separate financial `renewal-worker` prepares each distinct payment/reservation. The Stripe worker performs manual collection only when `UNDERGRID_SANDBOX_STRIPE_RENEWALS=manual_test_v1` is explicitly configured; other nonempty values fail closed.

An owner requests a hosted payer setup with `POST /v1/owner/subscriptions/{id}/billing-setup`, `Idempotency-Key`, and only `{ "consent_hash": "<exact immutable hash>" }`. GET at the same path returns the request state and hosted setup URL. There is one request per consent. Agents cannot create it, and arbitrary customer/payment-method IDs are rejected. The API retains no provider worker key.

The worker creates a fresh TEST Customer for that actor/consent/request, then hosted Checkout `mode=setup` for that exact customer. Canonical retrieval must prove a completed matching session, a succeeded off-session SetupIntent, and a card PaymentMethod attached to the same customer. Metadata binds actor, consent, subscription and setup request throughout. This is protocol-bound payer setup evidence, not proof of legal ownership or independent beneficial ownership. No global operator payment method represents other owners.

For each prepared renewal, the actor-locked payment transaction rechecks current credential, policy version, period, quote and consent, and commits its immutable invoice claim before provider I/O. The worker creates one empty draft invoice with `auto_advance=false`, automatic tax disabled, and unrelated pending items excluded. It adds one nondiscountable unit of the fixed server-selected price. Before finalization and explicit payment, it retrieves the exact invoice graph and rechecks current payment authority inside the financial transaction. It never installs a Stripe subscription, usage meter or proration schedule.

Finalized/paid flags alone are insufficient economic evidence. Exact invoice/line/price/product/customer/consent/ordinal bindings and no credits/taxes/discounts/proration are required. Canonical invoice-payment → PaymentIntent → captured Charge relationships must match exact amount, customer and stored payment method before a unique charge fact settles the local payment. Invoice-generated charges need not inherit Checkout metadata; refunds resolve their durable invoice graph and retain existing exact refund consent/capacity checks.

Each external operation has a durable attempt stage and stable key. Lost customer, setup session or invoice creation responses are searched by their original metadata under bounded lookup; no missing or truncated observation grants a new creation attempt. Lost item/finalize/pay responses are reconciled from the original invoice. A missing qualifying result remains unknown with its original reservation/provider key. Revocation or cancellation before pay stops advancement; a valid late paid result remains recordable after revocation. Expiry of an idempotency key or local consent is not proof that an unknown charge did not occur.

Provider references: the pinned Go dependency is `stripe-go/v87@v87.0.0`, including its API version. Primary contract references are [Checkout setup](https://docs.stripe.com/api/checkout/sessions/create), [SetupIntent customer attachment](https://docs.stripe.com/api/setup_intents), [manual invoice creation](https://docs.stripe.com/api/invoices/create), [finalization](https://docs.stripe.com/api/invoices/finalize) and [explicit payment](https://docs.stripe.com/api/invoices/pay). These protocol references do not qualify an actual account or deployment.

## Evidence checkpoint

Actual owner-route RED failed0.977s (missing route404); registered-migration owner route GREEN passed4.499s. An intermediate500 was an implementation registration omission, separately retained, not a fixture/provider qualification result. Hosted setup actual RED failed1.124s (request never processed); actual tick GREEN passed1.675s. Lost customer/session responses and mismatched SetupIntent/customer/consent/payment-method/LIVE evidence checks passed9.201s.

Actual renewal collection RED failed2.165s (prepared ordinal remained authorized without a fact); canonical manual two-renewal/third-denial/replay lifecycle GREEN passed5.231s. Lost invoice/item/finalize/pay, invisible unknown invoice, paid-after-revocation and revocation/expiry/cancellation-before-pay gates passed22.470s. The fixture checks committed database attempt stages before every provider mutation.

Invoice refund integration actual corrected-fixture RED failed2.866s (ordinary Checkout metadata verifier rejected the invoice charge); canonical invoice/refund/replay GREEN passed3.254s. The first refund attempt hit a missing fixture GET route (setup failure); it is preserved separately. CLI opt-in test first failed to compile due to missing configuration parser and subsequently passed0.006s. These are actual isolated PostgreSQL95 and controlled HTTP/SDK fixture results, not real Stripe verification.

Financial migration012 owns the new setup and invoice claim records. Applied migrations001–011 and all retained financial rows remain intact. Whole financial race, new event/concurrency/mismatch cases and final independent review remain pending at this checkpoint. No real provider call, account action, charge, deployment or release promotion has occurred.

## Qualification and recovery gates

Actual approved TEST account/product/customer setup, off-session card/mandate behavior, manual finalization/collection semantics, provider test-clock cancellation/revocation and replay/refund demonstrations remain explicit qualification gates. The implementation must not be deployed or described as provider-qualified from these fixture passes. Operators must retain the original account/product/price configuration for unresolved attempts; configuration changes do not authorize a new debit or release a reservation.

Rollback stops the opted-in Stripe worker and restores code/configuration as needed. Keep append-only economic facts, the original provider keys, setup/claim references and unresolved reservations. Do not drop migration012 records or resend an unknown operation under a new key.

Signed setup/invoice event reconciliation was then proved separately: actual RED failed1.770s because Checkout setup events were treated as one-shot payments; canonical setup/invoice event GREEN passed2.738s with raw signature verification, payload amounts ignored, durable inbox deduplication and exact retrieved invoice facts. Invoice failure/unpaid events retain unknown reservation rather than triggering another payment attempt.

Privilege review found table-level app INSERT could provide worker-owned readiness fields. An actual restricted-app privilege RED failed1.033s. Financial013 narrows INSERT to owner request identity/consent columns, preserving applied012 unchanged. Normal owner request plus worker-evidence exclusion passed2.227s. The app has no UPDATE grant on provider readiness. Financial012 SHA256 is `f7c14e482ad9267aba54f0ba5552eba1a9b1d5a76f33c13f02acd19346fed38f`;013 is `10da1a479706fbb859ba382b5ecb7b640cb1fb0d6919b9bcc431d6316a5d1c57`.

The focused race gate for eleven canonical invoice/line/price/proration/tax/payment/customer/LIVE/amount mismatches and concurrent collection passed27.359s. A further invoice-credit mismatch, cross-owner/canceled replay and actual `runStripe` unsupported-opt-in check are included in the final whole financial gate. The command opt-in/runtime rejection checks pass0.006s; vet and diff checks pass before the final gate. New monetary behavior remains confined to explicitly opted-in TEST worker execution.

Quality assessment: public owner setup requests cannot supply provider identities or insert provider-ready fields. Actor locks serialize immutable consent, setup and invoice stage transitions. One shared payment-authority function owns initial and later-stage dispatch checks; canonical provider observations remain outside database transactions. Complete invoice/payment/charge graph checks own settlement; receipt replay and unknown operation stages never mint a new provider key or release allowance. Refund validation reuses that graph without requiring metadata Stripe does not inherit onto invoice charges. Lookup and per-tick batches are bounded; unresolved/mismatched observations remain explicit unknown states. The remaining full financial race and independent review are required before merge.

## Independent actor-binding correction checkpoint

The initial exact candidate `0a28c78` completed the full isolated financial race
checkpoint: sandbox192.535s, command2.213s,83 top-level tests with zero failures or
skips (`/tmp/undergrid-renewal-full-financial-race95.jsonl`). This was before the
following independent review correction and is not a pass for later source.

A genuine non-superuser/non-bypass app LOGIN could insert its own actor ID with a
different owner's subscription UUID and arbitrary consent hash, consuming that
subscription's unique billing-setup slot. The owner HTTP route already rejects
foreign subscriptions; this is a database-role boundary defect, not a claim of
ordinary HTTP SQL reachability. A separately controlled worker snapshot could
also reach the billing transition callback with the wrong actor. Both new
behavioral oracles failed2.225s before correction
(`/tmp/undergrid-renewal-actor-binding-red95.log`). The insertion oracle always
rolls back its attempted attack and never corrupts retained data.

Normal additive financial migration014 now enforces the composite
(subscription_id,actor_id) relationship against the existing subscription owner
key. The worker independently compares the stored setup, supplied snapshot and
subscription actors and consent before its transition callback. Normal owner
setup still succeeds after a rejected foreign insertion; no provider call is
made for the invalid worker snapshot. Applied012/013 were not edited or replayed.
Applied014 is frozen at SHA256
`1b1d00062ab418533ce850dfecc6551b63afa49f0d2eaf29e9c7830a886b9c04`.

Focused real financial95 billing/renewal tests passed with race detection79.942s
(`/tmp/undergrid-renewal-actor-binding-green95.log`, selector
`^TestStripe(Billing|Renewal)`). This is scoped local evidence. Remaining review
items include completed Checkout sessions with nullable URL and exact unpaid
invoice balance before pay; the whole financial gate must be rerun after those
corrections. Provider/test-clock/account, legal and deployment qualifications
remain explicitly unproven. No real provider operation or release promotion was
performed. Quality: FK and worker checks protect different trust boundaries;
request ownership, consent and provider proof remain distinct and financial
credentials/databases remain separate from network discovery.

## Completed-session recovery and unpaid-balance review correction

Stripe's [Checkout Session object](https://docs.stripe.com/api/checkout/sessions/object)
defines its URL as nullable and present only while active. Recovery now accepts a
missing URL only for a completed, otherwise exactly bound setup session. Any
supplied URL still requires the configured hosted Stripe origin; an open session
with no URL and a completed session with an unrelated URL remain unknown. Session,
customer, SetupIntent and attached-method retrieval still establish readiness;
URL availability alone grants no payment authority. Lost creation responses
recover the same session without another customer/session POST.

Stripe's [Invoice object](https://docs.stripe.com/api/invoices/object) distinguishes
amount due, already paid and remaining. Under this bounded manual test product's
contract, an unpaid invoice must have zero already paid and the exact expected
remaining amount before advancement/payment. Empty draft invoices require zero
paid/remaining. Partial payment, reduced balance and zero remaining on an open
invoice fail closed with no pay POST, no monetary fact, and the same unknown
payment/provider key/reservation period. Already-paid invoices still follow the
separate canonical historical fact path, including recovery after credential
revocation; this check does not request a replacement debit.

Actual test-first RED11.180s captured completed null-URL recovery remaining
unknown and all three unexpected open balances incorrectly reaching one pay POST
and a settled fact (`/tmp/undergrid-renewal-url-balance-red95.log`). After correction,
focused actual financial95 race tests passed34.824s with no failures/skips
(`/tmp/undergrid-renewal-url-balance-green95.jsonl`). The selector includes lost
invoice/item/finalize/pay response recovery (including known paid proof after
revocation), current revocation/expiry/cancellation before pay, completed-null URL
recovery and open/unsafe URL denials. The nullable URL and unpaid-balance review
items above are now implemented locally; the exact current full financial gate
remains required. No migration changed and no actual provider account action,
qualification, deployment or release promotion is claimed.

## Exact reconciled financial gate

Source `4e0431ab6f3c189be1a1a23006b21dbf23dce70c` (including release
`7e5ba0f`) passed the full separate financial95 race gate:
`go test -json -race -p 1 ./internal/sandbox ./cmd/undergrid-sandbox -count=1 -timeout=20m`.
Sandbox244.170s and command2.219s;88 top-level tests, zero failures/skips.
Evidence: `/tmp/undergrid-renewal-full-financial-4e0431a95.jsonl`. This supersedes
the pending full-gate statement above. Provider fixtures remain controlled local
evidence; actual Stripe qualification, release review/promotion and deployment
are not established by this result.
