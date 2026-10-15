# Integration baseline cd56141

On 2026-10-15, the exact `release/v0.1` commit `cd56141` passed the full Go race suite across all seven packages. There were 291 passing cases, including subtests, and two explicit host-isolation skips. Network elapsed606.332s; financial sandbox52.394s. The independent Python A2A client used `a2a-sdk==1.2.2`.

Statement coverage from the completed profile:

| Scope | Coverage |
|---|---:|
| All packages | 73.0% |
| Network domain | 75.5% |
| Financial sandbox | 75.6% |
| Identity | 59.8% |
| Verification | 71.1% |
| Network command | 4.8% |
| Financial command | 30.5% |
| Verifier command | 0.0% |

These are Go statement measurements, not a requirement-completion score. Subprocess execution does not automatically contribute coverage to its separately built executable. The verifier-host tests `TestBubblewrapEvaluationEvidencePersistsThroughWorkerAndHTTP` and `TestBubblewrapVerifierIsolationEnforcesFilesystemNetworkAndLimits` were skipped because the host lacks delegated hard resource controls. Production verifier qualification remains open.

## Reproduction and fixture boundaries

With the separate network admin, financial admin and financial runtime fixture DSNs configured as described in the README:

```sh
UNDERGRID_A2A_PYTHON=/tmp/undergrid-a2a-client-venv/bin/python \
GOCACHE=/tmp/undergrid-gocache GOMODCACHE=/tmp/undergrid-gomod \
/tmp/undergrid-toolchain/go/bin/go test -json -race \
  -coverprofile=/tmp/undergrid-release-cd56141-full.cover \
  -count=1 -timeout=30m ./... \
  > /tmp/undergrid-release-cd56141-full.jsonl 2>&1
```

The toolchain was Go1.27.2 and PostgreSQL16. Retained network cluster55463 and financial cluster55485 used separate data directories, sockets, roles and schemas. Network migrations001–025 plus River readiness and financial migrations001–010 were preserved. No applied SQL, ledgers or retained jobs were reset to produce the result.

The financial fixture login `undergrid_financial_combined_fixture` is NOSUPERUSER/NOBYPASSRLS, owns no sandbox tables and belongs to both financial group roles so tests can exercise both transactions. An actual session inspection confirmed those attributes. Production API and worker startup tests reject that combined identity. Actual restricted-login network transport/source proofs are documented in their feature evidence. This does not establish deployed identity or provider qualification.

## Earlier failures and corrections

The preceding full run at `0ad4f80` failed. Its75.5% network coverage was not passing release evidence. Root supplied an app-only financial test login where the lifecycle harness requires the explicitly combined fixture; that caused worker-role permission failures and a mismatch in the combined-login rejection test. The fixture configuration was corrected without changing production authorization. The corrected standalone financial/command run passed61 cases before the complete rerun.

The sole network failure in that earlier full run was a partial-registry source test client subscribing to the shared default queue. It consumed unrelated retained job kinds and never reached its coordinator within the existing deadline. PR58 restricted that client to the dedicated source queue, routed its coordinator there and added an independently inspected default-job sentinel. Retained Source race passed12.003s and the final focused sentinel test1.722s before the full rerun. Production queue routing was already correct. Prior rows and failed logs remain preserved.

The watcher throughput changes were also proven against a recreated258-job backlog: the original five-second watcher assertion passed three consecutive times in15.973s, and the broader worker regression passed50.548s. Earlier two-worker failures remain recorded in `watcher-pair-throughput.md`; no timing assertion was relaxed.

## Process review and remaining gates

- Learned: inspect the actual connected session, not a modified connection-config object's original connection string. Shared explicit-login assertions now cover network protocol/source fixtures.
- Testing gap closed: partial worker registries must subscribe only to their supported queues. The retained default-job sentinel proves unrelated work remains untouched.
- Documentation gap closed: document the combined financial test identity separately from isolated deployment identities, and configure it explicitly rather than falling back to the migration owner.
- Amend existing lifecycle guidance: assert queue ownership and actual login privileges when testing durable workers. A new general-purpose skill is unnecessary for these existing workflow rules.
- Automation: keep the existing local race, schema, generated-type and build gates; GitHub CI remains disabled by owner instruction. Host-specific verifier qualification requires a suitable environment rather than a substituted fixture result.

This baseline includes merged PRs53–58 and the earlier implementation. External lead retrieval, coordinator succession and USDC canonical settlement are subsequent feature branches. Fit/ranking and notifications, remaining collaboration/privacy/financial workflows, production storage, load/security/restore, real permitted sources/providers and independent public participation remain unfinished or externally gated. No develop/main promotion or production deployment is claimed.
