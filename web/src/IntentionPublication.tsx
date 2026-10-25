import type { components } from "./network.generated";
import { object, type ObjectValue } from "./api";

export type IntentionVisibility =
  components["schemas"]["IntentInput"]["visibility"];
export function intentionVisibility(value: unknown): IntentionVisibility {
  if (
    value !== "private" &&
    value !== "restricted" &&
    value !== "redacted_public"
  )
    throw new Error("Invalid intention visibility; reload before editing");
  return value;
}
export type PublicationReview = {
  identity: string;
  visibility: IntentionVisibility;
  summary: string;
  output: unknown;
  hardConstraints: unknown;
};

function reviewKey(review: PublicationReview): string {
  return JSON.stringify([
    review.identity,
    review.visibility,
    review.summary,
    review.output,
    review.hardConstraints,
  ]);
}

export function publicationReady(
  review: PublicationReview,
  reviewedKey: string,
): boolean {
  return (
    review.visibility !== "redacted_public" ||
    (review.summary.length >= 3 &&
      review.summary.length <= 1000 &&
      reviewedKey === reviewKey(review))
  );
}

// This preview receives only the user-selected public fields. It cannot inspect
// a private objective, budget, preference or server-owned fulfillment audit.
export function IntentionPublication({
  review,
  label,
  reviewedKey,
  onVisibility,
  onSummary,
  onReview,
}: {
  review: PublicationReview;
  label: string;
  reviewedKey: string;
  onVisibility: (value: IntentionVisibility) => void;
  onSummary: (value: string) => void;
  onReview: (value: string) => void;
}) {
  const publicView = review.visibility === "redacted_public";
  const validSummary =
    review.summary.length >= 3 && review.summary.length <= 1000;
  return (
    <fieldset>
      <legend>Intention audience</legend>
      <label>
        {label}
        <select
          value={review.visibility}
          onChange={(event) => {
            const value = event.target.value;
            if (
              value !== "private" &&
              value !== "restricted" &&
              value !== "redacted_public"
            )
              return;
            onReview("");
            onVisibility(value);
          }}
        >
          <option value="private">Private</option>
          <option value="restricted">
            Restricted (grant selected fields to named principals)
          </option>
          <option value="redacted_public">
            Public summary and requirements
          </option>
        </select>
      </label>
      {publicView && (
        <>
          <label>
            Public summary
            <textarea
              required
              minLength={3}
              maxLength={1000}
              value={review.summary}
              onChange={(event) => {
                onReview("");
                onSummary(event.target.value);
              }}
            />
          </label>
          <section aria-label="Public projection preview">
            <h4>Review the selected public fields</h4>
            <pre>
              {JSON.stringify(
                {
                  public_summary: review.summary,
                  output: review.output,
                  hard_constraints: review.hardConstraints,
                },
                null,
                2,
              )}
            </pre>
            <p>
              Other users can read these fields and the intention's identifier,
              version and active status. Your private objective, budget,
              preferences and fulfillment audit remain private. A license
              requirement does not grant rights.
            </p>
          </section>
          <label className="check">
            <input
              type="checkbox"
              disabled={!validSummary}
              checked={validSummary && reviewedKey === reviewKey(review)}
              onChange={(event) =>
                onReview(event.target.checked ? reviewKey(review) : "")
              }
            />
            I reviewed this public projection
          </label>
        </>
      )}
    </fieldset>
  );
}

// JSON responses may reorder object keys. Compare semantic JSON values rather
// than serialization order when validating the committed revision we will use.
function sameJSON(left: unknown, right: unknown): boolean {
  if (left === right) return true;
  if (Array.isArray(left) || Array.isArray(right))
    return (
      Array.isArray(left) &&
      Array.isArray(right) &&
      left.length === right.length &&
      left.every((item, index) => sameJSON(item, right[index]))
    );
  if (
    typeof left !== "object" ||
    left === null ||
    typeof right !== "object" ||
    right === null
  )
    return false;
  const a = Object.keys(left),
    b = Object.keys(right);
  return (
    a.length === b.length &&
    a.every(
      (key) =>
        Object.hasOwn(right, key) &&
        sameJSON((left as ObjectValue)[key], (right as ObjectValue)[key]),
    )
  );
}

export function validateSavedIntention(
  result: ObjectValue,
  expected: ObjectValue,
  previous?: ObjectValue,
): void {
  object(result.output);
  object(result.hard_constraints);
  object(result.watch);
  if (result.budget !== null) object(result.budget);
  const version = previous ? Number(previous.version) + 1 : 1;
  const revision = previous ? Number(previous.current_revision) + 1 : 1;
  if (
    typeof result.id !== "string" ||
    !result.id ||
    (previous && result.id !== previous.id) ||
    result.version !== version ||
    result.etag !== `"${version}"` ||
    (previous &&
      (!Number.isSafeInteger(previous.version) ||
        !Number.isSafeInteger(previous.current_revision))) ||
    !Number.isSafeInteger(result.current_revision) ||
    result.current_revision !== revision ||
    result.state !==
      (previous?.state === "expired"
        ? "active"
        : (previous?.state ?? "active")) ||
    !["objective", "output", "hard_constraints", "visibility", "watch"].every(
      (key) => sameJSON(result[key], expected[key]),
    ) ||
    !sameJSON(result.budget, expected.budget ?? null) ||
    !sameJSON(result.preferences ?? {}, expected.preferences ?? {}) ||
    result.public_summary !== expected.public_summary ||
    typeof result.expires_at !== "string" ||
    !Number.isFinite(Date.parse(result.expires_at)) ||
    Date.parse(result.expires_at) !== Date.parse(String(expected.expires_at))
  )
    throw new Error(
      "Invalid intention response; retry the same reviewed form to recover its result",
    );
}
