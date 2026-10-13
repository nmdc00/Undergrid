# Network credential issuance key and recovery

API startup requires `UNDERGRID_CREDENTIAL_ISSUANCE_KEY_ID` and a dedicated random
32-byte base64 `UNDERGRID_CREDENTIAL_ISSUANCE_KEY`. Generate/store it in the network
secret manager, independently of browser-session, cursor, financial sandbox and
other deployment keys. There is no built-in key. Startup rejects missing/invalid
keys and session/cursor reuse. The worker does not need this key. Do not print the
key or include it in fixtures, repository files, process command arguments or logs.

The key derives an `ug_` bearer through HMAC-SHA256 over a fresh persisted 32-byte
nonce plus immutable credential/principal/agent ID, audience, generation, exact
scopes and expiry. Only the credential digest is used for API authentication.
The command result table (`idempotency`) stores nonsecret credential references,
not a token. Normalized issuance metadata is immutable; revocation is terminal.

Rotation/recovery replay lasts 15 minutes and requires a currently verified owner,
active target agent/principal, live unexpired nonsuperseded credential, current
scope policy and the matching issuance key. Restart with the same protected key
recovers a lost response. Old/revoked/expired/superseded replays fail closed. After
an unavailable replay, refresh and use explicit owner recovery for that agent;
it revokes all live agent credentials and returns a limited replacement. Network
recovery never recovers or authorizes a financial sandbox identity.

## Planned key rollover

1. Preserve the active issuance key in the network secret manager and encrypted
   network recovery backup. Database backups alone must not reveal bearer tokens.
2. Pause new owner credential rotations/recoveries at the network ingress while
   ordinary agent requests continue. Wait at least 15 minutes after the last
   issuance so no published replay window is cut short.
3. Generate a new independent random 32-byte key and new stable key ID; install
   both through the secret manager, then restart the API. Do not rekey the session,
   cursor or sandbox secrets as a side effect.
4. Resume owner issuance. Exercise ordinary owner self-service rotation/replay on
   a dedicated labeled operator test agent, verify old token denial and exact
   new-token replay, and revoke the test credential afterward. Do not log tokens.
5. Existing agent credentials remain authenticated by their digests until owner
   rotation/revocation/expiry. Earlier replay results deliberately become
   unavailable once their key is no longer configured. No old key is loaded by
   default. Retain encrypted old keys only under the approved incident/backup
   retention policy; destroy them when their required recovery window ends.

## Suspected key compromise or lost key

Stop issuing new credentials and rotate the dedicated key immediately. A party
with the old key and issuance metadata can reconstruct old HMAC credentials;
authentication must not be assumed safe just because the DB stores digests.
Identify affected `credentials.issuance_key_id` rows under restricted operator
access, require owner recovery/revocation of those credentials, and use the
approved incident process for any operator suspension. Do not broaden scopes,
change ownership, infer legal independence or reuse sandbox authority to recover
network access. Losing the key makes replay unavailable but does not by itself
revoke already issued tokens; owner recovery explicitly does so.

## Retention and verification

The app refuses bearer regeneration after `issuance_replay_until`; metadata stays
immutable for audit/lineage. Existing command-result retention must continue to
apply to reference-only results. Never extend a replay window by editing DB
metadata or disabling its immutability trigger. If an approved retention job later
purges expired results or old credentials, preserve existing FK lineage/evidence
and fail closed on missing replay data; do not add plaintext token caches.

Safe operator checks expose counts and flags only:

```sql
SELECT issuance_key_id,count(*)
FROM undergrid.credentials WHERE issuance_nonce IS NOT NULL
GROUP BY issuance_key_id;
SELECT count(*) FROM undergrid.credentials
WHERE issuance_replay_until>now() AND revoked_at IS NULL;
SELECT count(*) FROM undergrid.idempotency
WHERE operation LIKE '%/rotate' OR operation LIKE '%/recover-credential';
```

Concrete owner HTTP receipt/replay/revocation checks are the release contract;
health checks alone do not establish these effects. External provider signup,
production deployment and operator retention-policy approval remain explicit gates.
