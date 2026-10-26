import { object, text, type ObjectValue } from "./api";

export type Summary = {
  id: string;
  version: number;
  state: string;
  content_hash: string;
};
export type Plan = Summary & {
  swarm_id: string;
  canonical_json: string;
  specification: ObjectValue;
  tasks: ObjectValue[];
};
export type Review = Plan & {
  contribution_template_hash: string;
  terms_url: string;
};
function integer(x: unknown) {
  if (typeof x !== "number" || !Number.isSafeInteger(x) || x < 1)
    throw Error("Invalid plan version");
  return x;
}
export function planSummary(raw: unknown): Summary {
  const x = object(raw);
  const hash = text(x.content_hash);
  if (!/^[a-f0-9]{64}$/.test(hash)) throw Error("Invalid plan hash");
  return {
    id: text(x.id),
    version: integer(x.version),
    state: text(x.state),
    content_hash: hash,
  };
}
function strings(x: unknown): string[] {
  if (!Array.isArray(x)) throw Error("Invalid plan details");
  return x.map(text);
}
export function planDetails(raw: unknown): Plan {
  const x = object(raw),
    spec = object(x.specification),
    rights = object(spec.rights);
  text(spec.objective);
  text(spec.respond_by);
  for (const k of [
    "required_principals",
    "acceptance_principals",
    "publication_principals",
  ])
    strings(spec[k]);
  for (const k of ["template_id", "license_id", "publication"]) text(rights[k]);
  if (
    !Array.isArray(spec.tasks) ||
    spec.tasks.length < 1 ||
    spec.tasks.length > 50 ||
    !Array.isArray(x.tasks)
  )
    throw Error("Invalid plan tasks");
  for (const raw of spec.tasks) {
    const t = object(raw);
    text(t.key);
    text(t.assignee_agent_id);
    strings(t.depends_on);
    strings(t.checks);
    text(t.output_schema);
    integer(t.progress_interval_seconds);
    if (t.deadline != null) text(t.deadline);
  }
  return {
    ...planSummary(x),
    swarm_id: text(x.swarm_id),
    canonical_json: text(x.canonical_json),
    specification: spec,
    tasks: x.tasks.map(object),
  };
}
export function reviewedPlan(raw: unknown): Review {
  const x = object(raw);
  const hash = text(x.contribution_template_hash);
  if (!/^[a-f0-9]{64}$/.test(hash)) throw Error("Invalid terms hash");
  return {
    ...planDetails(x),
    contribution_template_hash: hash,
    terms_url: text(x.terms_url),
  };
}
