import { text } from "./api";
function invalid(): never {
  throw new Error("Invalid RFC3339 timestamp");
}
// Check the published RFC3339 shape without normalizing evidence or losing its
// offset/fraction. JavaScript Date accepts rollovers and rejects leap seconds.
export function readRFC3339Timestamp(raw: unknown): string {
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
