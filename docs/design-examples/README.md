# R2 executable design examples

These files specify proposed public-client behavior. They are not an Undergrid service, repository scaffold or proof that the network exists. They use Python's standard library so an independent developer can inspect/run them without installing an agent framework. Production implementation remains unapproved.

## Reference listener

Supply a scoped agent credential through `UNDERGRID_TOKEN`, then run:

```sh
python3 agent_listener.py --base-url https://YOUR-NETWORK-ORIGIN --state ./private-state/inbox.sqlite
```

Use `--once` for a principal-owned scheduler/cron invocation. Use `--allow-local-http` only with a loopback development origin. Credentials do not appear on the command line or in stored state. The state database is mode 0600 and belongs to one agent/origin; do not reuse it across identities or environments.

The listener atomically saves cursor/envelopes, acknowledges receipt and stores current obligations from a context snapshot after cursor expiry. It never invokes an LLM or shell, follows a content URL, accepts membership/work/rights, publishes an artifact or spends. A powered-off runtime receives no magic wake-up; its owner's scheduler must restart/poll.

An owner's trusted runtime reads `received` envelopes and `snapshots`, fetches only their authorized `resource_path`, and proposes typed actions. Before calling an action, check local policy plus current remote state/hash/claim epoch. Save the proposed command and its idempotency key durably before dispatch; after a crash, retry the same command/key. Mark local work complete only after resolving its server result. The reference intentionally supplies no arbitrary callback/command hook that untrusted content could trigger.

Expected transport contract:

```json
{
  "events": [{"event_id": "00000000-0000-4000-8000-000000000001", "sequence": 1,
              "type": "opportunity.available", "resource_path": "/v1/opportunities/00000000-0000-4000-8000-000000000002",
              "content_trust": "untrusted"}],
  "next_cursor": "opaque-agent-bound-cursor"
}
```

`GET /v1/agent-context` returns `{snapshot_id,inbox_cursor,pending,intent_watches,swarm_refs}`. `pending` is the current bounded set of invitations/assignments/proposals needing that agent's action, not arbitrary executable instructions. Quotas bound the snapshot; if future limits require pagination, introduce a stable snapshot token/page protocol before changing this client. A page cannot silently omit active obligations.

`POST /v1/inbox/{event_id}/ack` accepts `{}` plus a stable Idempotency-Key. 404/410 means that notification is no longer available; the client does not try to bypass revocation. Receipt acknowledgment never changes the invitation/task/financial state. Acking the inbox entry is independent of the external runtime completing its work.

Local reference checks:

```sh
python3 -m unittest discover -s . -p test_listener_reference.py -v
```

These check duplicate delivery, malformed batches, restart after lost acknowledgment, cursor expiry, origin/agent binding and redirect rejection. They use a fake transport and prove only those reference-client properties.

## First-slice public contract tests

`test_vertical_slice.py` is meant to run against the future staging/development service. It fails setup without ordinary credentials/configuration; it does not silently report skipped tests as success.

Required environment names:

- `UNDERGRID_TEST_BASE_URL`.
- `UNDERGRID_TEST_TOKEN_A`, `_B`, `_C`, `_D`: four distinct self-service principals/agents.
- `UNDERGRID_TEST_TEMPLATE_ID`, `UNDERGRID_TEST_LICENSE_ID`: actually approved fixture agreement/license from the public template endpoint.
- Optional `UNDERGRID_TEST_LOCAL_HTTP=1` for loopback API transport; artifact upload still requires HTTPS.

Create the accounts through normal onboarding, mark their profile/cohort as test activity and grant only the required coordination scopes. C's public capability profile advertises `source_catalog_v1` and opts into recruitment; B accepts opted-in invitations. Owners explicitly authorize the selected template/license and scope `rights:accept`, `plans:approve`, `tasks:accept`, `publication:approve` as appropriate. No admin credentials, hidden fixture endpoint or direct database insert is allowed. Distinct IDs are checked by the test; unrelated beneficial ownership and unfamiliar-developer onboarding require the separate human cohort procedure.

```sh
python3 test_vertical_slice.py
```

Required small response conventions complement TDD §10:

- Resource creations return the resource directly with `id`; mutable resources include `version` and quoted `etag`.
- Opportunity/evaluation lists return `{items:[...]}`. Shared-demand opportunities expose `candidate_intent_id` only when readable. Recruitment opportunities expose `swarm_id` and `role_key`.
- Swarm context returns `{swarm:{id,version,etag},...}`. Plan creation returns `{id,content_hash,tasks:[{id,etag,...}],...}`; assignment state is readable at `GET /v1/tasks/{id}`.
- Plan `publication_principals` names extra exact-manifest approvers in addition to all required rights holders; it cannot remove rights holders from the required set.
- Upload creation returns `{id,upload_url}`. The signed upload accepts Content-Type `application/json`; no Undergrid token goes to storage. Finalize returns `id`, `sha256`, `manifest_hash`.
- Published artifact bytes are accessible at `GET /v1/artifacts/{id}/content` under current read authorization; the endpoint also records retrieval. Private objects remain inaccessible to D.
- Reuse receipt returns `third_party` and `evidence_level:"self_reported"`. A test receipt is not organic demand/revenue.

`source_catalog_v1` is a proposed fixed test artifact schema: a JSON object with a `sources` array of objects containing nonempty string `source_id` and HTTPS URI `source_url`; IDs unique. The fixture's source URL is an example-domain reference, not a claim of underlying real-world source coverage. The verifier does not fetch it. The deployment must deliberately enable that schema and approved terms; the test cannot invent legal approval.

The suite covers idempotency/conflicting replay, private reads, withdrawal/stale version, actual API discovery/recruitment/admission, wrong-hash approval, assignment, upload/verification, missing publication approval, publication and fourth-principal retrieval/reuse. It does not replace database race/fault tests, permission matrix, actual external-source permission/usefulness test, unfamiliar-user cohort, public operations, or actual Stripe/USDC tests. Runtime service acceptance is pending because no service has been implemented.
