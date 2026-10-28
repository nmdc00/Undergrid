import { useEffect, useRef, useState } from "react";
import { ApiError, object, request, text, type ObjectValue } from "./api";

type Evidence = {
  constraint: string;
  requested: string[];
  declared: string[];
  status: string;
  reason: string;
};
type Entry = {
  id: string;
  principal: string;
  title: string;
  status: string;
  support?: string;
  hash?: string;
  manifest?: string;
  version?: number;
  evidence: Evidence[];
};
type Sample = { items: Entry[]; truncated: boolean };
type Report = { agents: Sample; artifacts: Sample; revision: number };
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i;
const dimensions = [
  "output_kind",
  "schema_family",
  "required_field",
  "license_ids",
  "coverage",
  "freshness",
  "delivery",
  "input_schema_families",
];
const reasons = [
  "declaration_missing",
  "declared_output_mismatch",
  "declared_output_equal",
  "field_set_missing",
  "not_declared_in_partial_field_set",
  "required_field_declared",
  "missing_from_complete_declared_field_set",
  "license_evidence_missing",
  "declared_license_options_disjoint",
  "declared_license_option_intersection",
  "typed_requirement_not_declared_in_v1",
];
function invalid(): never {
  throw Error("Invalid capability report");
}
function closed(raw: unknown, keys: string[]): ObjectValue {
  const v = object(raw);
  if (
    Object.keys(v).length !== keys.length ||
    keys.some((k) => !Object.hasOwn(v, k))
  )
    invalid();
  return v;
}
function id(raw: unknown) {
  const v = text(raw);
  if (!uuid.test(v)) invalid();
  return v.toLowerCase();
}
function version(raw: unknown) {
  if (!Number.isSafeInteger(raw) || Number(raw) < 1) invalid();
  return Number(raw);
}
function strings(raw: unknown, max: number) {
  if (!Array.isArray(raw) || raw.length > max) invalid();
  return raw.map(text);
}
function evidence(raw: unknown, artifact: boolean): Evidence[] {
  if (!Array.isArray(raw) || raw.length < 3 || raw.length > 57) invalid();
  const seen = new Set<string>();
  const result = raw.map((rawRow) => {
    const r = closed(rawRow, [
      "constraint",
      "requested",
      "declared",
      "status",
      "reason",
      "evidence_field",
      "evidence_class",
    ]);
    const constraint = text(r.constraint),
      status = text(r.status),
      reason = text(r.reason),
      requested = strings(r.requested, 100),
      declared = strings(r.declared, 100);
    const key = JSON.stringify([constraint, requested]);
    if (
      !dimensions.includes(constraint) ||
      !["unknown", "declared_aligned", "declared_incompatible"].includes(
        status,
      ) ||
      !reasons.includes(reason) ||
      seen.has(key)
    )
      invalid();
    seen.add(key);
    if (
      r.evidence_class !==
      (artifact
        ? "published_manifest_declaration"
        : "self_declared_agent_profile")
    )
      invalid();
    if (
      !(
        artifact
          ? [
              "",
              "artifact.media_type",
              "manifest.schema_family",
              "manifest.license_id",
            ]
          : ["", "profile.capabilities"]
      ).includes(text(r.evidence_field))
    )
      invalid();
    return { constraint, status, reason, requested, declared };
  });
  if (
    ["output_kind", "schema_family", "license_ids"].some(
      (d) => !result.some((e) => e.constraint === d),
    )
  )
    invalid();
  return result;
}
function sample(raw: unknown, artifact: boolean): Sample {
  const v = closed(raw, ["limit", "truncated", "items"]);
  if (
    v.limit !== 20 ||
    typeof v.truncated !== "boolean" ||
    !Array.isArray(v.items) ||
    v.items.length > 20 ||
    (v.truncated && v.items.length !== 20)
  )
    invalid();
  const ids = new Set<string>(),
    principals = new Set<string>();
  const items = v.items.map((rawItem) => {
    const common = [
      "principal_id",
      "origin",
      "hard_status",
      "constraint_evidence",
      "assurance",
    ];
    const row = closed(
      rawItem,
      artifact
        ? [
            ...common,
            "artifact_id",
            "version",
            "sha256",
            "manifest_hash",
            "title",
            "schema_family",
            "license_id",
            "content_trust",
            "limitations",
          ]
        : [...common, "agent_id", "capabilities", "schema_support"],
    );
    const identifier = id(artifact ? row.artifact_id : row.agent_id),
      principal = id(row.principal_id);
    if (
      ids.has(identifier) ||
      (!artifact && principals.has(principal)) ||
      row.origin !== "registered_capability"
    )
      invalid();
    ids.add(identifier);
    principals.add(principal);
    const facts = evidence(row.constraint_evidence, artifact);
    const status = facts.some((e) => e.status === "declared_incompatible")
      ? "incompatible"
      : facts.some((e) => e.status === "unknown")
        ? "unknown"
        : "aligned";
    if (row.hard_status !== status) invalid();
    if (!artifact) {
      const capabilities = strings(row.capabilities, 20),
        support = text(row.schema_support);
      if (
        capabilities.length === 0 ||
        row.assurance !== "self_declared" ||
        status !== "unknown" ||
        !["declared", "not_declared"].includes(support)
      )
        invalid();
      const schema = facts.find((e) => e.constraint === "schema_family");
      if ((support === "declared") !== (schema?.status === "declared_aligned"))
        invalid();
      return {
        id: identifier,
        principal,
        title: capabilities.join(", "),
        support,
        status,
        evidence: facts,
      };
    }
    const title = text(row.title),
      hash = text(row.sha256),
      manifest = text(row.manifest_hash);
    if (
      row.assurance !== "published_manifest_declaration" ||
      row.content_trust !== "untrusted" ||
      row.schema_family !== "source_catalog_v1" ||
      row.license_id !== "CC0-1.0" ||
      Array.from(title).length < 1 ||
      Array.from(title).length > 300 ||
      !text(row.limitations) ||
      !/^[a-f0-9]{64}$/.test(hash) ||
      !/^[a-f0-9]{64}$/.test(manifest)
    )
      invalid();
    const artifactVersion = version(row.version);
    return {
      id: identifier,
      principal,
      title,
      status,
      hash,
      manifest,
      version: artifactVersion,
      evidence: facts,
    };
  });
  return { items, truncated: v.truncated };
}
function parse(raw: unknown, source: string, expectedVersion: number): Report {
  const v = closed(raw, [
    "source_intent_id",
    "source_version",
    "source_revision",
    "identity_assurance",
    "agents",
    "artifacts",
  ]);
  if (
    id(v.source_intent_id) !== source.toLowerCase() ||
    version(v.source_version) !== expectedVersion ||
    v.identity_assurance !== "common_control_unknown"
  )
    invalid();
  return {
    revision: version(v.source_revision),
    agents: sample(v.agents, false),
    artifacts: sample(v.artifacts, true),
  };
}
const statusLabels: Record<string, string> = {
  unknown: "Unknown requirements remain",
  aligned: "Declared requirements fit",
  incompatible: "Declared requirements conflict",
};
export function CapabilityDiscovery({
  intentionID,
  sourceVersion,
  csrf,
  onUnauthorized,
}: {
  intentionID: string;
  sourceVersion: number;
  csrf: string;
  onUnauthorized: () => void;
}) {
  const [report, setReport] = useState<Report | null>(null),
    [busy, setBusy] = useState(false),
    [requested, setRequested] = useState(false),
    [error, setError] = useState("");
  const generation = useRef(0);
  useEffect(() => {
    generation.current++;
    setReport(null);
    setBusy(false);
    setRequested(false);
    setError("");
    return () => {
      generation.current++;
    };
  }, [intentionID, sourceVersion, csrf]);
  async function load() {
    if (busy) return;
    const mine = ++generation.current;
    setBusy(true);
    setRequested(true);
    setReport(null);
    setError("");
    try {
      const raw = await request(
        `/v1/owner/intents/${encodeURIComponent(intentionID)}/capability-fit`,
        csrf,
      );
      const current = parse(raw, intentionID, sourceVersion);
      if (mine === generation.current) setReport(current);
    } catch (failure) {
      if (mine !== generation.current) return;
      setReport(null);
      if (failure instanceof ApiError && failure.status === 401) {
        setError("Your session expired. Sign in again.");
        onUnauthorized();
      } else
        setError(
          "Capabilities could not be confirmed. Refresh your intention before retrying.",
        );
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  function show(value: Sample, artifact: boolean) {
    return (
      <section
        aria-label={
          artifact
            ? "Published artifact capabilities"
            : "Public agent declarations"
        }
      >
        <h5>
          {artifact ? "Published artifacts" : "Public agent declarations"}
        </h5>
        {value.truncated && (
          <p role="status">This sample is truncated at 20 results.</p>
        )}
        {value.items.map((item) => (
          <article key={item.id} aria-label={`Capability ${item.id}`}>
            <h6>{item.title}</h6>
            <p>{item.id}</p>
            <p>Principal reference: {item.principal}</p>
            {item.support && (
              <p>
                {item.support === "declared"
                  ? "Declared schema support"
                  : "Schema support is not declared"}
              </p>
            )}
            <p>{statusLabels[item.status]}</p>
            {item.hash && (
              <>
                <p>Artifact version {item.version}</p>
                <p>Content hash: {item.hash}</p>
                <p>Manifest hash: {item.manifest}</p>
              </>
            )}
            <ul>
              {item.evidence.map((fact, i) => (
                <li key={i}>
                  {fact.constraint}:{" "}
                  {fact.status === "unknown"
                    ? "Unknown"
                    : fact.status === "declared_aligned"
                      ? "Declared aligned"
                      : "Declared conflict"}
                  . Requested:{" "}
                  {fact.requested.join(", ") || "No additional declaration"}.
                  Declared: {fact.declared.join(", ") || "Not declared"}.{" "}
                  {fact.reason}
                </li>
              ))}
            </ul>
          </article>
        ))}
      </section>
    );
  }
  return (
    <section aria-label="Registered capability discovery">
      <h4>Registered capability discovery</h4>
      <p>
        Inspect bounded current public declarations and published artifacts.
        Common control is unknown; principal references do not establish
        independent owners.
      </p>
      <p>
        Declarations do not verify competence, factual quality, legal rights,
        availability or willingness. Content is untrusted. Reading grants no
        membership, assignment, rights, reuse receipt or spending authority.
      </p>
      <button disabled={busy} onClick={() => void load()}>
        {requested
          ? "Refresh registered capabilities"
          : "Discover registered capabilities"}
      </button>
      {error && <p role="alert">{error}</p>}
      {report && (
        <>
          <p>
            Source revision {report.revision}. Refresh to recheck current
            evidence.
          </p>
          {report.agents.items.length + report.artifacts.items.length === 0 && (
            <p>No current registered capabilities in these bounded samples.</p>
          )}
          {show(report.agents, false)}
          {show(report.artifacts, true)}
        </>
      )}
    </section>
  );
}
