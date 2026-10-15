# OAuth access-token expiry at domain authorization

TDD ID-01/ID-03 and §§4/9/10 require current credential authority. A normalized OAuth grant can validly last longer than a subsequently issued JWT. The request must hold both local grant authority and an unexpired signed access token after acquiring authority locks. Arrival-time signature verification alone does not establish this.

Acceptance: actual signed RFC9068 JWTs issued by the existing controlled TLS issuer, with a longer-lived normalized grant, must receive HTTP401 if they expire during a real principal, agent or credential row wait. No intention, intent-created event or command receipt may persist. An unexpired JWT with the same binding must still create an intention. An actual restricted app LOGIN owns each HTTP transaction; no credential issuance metadata, SQL ledgers or clocks are rewritten.

Test-first status: behavioral test authored and compiler-only check passed. Actual red/green and affected protocol gates pending. No implementation or release claim yet.
