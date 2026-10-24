import { useEffect, useRef, useState } from "react";
import { ApiError, object, request, type ObjectValue } from "./api";
export type PartialReview = {
  intentionID: string;
  leadID: string;
  revision: number;
  version: number;
  etag: string;
  sourceVersion: string;
  projectionHash: string;
  reportHash: string;
  requiredFields: string[];
};
function equalFields(value: unknown, expected: string[]): boolean {
  return (
    Array.isArray(value) &&
    value.length === expected.length &&
    value.every((field, index) => field === expected[index])
  );
}
export function PartialSatisfaction({
  review,
  csrf,
  onRecorded,
  onUnauthorized,
  onUnavailable,
}: {
  review: PartialReview;
  csrf: string;
  onRecorded: (result: ObjectValue) => void;
  onUnauthorized: () => void;
  onUnavailable: () => void;
}) {
  const [selected, setSelected] = useState<string[]>([]),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const generation = useRef(0);
  const reviewIdentity = JSON.stringify([
    review.intentionID,
    review.leadID,
    review.revision,
    review.version,
    review.etag,
    review.sourceVersion,
    review.projectionHash,
    review.reportHash,
    review.requiredFields,
  ]);
  useEffect(() => {
    generation.current++;
    setSelected([]);
    setBusy(false);
    setError("");
    return () => {
      generation.current++;
    };
  }, [reviewIdentity, csrf]);
  const fulfilled = review.requiredFields.filter((field) =>
    selected.includes(field),
  );
  const remaining = review.requiredFields.filter(
    (field) => !selected.includes(field),
  );
  async function save() {
    if (busy || !fulfilled.length || !remaining.length) return;
    const mine = ++generation.current;
    setBusy(true);
    setError("");
    try {
      const result = await request(
        `/v1/owner/intents/${encodeURIComponent(review.intentionID)}/partial-satisfaction`,
        csrf,
        "POST",
        {
          lead_id: review.leadID,
          source_version: review.sourceVersion,
          projection_hash: review.projectionHash,
          report_hash: review.reportHash,
          intention_revision: review.revision,
          fulfilled_required_fields: fulfilled,
        },
        review.etag,
        (out) => {
          // The owner consumer uses this entire revision after request clears
          // its retry key. Validate its consumed fields before acknowledging a
          // potentially committed but unusable response.
          const audit = object(out.partial_satisfaction),
            output = object(out.output),
            constraints = object(out.hard_constraints),
            watch = object(out.watch),
            preferences =
              out.preferences === undefined ? {} : object(out.preferences);
          if (out.budget !== null) object(out.budget);
          if (
            out.id !== review.intentionID ||
            !Number.isSafeInteger(out.version) ||
            out.version !== review.version + 1 ||
            out.etag !== `"${review.version + 1}"` ||
            !Number.isSafeInteger(out.current_revision) ||
            out.current_revision !== review.revision + 1 ||
            out.state !== "active" ||
            typeof out.objective !== "string" ||
            out.objective.trim().length < 3 ||
            typeof out.expires_at !== "string" ||
            !Number.isFinite(Date.parse(out.expires_at)) ||
            output.kind !== "json_dataset" ||
            output.schema_family !== "source_catalog_v1" ||
            typeof out.visibility !== "string" ||
            !["private", "restricted", "redacted_public"].includes(
              out.visibility,
            ) ||
            (out.public_summary !== undefined &&
              typeof out.public_summary !== "string") ||
            (out.visibility === "redacted_public" &&
              typeof out.public_summary !== "string") ||
            Object.values(preferences).some(
              (value) => typeof value !== "string",
            ) ||
            !Array.isArray(constraints.license_ids) ||
            constraints.license_ids.some(
              (value) => typeof value !== "string",
            ) ||
            watch.enabled !== true ||
            !equalFields(watch.channels, ["inbox"]) ||
            !equalFields(constraints.required_fields, remaining) ||
            audit.evidence_class !== "owner_declared_partial_satisfaction" ||
            audit.basis_revision !== review.revision ||
            audit.result_revision !== review.revision + 1 ||
            audit.lead_id !== review.leadID ||
            audit.source_version !== review.sourceVersion ||
            audit.projection_hash !== review.projectionHash ||
            audit.report_hash !== review.reportHash ||
            !equalFields(audit.fulfilled_required_fields, fulfilled) ||
            !equalFields(audit.remaining_required_fields, remaining)
          )
            throw new Error(
              "Invalid fulfillment response; retry the same selection to recover the result",
            );
        },
      );
      if (mine !== generation.current) return;
      setSelected([]);
      onRecorded(result);
    } catch (failure) {
      if (mine !== generation.current) return;
      if (failure instanceof ApiError && failure.status === 401)
        onUnauthorized();
      else if (
        failure instanceof ApiError &&
        [403, 404, 409].includes(failure.status)
      )
        onUnavailable();
      setError(
        failure instanceof ApiError && [403, 404, 409].includes(failure.status)
          ? "This assessment changed. Review your intention for any recorded decision before loading a new assessment."
          : failure instanceof Error
            ? failure.message
            : "The result is uncertain; retry the same selection to recover it",
      );
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  return (
    <section aria-label="Record partial fulfillment">
      <h5>Record partial fulfillment</h5>
      <p>
        Select only fields you consider fulfilled. This records your
        declaration; provider capability, identity, rights and other constraints
        remain unverified. The remaining fields stay monitored with your
        existing constraints and expiry.
      </p>
      <fieldset disabled={busy}>
        <legend>Fields I have fulfilled</legend>
        {review.requiredFields.map((field) => (
          <label key={field}>
            <input
              type="checkbox"
              checked={selected.includes(field)}
              onChange={(event) =>
                setSelected((previous) =>
                  event.target.checked
                    ? [...previous, field]
                    : previous.filter((value) => value !== field),
                )
              }
            />
            {`I have fulfilled ${field}`}
          </label>
        ))}
      </fieldset>
      <div aria-label="Unmet required fields">
        {remaining.length ? (
          <ul>
            {remaining.map((field) => (
              <li key={field}>{field}</li>
            ))}
          </ul>
        ) : (
          <p>
            Partial fulfillment must leave unmet required fields. Full
            satisfaction is a separate decision.
          </p>
        )}
      </div>
      <button
        disabled={busy || !fulfilled.length || !remaining.length}
        onClick={() => void save()}
      >
        Save unmet remainder
      </button>
      {error && <p role="alert">{error}</p>}
    </section>
  );
}
