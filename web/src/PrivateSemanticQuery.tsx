import { useEffect, useRef, useState } from "react";
import type { components } from "./network.generated";
import { request, text, type ObjectValue } from "./api";
export type PrivateQuery = NonNullable<
  components["schemas"]["IntentInput"]["private_semantic_query"]
>;

export function PrivateSemanticQuery({
  csrf,
  onChange,
  initialProjection = "",
}: {
  csrf: string;
  initialProjection?: string;
  onChange: (value: PrivateQuery | undefined) => void;
}) {
  const [policy, setPolicy] = useState<ObjectValue | null>(null);
  const [projection, setProjection] = useState(initialProjection);
  const [needsReview, setNeedsReview] = useState(Boolean(initialProjection));
  const [accepted, setAccepted] = useState(false);
  const [status, setStatus] = useState("");
  const [busy, setBusy] = useState(false);
  const generation = useRef(0);
  const queryValid =
    [...projection.trim()].length >= 3 &&
    new TextEncoder().encode(projection).length <= 1000;
  useEffect(
    () => () => {
      generation.current++;
    },
    [],
  );
  async function review() {
    const mine = ++generation.current;
    setBusy(true);
    setPolicy(null);
    setAccepted(false);
    onChange(undefined);
    setStatus("");
    try {
      const p = await request("/v1/owner/matching-policy", csrf);
      if (mine !== generation.current) return;
      if (
        p.enabled !== true ||
        p.allow_private_projections !== true ||
        !/^[a-f0-9]{64}$/.test(text(p.profile_hash)) ||
        !/^[a-f0-9]{64}$/.test(text(p.privacy_policy_sha256))
      )
        throw new Error("unavailable");
      for (const field of [
        "provider_id",
        "model",
        "model_revision",
        "permission_evidence",
      ])
        if (!text(p[field]).trim()) throw new Error("unavailable");
      setPolicy(p);
    } catch {
      if (mine === generation.current)
        setStatus(
          "Private matching policy unavailable. You can create an intention without external query processing.",
        );
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  return (
    <fieldset>
      <legend>Optional private semantic matching</legend>
      <p>
        Only the query you select here is sent for external matching. Your
        private objective is not selected automatically. This does not publish
        your intention.
      </p>
      <button type="button" disabled={busy} onClick={() => void review()}>
        Review private matching policy
      </button>
      {status && <p role="status">{status}</p>}
      {needsReview && !policy && (
        <>
          <p>Existing selected query: {projection}</p>
          <label>
            <input
              type="checkbox"
              required
              checked={false}
              onChange={() => void review()}
            />
            Review current provider policy before keeping this query
          </label>
        </>
      )}
      {(policy || needsReview) && (
        <button
          type="button"
          onClick={() => {
            generation.current++;
            setBusy(false);
            setPolicy(null);
            setProjection("");
            setAccepted(false);
            setNeedsReview(false);
            onChange(undefined);
          }}
        >
          Remove private matching query
        </button>
      )}
      {policy && (
        <>
          <p>
            {text(policy.provider_id)} / {text(policy.model)} /{" "}
            {text(policy.model_revision)}
          </p>
          <p>
            Provider profile: <code>{text(policy.profile_hash)}</code>
          </p>
          <p>
            Privacy policy: <code>{text(policy.privacy_policy_sha256)}</code>
          </p>
          <p>{text(policy.permission_evidence)}</p>
          <label>
            Selected private matching query
            <textarea
              required
              ref={(node) =>
                node?.setCustomValidity(
                  queryValid
                    ? ""
                    : "Enter at least three non-whitespace characters within 1000 UTF-8 bytes.",
                )
              }
              minLength={3}
              maxLength={1000}
              value={projection}
              onChange={(e) => {
                setProjection(e.target.value);
                setAccepted(false);
                onChange(undefined);
              }}
            />
          </label>
          <label>
            <input
              type="checkbox"
              required
              checked={accepted}
              disabled={!queryValid}
              onChange={(e) => {
                setAccepted(e.target.checked);
                onChange(
                  e.target.checked
                    ? {
                        projection,
                        profile_hash: text(policy.profile_hash),
                        acknowledge_external_processing: true,
                      }
                    : undefined,
                );
              }}
            />
            Allow this selected query to be processed by this provider profile
          </label>
        </>
      )}
    </fieldset>
  );
}
