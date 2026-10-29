import { object, text, type ObjectValue } from "./api";
import {
  planDetails,
  planSummary,
  planInteger as positive,
  planStrings as strings,
} from "./plan-contract";
import type { components } from "./network.generated";

export type PlanInput = components["schemas"]["PlanSpec"];
export type TaskInput = components["schemas"]["TaskSpec"];
export type ProposalContext = components["schemas"]["OwnerPlanProposalContext"];
export type Participant = components["schemas"]["OwnerPlanParticipant"];
export type TemplateChoice = {
  id: string;
  sha256: string;
  license_ids: string[];
  terms_url: string;
  test_only: boolean;
  input_grant_policy_hash: string;
};
export type TaskDraft = Omit<TaskInput, "deadline" | "depends_on"> & {
  deadline: string;
  depends_on: string[];
  originalDeadline?: string;
};
export type Draft = Omit<PlanInput, "tasks"> & {
  tasks: TaskDraft[];
  originalRespondBy?: string;
};

export function proposalContext(
  value: unknown,
  roomID: string,
): ProposalContext {
  const raw = object(value),
    room = object(raw.swarm),
    version = positive(room.version);
  if (
    room.id !== roomID ||
    room.etag !== `"${version}"` ||
    room.state !== "active" ||
    raw.limit !== 100 ||
    !Array.isArray(raw.items) ||
    raw.items.length > 100
  )
    throw Error("Invalid plan context");
  const items = raw.items.map((value): Participant => {
    const row = object(value),
      role = text(row.role);
    if (role !== "coordinator" && role !== "contributor" && role !== "observer")
      throw Error("Invalid participant role");
    return {
      id: text(row.id),
      agent_id: text(row.agent_id),
      principal_id: text(row.principal_id),
      role,
    };
  });
  const next = text(raw.next_after);
  if (
    next &&
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(next)
  )
    throw Error("Invalid participant cursor");
  return {
    swarm: { id: roomID, version, etag: text(room.etag), state: "active" },
    latest_plan: raw.latest_plan === null ? null : planSummary(raw.latest_plan),
    items,
    next_after: next,
    limit: 100,
  };
}
export function enabledTemplates(value: ObjectValue): TemplateChoice[] {
  const raw = object(value.contribution_templates).templates;
  if (!Array.isArray(raw)) throw Error("Invalid contribution templates");
  return raw.map((value) => {
    const row = object(value),
      id = text(row.id),
      sha256 = text(row.sha256),
      license_ids = strings(row.license_ids),
      terms_url = text(row.terms_url);
    const url = new URL(terms_url);
    if (
      !id ||
      !/^[a-f0-9]{64}$/.test(sha256) ||
      !license_ids.length ||
      typeof row.test_only !== "boolean" ||
      url.protocol !== "https:" ||
      url.username ||
      url.password
    )
      throw Error("Invalid enabled contribution template");
    const inputHash =
      row.input_grant_policy_hash === undefined
        ? ""
        : text(row.input_grant_policy_hash);
    if (inputHash && !/^[a-f0-9]{64}$/.test(inputHash))
      throw Error("Invalid input grant policy hash");
    return {
      id,
      sha256,
      license_ids,
      terms_url,
      test_only: row.test_only,
      input_grant_policy_hash: inputHash,
    };
  });
}
export function utcEditor(value: string): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) throw Error("Invalid plan deadline");
  return date.toISOString().slice(0, -1);
}
function utcInstant(value: string): string {
  if (!value) throw Error("A plan response deadline is required");
  const date = new Date(value + "Z");
  if (!Number.isFinite(date.getTime())) throw Error("Invalid plan deadline");
  return date.toISOString();
}
// Native date controls display milliseconds. Preserve the validated source ISO
// when that display is unchanged; the immutable plan may contain finer precision.
function preservedInstant(
  editor: string,
  original: string | undefined,
): string {
  return original && editor === utcEditor(original)
    ? original
    : utcInstant(editor);
}
function exactInstant(value: string): string {
  const match =
    /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/.exec(
      value,
    );
  if (!match) throw Error("Invalid exact plan deadline");
  const wholeSecond = Date.parse(match[1] + match[3]);
  if (!Number.isFinite(wholeSecond)) throw Error("Invalid exact plan deadline");
  return `${wholeSecond / 1000}.${(match[2] ?? "").padEnd(9, "0")}`;
}
export function emptyTask(index: number): TaskDraft {
  return {
    key: `task-${index}`,
    assignee_agent_id: "",
    depends_on: [],
    output_schema: "source_catalog_v1",
    checks: ["json_schema_v1"],
    progress_interval_seconds: 86400,
    deadline: "",
  };
}
export function newDraft(): Draft {
  return {
    parent_version: null,
    objective: "",
    required_principals: [],
    acceptance_principals: [],
    publication_principals: [],
    rights: {
      template_id: "",
      license_id: "",
      publication: "exact_manifest_approval",
    },
    tasks: [emptyTask(1)],
    respond_by: utcEditor(new Date(Date.now() + 86400000).toISOString()),
  };
}
export function inputFromDraft(draft: Draft): PlanInput {
  const { originalRespondBy, ...specification } = draft;
  return {
    ...specification,
    respond_by: preservedInstant(draft.respond_by, originalRespondBy),
    tasks: draft.tasks.map((task) => {
      const { deadline, originalDeadline, ...spec } = task;
      return deadline
        ? { ...spec, deadline: preservedInstant(deadline, originalDeadline) }
        : spec;
    }),
  };
}
function parsedInput(value: unknown): PlanInput {
  const raw = object(value),
    rights = object(raw.rights);
  if (raw.parent_version !== null) positive(raw.parent_version);
  if (!Array.isArray(raw.tasks)) throw Error("Invalid plan tasks");
  const tasks = raw.tasks.map((value) => {
    const task = object(value),
      progress = positive(task.progress_interval_seconds);
    if (
      task.output_schema !== "source_catalog_v1" ||
      !Array.isArray(task.checks) ||
      task.checks.length !== 1 ||
      task.checks[0] !== "json_schema_v1"
    )
      throw Error("Invalid supported task output or checks");
    const result: TaskInput = {
      key: text(task.key),
      assignee_agent_id: text(task.assignee_agent_id),
      depends_on: task.depends_on === null ? null : strings(task.depends_on),
      output_schema: "source_catalog_v1",
      checks: ["json_schema_v1"],
      progress_interval_seconds: progress,
    };
    if (task.deadline != null) result.deadline = text(task.deadline);
    return result;
  });
  if (rights.publication !== "exact_manifest_approval")
    throw Error("Invalid publication mode");
  return {
    parent_version:
      raw.parent_version === null ? null : positive(raw.parent_version),
    objective: text(raw.objective),
    required_principals: strings(raw.required_principals),
    acceptance_principals: strings(raw.acceptance_principals),
    publication_principals:
      raw.publication_principals === null
        ? null
        : strings(raw.publication_principals),
    rights: {
      template_id: text(rights.template_id),
      license_id: text(rights.license_id),
      publication: "exact_manifest_approval",
    },
    tasks,
    respond_by: text(raw.respond_by),
  };
}
function inputIdentity(spec: PlanInput): string {
  return JSON.stringify({
    ...spec,
    respond_by: exactInstant(spec.respond_by),
    tasks: spec.tasks.map((task) => ({
      ...task,
      ...(task.deadline ? { deadline: exactInstant(task.deadline) } : {}),
    })),
  });
}
export function revisionDraft(value: unknown, context: ProposalContext): Draft {
  const plan = planDetails(value),
    latest = context.latest_plan;
  if (
    !latest ||
    plan.id !== latest.id ||
    plan.swarm_id !== context.swarm.id ||
    plan.version !== latest.version ||
    plan.content_hash !== latest.content_hash
  )
    throw Error("The latest plan changed. Reload the plan context.");
  const spec = parsedInput(plan.specification);
  return {
    ...spec,
    parent_version: latest.version,
    respond_by: utcEditor(spec.respond_by),
    originalRespondBy: spec.respond_by,
    tasks: spec.tasks.map((task) => ({
      ...task,
      depends_on: task.depends_on ?? [],
      deadline: task.deadline ? utcEditor(task.deadline) : "",
      originalDeadline: task.deadline,
    })),
  };
}
export function draftProblem(
  spec: PlanInput,
  members: Participant[],
  template: TemplateChoice | undefined,
): string {
  const bytes = (value: string) => new TextEncoder().encode(value).length;
  if (bytes(spec.objective) < 3 || bytes(spec.objective) > 4000)
    return "Enter a plan objective of 3–4000 bytes.";
  if (!template || !template.license_ids.includes(spec.rights.license_id))
    return "Select an enabled contribution template and license.";
  const deadline = Date.parse(spec.respond_by),
    now = Date.now();
  if (
    !Number.isFinite(deadline) ||
    deadline <= now ||
    deadline > now + 7 * 86400000
  )
    return "Set a future response deadline within seven days.";
  const required = spec.required_principals,
    acceptance = spec.acceptance_principals,
    publication = spec.publication_principals ?? [];
  const contributors = new Set(
    members
      .filter((row) => row.role !== "observer")
      .map((row) => row.principal_id),
  );
  const principals = new Set(members.map((row) => row.principal_id));
  if (
    !required.length ||
    !acceptance.length ||
    [required, acceptance, publication].some(
      (set) => set.length > 50 || new Set(set).size !== set.length,
    )
  )
    return "Select bounded, distinct required and acceptance principals.";
  if (
    required.some((id) => !contributors.has(id)) ||
    acceptance.some((id) => !required.includes(id)) ||
    publication.some((id) => !principals.has(id))
  )
    return "Review the current member and principal selections; load more participants when needed.";
  if (!spec.tasks.length || spec.tasks.length > 50)
    return "A plan requires 1–50 tasks.";
  const keys = spec.tasks.map((task) => task.key);
  if (
    new Set(keys).size !== keys.length ||
    keys.some((key) => bytes(key) < 1 || bytes(key) > 80)
  )
    return "Task keys must be distinct and 1–80 bytes.";
  let edges = 0;
  for (const task of spec.tasks) {
    const member = members.find(
      (row) =>
        row.agent_id === task.assignee_agent_id && row.role !== "observer",
    );
    if (!member || !required.includes(member.principal_id))
      return "Each task needs a current contributor whose principal must approve the plan.";
    if (
      task.output_schema !== "source_catalog_v1" ||
      task.checks.length !== 1 ||
      task.checks[0] !== "json_schema_v1" ||
      !Number.isInteger(task.progress_interval_seconds) ||
      task.progress_interval_seconds < 60 ||
      task.progress_interval_seconds > 604800
    )
      return "Review supported task output, checks and progress intervals.";
    if (
      task.deadline &&
      (!Number.isFinite(Date.parse(task.deadline)) ||
        Date.parse(task.deadline) <= now)
    )
      return "Optional task deadlines must be in the future.";
    edges += (task.depends_on ?? []).length;
  }
  if (edges > 100) return "A plan supports at most 100 dependency edges.";
  const visiting = new Set<string>(),
    done = new Set<string>();
  function visit(key: string): boolean {
    if (done.has(key)) return true;
    const task = spec.tasks.find((task) => task.key === key);
    if (!task || visiting.has(key)) return false;
    visiting.add(key);
    if (!(task.depends_on ?? []).every(visit)) return false;
    visiting.delete(key);
    done.add(key);
    return true;
  }
  return keys.every(visit)
    ? ""
    : "Dependencies must name existing tasks without a cycle.";
}
export function validateProposalReceipt(
  raw: ObjectValue,
  expected: PlanInput,
  context: ProposalContext,
  template: TemplateChoice,
): void {
  const plan = planDetails(raw),
    canonical = object(JSON.parse(plan.canonical_json));
  if (
    !plan.id ||
    plan.swarm_id !== context.swarm.id ||
    plan.version !== (expected.parent_version ?? 0) + 1 ||
    plan.state !== "proposed" ||
    plan.tasks.length !== expected.tasks.length ||
    inputIdentity(parsedInput(plan.specification)) !==
      inputIdentity(expected) ||
    inputIdentity(parsedInput(canonical.plan)) !== inputIdentity(expected) ||
    canonical.template_hash !== template.sha256 ||
    (canonical.input_grant_policy_hash ?? "") !==
      template.input_grant_policy_hash ||
    plan.tasks.some(
      (task) =>
        !text(task.id) ||
        task.state !== "offered" ||
        task.claim_epoch !== 1 ||
        task.dependency_blocked !== false,
    )
  )
    throw Error(
      "Invalid proposal receipt; retry the same reviewed draft or reload server state.",
    );
}
