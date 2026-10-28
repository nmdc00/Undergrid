import { object, text } from "./api";
import { readRFC3339Timestamp } from "./rfc3339";
function invalid(): never {
  throw new Error("Invalid task execution response");
}
function id(raw: unknown) {
  const x = text(raw);
  if (
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(x)
  )
    invalid();
  return x;
}
function integer(x: unknown) {
  if (typeof x !== "number" || !Number.isSafeInteger(x) || x < 1) invalid();
  return x;
}
function timestamp(x: unknown) {
  const value = readRFC3339Timestamp(x);
  if (!Number.isFinite(Date.parse(value))) invalid();
  return value;
}
function nullableTime(x: unknown) {
  return x === null ? null : timestamp(x);
}
function nullableID(x: unknown) {
  return x === null ? null : id(x);
}
function strings(x: unknown, max: number) {
  if (x === null) return [];
  if (!Array.isArray(x) || x.length > max) invalid();
  return x.map(text);
}
export function executionTask(raw: unknown) {
  const x = object(raw),
    keys = [
      "id",
      "plan_id",
      "assigned_agent_id",
      "state",
      "version",
      "etag",
      "claim_epoch",
      "response_deadline",
      "progress_deadline",
      "cure_deadline",
      "submitted_artifact_id",
      "credited_progress_evidence_id",
      "dependency_blocked",
      "specification",
    ];
  if (Object.keys(x).length !== keys.length || keys.some((k) => !(k in x)))
    invalid();
  const v = integer(x.version),
    epoch = integer(x.claim_epoch);
  if (x.etag !== `"${v}"` || typeof x.dependency_blocked !== "boolean")
    invalid();
  const state = text(x.state);
  if (
    ![
      "open",
      "offered",
      "accepted",
      "in_progress",
      "submitted",
      "accepted_output",
      "changes_requested",
      "at_risk",
      "withdrawn",
      "abandoned",
      "expired",
      "cancelled",
    ].includes(state)
  )
    invalid();
  const spec = object(x.specification),
    required = [
      "key",
      "assignee_agent_id",
      "depends_on",
      "output_schema",
      "checks",
      "progress_interval_seconds",
    ];
  if (
    required.some((k) => !(k in spec)) ||
    Object.keys(spec).some((k) => ![...required, "deadline"].includes(k))
  )
    invalid();
  text(spec.key);
  if (id(spec.assignee_agent_id) !== id(x.assigned_agent_id)) invalid();
  text(spec.output_schema);
  strings(spec.depends_on, 50);
  strings(spec.checks, 20);
  integer(spec.progress_interval_seconds);
  if (spec.deadline !== undefined && spec.deadline !== null)
    timestamp(spec.deadline);
  return {
    id: id(x.id),
    plan_id: id(x.plan_id),
    assigned_agent_id: id(x.assigned_agent_id),
    state,
    version: v,
    etag: text(x.etag),
    claim_epoch: epoch,
    response_deadline: timestamp(x.response_deadline),
    progress_deadline: nullableTime(x.progress_deadline),
    cure_deadline: nullableTime(x.cure_deadline),
    submitted_artifact_id: nullableID(x.submitted_artifact_id),
    credited_progress_evidence_id: nullableID(x.credited_progress_evidence_id),
    dependency_blocked: x.dependency_blocked,
    specification: spec,
  };
}
export type ExecutionTask = ReturnType<typeof executionTask>;
