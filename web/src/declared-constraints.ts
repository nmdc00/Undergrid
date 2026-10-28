import { object, text, type ObjectValue } from "./api";

function invalid(): never {
  throw new Error("Invalid declared constraints");
}
function closed(raw: unknown, keys: string[]): ObjectValue {
  const value = object(raw);
  if (Object.keys(value).some((key) => !keys.includes(key))) invalid();
  return value;
}
function labels(raw: unknown, limit: number, nullable = false): string[] {
  if (raw === null && nullable) return [];
  if (!Array.isArray(raw) || raw.length > limit) invalid();
  return raw.map((value) => text(value));
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
export function readDeclaredConstraints(raw: unknown): ObjectValue {
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
