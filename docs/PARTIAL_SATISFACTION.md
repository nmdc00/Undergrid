# HP1 partial satisfaction

Work in progress on `feat/v0.1-intention-partial-satisfaction`, based on release
`81e4730`. This closes HP1 step 3/TDD §5.3 for the existing bounded JSON dataset
required-field model. It does not claim full HP1 completion or provider verification.

The new owner/agent command explicitly records an owner-declared proper subset
of current required fields as fulfilled, creates an immutable revision containing
the remaining fields and keeps the current watch active. All other input
constraints are preserved. The audit is server-owned/private and bounded to the
last action, with original/result revision links; later generic revisions carry
it unchanged as historical provenance. It is absent from public/granted views.
Current actor/source authority and original immutable report pins precede receipt
recovery. Exact retries survive their own revision but not unrelated later heads.

## Checkpoint evidence

- Actual restricted-login owner HTTP RED: 405, 0.973s;
  `/tmp/undergrid-partial-satisfaction-red96.log`.
- Initial remainder/active-watch/exact-retry GREEN: 1.326s;
  `/tmp/undergrid-partial-satisfaction-first-green96.log`.
- Five top-level adverse cases PASS, 10.030s;
  `/tmp/undergrid-partial-satisfaction-adverse96.jsonl`. Covers unchanged source
  recheck, changed claim/permission/actor, private projections, forged metadata,
  historical audit and initial enqueue-failure rollback. Subsequent stronger
  inbox-sequence/later-fanout rollback assertions remain unrun.
- UI scaffold RED: three behavioral failures, 12.41s;
  `/tmp/undergrid-partial-satisfaction-ui-red.log`. UI implementation pending.
- Actual MCP/A2A recovery RED: command/retry committed once, but task recovery
  lacked its live resource snapshot, 2.626s;
  `/tmp/undergrid-partial-satisfaction-a2a-recovery-red96.log`. Narrow new-operation
  resource reference correction pending.

Migration 041 was normally applied on isolated network fixture 96. Frozen SHA256:
`611a2ee732e5ecb3e0a1bc848ce20647e8bad36f0511febe5d189e6682abb206`.
It locks current source/observation eligibility via a narrow boolean helper;
no direct private source-table access or configuration output is granted.

Canonical request/private audit schemas are authored; generated-type parity,
final adverse/race/static checks, complete UI/build validation and review remain
pending. Controlled HTTPS data is fixture evidence only. Full scheduler/browser
HP1 walkthrough, real source qualification and deployment remain separate gates.
