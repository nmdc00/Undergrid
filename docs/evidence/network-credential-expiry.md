# Credential expiry after authority waits

TDD§§4/9/10 and threat-model current authorization. The shared network transaction boundary now rejects normalized credentials that expire while waiting for principal, agent or credential authority locks. The corrected path is `authenticate` → `lockActor`; REST/MCP/A2A and capability paths using that boundary retain one domain authorization implementation. Owner sessions and financial authority remain separate.

## Actual reproduction

The original draft fixture attempted to mutate an issued credential's expiry and was rejected by the immutable-issuance trigger; that was a fixture failure, not product-red evidence. The corrected fixture creates a **new** two-second API token on the existing agent with fixed `intents:read`/`intents:write` scopes and network audience. It uses an actual separately logged-in NOSUPERUSER/NOBYPASSRLS app role, verified through `session_user`; a separate administrator connection holds one selected authority row.

For each of principal, agent and credential locks, actual HTTP intention creation starts while the new credential is valid. `pg_blocking_pids` proves the request is blocked on the held connection; server `clock_timestamp()` confirms the immutable expiry passes before releasing the lock. No clocks, triggers, credential metadata, migration SQL or ledgers are modified.

Actual behavioral red: **FAIL7.300s** on retained NETWORK55493. All three cases returned HTTP201 after expiry. `/tmp/undergrid-credential-expiry-red.log` records those failures. Earlier wrong-DSN/immutable-update errors are not counted as product reproduction.

## Correction and recovery contract

`now()` represents transaction start, so the old locking predicate could remain valid after a wait. Replacing it with `clock_timestamp()` in the same locking WHERE would still permit predicate evaluation before waiting on the credential row. The query now materializes and locks the credential row first; the outer query checks current revocation and `expires_at > clock_timestamp()` only after that acquisition. Principal/agent ordering, scopes and existing safe401 rendering remain unchanged. This is one bounded authorization query change, with no migration or financial coupling.

Repeated focused green: **PASS15.346s**, all three wait types twice. HTTP401 leaves no intention, intent-created event, actor-scoped command receipt or queued job. An intermediate added oracle initially counted retained red-run idempotency keys globally; it was corrected to the actual command actor/key, preserving all prior data. The final regression additionally proves an ordinary still-valid credential on the same agent can create an intention normally after each expired request.

## Qualification

Final affected authorization race gate on retained NETWORK55493 passed **58cases/subcases**, zero skips/failures: network122.237s and command9.787s. It includes the strengthened three-wait regression with valid-credential controls, MCP current scope/revocation, A2A current authority/receipts and explicit independent PythonSDK1.2.2, OAuth/rotation/recovery, external lead authorization and executable routes. Log `/tmp/undergrid-credential-expiry-authorization.jsonl`.

Retained NETWORK55463 watcher/durable-worker/shutdown/obsolete-pair gate passed **31.243s**, seven cases/subcases, preserving the existing five-second old-watcher assertion and bounded shutdown. Log `/tmp/undergrid-credential-expiry-watch.jsonl`. Tests were serialized; no backlog or history was reset. `go vet ./internal/network ./cmd/undergrid`, executable build and `git diff --check` pass.

PR/release integration remains root-owned. No new migration, altered applied SQL, financial change, statement-coverage percentage, provider qualification, promotion or deployment is claimed here.

At this fix's checkpoint, a verified OAuth JWT's own `Expires` was checked before database entry but not after authority waits when its normalized local grant lived longer. That separate boundary was subsequently reproduced and corrected through the shared transaction path; see [OAuth access-token expiry evidence](oauth-access-token-expiry.md). The local-grant red/green evidence above remains scoped to normalized credential expiry.

Quality assessment: one existing transaction owner retains lock order and current scopes, with a materialization barrier explaining the otherwise subtle time/lock ordering. Behavioral tests observe real PostgreSQL waits and committed effects rather than reproducing an expiry formula. Normal valid authority remains usable; no adapter-level preflight or duplicated domain policy was added.
