# Fixed JSON verifier evidence

Scope: DEL-02, TDD r2 §8.3 and malicious-artifact/resource-exhaustion threat cases. This slice implements the fixed verifier and an executable stdin/stdout boundary. It does **not** complete N4: quarantine sealing, dispatch, evaluations persistence, task epochs, rights approvals, publication and object access control require network integration.

## Runtime integration contract

Build `go build -o undergrid-verifier ./cmd/undergrid-verifier`. Send one JSON envelope on stdin with `data` and `manifest` as base64 exact bytes, and lowercase `input_sha256`/`manifest_sha256` hex digests. The dispatcher must read the sealed immutable object generation and canonical manifest bytes, never follow user URLs. Maximum envelope 15 MiB, data 10 MiB, manifest 64 KiB. No flags or job fields select executables, paths, remote schemas or libraries. Unexpected envelope fields fail closed.

One stdout JSON document reports `passed`, `failed`, `error` or `unsupported`, a stable code, actual input/manifest hashes, fixed spec hash, executable hash (`verifier_sha256`), UTC check time and source count. Trusted deployment configuration may provide `UNDERGRID_VERIFIER_IMAGE_SHA256`; it is recorded separately from the executable hash. Missing image identity remains visibly empty: dispatch must require its pinned deployed image digest before accepting production evidence. Operational exit 2 or timeout 124 means no usable evidence; exit 0 requires inspecting `result`, not treating execution itself as a pass. Retry creates/reuses the same request identity in the calling worker, not a new accepted evaluation.

The checked manifest contract follows the release fixture: title, schema_family, template_id, license_id, attribution (principal_id/text), inputs, source_declaration. Input records use artifact_id, sha256, license_id; at most 50, unique by artifact ID. That previously unspecified narrow input representation is an implementation assumption for integration review. The dispatcher must verify these declarations against immutable artifact lineage and approved rights; the checker cannot authenticate their truth. This binary compares exact bytes; it does not create a canonical legal manifest or infer authorization.

The supported `source_catalog_v1` shape comes from the release acceptance fixture: a nonempty `sources` array with unique nonempty string source_id and absolute HTTPS source_url. Ordinary additional data fields remain inert and permitted. Optional observed_at/updated_at, when declared, must be RFC3339 strings. These timestamp names and bounds (50,000 sources, 256-byte IDs, 4,096-byte URLs, depth 32, 500,000 tokens) are narrow implementation defaults, not inferred PRD business requirements. Other schemas and manifest options are unsupported. No arbitrary supplied JSON Schema, remote `$ref`, fixture coverage language or executable check is interpreted. URL references are never fetched, including localhost/private hosts; this verifier asserts syntax/completeness, not reachability, safety, freshness, license validity or factual accuracy.

## Isolation deployment requirement

The executable has a 30-second wall deadline and a 192 MiB Go GC target. These are not a substitute for OS hard limits. Build a static binary (`CGO_ENABLED=0`) into `cmd/undergrid-verifier/Containerfile`, pin the built image digest, and launch each job with:

```
docker run --rm -i --network=none --read-only --memory=256m --memory-swap=256m \
  --cpus=1 --pids-limit=32 --cap-drop=ALL --security-opt=no-new-privileges \
  --user=65532:65532 --ulimit=nofile=32:32 --ulimit=cpu=30:30 \
  -e UNDERGRID_VERIFIER_IMAGE_SHA256=<pinned-image-digest-hex> <pinned-image>
```

No volumes, Docker socket, DB credentials, network credentials or inherited host environment. The dispatcher also applies a 30-second deadline and bounds stdout to 64 KiB; abnormal termination is operational error, never failed artifact acceptance. Docker is unavailable in this workspace, so hard container isolation/resource tests remain an explicit unexecuted release gate. The actual process tests below validate binary behavior, not container security.

## Actual test evidence

- Red: `go test ./internal/verification` failed on absent Job/Verify symbols before implementation.
- Red: executable test failed on absent executable identity field before entrypoint implementation.
- Review-driven red: wrong-case SOURCES and explicit null observed_at both incorrectly passed; exact-key field decoding fixed both.
- Green: `go test ./internal/verification ./cmd/undergrid-verifier -count=1 -v` builds and launches the real executable, verifies exact hashes and prompt-shaped inert additional data, rejects duplicate keys/IDs, missing references, unsafe URL schemes, invalid timestamps, malformed/trailing JSON, depth bombs, oversized data, unsupported schema and wrong hashes.
- Static: `go vet ./internal/verification ./cmd/undergrid-verifier` and `git diff --check`.

Tests use synthetic source data, not real external ownership/rights evidence. No publication, container deployment or N4 completion is claimed.

Quality review: fixed checks own the schema knowledge; adapters only decode envelopes/record runtime identity. No DB, network client, dynamic evaluator or framework is imported. Hash evidence is separate from rights decisions. Review caught standard Go JSON case-insensitive field matching/null handling and replaced data-field extraction with exact map keys. Extra fields are validated for bounded syntax but remain inert. Manifest parsing is deliberately closed; future schema additions need versioned checks and explicit tests rather than silent interpretation.
