# Response ETag headers

TDD10.3: successful mutable-resource responses with an existing top-level body etag now emit the same HTTP ETag. Domain commands continue to own If-Match authorization/version checks. Lists, compound snapshots and errors receive no guessed validator. Idempotent replay emits the original response version rather than relabeling it with newer persisted state.

Actual HTTP red: TestResponseETagTracksVersionAndOriginalReplay created an intention successfully with body etag\"1\" but no response header (0.828s). After implementation, create/read/edit/original replay/stale conflict/foreign private denial and existing revision/error tests passed under-race2.930s on retained001–020 network fixture55463. Vet/network command build passed. No migration or authorization change.

The renderer copies the domain-owned validator at one boundary; no adapter recomputes a resource version. This preserves idempotency and avoids extra database reads or leaking private versions through error replies. Current resources lacking a top-level body etag are outside this header-only change and must be documented accurately by the public schema. Full release gate remains pending.
