# Financial sandbox credential expiry after authority waits

TDD §§3/4/10 and financial authority invariants require a current sandbox credential inside the financial domain transaction. Network credentials, databases and identities remain separate.

Acceptance: an actual restricted app LOGIN authorizes a quoted TEST USD payment through HTTP while another connection holds its actor or credential row. If server time reaches the newly issued immutable credential deadline before releasing that real wait, the request must return401 and leave the payment quoted, with no authority proof, reservation period, authorization receipt or economic effect. An ordinary valid credential on the actor remains usable. No provider operation occurs.

Behavioral tests authored before runtime changes. Actual red/green and affected financial regression evidence pending.
