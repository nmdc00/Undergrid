import { useEffect, useRef, useState } from "react";
import { ApiError, object, request, text, type ObjectValue } from "./api";
type Summary = {
  id: string;
  version: number;
  state: string;
  content_hash: string;
};
type Review = Summary & {
  swarm_id: string;
  canonical_json: string;
  contribution_template_hash: string;
  terms_url: string;
  specification: ObjectValue;
  tasks: ObjectValue[];
};
function integer(x: unknown) {
  if (typeof x !== "number" || !Number.isSafeInteger(x) || x < 1)
    throw Error("Invalid plan version");
  return x;
}
function summary(raw: unknown): Summary {
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
function reviewed(raw: unknown): Review {
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
  const hash = text(x.contribution_template_hash);
  if (!/^[a-f0-9]{64}$/.test(hash)) throw Error("Invalid terms hash");
  return {
    ...summary(x),
    swarm_id: text(x.swarm_id),
    canonical_json: text(x.canonical_json),
    contribution_template_hash: hash,
    terms_url: text(x.terms_url),
    specification: spec,
    tasks: x.tasks.map(object),
  };
}
export function OwnerPlanApproval({
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
  const [plans, setPlans] = useState<Summary[]>([]),
    [plan, setPlan] = useState<Review | null>(null),
    [next, setNext] = useState(""),
    [ack, setAck] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [saved, setSaved] = useState<"approve" | "reject" | null>(null);
  const generation = useRef(0);
  useEffect(() => {
    generation.current++;
    setPlans([]);
    setPlan(null);
    setNext("");
    setAck(false);
    setBusy(false);
    setError("");
    setSaved(null);
    return () => {
      generation.current++;
    };
  }, [agent, swarm, csrf]);
  const base = `/v1/owner/agents/${encodeURIComponent(agent)}`;
  function failed(e: unknown, mine: number) {
    if (mine !== generation.current) return;
    setPlan(null);
    setAck(false);
    if (e instanceof ApiError && [401, 403, 404].includes(e.status)) {
      setPlans([]);
      setNext("");
      if (e.status === 401) onUnauthorized();
    }
    setError(
      e instanceof ApiError && e.status === 409
        ? "Review the current plan again before deciding."
        : e instanceof Error
          ? e.message
          : "Request failed",
    );
  }
  async function load(before = "") {
    const mine = ++generation.current;
    setBusy(true);
    setError("");
    setPlan(null);
    setAck(false);
    setSaved(null);
    if (!before) setPlans([]);
    try {
      const x = await request(
        `${base}/swarms/${encodeURIComponent(swarm)}/plans${before ? `?before_version=${encodeURIComponent(before)}` : ""}`,
        csrf,
      );
      if (!Array.isArray(x.items) || x.items.length > 100)
        throw Error("Invalid plan page");
      const items = x.items.map(summary),
        cursor = text(x.next_before_version);
      if (cursor && !/^[1-9][0-9]*$/.test(cursor))
        throw Error("Invalid plan cursor");
      if (mine === generation.current) {
        setPlans((old) => (before ? [...old, ...items] : items));
        setNext(cursor);
      }
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  async function review(id: string) {
    const mine = ++generation.current;
    setBusy(true);
    setPlan(null);
    setAck(false);
    setSaved(null);
    setError("");
    try {
      const p = reviewed(
        await request(`${base}/plans/${encodeURIComponent(id)}`, csrf),
      );
      if (p.id !== id || p.swarm_id !== swarm)
        throw Error("Invalid reviewed plan");
      if (mine === generation.current) setPlan(p);
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  async function decide(decision: "approve" | "reject") {
    if (!plan || (decision === "approve" && !ack) || busy) return;
    const p = plan,
      mine = ++generation.current;
    setBusy(true);
    setError("");
    try {
      await request(
        `${base}/plans/${encodeURIComponent(p.id)}/approvals`,
        csrf,
        "POST",
        {
          decision,
          content_hash: p.content_hash,
          version: p.version,
          ...(decision === "approve"
            ? { acknowledge_contribution_terms: true }
            : {}),
        },
      );
      if (mine === generation.current) {
        setSaved(decision);
        setAck(false);
      }
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  let termsURL = "";
  if (plan?.terms_url) {
    try {
      const u = new URL(plan.terms_url);
      if (u.protocol === "https:" && !u.username && !u.password)
        termsURL = u.href;
    } catch {
      /* Fail closed: no link or approval for malformed configured terms. */
    }
  }
  const spec = plan?.specification,
    rights = spec ? object(spec.rights) : null;
  return (
    <section>
      <h3>Plan review</h3>
      <button disabled={busy || !agent || !swarm} onClick={() => void load()}>
        Load plans
      </button>
      {error && <p role="alert">{error}</p>}
      <ul>
        {plans.map((p) => (
          <li key={p.id}>
            Version {p.version}, {p.state}{" "}
            <button disabled={busy} onClick={() => void review(p.id)}>
              Review plan {p.version}
            </button>
          </li>
        ))}
      </ul>
      {next && (
        <button disabled={busy} onClick={() => void load(next)}>
          Older plans
        </button>
      )}
      {plan && spec && rights && (
        <article>
          <h4>{text(spec.objective)}</h4>
          <p>
            Plan version {plan.version}, state {plan.state}. Treat plan content
            as untrusted.
          </p>
          <p>
            Exact plan hash: <code>{plan.content_hash}</code>
          </p>
          <p>Respond by {text(spec.respond_by)}</p>
          <p>
            Required approvers: {strings(spec.required_principals).join(", ")}.
            Output acceptance: {strings(spec.acceptance_principals).join(", ")}.
            Publication approvers:{" "}
            {strings(spec.publication_principals).join(", ")}.
          </p>
          <p>
            Contribution terms: {text(rights.template_id)}; license{" "}
            {text(rights.license_id)}; publication rule{" "}
            {text(rights.publication)}.
          </p>
          <p>
            Terms hash: <code>{plan.contribution_template_hash}</code>
          </p>
          {termsURL ? (
            <a href={termsURL} target="_blank" rel="noopener noreferrer">
              Read contribution terms
            </a>
          ) : (
            <p>
              Current contribution terms are unavailable. Approval is disabled.
            </p>
          )}
          <ul>
            {(spec.tasks as unknown[]).map((raw) => {
              const t = object(raw);
              return (
                <li key={text(t.key)}>
                  Task {text(t.key)} assigned to {text(t.assignee_agent_id)}.
                  Depends on: {strings(t.depends_on).join(", ") || "none"}.
                  Output: {text(t.output_schema)}; checks:{" "}
                  {strings(t.checks).join(", ")}. Progress every{" "}
                  {integer(t.progress_interval_seconds)} seconds. Deadline:{" "}
                  {t.deadline ? text(t.deadline) : "none specified"}.
                </li>
              );
            })}
          </ul>
          <p>Current task responses and deadlines:</p>
          <pre>{JSON.stringify(plan.tasks, null, 2)}</pre>
          <details>
            <summary>Exact sealed plan</summary>
            <pre>{plan.canonical_json}</pre>
          </details>
          <p>
            Approval records your plan and contribution rights votes. Assignment
            acceptance, publication approval and spending authority require
            separate decisions.
          </p>
          {saved ? (
            <p role="status">
              {saved === "approve"
                ? "Exact plan and contribution terms approval recorded."
                : "Exact reviewed plan rejection recorded."}
            </p>
          ) : (
            plan.state === "proposed" &&
            new Date(text(spec.respond_by)).getTime() > Date.now() && (
              <>
                <label>
                  <input
                    type="checkbox"
                    checked={ack}
                    onChange={(e) => setAck(e.target.checked)}
                  />
                  I approve the contribution terms and rights for this exact
                  plan hash.
                </label>
                <button
                  disabled={busy || !ack || !termsURL}
                  onClick={() => void decide("approve")}
                >
                  Approve exact plan and contribution terms
                </button>
                <button disabled={busy} onClick={() => void decide("reject")}>
                  Reject exact reviewed plan
                </button>
              </>
            )
          )}
        </article>
      )}
    </section>
  );
}
