# Semantic discovery: public pipeline checkpoint

Requirements: INT-02/03/04 and TDD §5.4. Branch
`feat/v0.1-semantic-discovery`, not merged or release-qualified. Initial public
pipeline source88be383; current-projection/RLS/contract correction542f758;
final test fixture/withdrawal checkpoint5bcd158. Complete semantic discovery,
compatible groups and the full TDD remain unfinished.

## Actual behavior and proof

The original existing-entrypoint test failed24.841s on source135b3fe:
nonlexical public paraphrases produced no opportunity, and the configured
controlled TLS endpoint received no inputs. This was the actual missing-runtime
RED, before implementation. The endpoint vectors test mechanics, not actual
model relevance or provider qualification.

The first public pipeline passed5.038s (`-v`, without race), including the real
restricted API/worker lifecycle, existing deterministic hard-constraint
characterization and bounded provider cases. Two private objective/budget
markers were absent from endpoint input. Actual pgvector retrieval produced a
pinned semantic opportunity; no string-equality substitute was introduced.

Stronger evidence tests then found real defects in the unmerged prototype:

- Conflicting candidate revision left an old suggestion visible: actual
  RED2.730s, `/tmp/undergrid-semantic-revision-stale-red97.log`.
- Evidence exposed the full private candidate document hash rather than an
  approved public projection identity: actual RED2.491s,
  `/tmp/undergrid-semantic-public-evidence-red97.log`. Source/candidate public
  projection hashes replace those fields; private revision hashes are forbidden
  by the published response schema.
- An intermediate correction directly joined a foreign private revision in the
  API query. Actual app-role RLS correctly hid it, so the semantic lifecycle
  failed8.857s despite successful embedding calls. This was an implementation
  regression, not a reason to weaken RLS. Migration034 adds a narrow eligible
  public metadata function returning only id/profile/projection hash. Current
  input evidence is filtered before the page limit and independently checked
  again in Go; conflicting revisions no longer re-present old evidence.

The corrected runtime/provider/constraint race selection passed3.671s. Adding
independent actual HTTP validation then failed6.934s against the old closed
Compatibility schema. Canonical OpenAPI/generated types now explicitly describe
finite algorithms, model/profile pins and approved projection identities.
`additionalProperties:false` remains; independent negative oracles reject
private document fingerprints, a confidence field and missing profile identity.
Cosine similarity is explicitly not a probability or assurance.

The affected race gate passed28.386s,12 cases/subcases, no failures/skips:
`/tmp/undergrid-semantic-public-affected97.jsonl`. It includes the complete
existing real HTTP/OpenAPI artifact lifecycle, the actual semantic HTTP/Ajv
lifecycle, deterministic hard constraints and provider validation/redirect/size
cases. A selector named SharedDemand selected no standing-demand test; that
legacy case was subsequently selected explicitly, rather than counted here.

A separate actual worker-domain authority characterization passed2.328s with
the existing exact standing-demand test. While the controlled provider request
is blocked, the ordinary versioned withdrawal completes before the response is
released. Finishing that worker call leaves the intention withdrawn, monitoring
off and no embedding/opportunity. Provider I/O therefore does not hold the
principal/agent/intention authority transaction; current state is rechecked
before persistence. This is a characterization of the already implemented
fresh-state guard, not an invented additional RED.

Final public-slice race gate on the bytes committed as5bcd158 passed6.861s,
13 cases/subcases, zero failures/skips:
`/tmp/undergrid-semantic-final-public-slice97.jsonl`. It includes that withdrawal
race and the explicitly named existing exact standing-demand lifecycle. Final
network/command vet, diff checks,129-schema validation/generated-type parity and
TypeScript/Vite build passed. A provider unit-test float32/float64 mismatch and
an early undefined worker-receiver identifier failed compilation before tests;
those are setup/build failures, not behavioral RED or passing gates. All prior
logs remain separate; no all-package or release coverage percentage is claimed.

## Persistence and privileges

Only selected public summaries go to the provider in this checkpoint; public
full-text uses the approved projection. Private query consent is not implemented
and private objectives/budgets are not automatically embedded. Network and
financial identities, databases and runtime code remain separate. Model identity
and operator permission text are assertions until actual qualification.

The provider boundary checks configured HTTPS/test policy, bounded text/body,
model/revision/input hash, dimensions, finite float32 values and nonzero norm.
It rejects redirects, wrong pins and oversized responses without returning
provider error bodies or operator secrets. API keys are separate operator
secrets, never participant/network credentials. Test CA/loopback configuration
is rejected in production.

Embedding reads are autocommitted before external I/O. Persistence subsequently
locks current principal→agent→intention, rechecks revision/state/disclosure and
conditionally fills the complete immutable tuple once. The database enforces
approved public input/projection hashes and complete metadata; application
credentials cannot update embeddings. The lifecycle confirms a worker cannot
replace completed model evidence. Exact comparison and semantic comparison
share deterministic hard-constraint knowledge; semantic score cannot bypass it.

Network migrations033/034 were applied by the normal migrator on the separate
qualified pgvector97 fixture. Applied bytes are frozen:

-033 `9d3a33f0f5add520deed4d9eb6768d0eee0f6f0490730bf24a56727830b0d3df`
-034 `ec7ae88a40bfebc93420d326c2e2b77d3094de0d2cc895a1aa301cc35c60517c`

No extension catalog, migration ledger or prior SQL was replayed/faked. The
older94/96 fixtures have not been advanced to033/034; their original server
prefix lacks pgvector. Broader integration requires the actual packaged
extension available to their server binaries while preserving PGDATA/roles and
normal migration order. The separate financial95 database remains untouched.

## Still required before feature completion

Explicit private query projection consent/profile acknowledgment; per-user
bounded/logged thresholds; typed coverage/freshness/deadline/input compatibility;
complementary discovery and principal-deduped nontransitive compatible groups;
worker invalid-response/retry/model-cutover and publication races; independent
full-text/nearest-neighbor ranking and hard-conflict/unknown oracles; metadata
privacy and temporary-relation adversarial qualification; current recovery and
notifications/dismissal behavior; configured semantic watcher throughput and
shutdown gates; broader network/protocol/UI integration. Real provider/model
relevance, permission/legal policy and deployment are separate external gates.
No PR, merge, actual provider verification or full §5.4 completion is claimed.
