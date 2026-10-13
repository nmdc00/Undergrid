# Command request resource boundary

Scope: TDD r2 sections 4, 9 and 12.2, public untrusted-input admission. Network JSON commands accept at most 64 KiB including trailing whitespace; oversize returns `413 body_too_large` before domain mutation. The parser still rejects unknown fields, trailing JSON values and truncated JSON. This is a command-body limit, not the separate 10 MiB artifact-data limit.

Red: `go test ./internal/network -run TestCommandBody -count=1 -v` reproduced an oversized valid JSON prefix followed by whitespace being accepted. The previous limited decoder interpreted its truncated reader as EOF.

Green: the same command passes after a bounded max-plus-one read followed by decoding. Memory remains bounded; there is no full unbounded body read. Exact-limit and malformed-shape cases also pass. This focused parser test requires no persistence; full integrated PostgreSQL regression remains a separate gate.

Quality assessment: the shared command decoder owns one body limit, avoiding route-specific duplication. Explicit size checking separates transport bounds from typed domain validation. No generalized validation framework or new dependency was introduced.
