import { PartialSatisfaction, type PartialReview } from "./PartialSatisfaction";
import { useEffect, useRef, useState } from "react";
import { ApiError, object, request, text, type ObjectValue } from "./api";

type Constraint = {
  name: string;
  requested: string[];
  declared: string[];
  status: string;
  reason: string;
};
type Report = {
  revision: number;
  assessment: string;
  claimStatus: string;
  constraints: Constraint[];
  partialReview?: PartialReview;
};
const assessments: Record<string, string> = {
  declared_aligned: "Declared alignment",
  incompatible: "Declared mismatch",
  needs_clarification: "Needs clarification",
};
const statuses: Record<string, string> = {
  declared_aligned: "Declared alignment",
  declared_incompatible: "Declared mismatch",
  unknown: "Unknown",
};
const names: Record<string, string> = {
  output_kind: "Output type",
  schema_family: "Schema family",
  required_field: "Required field",
  license_ids: "License options",
};
const explanations: Record<string, string> = {
  declaration_missing: "No declaration available.",
  field_set_missing: "No field list declared.",
  declared_output_mismatch: "Publisher lists a different output.",
  declared_output_equal: "Publisher lists the requested output.",
  required_field_declared: "Publisher lists this field.",
  missing_from_complete_declared_field_set:
    "Field is absent from the publisher's complete list.",
  not_declared_in_partial_field_set:
    "Publisher's partial list does not declare this field.",
  license_evidence_missing: "License information needs clarification.",
  declared_license_options_disjoint: "Declared license options do not overlap.",
  declared_license_option_intersection: "A declared license option matches.",
};
function strings(value: unknown): string[] {
  if (!Array.isArray(value) || value.length > 100)
    throw new Error("Invalid constraint evidence");
  return value.map(text);
}
function parseReport(
  raw: ObjectValue,
  intention: string,
  lead: string,
): Report {
  if (
    raw.intention_id !== intention ||
    object(raw.lead).id !== lead ||
    !Number.isInteger(raw.intention_revision) ||
    Number(raw.intention_revision) < 1 ||
    raw.verification !== "unverified_publisher_claim" ||
    raw.identity_assurance !== "unknown" ||
    raw.rights_assurance !== "unknown"
  )
    throw new Error("Invalid report identity");
  const assessment = text(raw.assessment),
    claimStatus = text(raw.claim_status);
  if (
    !Object.hasOwn(assessments, assessment) ||
    !["declared", "missing", "invalid"].includes(claimStatus) ||
    !Array.isArray(raw.constraints) ||
    raw.constraints.length > 128
  )
    throw new Error("Invalid report shape");
  const constraints = raw.constraints.map((value) => {
    const row = object(value),
      name = text(row.constraint),
      status = text(row.status);
    if (!Object.hasOwn(names, name) || !Object.hasOwn(statuses, status))
      throw new Error("Invalid constraint status");
    return {
      name,
      requested: strings(row.requested),
      declared: strings(row.declared),
      status,
      reason: text(row.reason),
    };
  });
  let partialReview: PartialReview | undefined;
  if (raw.intention_etag !== undefined || raw.intention_version !== undefined) {
    const source = object(raw.lead),
      requiredFields = constraints
        .filter((row) => row.name === "required_field")
        .flatMap((row) => row.requested);
    const projectionHash = text(source.projection_hash),
      reportHash = text(raw.report_hash),
      sourceVersion = text(source.source_version),
      version = Number(raw.intention_version),
      etag = text(raw.intention_etag);
    if (
      !Number.isSafeInteger(version) ||
      version < 1 ||
      etag !== `"${version}"` ||
      !sourceVersion.length ||
      !/^[a-f0-9]{64}$/.test(projectionHash) ||
      !/^[a-f0-9]{64}$/.test(reportHash) ||
      requiredFields.some((field) => !field.trim()) ||
      new Set(requiredFields).size !== requiredFields.length
    )
      throw new Error("Invalid fulfillment review pins");
    partialReview = {
      intentionID: intention,
      leadID: lead,
      revision: Number(raw.intention_revision),
      version,
      etag,
      sourceVersion,
      projectionHash,
      reportHash,
      requiredFields,
    };
  }
  return {
    revision: Number(raw.intention_revision),
    assessment,
    claimStatus,
    constraints,
    partialReview,
  };
}
export function ExternalFitReport({
  intentionID,
  leadID,
  csrf,
  onUnauthorized,
  onUnavailable,
  onClose,
  onRecorded,
  onDecisionUnavailable,
}: {
  intentionID: string;
  leadID: string;
  csrf: string;
  onUnauthorized: () => void;
  onUnavailable: () => void;
  onClose: () => void;
  onRecorded?: (result: ObjectValue) => void;
  onDecisionUnavailable?: () => void;
}) {
  const [report, setReport] = useState<Report | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const generation = useRef(0);
  async function load() {
    const mine = ++generation.current;
    setReport(null);
    setError("");
    setBusy(true);
    try {
      const query = new URLSearchParams({ lead_id: leadID });
      const result = await request(
        `/v1/owner/intents/${encodeURIComponent(intentionID)}/external-fit-report?${query}`,
        csrf,
      );
      const next = parseReport(result, intentionID, leadID);
      if (mine === generation.current) setReport(next);
    } catch (failure) {
      if (mine !== generation.current) return;
      if (failure instanceof ApiError && failure.status === 401)
        onUnauthorized();
      else if (
        failure instanceof ApiError &&
        (failure.status === 403 || failure.status === 404)
      )
        onUnavailable();
      else setError("The assessment could not be loaded. Try again.");
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  useEffect(() => {
    void load();
    return () => {
      generation.current++;
    };
  }, [intentionID, leadID, csrf]);
  return (
    <section aria-label="Selected lead assessment">
      <h5>Selected lead assessment</h5>
      <p>
        Assessment uses unverified publisher declarations. Identity, rights and
        availability are unverified.
      </p>
      {busy && <p role="status">Loading declared fit…</p>}
      {error && <p role="alert">{error}</p>}
      {report && (
        <>
          <h5>{assessments[report.assessment]}</h5>
          <p>Intention revision {report.revision}</p>
          {report.claimStatus !== "declared" && (
            <p>
              Usable publisher declarations are missing. Clarification is
              required.
            </p>
          )}
          <table>
            <caption>Declared constraints</caption>
            <thead>
              <tr>
                <th>Requirement</th>
                <th>Requested</th>
                <th>Publisher declaration</th>
                <th>Assessment</th>
                <th>Explanation</th>
              </tr>
            </thead>
            <tbody>
              {report.constraints.map((row, index) => (
                <tr key={`${row.name}:${index}`}>
                  <th scope="row">{names[row.name]}</th>
                  <td>{row.requested.join(", ") || "Not specified"}</td>
                  <td>{row.declared.join(", ") || "Not declared"}</td>
                  <td>{statuses[row.status]}</td>
                  <td>
                    {Object.hasOwn(explanations, row.reason)
                      ? explanations[row.reason]
                      : "Review the source declaration."}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {report.partialReview && (
            <PartialSatisfaction
              review={report.partialReview}
              csrf={csrf}
              onRecorded={(result) => {
                setReport(null);
                onRecorded?.(result);
              }}
              onUnauthorized={onUnauthorized}
              onUnavailable={onDecisionUnavailable ?? onUnavailable}
            />
          )}
        </>
      )}
      <button type="button" disabled={busy} onClick={() => void load()}>
        Reload assessment
      </button>
      <button type="button" onClick={onClose}>
        Close assessment
      </button>
    </section>
  );
}
