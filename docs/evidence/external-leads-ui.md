# Owner external-lead discovery UI

TDD §§5.2–5.4/9: an authenticated owner reviewing an active intention can explicitly retrieve currently permitted external leads, filter by origin/text, page through bounded results and review source provenance. External identity and fit remain unverified. No implicit fetch/installation/purchase or claim of independent demand follows a listing.

Scope: owner console and existing read API. Current domain authorization remains in the server. Session loss/unavailable intentions clear visible results; changing intention/revision discards stale responses. Safe links require HTTPS and explicit user action. UI fit reports, semantic ranking and persisted external opportunities/notifications remain separate work.

Actual App entrypoint behavioral red: the active intention review lacked the discovery button/section; Vitest failed the expected accessible-button query (15.79s total, 1.49s test; log `/tmp/undergrid-external-leads-ui-red.log`).

Implemented owner-only discovery component uses the existing cookie-authenticated read API, bounded20-item pages and200 visible records, source origin/publisher/public notice/check/freshness metadata, text rendering and explicit HTTPS source links. Applied search filters remain bound to pagination even while the user edits a draft. Active intention/version changes discard old asynchronous responses; unavailable/unauthorized reads clear old records. Paused/terminal/expired intentions do not offer discovery. No browser bearer keys, embedded-link fetch, purchase, installation, autonomous agent action or GET mutation is introduced.

Focused App/component tests PASS16 cases; all UI tests PASS28 in12.45s and TS/Vite build passed. Additional inactive-intention and stale401 regressions are pending final validation. Backend permission/freshness/owner authorization proof is recorded separately in external-lead API evidence; browser fixtures do not establish real publisher qualification.
