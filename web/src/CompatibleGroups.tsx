import { readDeclaredConstraints } from "./declared-constraints";
import { useEffect, useRef, useState } from "react";
import { ApiError, object, request, text, type ObjectValue } from "./api";

type Member = { intent: string; principal: string; constraints: ObjectValue };
type Group = { members: Member[]; licenses: string[]; mode: string };
type View = { groups: Group[]; truncated: boolean };
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
function invalid(): never {
  throw new Error(
    "Invalid compatible group response. Refresh your intention before retrying.",
  );
}
function closed(raw: unknown, keys: string[]) {
  const value = object(raw);
  if (Object.keys(value).some((key) => !keys.includes(key))) invalid();
  return value;
}
function labels(raw: unknown, limit: number, nullable = false): string[] {
  if (raw === null && nullable) return [];
  if (!Array.isArray(raw) || raw.length > limit) invalid();
  return raw.map((value: unknown) => text(value));
}
function identifier(raw: unknown): string {
  const value = text(raw);
  if (!uuid.test(value)) invalid();
  return value;
}
function constraints(raw: unknown): ObjectValue {
  try {
    return readDeclaredConstraints(raw);
  } catch {
    return invalid();
  }
}
function parseView(raw: ObjectValue, intention: string): View {
  const value = closed(raw, [
    "items",
    "candidate_limit",
    "group_limit",
    "truncated",
    "limitations",
  ]);
  if (
    value.candidate_limit !== 20 ||
    value.group_limit !== 10 ||
    typeof value.truncated !== "boolean" ||
    !Array.isArray(value.items) ||
    value.items.length > 10
  )
    invalid();
  const limitations = labels(value.limitations, 2);
  if (
    new Set(limitations).size !== limitations.length ||
    limitations.some(
      (item) =>
        ![
          "bounded_nonexhaustive_views",
          "principal_ids_do_not_prove_independent_owners",
        ].includes(item),
    )
  )
    invalid();
  const groups = value.items.map((raw: unknown) => {
    const group = closed(raw, [
      "members",
      "common_license_ids",
      "comparison_mode",
      "identity_assurance",
    ]);
    if (
      !Array.isArray(group.members) ||
      group.members.length < 2 ||
      group.members.length > 21 ||
      group.identity_assurance !== "common_control_unknown"
    )
      invalid();
    const mode = text(group.comparison_mode);
    if (
      ![
        "exact_declared_outcome",
        "pairwise_semantic_and_hard_constraints",
      ].includes(mode)
    )
      invalid();
    const licenses = labels(group.common_license_ids, 20);
    if (!licenses.length || new Set(licenses).size !== licenses.length)
      invalid();
    const members = group.members.map((raw: unknown): Member => {
      const member = closed(raw, [
        "intent_id",
        "principal_id",
        "hard_constraints",
      ]);
      return {
        intent: identifier(member.intent_id),
        principal: identifier(member.principal_id),
        constraints: constraints(member.hard_constraints),
      };
    });
    if (
      !members.some((member) => member.intent === intention) ||
      new Set(members.map((member) => member.intent.toLowerCase())).size !==
        members.length ||
      new Set(members.map((member) => member.principal.toLowerCase())).size !==
        members.length
    )
      invalid();
    return { members, licenses, mode };
  });
  return { groups, truncated: value.truncated };
}

export function CompatibleGroups({
  intentionID,
  csrf,
  onUnauthorized,
}: {
  intentionID: string;
  csrf: string;
  onUnauthorized: () => void;
}) {
  const [view, setView] = useState<View | null>(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [requested, setRequested] = useState(false);
  const generation = useRef(0);
  useEffect(() => {
    generation.current++;
    setView(null);
    setBusy(false);
    setError("");
    setRequested(false);
    return () => {
      generation.current++;
    };
  }, [intentionID, csrf]);
  async function load() {
    if (busy) return;
    const mine = ++generation.current;
    setBusy(true);
    setView(null);
    setError("");
    setRequested(true);
    try {
      const result = await request(
        `/v1/owner/intents/${encodeURIComponent(intentionID)}/compatible-groups`,
        csrf,
      );
      const parsed = parseView(result, intentionID);
      if (mine === generation.current) setView(parsed);
    } catch (failure) {
      if (mine !== generation.current) return;
      setView(null);
      if (failure instanceof ApiError && failure.status === 401) {
        setError("Your session expired. Sign in again.");
        onUnauthorized();
      } else if (
        failure instanceof ApiError &&
        [403, 404].includes(failure.status)
      ) {
        setError(
          "This intention is no longer available for discovery. Refresh your intentions.",
        );
      } else
        setError(
          failure instanceof Error
            ? failure.message
            : "Compatible groups could not be loaded.",
        );
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  return (
    <section aria-label="Compatible groups">
      <h4>Compatible group views</h4>
      <p>
        These are bounded, nonexhaustive views of declared requirements.
        Participant common control is unknown; principal IDs do not establish
        independent owners.
      </p>
      <p>
        Views create no membership, assignment, rights or spending authority.
        Common licenses are declared options, with consent reviewed separately.
      </p>
      <button disabled={busy} onClick={() => void load()}>
        {requested ? "Refresh compatible groups" : "View compatible groups"}
      </button>
      {error && <p role="alert">{error}</p>}
      {view && (
        <>
          {view.truncated && (
            <p role="status">
              Results are truncated: at most 20 candidates and 10 views are
              considered.
            </p>
          )}
          {view.groups.length === 0 && (
            <p>
              No current compatible groups. An empty view is a discovery gap and
              establishes no independent demand.
            </p>
          )}
          <ol>
            {view.groups.map((group, index) => (
              <li key={index}>
                <h5>Compatible view {index + 1}</h5>
                <p>
                  {group.mode === "exact_declared_outcome"
                    ? "Exact declared outcome and hard constraints"
                    : "Pairwise semantic similarity and hard constraints"}
                </p>
                <p>
                  Common declared license options: {group.licenses.join(", ")}
                </p>
                <ul>
                  {group.members.map((member) => (
                    <li key={member.intent}>
                      <p>
                        Intention: <code>{member.intent}</code>
                      </p>
                      <p>
                        Principal: <code>{member.principal}</code>
                      </p>
                      <p>Declared hard constraints:</p>
                      <pre>{JSON.stringify(member.constraints, null, 2)}</pre>
                    </li>
                  ))}
                </ul>
              </li>
            ))}
          </ol>
        </>
      )}
    </section>
  );
}
