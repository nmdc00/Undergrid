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
function constraintLabels(raw: unknown, limit: number, minimum = 0): string[] {
  const values = labels(raw, limit);
  if (
    values.length < minimum ||
    new Set(values).size !== values.length ||
    values.some((value) => {
      const length = Array.from(value).length;
      return length < 1 || length > 128;
    })
  )
    invalid();
  return values;
}
// Check the published RFC3339 shape without normalizing evidence or losing its
// offset/fraction. JavaScript Date accepts rollovers and rejects leap seconds.
function deliveryTimestamp(raw: unknown): string {
  const value = text(raw);
  const match =
    /^(\d{4})-(\d\d)-(\d\d)[Tt](\d\d):(\d\d):(\d\d)(?:\.\d+)?([Zz]|([+-])(\d\d):(\d\d))$/.exec(
      value,
    );
  if (!match) invalid();
  const [year, month, day, hour, minute, second] = match
    .slice(1, 7)
    .map(Number);
  const zoneHour = match[9] === undefined ? 0 : Number(match[9]);
  const zoneMinute = match[10] === undefined ? 0 : Number(match[10]);
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (
    month < 1 ||
    month > 12 ||
    day < 1 ||
    day > days[month - 1] ||
    hour > 23 ||
    minute > 59 ||
    second > 60 ||
    zoneHour > 23 ||
    zoneMinute > 59
  )
    invalid();
  const offset = (match[8] === "-" ? -1 : 1) * (zoneHour * 60 + zoneMinute);
  const utcMinute = (((hour * 60 + minute - offset) % 1440) + 1440) % 1440;
  if (second === 60 && utcMinute !== 1439) invalid();
  return value;
}
function constraints(raw: unknown): ObjectValue {
  const value = closed(raw, [
    "required_fields",
    "license_ids",
    "coverage",
    "freshness",
    "delivery",
    "input_schema_families",
  ]);
  const result: ObjectValue = {
    required_fields: labels(value.required_fields, 50, true),
    license_ids: labels(value.license_ids, 20, true),
  };
  if (value.coverage != null) {
    const c = closed(value.coverage, ["required", "excluded"]);
    result.coverage = {
      required: constraintLabels(c.required, 50),
      excluded: constraintLabels(c.excluded, 50),
    };
  }
  if (value.freshness != null) {
    const f = closed(value.freshness, ["max_age_seconds"]);
    if (
      !Number.isSafeInteger(f.max_age_seconds) ||
      Number(f.max_age_seconds) < 1 ||
      Number(f.max_age_seconds) > 31536000
    )
      invalid();
    result.freshness = { max_age_seconds: f.max_age_seconds };
  }
  if (value.delivery != null) {
    const d = closed(value.delivery, ["not_before", "deliver_by"]);
    const parsed: ObjectValue = {};
    for (const key of ["not_before", "deliver_by"]) {
      if (d[key] === undefined) continue;
      parsed[key] = deliveryTimestamp(d[key]);
    }
    if (!Object.keys(parsed).length) invalid();
    result.delivery = parsed;
  }
  if (value.input_schema_families != null)
    result.input_schema_families = constraintLabels(
      value.input_schema_families,
      20,
      1,
    );
  return result;
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
