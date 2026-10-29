import { ApiError, object, text } from "./api";
import { readRFC3339Timestamp } from "./rfc3339";
import type { components } from "./network.generated";
type Schemas = components["schemas"];
export type OwnerUpload = Schemas["OwnerArtifactUpload"];
export type ContributionManifest = Schemas["Manifest"];
export type TaskArtifact = Schemas["PrivateArtifact"];
export type ContributionContext = { agent: string; plan: string; task: string };
export const maxContributionBytes = 10 * 1024 * 1024;
function invalid(): never {
  throw Error("Invalid contribution response or manifest");
}
function keys(
  x: Record<string, unknown>,
  required: string[],
  optional: string[] = [],
) {
  if (
    required.some((k) => !(k in x)) ||
    Object.keys(x).some((k) => ![...required, ...optional].includes(k))
  )
    invalid();
}
export function contributionID(x: unknown) {
  const value = text(x);
  if (
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(
      value,
    )
  )
    invalid();
  return value;
}
function bounded(x: unknown, max: number, min = 1) {
  const value = text(x);
  if (value.length < min || value.length > max) invalid();
  return value;
}
function integer(x: unknown, min = 1, max = Number.MAX_SAFE_INTEGER) {
  if (typeof x !== "number" || !Number.isSafeInteger(x) || x < min || x > max)
    invalid();
  return x;
}
function hash(x: unknown, empty = false) {
  const value = text(x);
  if (!/^[0-9a-f]{64}$/.test(value) && !(empty && value === "")) invalid();
  return value;
}
function audience(x: Record<string, unknown>) {
  if (x.purpose === "output") {
    if ("audience_principal_ids" in x || "reviewer_principal_id" in x)
      invalid();
    return {};
  }
  if (
    x.purpose !== "progress" ||
    !Array.isArray(x.audience_principal_ids) ||
    x.audience_principal_ids.length < 1 ||
    x.audience_principal_ids.length > 50
  )
    invalid();
  const ids = x.audience_principal_ids.map(contributionID),
    reviewer = contributionID(x.reviewer_principal_id);
  if (new Set(ids).size !== ids.length || !ids.includes(reviewer)) invalid();
  return { audience_principal_ids: ids, reviewer_principal_id: reviewer };
}
function uploadTarget(raw: unknown, agent: string, artifact: string) {
  const url = new URL(text(raw)),
    expected = `/v1/owner/agents/${contributionID(agent)}/artifacts/${contributionID(artifact)}/upload`;
  if (
    url.origin !== window.location.origin ||
    url.username ||
    url.password ||
    url.hash ||
    url.pathname !== expected ||
    url.searchParams.size !== 1 ||
    !/^\?token=[0-9a-f]{64}$/.test(url.search)
  )
    invalid();
  return url.pathname + url.search;
}
export function ownerUpload(raw: unknown, agent: string): OwnerUpload {
  const x = object(raw);
  keys(
    x,
    [
      "id",
      "state",
      "purpose",
      "generation",
      "upload_url",
      "expires_at",
      "size_bytes",
      "media_type",
      "upload_authentication",
    ],
    ["audience_principal_ids", "reviewer_principal_id"],
  );
  if (
    x.state !== "quarantine" ||
    x.upload_authentication !== "owner_session" ||
    x.media_type !== "application/json"
  )
    invalid();
  const id = contributionID(x.id),
    extras = audience(x);
  uploadTarget(x.upload_url, agent, id);
  const expires = readRFC3339Timestamp(x.expires_at);
  if (!Number.isFinite(Date.parse(expires))) invalid();
  return {
    id,
    state: "quarantine",
    purpose: x.purpose as "output" | "progress",
    generation: contributionID(x.generation),
    upload_url: text(x.upload_url),
    expires_at: expires,
    size_bytes: integer(x.size_bytes, 1, maxContributionBytes),
    media_type: "application/json",
    upload_authentication: "owner_session",
    ...extras,
  };
}
export function contributionManifest(raw: unknown): ContributionManifest {
  const x = object(raw);
  keys(x, [
    "title",
    "schema_family",
    "template_id",
    "license_id",
    "attribution",
    "inputs",
    "source_declaration",
  ]);
  if (
    x.schema_family !== "source_catalog_v1" ||
    x.license_id !== "CC0-1.0" ||
    !Array.isArray(x.attribution) ||
    x.attribution.length < 1 ||
    x.attribution.length > 50
  )
    invalid();
  const attribution = x.attribution.map((raw) => {
    const a = object(raw);
    keys(a, ["principal_id", "text"]);
    return {
      principal_id: contributionID(a.principal_id),
      text: bounded(a.text, 4000),
    };
  });
  if (x.inputs !== null && (!Array.isArray(x.inputs) || x.inputs.length > 50))
    invalid();
  const inputs =
    x.inputs === null
      ? null
      : (x.inputs as unknown[]).map((raw) => {
          const i = object(raw);
          keys(i, ["artifact_id", "sha256", "license_id"]);
          if (i.license_id !== "CC0-1.0") invalid();
          return {
            artifact_id: contributionID(i.artifact_id),
            sha256: hash(i.sha256),
            license_id: text(i.license_id),
          };
        });
  if (
    inputs &&
    new Set(inputs.map((x) => x.artifact_id)).size !== inputs.length
  )
    invalid();
  // Preserve the published manifest field order used by the immutable server bytes.
  return {
    title: bounded(x.title, 300),
    schema_family: "source_catalog_v1",
    template_id: bounded(x.template_id, 200),
    license_id: "CC0-1.0",
    attribution,
    inputs,
    source_declaration: bounded(x.source_declaration, 4000),
  };
}
export function taskArtifact(
  raw: unknown,
  context: ContributionContext,
): TaskArtifact {
  const x = object(raw);
  keys(
    x,
    [
      "id",
      "state",
      "sha256",
      "manifest_hash",
      "size_bytes",
      "media_type",
      "license_id",
      "limitations",
      "content_trust",
      "swarm_id",
      "plan_id",
      "task_id",
      "purpose",
      "claim_epoch",
      "agreement_hash",
      "submission_attestation",
    ],
    ["manifest", "audience_principal_ids", "reviewer_principal_id"],
  );
  if (
    x.task_id !== context.task ||
    x.plan_id !== context.plan ||
    x.content_trust !== "untrusted" ||
    x.media_type !== "application/json" ||
    x.submission_attestation !==
      "authenticated_platform_submission_not_agent_signature_or_legal_ownership_proof" ||
    ![
      "quarantine",
      "sealed",
      "verified",
      "failed",
      "published",
      "held",
    ].includes(text(x.state))
  )
    invalid();
  const quarantine = x.state === "quarantine";
  if (
    quarantine &&
    ("manifest" in x ||
      x.license_id !== "" ||
      x.sha256 !== "" ||
      x.manifest_hash !== "" ||
      x.agreement_hash !== "")
  )
    invalid();
  if (!quarantine && x.license_id !== "CC0-1.0") invalid();
  return {
    id: contributionID(x.id),
    state: text(x.state),
    sha256: hash(x.sha256, quarantine),
    manifest_hash: hash(x.manifest_hash, quarantine),
    size_bytes: integer(x.size_bytes, 1, maxContributionBytes),
    media_type: "application/json",
    license_id: text(x.license_id),
    limitations: bounded(x.limitations, 4000),
    content_trust: "untrusted",
    swarm_id: contributionID(x.swarm_id),
    plan_id: contributionID(x.plan_id),
    task_id: contributionID(x.task_id),
    purpose: x.purpose as "output" | "progress",
    claim_epoch: integer(x.claim_epoch),
    agreement_hash: hash(x.agreement_hash, quarantine),
    submission_attestation: text(x.submission_attestation),
    ...audience(x),
    ...(!quarantine ? { manifest: contributionManifest(x.manifest) } : {}),
  };
}
export function taskArtifactPage(raw: unknown, context: ContributionContext) {
  const x = object(raw);
  keys(x, ["items", "next_after_id"]);
  if (!Array.isArray(x.items) || x.items.length > 100) invalid();
  const items = x.items.map((x) => taskArtifact(x, context)),
    next = text(x.next_after_id);
  if (
    new Set(items.map((x) => x.id)).size !== items.length ||
    items.some((x, i) => i > 0 && x.id <= items[i - 1].id)
  )
    invalid();
  if (
    next !== "" &&
    (items.length !== 100 ||
      contributionID(next) !== items[items.length - 1].id)
  )
    invalid();
  return { items, next_after_id: next };
}
export async function putOwnerContribution(
  upload: OwnerUpload,
  agent: string,
  csrf: string,
  bytes: Uint8Array<ArrayBuffer>,
) {
  const current = ownerUpload(upload, agent),
    path = uploadTarget(current.upload_url, agent, current.id);
  if (
    bytes.byteLength !== current.size_bytes ||
    Date.parse(current.expires_at) <= Date.now()
  )
    throw Error(
      "Upload size changed or capability expired. Review the contribution again.",
    );
  const response = await fetch(path, {
    method: "PUT",
    headers: {
      "X-CSRF-Token": csrf,
      "Content-Type": "application/octet-stream",
    },
    body: bytes,
    credentials: "same-origin",
    redirect: "error",
    cache: "no-store",
  });
  if (response.status === 204) return;
  const raw: unknown = await response.json().catch(() => null);
  let code = `HTTP ${response.status}`;
  if (raw !== null) {
    const x = object(raw);
    if (x.error) {
      const e = object(x.error);
      if (typeof e.code === "string") code = e.code;
    }
  }
  if (!response.ok) throw new ApiError(response.status, code);
  throw Error("Invalid byte-transfer receipt");
}

