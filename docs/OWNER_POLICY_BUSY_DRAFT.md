# Coordination-policy draft during workspace refresh

HP2 prerequisite, owning TDD owner-policy/versioned-authority contract. Branch
`feat/v0.1-owner-policy-busy-draft` starts at release `6a93b05`.

The real owner-plan browser found that initial intention creation could still be
refreshing the owner workspace when policy inputs became editable. `OwnerPolicy`
replaces its draft when the authoritative `record` changes; edits made during
that pending refresh were discarded. Only Save was disabled.

The bounded correction disables the policy form's editable controls together
while its existing parent `busy` flag is true. Refreshed authoritative state still
loads; after completion the owner can explicitly edit and save. This changes no
server policy validation, session identity, authorization, consent or database
transaction. It does not preserve arbitrary drafts across independent later
record replacements or qualify the complete HP2 journey.

## Validation

Test-only source `afe72b8` added a behavioral test of all six input kinds,
attempted edits while busy, refresh/re-enable and subsequent explicit save. Its
first command failed because this repository does not install the proposed
`toBeDisabled` matcher: one failed/two passed, 1.54 seconds, preserved at
`/tmp/undergrid-owner-policy-afe72b8-red-oct27.log`. This was a test-fixture failure,
not behavioral RED. Test-only correction `d388d6a` uses native `:disabled` state
and input values. The corrected focused command failed one/two passed in 1.52
seconds because the busy inputs remained enabled:
`/tmp/undergrid-owner-policy-native-red-oct27.log`.

After the two-line disabled-fieldset change, `npm test --
src/OwnerPolicy.test.tsx src/App.test.tsx` passed all 26 cases in 9.69 seconds:
`/tmp/undergrid-owner-policy-focused-green-oct27.log`. This includes the existing
explicit succession default/retention cases and owner-console regressions.
The first TypeScript/build command rejected an unsupported `exact` option in the
test's role query; its failure is retained at
`/tmp/undergrid-owner-policy-build-oct27.log`. Removing that redundant option
produced final product/test source `589eadc`; the same 26 focused cases passed
in 9.59 seconds (`/tmp/undergrid-owner-policy-final-green-oct27.log`).
`npm run build` passed TypeScript and Vite production compilation on `589eadc`
(Vite 1.45 seconds), retained at
`/tmp/undergrid-owner-policy-final-build-oct27.log`. No build manifest/browser
proof is claimed by this command.
No runtime/database test is needed for this browser-only pending-state change;
the actual owner-plan browser remains a separate integration gate.

## Quality and release status

One disabled fieldset owns the form's existing shared pending-state rule; policy
knowledge stays in the existing save/domain command. No new abstraction or
network/financial dependency is introduced. Implemented and locally checked; root reviewed the bounded code/test scope.
Final documentation review and a feature PR to `release/v0.1` remain pending. The original
owner-plan browser failures remain preserved separately.
