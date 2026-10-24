# Versioned network agent skill

TDD §9 interoperability behavior, implementation source `1d18a6b`.

The public download returns the exact installable frontmatter-bearing source
from the built executable. It requires no database, authentication or owner
session. GET returns Markdown as an attachment, HEAD returns metadata without
a body, mutation attempts return405, and unknown skill versions/files return404
before database access. OpenAPI describes the finite public path; generated
TypeScript remains derived from that contract.

Behavioral RED for the missing route: `TestVersionedAgentSkillIsPublicDownloadWithoutDatabase`
failed with500 because the old router attempted database access (0.006s,
`/tmp/undergrid-agent-skill-red.log`). A second actual RED for an unknown version
returned500 instead of404 (0.007s, `/tmp/undergrid-agent-skill-version-red.log`).
After implementation, both behaviors and public OpenAPI retrieval pass together
with the race detector: three named tests, no database or skips, 1.167s
(`/tmp/undergrid-agent-skill-final.log`).

Skill-creator frontmatter validation passes. Canonical OpenAPI lint, compilation
of127 JSON Schema components and generated-type checks pass. TypeScript/Vite
build passes (1.66s); Go network vet and all command builds pass. An initial
build command named the nonexistent `cmd/undergrid-network` directory; the
corrected `go build ./cmd/...` completed successfully. These focused checks are
not a complete release-suite or coverage result.

Quality review: one embedded installable source avoids divergent documentation
copies; HTTP concerns stay outside authority-bearing transactions. The skill
directs clients to canonical schemas/tool discovery rather than duplicating
mutation payloads or a stale tool inventory. It preserves separate admission,
assignment, plan, rights, publication and financial authority, current
authorization on recovery, fixed verifier limitations and untrusted agent data.
No credential, installation hook, automatic runtime configuration or new
database policy is introduced. Public deployment, actual identity/provider
onboarding, independent client qualification, reporting contacts and the
unfamiliar-owner outcome remain separate requirements/gates.

Runtime wiring correction: the original focused tests above exercise the network
service directly. Review found the executable mux lacked the skill prefix, so
PR70 alone did not expose this download through the running executable. An
executable-router regression reproduced the static fallback (418 in the sentinel
fixture,0.006s, `/tmp/undergrid-agent-skill-runtime-red.log`). The correction
registers the narrow `/skills/undergrid-network/` prefix with the network service.
The actual executable route now owns GET/HEAD, mutation405 and unknown-version404;
the existing static console remains the fallback for unrelated paths. Exact
merged-release verification is required before claiming the download is wired.

Correction source `59f3787` passes seven focused tests with the race detector:
all four executable route checks (skill, OpenAPI, MCP, A2A),12.349s, and the
three public service-document checks,1.145s
(`/tmp/undergrid-agent-skill-runtime-green.log`). Command vet passes. The
regression uses a418 static fallback sentinel, so a public error response
cannot accidentally count as correct routing. Quality review keeps one narrow
prefix registration in the existing executable route inventory and reuses the
service's actual method/version/document contract without duplicate handlers.
