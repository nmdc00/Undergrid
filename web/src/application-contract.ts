import { object, text } from "./api";
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
function fail(): never {
  throw new Error("Invalid application response");
}
function closed(raw: unknown, keys: string[]) {
  const x = object(raw);
  if (Object.keys(x).length !== keys.length || keys.some((k) => !(k in x)))
    fail();
  return x;
}
export function applicationID(x: unknown) {
  const v = text(x);
  if (!uuid.test(v)) fail();
  return v;
}
function version(x: unknown) {
  if (typeof x !== "number" || !Number.isSafeInteger(x) || x < 1) fail();
  return x;
}
function choice<T extends string>(x: unknown, values: readonly T[]): T {
  const v = text(x);
  const found = values.find((i) => i === v);
  if (!found) fail();
  return found;
}
function digest(x: unknown) {
  const v = text(x);
  if (!/^[0-9a-f]{64}$/.test(v)) fail();
  return v;
}
function role(raw: unknown) {
  const x = closed(raw, ["key", "schema_family", "description"]);
  return {
    key: text(x.key),
    schema_family: choice(x.schema_family, ["source_catalog_v1"]),
    description: text(x.description),
  };
}
function refs(raw: unknown) {
  if (raw === null) return null;
  if (!Array.isArray(raw) || raw.length > 20) fail();
  return raw.map(text);
}
function claims(raw: unknown) {
  if (raw === null) return null;
  const x = closed(raw, [
    "schema",
    "role_key",
    "capability_refs",
    "role",
    "swarm_version",
  ]);
  return {
    schema: choice(x.schema, ["application-claims-v1"]),
    role_key: text(x.role_key),
    capability_refs: refs(x.capability_refs),
    role: role(x.role),
    swarm_version: version(x.swarm_version),
  };
}
export function readRecruitment(raw: unknown) {
  const x = closed(raw, [
    "id",
    "kind",
    "origin",
    "swarm_id",
    "objective",
    "visibility",
    "swarm_version",
    "swarm_etag",
    "role",
    "input_hash",
    "evidence",
    "history_policy",
    "identity_assurance",
    "content_trust",
  ]);
  const v = version(x.swarm_version);
  if (x.swarm_etag !== `"${v}"`) fail();
  const e = closed(x.evidence, ["algorithm", "reason", "assurance"]);
  choice(e.algorithm, ["declared-capability-v1"]);
  choice(e.reason, ["declared_schema_capability"]);
  choice(e.assurance, ["self_declared"]);
  choice(x.kind, ["builder_recruitment"]);
  choice(x.origin, ["registered_capability"]);
  choice(x.history_policy, ["full"]);
  choice(x.identity_assurance, ["common_control_unknown"]);
  choice(x.content_trust, ["untrusted"]);
  return {
    id: applicationID(x.id),
    swarm_id: applicationID(x.swarm_id),
    objective: text(x.objective),
    visibility: choice(x.visibility, ["public", "restricted"]),
    swarm_version: v,
    swarm_etag: text(x.swarm_etag),
    role: role(x.role),
    input_hash: digest(x.input_hash),
  };
}
export function readApplication(raw: unknown) {
  const x = closed(raw, [
    "id",
    "swarm_id",
    "agent_id",
    "principal_id",
    "state",
    "version",
    "etag",
    "respond_by",
    "role_key",
    "application_claims",
    "claims_state",
    "application_claims_sha256",
    "objective",
    "visibility",
    "swarm_version",
    "history_policy",
    "content_trust",
  ]);
  const v = version(x.version);
  if (x.etag !== `"${v}"`) fail();
  const deadline = text(x.respond_by);
  if (!Number.isFinite(Date.parse(deadline))) fail();
  choice(x.history_policy, ["full"]);
  choice(x.content_trust, ["untrusted"]);
  const c = claims(x.application_claims),
    state = choice(x.claims_state, [
      "included",
      "historical_unknown",
      "retained_over_budget",
    ]),
    hash =
      x.application_claims_sha256 === null
        ? null
        : digest(x.application_claims_sha256);
  if (
    (state === "included") !== (c !== null) ||
    (state === "historical_unknown") !== (hash === null)
  )
    fail();
  const key = x.role_key === null ? null : text(x.role_key);
  if (c && (c.role_key !== key || c.role.key !== key)) fail();
  return {
    id: applicationID(x.id),
    swarm_id: applicationID(x.swarm_id),
    agent_id: applicationID(x.agent_id),
    principal_id: applicationID(x.principal_id),
    state: choice(x.state, ["applied", "offered"]),
    version: v,
    etag: text(x.etag),
    respond_by: deadline,
    role_key: key,
    application_claims: c,
    claims_state: state,
    application_claims_sha256: hash,
    objective: text(x.objective),
    visibility: choice(x.visibility, ["public", "restricted", "private"]),
    swarm_version: version(x.swarm_version),
  };
}
export function readApplicationPage<T>(
  raw: unknown,
  read: (x: unknown) => T & { id: string },
  recruitment = false,
) {
  const x = closed(
    raw,
    recruitment
      ? ["items", "next_after", "candidate_limit"]
      : ["items", "next_after"],
  );
  if (recruitment && x.candidate_limit !== 50) fail();
  if (!Array.isArray(x.items) || x.items.length > (recruitment ? 50 : 100))
    fail();
  const items = x.items.map(read);
  if (new Set(items.map((i) => i.id)).size !== items.length) fail();
  const next = text(x.next_after);
  if (next) applicationID(next);
  return { items, next };
}
export type RecruitmentReview = ReturnType<typeof readRecruitment>;
export type ApplicationReview = ReturnType<typeof readApplication>;
