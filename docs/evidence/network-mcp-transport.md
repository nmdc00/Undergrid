# Network MCP transport evidence

Scope: TDD §§4/9, ID-04 and the shared network command contracts. This implements the actual `/mcp` route through both Service and the executable route-registration helper, using official Go SDK v1.7.0, stateless Streamable HTTP with protocol2026-07-28. It does not implement an authorization server, financial tools, owner tools, executable resources, arbitrary agent-content interpretation, or external AS/provider onboarding.

## Runtime and authority

The finite43-operation registry in `network_commands.go` derives closed typed argument and successful response schemas from the published canonical OpenAPI document. Only reachable shared definitions appear in each tool, rather than copying117 definitions into every wire description. Known resource UUIDs, canonical request bodies, exact command-reference/If-Match headers and supported finite query fields build a request for the same domain command transaction. Tool text never chooses a method/path outside that registry. Binary artifact transport remains the existing bounded upload capability, not an executable MCP resource.

MCP and REST share `verifyNetworkRequest` for pre-lock signature validation and the transaction-owned `authenticateAgentRequest`/`executeAgentCommandTx` seam. The domain reloads and locks current principal/agent/credential authority and intersects local grant scopes with the presented signed access token. Discovery also checks current credentials in a real domain transaction. No cookie, SDK session, cached tool list or protocol request ID supplies authority. Mutations use the existing domain command-reference storage/events/jobs, not transport-only receipt acknowledgments.

The domain's existing scope checks now carry required-scope alternatives and the current local grant as error metadata; their conditions/effective scopes/error codes are unchanged. An actual rolled-back token-scope denial becomes HTTP403 with RFC6750 `insufficient_scope`, the minimum satisfiable scope and configured protected-resource metadata. A thin writer around the real SDK JSON response supplies this HTTP status before the first response byte. It does not run the command twice, manufacture a REST response, decide authorization, or buffer and replay another adapter's HTTP exchange. The ordinary SDK then owns step-up and retries the original request. No batch can combine a successful command with another command's scope challenge.

A challenge is offered only when the signed token lacks a sufficient scope alternative **and** the current local grant could satisfy it. A full token with a narrower local binding remains a tool authorization error with no false AS step-up; owner policy, membership and rights denials also remain distinct. Invalid/revoked authentication receives HTTP401. API `ug_` tokens remain explicitly optional compatibility, not OAuth interoperability qualification.

The adapter accepts only POST, validates an optional exact configured HTTPS Origin, bounds the entire message at64KiB and30 seconds, and delegates JSON-RPC version, per-request metadata and standard header consistency checks to the official SDK. Capabilities explicitly advertise fixed tools only: no default logging, tool-list-change subscription, prompts, roots, resource execution or sampling. Normal2026 discovery plus legacy2025-11-25 initialization are tested; no durable MCP sessions are claimed.

## Behavioral chronology

- Missing-route red: authenticated raw `server/discover` returned404 in17.279s on newly initialized network fixture55483.
- Runtime green: independent standard SDK clients discovered/listed tools, created and read a private intention, retried the same canonical mutation with exactly one normalized `intent.created` event and one `intention_scan_v1` cause job, and denied a foreign owner and unknown schema argument with no extra intention.
- Scope-challenge red: domain `insufficient_scope` was originally only an HTTP200 MCP tool error (2.160s), so the ordinary SDK would not invoke OAuth authorization. Focused green0.952s returned actualHTTP403, required scope and resource challenge from the same domain rejection.
- The independent SDK OAuthHandler received exactly one scope challenge, supplied an already-issued wider signed fixture access token, and the SDK retried the exact command to produce one event/job. This proves standard transport retry/step-up behavior, not AS consent, registration or selected-provider qualification.
- Actual SDK and raw HTTP full-token/local-read-binding tests denied mutation, preserved zero intentions and offered zero false authorization retries/challenges. The OR scope metadata also records accepted alternatives rather than inventing an AND requirement.
- Revoking an OAuth credential caused the next raw MCP request to return401 with resource metadata; discovery does not preserve authority after revocation.
- Header/version guards use valid per-request metadata: header mismatch is specifically JSON-RPC-32020, unsupported protocol specifically-32022, rather than accepting an unrelated400. Foreign Origin and oversized body independently deny.
- Capability red: SDK defaults advertised logging and tools.listChanged=true (0.780s). Explicit fixed-tools-only capabilities removed those unimplemented claims.

Source `fb936c7` passed the combined MCP, executable route, OAuth, owner issuance, durable participation and shared error-contract race selector: network73.440s, command7.333s. This includes an ordinary SDK negotiating2026-07-28, raw legacy2025-11-25 negotiation without an MCP session ID, and the literal executable registration helper protecting `/mcp` ahead of static fallback. Vet and build passed.

Final reconciliation source `907e656581dc2ef47e7968e6f244af6732837213` includes the reviewed financial payer authorization release, preserving separate financial008 SHA256 `5fe8b8871772fa9d0a99ccd4ae61c4ca2187b5ddf8ab1e2a0acb207695f0cfe4`. Its financial010 checksum is `d69263db35b188aa92bd749da473c315ae13ad65931a7fcad34b2cfcc45902a4`; no financial database or payment execution was used by MCP. Final focused reconciliation passed on that exact source: network65.574s and command7.041s, including the real namespace boundary, sealed executable checks and fail-closed verifier configuration. Vet and all-command build also passed on that source.

## Database and checks

