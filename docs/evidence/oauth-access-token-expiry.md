# OAuth access-token expiry at domain authorization

TDD ID-01/ID-03 and §§4/9/10 require current credential authority. A normalized OAuth grant can validly last longer than a subsequently issued JWT. The request must hold both local grant authority and an unexpired signed access token after acquiring authority locks. Arrival-time signature verification alone does not establish this.

Acceptance: actual signed RFC9068 JWTs issued by the existing controlled TLS issuer, with a longer-lived normalized grant, must receive HTTP401 if they expire during a real principal, agent or credential row wait. No intention, intent-created event or command receipt may persist. An unexpired JWT with the same binding must still create an intention. An actual restricted app LOGIN owns each HTTP transaction; no credential issuance metadata, SQL ledgers or clocks are rewritten.

Actual test-first red: FAIL9.113s on retained NETWORK55493, all three authority waits returned201 after the actual signed JWT expired. Log `/tmp/undergrid-oauth-expiry-red.log`. The bound local grant remained valid for one hour. The shared `authenticate` path now rechecks the signed token's own deadline after `lockActor` completes, using the same application wall clock as signature verification.

Repeated focused green: PASS20.140s (`-race -count=2`), all three real authority waits twice, with401 and no intentions, intent-created events or actor-scoped receipts; the same binding's unexpired JWT still creates an intention. Compiler-only check, vet and diff checks passed. Final combined local-grant/OAuth/protocol regression gates remain pending. No provider qualification, coverage percentage, promotion or deployment claimed.

Quality review: one shared transaction boundary owns deadline enforcement for REST/MCP/A2A; no adapter policy copy, new abstraction, migration or financial credential coupling. Signature validation still runs before database locks; the later deadline check addresses time elapsed while waiting. Current local grant validity and token scope remain independent requirements.