export function contributionEvaluations(
  raw: unknown,
  artifact: TaskArtifact,
): Schemas["Evaluation"][] {
  const page = object(raw);
  keys(page, ["items"]);
  if (!Array.isArray(page.items) || page.items.length > 100) invalid();
  const items = page.items.map((raw) => {
    const x = object(raw);
    keys(x, [
      "id",
      "result",
      "code",
      "input_sha256",
      "manifest_sha256",
      "spec_sha256",
      "image_sha256",
      "verifier_sha256",
      "runtime_isolation",
      "bubblewrap_sha256",
      "prlimit_sha256",
      "resource_limits_evidence",
      "image_identity_evidence",
      "checked_at",
      "source_count",
      "limitations",
    ]);
    if (
      !["passed", "failed", "error", "unsupported"].includes(text(x.result)) ||
      x.input_sha256 !== artifact.sha256 ||
      x.manifest_sha256 !== artifact.manifest_hash
    )
      invalid();
    return {
      id: contributionID(x.id),
      result: text(x.result),
      code: bounded(x.code, 300),
      input_sha256: hash(x.input_sha256),
      manifest_sha256: hash(x.manifest_sha256),
      spec_sha256: hash(x.spec_sha256),
      image_sha256: hash(x.image_sha256),
      verifier_sha256: hash(x.verifier_sha256),
      runtime_isolation: bounded(x.runtime_isolation, 200),
      bubblewrap_sha256:
        x.bubblewrap_sha256 === null ? null : hash(x.bubblewrap_sha256),
      prlimit_sha256: x.prlimit_sha256 === null ? null : hash(x.prlimit_sha256),
      resource_limits_evidence: bounded(x.resource_limits_evidence, 4000),
      image_identity_evidence: bounded(x.image_identity_evidence, 4000),
      checked_at: readRFC3339Timestamp(x.checked_at),
      source_count: integer(x.source_count, 0, 50000),
      limitations: bounded(x.limitations, 4000),
    };
  });
  if (new Set(items.map((x) => x.id)).size !== items.length) invalid();
  return items;
}
