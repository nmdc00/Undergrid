import { useEffect, useRef, useState, type FormEvent } from "react";
import { ApiError, request, text } from "./api";
import {
  draftProblem,
  emptyTask,
  enabledTemplates,
  inputFromDraft,
  newDraft,
  proposalContext,
  revisionDraft,
  validateProposalReceipt,
  type Draft,
  type PlanInput,
  type ProposalContext,
  type TaskDraft,
  type TemplateChoice,
} from "./plan-proposal-contract";

export function OwnerPlanProposal({
  agent,
  swarm,
  csrf,
  onUnauthorized,
}: {
  agent: string;
  swarm: string;
  csrf: string;
  onUnauthorized: () => void;
}) {
  const [context, setContext] = useState<ProposalContext | null>(null),
    [templates, setTemplates] = useState<TemplateChoice[]>([]),
    [draft, setDraft] = useState<Draft | null>(null);
  const [busy, setBusy] = useState(false),
    [reviewed, setReviewed] = useState(false),
    [error, setError] = useState(""),
    [receipt, setReceipt] = useState<{ id: string; hash: string } | null>(null);
  const generation = useRef(0),
    identity = JSON.stringify([agent, swarm, csrf]),
    currentIdentity = useRef(identity);
  currentIdentity.current = identity;
  const base = `/v1/owner/agents/${encodeURIComponent(agent)}/swarms/${encodeURIComponent(swarm)}`;
  useEffect(() => {
    generation.current++;
    setContext(null);
    setTemplates([]);
    setDraft(null);
    setReviewed(false);
    setReceipt(null);
    setBusy(false);
    setError("");
    return () => {
      generation.current++;
    };
  }, [agent, swarm, csrf]);
  function current(mine: number, actor: string) {
    return mine === generation.current && actor === currentIdentity.current;
  }
  function failed(value: unknown, mine: number, actor: string) {
    if (!current(mine, actor)) return;
    setReviewed(false);
    if (
      value instanceof ApiError &&
      [401, 403, 404, 409].includes(value.status)
    ) {
      setContext(null);
      setDraft(null);
      setReceipt(null);
      if (value.status === 401) onUnauthorized();
    }
    setError(
      value instanceof ApiError && value.status === 409
        ? "The room or proposal basis changed. Load the plan context and review again."
        : value instanceof Error
          ? value.message
          : "Request failed",
    );
  }
  async function load(after = "") {
    if (busy) return;
    const mine = ++generation.current,
      actor = identity,
      previous = context;
    setBusy(true);
    setError("");
    setReviewed(false);
    if (!after) {
      setContext(null);
      setDraft(null);
      setReceipt(null);
    }
    try {
      const raw = await request(
        `${base}/plan-context${after ? `?after=${encodeURIComponent(after)}` : ""}`,
        csrf,
      );
      const next = proposalContext(raw, swarm);
      if (after) {
        if (
          !previous ||
          next.swarm.etag !== previous.swarm.etag ||
          JSON.stringify(next.latest_plan) !==
            JSON.stringify(previous.latest_plan)
        )
          throw new ApiError(409, "plan_basis_changed");
        const seen = new Set(previous.items.map((row) => row.id));
        next.items = [
          ...previous.items,
          ...next.items.filter((row) => !seen.has(row.id)),
        ];
      } else {
        const choices = enabledTemplates(
          await request("/v1/owner/config", csrf),
        );
        if (!current(mine, actor)) return;
        setTemplates(choices);
      }
      if (!current(mine, actor)) return;
      setContext(next);
      if (!after && !next.latest_plan) setDraft(newDraft());
    } catch (value) {
      failed(value, mine, actor);
    } finally {
      if (current(mine, actor)) setBusy(false);
    }
  }
  async function loadRevision() {
    if (busy || !context?.latest_plan) return;
    const basis = context,
      mine = ++generation.current,
      actor = identity;
    setBusy(true);
    setError("");
    setReviewed(false);
    setReceipt(null);
    try {
      const raw = await request(
        `/v1/owner/agents/${encodeURIComponent(agent)}/plans/${encodeURIComponent(basis.latest_plan!.id)}`,
        csrf,
      );
      const value = revisionDraft(raw, basis);
      if (current(mine, actor)) setDraft(value);
    } catch (value) {
      failed(value, mine, actor);
    } finally {
      if (current(mine, actor)) setBusy(false);
    }
  }
  function change(update: (draft: Draft) => Draft) {
    if (busy || receipt) return;
    setDraft((previous) => (previous ? update(previous) : previous));
    setReviewed(false);
    setError("");
  }
  function taskChange(index: number, patch: Partial<TaskDraft>) {
    change((previous) => {
      const oldKey = previous.tasks[index].key;
      return {
        ...previous,
        tasks: previous.tasks.map((task, position) =>
          position === index
            ? { ...task, ...patch }
            : patch.key !== undefined && patch.key !== oldKey
              ? {
                  ...task,
                  depends_on: task.depends_on.map((key) =>
                    key === oldKey ? patch.key! : key,
                  ),
                }
              : task,
        ),
      };
    });
  }
  const selectedTemplate = templates.find(
    (template) => template.id === draft?.rights.template_id,
  );
  let input: PlanInput | null = null,
    problem = "Load the plan context before proposing.";
  if (draft && context) {
    try {
      input = inputFromDraft(draft);
      problem = draftProblem(input, context.items, selectedTemplate);
    } catch (value) {
      problem =
        value instanceof Error ? value.message : "Review the plan fields.";
    }
  }
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (busy || receipt || !reviewed || !draft || !context || !selectedTemplate)
      return;
    const submitted = inputFromDraft(draft);
    if (draftProblem(submitted, context.items, selectedTemplate)) {
      setReviewed(false);
      return;
    }
    const mine = ++generation.current,
      actor = identity,
      basis = context,
      template = selectedTemplate;
    setBusy(true);
    setError("");
    try {
      const saved = await request(
        `${base}/plans`,
        csrf,
        "POST",
        submitted,
        basis.swarm.etag,
        (raw) => validateProposalReceipt(raw, submitted, basis, template),
      );
      if (!current(mine, actor)) return;
      setReceipt({ id: text(saved.id), hash: text(saved.content_hash) });
      setReviewed(false);
    } catch (value) {
      failed(value, mine, actor);
    } finally {
      if (current(mine, actor)) setBusy(false);
    }
  }
  const participants = context?.items ?? [],
    principals = [
      ...new Set(participants.map((row) => row.principal_id)),
    ].sort(),
    contributors = new Set(
      participants
        .filter((row) => row.role !== "observer")
        .map((row) => row.principal_id),
    );
  const locked = busy || receipt !== null;
  return (
    <section aria-label="Owner plan proposal">
      <h3>Plan proposal</h3>
      <p>
        Propose an immutable plan or a new revision. Proposal creates offered
        tasks. Each principal's exact plan and rights decisions and each
        assignee's responsibility acceptance remain separate.
      </p>
      <button disabled={busy || !agent || !swarm} onClick={() => void load()}>
        Load plan context
      </button>
      {error && <p role="alert">{error}</p>}
      {context && (
        <>
          <p>
            Room version {context.swarm.version}. Accepted participant IDs
            identify authorities; they do not prove independent beneficial
            ownership. Current membership and identity activity are checked
            again when submitting.
          </p>
          <ul aria-label="Accepted plan participants">
            {participants.map((row) => (
              <li key={row.id}>
                Agent {row.agent_id}; principal {row.principal_id}; {row.role}
              </li>
            ))}
          </ul>
          {context.next_after && (
            <button
              disabled={busy || receipt !== null}
              onClick={() => void load(context.next_after)}
            >
              More plan participants
            </button>
          )}
          {context.latest_plan && (
            <>
              <p>
                Latest plan version {context.latest_plan.version}:{" "}
                {context.latest_plan.state}. Hash{" "}
                {context.latest_plan.content_hash}.
              </p>
              <button disabled={locked} onClick={() => void loadRevision()}>
                Load latest plan for revision
              </button>
            </>
          )}
        </>
      )}
      {draft && context && (
        <form onSubmit={(event) => void submit(event)}>
          <fieldset disabled={locked}>
            <legend>
              {draft.parent_version === null
                ? "Initial plan"
                : `New revision of plan ${draft.parent_version}`}
            </legend>
            <label>
              Plan objective
              <textarea
                required
                maxLength={4000}
                value={draft.objective}
                onChange={(event) =>
                  change((value) => ({
                    ...value,
                    objective: event.target.value,
                  }))
                }
              />
            </label>
            <label>
              Plan response deadline (UTC)
              <input
                type="datetime-local"
                step="0.001"
                required
                value={draft.respond_by}
                onChange={(event) =>
                  change((value) => ({
                    ...value,
                    respond_by: event.target.value,
                  }))
                }
              />
            </label>
            {(
              [
                [
                  "required_principals",
                  "Required approvals",
                  "Required approval",
                ],
                [
                  "acceptance_principals",
                  "Acceptance principals",
                  "Acceptance principal",
                ],
                [
                  "publication_principals",
                  "Publication principals",
                  "Publication principal",
                ],
              ] as const
            ).map(([field, title, label]) => (
              <fieldset key={field}>
                <legend>{title}</legend>
                {principals
                  .filter(
                    (id) =>
                      field === "publication_principals" ||
                      contributors.has(id),
                  )
                  .map((id) => (
                    <label className="check" key={id}>
                      <input
                        type="checkbox"
                        checked={(draft[field] ?? []).includes(id)}
                        onChange={(event) =>
                          change((value) => ({
                            ...value,
                            [field]: event.target.checked
                              ? [...(value[field] ?? []), id]
                              : (value[field] ?? []).filter(
                                  (selected) => selected !== id,
                                ),
                          }))
                        }
                      />
                      {label}: {id}
                    </label>
                  ))}
              </fieldset>
            ))}
            <label>
              Contribution template
              <select
                value={draft.rights.template_id}
                onChange={(event) =>
                  change((value) => ({
                    ...value,
                    rights: {
                      ...value.rights,
                      template_id: event.target.value,
                      license_id: "",
                    },
                  }))
                }
              >
                <option value="">Select enabled terms</option>
                {templates.map((template) => (
                  <option key={template.id} value={template.id}>
                    {template.id}
                    {template.test_only ? " (test only)" : ""}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Dataset license
              <select
                value={draft.rights.license_id}
                onChange={(event) =>
                  change((value) => ({
                    ...value,
                    rights: { ...value.rights, license_id: event.target.value },
                  }))
                }
              >
                <option value="">Select license</option>
                {selectedTemplate?.license_ids.map((license) => (
                  <option key={license} value={license}>
                    {license}
                  </option>
                ))}
              </select>
            </label>
            {selectedTemplate && (
              <p>
                Selected contribution terms hash: {selectedTemplate.sha256}.{" "}
                <a
                  href={selectedTemplate.terms_url}
                  target="_blank"
                  rel="noreferrer"
                >
                  Read selected contribution terms
                </a>
                {selectedTemplate.test_only
                  ? " Test-only configuration does not approve production legal terms."
                  : ""}
              </p>
            )}
            {!templates.length && (
              <p>
                No contribution templates are enabled. A plan cannot be proposed
                until approved terms are configured.
              </p>
            )}
            {draft.tasks.map((task, index) => (
              <fieldset key={index}>
                <legend>Task {index + 1}</legend>
                <label>
                  Task {index + 1} key
                  <input
                    required
                    maxLength={80}
                    value={task.key}
                    onChange={(event) =>
                      taskChange(index, { key: event.target.value })
                    }
                  />
                </label>
                <label>
                  Task {index + 1} assignee
                  <select
                    value={task.assignee_agent_id}
                    onChange={(event) =>
                      taskChange(index, {
                        assignee_agent_id: event.target.value,
                      })
                    }
                  >
                    <option value="">Select current contributor</option>
                    {participants
                      .filter((row) => row.role !== "observer")
                      .map((row) => (
                        <option key={row.id} value={row.agent_id}>
                          {row.agent_id} (principal {row.principal_id})
                        </option>
                      ))}
                  </select>
                </label>
                <p>Output: source_catalog_v1. Fixed check: json_schema_v1.</p>
                <label>
                  Task {index + 1} progress interval (seconds)
                  <input
                    type="number"
                    min={60}
                    max={604800}
                    required
                    value={task.progress_interval_seconds}
                    onChange={(event) =>
                      taskChange(index, {
                        progress_interval_seconds: Number(event.target.value),
                      })
                    }
                  />
                </label>
                <label>
                  Task {index + 1} final deadline (UTC, optional)
                  <input
                    type="datetime-local"
                    step="0.001"
                    value={task.deadline}
                    onChange={(event) =>
                      taskChange(index, { deadline: event.target.value })
                    }
                  />
                </label>
                <fieldset>
                  <legend>Task {index + 1} dependencies</legend>
                  {draft.tasks
                    .filter((other) => other !== task && other.key)
                    .map((other, position) => (
                      <label className="check" key={position}>
                        <input
                          type="checkbox"
                          checked={task.depends_on.includes(other.key)}
                          onChange={(event) =>
                            taskChange(index, {
                              depends_on: event.target.checked
                                ? [...task.depends_on, other.key]
                                : task.depends_on.filter(
                                    (key) => key !== other.key,
                                  ),
                            })
                          }
                        />
                        Task {index + 1} depends on {other.key}
                      </label>
                    ))}
                </fieldset>
                <button
                  type="button"
                  disabled={draft.tasks.length <= 1}
                  onClick={() =>
                    change((value) => ({
                      ...value,
                      tasks: value.tasks
                        .filter((_task, position) => position !== index)
                        .map((remaining) => ({
                          ...remaining,
                          depends_on: remaining.depends_on.filter(
                            (key) => key !== task.key,
                          ),
                        })),
                    }))
                  }
                >
                  Remove task {index + 1}
                </button>
              </fieldset>
            ))}
            <button
              type="button"
              disabled={draft.tasks.length >= 50}
              onClick={() =>
                change((value) => ({
                  ...value,
                  tasks: [...value.tasks, emptyTask(value.tasks.length + 1)],
                }))
              }
            >
              Add task
            </button>
            <section aria-label="Plan proposal preview">
              <h4>Exact draft for review</h4>
              <pre>
                {input
                  ? JSON.stringify(input, null, 2)
                  : "Complete valid deadlines to inspect the outgoing specification."}
              </pre>
            </section>
            {problem && <p>{problem}</p>}
            <label className="check">
              <input
                type="checkbox"
                checked={reviewed}
                disabled={!!problem}
                onChange={(event) => setReviewed(event.target.checked)}
              />
              I reviewed this plan proposal
            </label>
            <button disabled={locked || !!problem || !reviewed}>
              Propose plan
            </button>
          </fieldset>
        </form>
      )}
      {receipt && (
        <p role="status">
          Plan proposed: {receipt.id}. Hash {receipt.hash}. No approvals or
          assignment acceptances were recorded. Load the plan context to propose
          another revision; use the separate plan review and assignment controls
          for decisions.
        </p>
      )}
    </section>
  );
}
