import { useEffect, useRef, useState } from "react";
import { ApiError, object, request, text, type ObjectValue } from "./api";
import { readDeclaredConstraints } from "./declared-constraints";

type Evidence = { dimension: string; status: string; reason: string };
type Candidate = {
  id: string;
  version: number;
  principal: string;
  summary: string;
  constraints: ObjectValue;
  status: string;
  outcome: string;
  evidence: Evidence[];
};
type Report = { items: Candidate[]; truncated: boolean; revision: number };
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i;
const dimensions: Record<string, string> = {
  output: "Output schema",
  license_ids: "Permitted licenses",
  required_fields: "Required fields",
  coverage: "Coverage",
  freshness: "Freshness",
  delivery: "Delivery window",
  input_schema_families: "Input schema alternatives",
};
function invalid(): never {
  throw new Error("Invalid requirement report");
}
function closed(raw: unknown, keys: string[]): ObjectValue {
  const value = object(raw);
  if (Object.keys(value).some((key) => !keys.includes(key))) invalid();
  return value;
}
function identifier(raw: unknown): string {
  const value = text(raw);
  if (!uuid.test(value)) invalid();
  return value.toLowerCase();
}
function version(raw: unknown): number {
  if (!Number.isSafeInteger(raw) || Number(raw) < 1) invalid();
  return Number(raw);
}
function parseReport(
  raw: ObjectValue,
  source: string,
  expectedVersion: number,
): Report {
  const value = closed(raw, [
    "source_intent_id",
    "source_version",
    "source_revision",
    "scope",
    "identity_assurance",
    "candidate_limit",
    "truncated",
    "items",
  ]);
  if (
    identifier(value.source_intent_id) !== source.toLowerCase() ||
    version(value.source_version) !== expectedVersion ||
    value.scope !== "registered_demand_hard_evidence_v1" ||
    value.identity_assurance !== "common_control_unknown" ||
    value.candidate_limit !== 20 ||
    typeof value.truncated !== "boolean" ||
    !Array.isArray(value.items) ||
    value.items.length > 20 ||
    (value.truncated && value.items.length !== 20)
  )
    invalid();
  const ids = new Set<string>(),
    principals = new Set<string>();
  const items = value.items.map((rawItem) => {
    const item = closed(rawItem, [
      "candidate_intent_id",
      "candidate_version",
      "principal_id",
      "public_summary",
      "output",
      "hard_constraints",
      "hard_status",
      "outcome_comparison",
      "clarification_required",
      "constraint_evidence",
    ]);
    const id = identifier(item.candidate_intent_id),
      principal = identifier(item.principal_id);
    if (id === source.toLowerCase() || ids.has(id) || principals.has(principal))
      invalid();
    ids.add(id);
    principals.add(principal);
    const summary = text(item.public_summary);
    if (Array.from(summary).length > 4000) invalid();
    const output = closed(item.output, ["kind", "schema_family"]);
    if (
      output.kind !== "json_dataset" ||
      output.schema_family !== "source_catalog_v1"
    )
      invalid();
    const constraints = readDeclaredConstraints(item.hard_constraints);
    if (
      !Array.isArray(item.constraint_evidence) ||
      item.constraint_evidence.length < 3 ||
      item.constraint_evidence.length > 7
    )
      invalid();
    const seen = new Set<string>();
    const evidence = item.constraint_evidence.map((rawEvidence) => {
      const row = closed(rawEvidence, ["dimension", "status", "reason"]),
        dimension = text(row.dimension),
        status = text(row.status);
      if (
        !Object.hasOwn(dimensions, dimension) ||
        seen.has(dimension) ||
        !["aligned", "unknown", "incompatible"].includes(status)
      )
        invalid();
      seen.add(dimension);
      return { dimension, status, reason: text(row.reason) };
    });
    if (
      !["output", "license_ids", "required_fields"].every((dimension) =>
        seen.has(dimension),
      )
    )
      invalid();
    const unknown = evidence.some((row) => row.status === "unknown"),
      conflicting = evidence.some((row) => row.status === "incompatible");
    const status = conflicting
      ? "incompatible"
      : unknown
        ? "unknown"
        : "aligned";
    if (
      item.hard_status !== status ||
      item.clarification_required !== unknown ||
      !["same_declared_outcome", "not_evaluated"].includes(
        text(item.outcome_comparison),
      )
    )
      invalid();
    return {
      id,
      version: version(item.candidate_version),
      principal,
      summary,
      constraints,
      status,
      outcome: text(item.outcome_comparison),
      evidence,
    };
  });
  return {
    items,
    truncated: value.truncated,
    revision: version(value.source_revision),
  };
}

export function ConstraintClarifications({
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
        `/v1/owner/intents/${encodeURIComponent(intentionID)}/constraint-assessments`,
        csrf,
      );
      const current = parseReport(raw, intentionID, sourceVersion);
      if (mine === generation.current) setReport(current);
    } catch (failure) {
      if (mine !== generation.current) return;
      setReport(null);
      if (failure instanceof ApiError && failure.status === 401) {
        setError("Your session expired. Sign in again.");
        onUnauthorized();
      } else if (
        failure instanceof ApiError &&
        [403, 404].includes(failure.status)
      )
        setError(
          "These requirements are no longer available. Refresh your intentions.",
        );
      else
        setError(
          "Requirement gaps could not be confirmed. Refresh your intention before retrying.",
        );
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  return (
    <section aria-label="Requirement clarification">
      <h4>Requirement clarification</h4>
      <p>
        Review a bounded sample of current public demand declarations. Common
        control is unknown; principal references do not establish independent
        owners.
      </p>
      <p>
        These views grant no membership, assignment, rights or spending
        authority. License options require separate consent.
      </p>
      <button disabled={busy} onClick={() => void load()}>
        {requested ? "Refresh requirement gaps" : "Check requirement gaps"}
      </button>
      {error && <p role="alert">{error}</p>}
      {report && (
        <>
          <p>
            Source revision {report.revision}. Refresh to recheck current
            declarations.
          </p>
          {report.truncated && (
            <p role="status">
              The sample is truncated at 20 public participant declarations.
            </p>
          )}
          {report.items.length === 0 && (
            <p role="status">
              No current public declarations in this bounded sample.
            </p>
          )}
          {report.items.map((item) => (
            <article key={item.id} aria-label={`Public request ${item.id}`}>
              <h5>{item.summary || "Public request"}</h5>
              <p>
                Request {item.id}; declaration version {item.version}.
                Participant reference {item.principal}.
              </p>
              <p>
                {item.status === "unknown"
                  ? "Needs clarification"
                  : item.status === "incompatible"
                    ? "Declared requirements conflict"
                    : "Declared requirements can combine"}
              </p>
              <p>
                {item.outcome === "not_evaluated"
                  ? "Outcome similarity is not evaluated here."
                  : "The declared outcome text agrees; delivery and participation require separate agreement."}
              </p>
              <dl>
                {item.evidence.map((row) => (
                  <div key={row.dimension}>
                    <dt>{dimensions[row.dimension]}</dt>
                    <dd>
                      {row.status === "unknown"
                        ? "Clarify this declaration before combining requirements."
                        : row.status === "incompatible"
                          ? "The declared constraints conflict."
                          : "The declared requirements can be combined."}
                    </dd>
                  </div>
                ))}
              </dl>
              <details>
                <summary>Declared requirements</summary>
                <pre>{JSON.stringify(item.constraints, null, 2)}</pre>
              </details>
            </article>
          ))}
        </>
      )}
    </section>
  );
}