A whole fresh PostgreSQL16 cluster was initialized at `/tmp/undergrid-pg-mcp-55483`, port55483, database `postgres`, UTF8/C/C. Unmodified application Migrate applied raw network001–021 and River readiness:22 ledger rows. No SQL migration was changed, skipped, replayed or ledger-edited. Subsequent runs preserved that fixture. The distinct `undergrid_mcp_api_runner` login is verified NOSUPERUSER/NOBYPASSRLS and owns no network tables; all final MCP HTTP/SDK scenarios actually use it. Underlying app/worker roles remain NOLOGIN/NOSUPERUSER/NOBYPASSRLS. Migration/admin identity is separate from runtime identity in the fixture. This is network-only proof, not production identity/deployment or independent beneficial-owner evidence.

`go vet ./internal/network ./cmd/undergrid`, `go build ./cmd/...`, formatting and `git diff --check` pass. SDK dependencies pin v1.7.0 and upgrade x/sys to the SDK-required v0.41.0; namespace/sealed executable/fail-closed verifier regressions are included in final focused reconciliation checks. Successful hard cgroup enforcement remains unqualified on this host; no host controller policy was changed.

Final selector:

```sh
GOCACHE=/tmp/undergrid-gocache GOMODCACHE=/tmp/undergrid-gomod \
UNDERGRID_TEST_DATABASE_URL='host=/tmp/undergrid-pg-mcp-55483 port=55483 user=nmcdc dbname=postgres' \
/tmp/undergrid-toolchain/go/bin/go test -race ./internal/network ./cmd/undergrid \
-run '^TestMCP|^TestExecutableRoutes|^TestBubblewrapProcessBoundaryWithoutQualification$|^TestVerifierAndLauncherBytesAreSealedBeforeExecution$|^TestSealedVerifierExecutesAndReportsItsCopiedBinaryHash$|^TestIsolatedVerifierConfigurationFailsClosedWithoutDelegatedCgroups$' \
-count=1 -timeout=10m
```

## Quality and remaining gates

DRY: canonical schemas and one neutral finite registry own command wire knowledge; shared signature/authentication/transaction helpers avoid separate adapter policy. Scope metadata comes from current domain checks, including accepted alternatives, rather than a tool-to-scope policy table. Orthogonality: SDK transport and HTTP challenge mapping are separate from normalized command execution and financial authority. Contracts: closed arguments, explicit version/capability declarations, current locks and exact retry keys remain observable. Recovery: invalid/revoked authority fails closed, step-up retries retain the command key, and no protocol session grants authority. The neutral transaction seam lets later A2A receipt persistence share the same commit without prematurely implementing that protocol here.

This focused proof does not replace the all-package integrated release gate, security/load/restore qualification, production image/storage, source permissions/legal approval, actual external OAuth-provider selection/compatibility, financial settlement or onboarding/cohort evidence. No deploy/promotion or provider qualification occurred.

Primary sources actually inspected: [official SDK release v1.7.0](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.7.0), [pinned MCP authorization](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization) (runtime insufficient-scope handling), and pinned SDK source `auth/client.go` and `mcp/streamable.go` (OAuthHandler invocation only onHTTP401/403, JSON-mode response completion).

## Network migration SHA256 manifest

```text
efb10be411cfff46746246c12fc8077cc46d1971cde7a5959ddfa18f3826435d  001_participation.sql
9e6a312d8cdb792c92bb8b88ad0becf56093b3b943d89bf83cd54b33fbe6d473  002_shared_demand.sql
e854e3c6d2ea4bf29d6dbbde8fe5761638f01926fc7abb1d7d008880ce95aa2e  003_bound_public_reads.sql
42ee55b4613f16ce8d471af6ba241f1be913a94f126a171e79058677f735eeac  004_swarms.sql
f298adb7b16409ae4f72eba8cb6d8cc28b665b378186109fc501ca67de0c8bb3  005_contributor_roles.sql
af6c33ce60ec22ae07af894e350485f58997826fd703b50de9d19471f63c74d9  006_offer_lock_visibility.sql
9a5f6ca7c514d7ce3b7243f85089a5404dacd222697474120f60dd3327b0d1c0  007_recruitment.sql
2a06e32a597e4ec950297259e23988b5d055e37899953bb3b02bd4753a7d343d  008_plans.sql
7435e0a9bb975b2eda945b0f36d0b70b657b203958d3db0529a9d047d5095fa7  009_task_withdrawal.sql
3ccd95b365441b668f28153f3eee95634840d43f69744cd16091bcc4f448afcb  010_principal_quotas.sql
b135b3a2b3490cfa16135ad6d4fce5854f69b4ad44bb9bea14e34a06ac4e6fb2  011_coordination_deadlines.sql
c6762cdd3beed62f089a3094e30ab796d7af95275efcc2217e36f93509c7d540  012_artifacts.sql
79f41576f4e380486891aecf0026952b08ffc487d2caf0948388860f18d32f4c  013_artifact_authority.sql
32ca6ad8e60fb9886a8ca2b782175359454f94205a7452dc4e86e620f9541163  014_agent_control_rotation.sql
1e8f7e0d0a08100b2cdd0abca67b1ffca8688a504cc526e908d84bc323ea05a2  015_private_artifact_grants.sql
e7c439471609dbff95f9d5f4bae48e5bda4abf43a0f90d09cc24265c71f1377b  016_task_progress_evidence.sql
9828ddeae1accf601d67123e23e5a80da6a5c5abcdc77ff6bb37e448df1e199e  017_owner_agent_issuance_recovery.sql
3c873b2a27a3bfc83669e73deee326992110a619650dfb5320d3003b4be7d537  018_verifier_isolation_evidence.sql
89bb49a9182de558293263cb5b79796858801f849351fa3d3e3b1543563e4558  019_private_report_intake.sql
2f9c2401d74bb346964267ea81fd337e0180397af086a875dfece05475398b6b  020_intent_grant_revision.sql
32e9538514d5d597dd61e94c6f9b516cd6afbf06e90af419709c1a3c644a827e  021_network_oauth_credentials.sql
```
