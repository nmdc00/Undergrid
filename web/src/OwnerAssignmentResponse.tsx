import { OwnerContributions } from "./OwnerContributions";
import { OwnerTaskExecution } from "./OwnerTaskExecution";
import { useEffect, useRef, useState } from "react";
import { ApiError, object, request, text, type ObjectValue } from "./api";
type Task = {
  id: string;
  plan_id: string;
  assigned_agent_id: string;
  state: string;
  version: number;
  etag: string;
  response_deadline: string;
  dependency_blocked: boolean;
  specification: ObjectValue;
};
function taskResult(raw: unknown): Task {
  const x = object(raw),
    spec = object(x.specification);
  if (
    typeof x.version !== "number" ||
    !Number.isSafeInteger(x.version) ||
    x.version < 1 ||
    typeof x.dependency_blocked !== "boolean"
  )
    throw Error("Invalid assignment response");
  for (const k of ["key", "assignee_agent_id", "output_schema"]) text(spec[k]);
  for (const k of ["depends_on", "checks"]) {
    if (!Array.isArray(spec[k])) throw Error("Invalid assignment details");
    (spec[k] as unknown[]).forEach(text);
  }
  if (
    typeof spec.progress_interval_seconds !== "number" ||
    !Number.isSafeInteger(spec.progress_interval_seconds) ||
    spec.progress_interval_seconds < 1
  )
    throw Error("Invalid assignment progress window");
  if (text(x.etag) !== `"${x.version}"`) throw Error("Invalid assignment ETag");
  const deadline = text(x.response_deadline);
  if (!Number.isFinite(Date.parse(deadline)))
    throw Error("Invalid assignment deadline");
  return {
    id: text(x.id),
    plan_id: text(x.plan_id),
    assigned_agent_id: text(x.assigned_agent_id),
    state: text(x.state),
    version: x.version,
    etag: text(x.etag),
    response_deadline: deadline,
    dependency_blocked: x.dependency_blocked,
    specification: spec,
  };
}
export function OwnerAssignmentResponse({
  agent,
  plan,
  hash,
  termsURL,
  csrf,
  onUnauthorized,
  planAccepted = false,
}: {
  agent: string;
  plan: string;
  hash: string;
  termsURL: string;
  csrf: string;
  onUnauthorized: () => void;
  planAccepted?: boolean;
}) {
  const [tasks, setTasks] = useState<Task[] | null>(null),
    [ack, setAck] = useState<Record<string, boolean>>({}),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [saved, setSaved] = useState<Record<string, string>>({});
  const generation = useRef(0);
  useEffect(() => {
    generation.current++;
    setTasks(null);
    setAck({});
    setBusy(false);
    setError("");
    setSaved({});
    return () => {
      generation.current++;
    };
  }, [agent, plan, hash, csrf]);
  const base = `/v1/owner/agents/${encodeURIComponent(agent)}`;
  function failed(e: unknown, mine: number) {
    if (mine !== generation.current) return;
    setTasks(null);
    setAck({});
    setError(e instanceof Error ? e.message : "Assignment request failed");
    if (e instanceof ApiError && e.status === 401) onUnauthorized();
  }
  async function load() {
    const mine = ++generation.current;
    setBusy(true);
    setTasks(null);
    setAck({});
    setSaved({});
    setError("");
    try {
      const x = await request(
        `${base}/plans/${encodeURIComponent(plan)}/assignments`,
        csrf,
      );
      if (!Array.isArray(x.items) || x.items.length > 50)
        throw Error("Invalid assignment review");
      const found = x.items.map(taskResult);
      if (
        found.some((t) => t.plan_id !== plan || t.assigned_agent_id !== agent)
      )
        throw Error("Invalid assignment attribution");
      if (mine === generation.current) setTasks(found);
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  async function decide(t: Task, decision: "accept" | "decline") {
    if (busy || (decision === "accept" && !ack[t.id])) return;
    const mine = ++generation.current;
    setBusy(true);
    setError("");
    try {
      await request(
        `${base}/tasks/${encodeURIComponent(t.id)}/assignment-response`,
        csrf,
        "POST",
        {
          decision,
          plan_hash: hash,
          ...(decision === "accept"
            ? { acknowledge_responsibility_and_terms: true }
            : {}),
        },
        t.etag,
        (raw) => {
          const result = taskResult(raw);
          if (
            result.id !== t.id ||
            result.plan_id !== plan ||
            result.assigned_agent_id !== agent ||
            result.version !== t.version + 1 ||
            result.state !== (decision === "accept" ? "accepted" : "open")
          )
            throw Error("Invalid assignment decision response");
        },
      );
      if (mine === generation.current) {
        setSaved((old) => ({ ...old, [t.id]: decision }));
        setAck((old) => ({ ...old, [t.id]: false }));
      }
    } catch (e) {
      if (e instanceof ApiError && e.status < 500) failed(e, mine);
      else if (mine === generation.current) {
        setAck((old) => ({ ...old, [t.id]: false }));
        setError(
          "Assignment decision could not be confirmed. Retry the same decision or review assignments again.",
        );
      }
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  return (
    <section>
      <h4>Assignment responsibilities</h4>
      <p>
        Assignment acceptance records your responsibility. It does not create
        plan, contribution rights, publication or spending votes. Current rights
        policy is still required.
      </p>
      <button disabled={busy} onClick={() => void load()}>
        Review my assignments
      </button>
      {error && <p role="alert">{error}</p>}
      {tasks?.length === 0 && <p>No assignments for this agent.</p>}
      {tasks?.map((t) => (
        <article key={t.id}>
          <h5>Task {text(t.specification.key)}</h5>
          <p>
            State: {t.state}. Respond by {t.response_deadline}.
          </p>
          <pre>{JSON.stringify(t.specification, null, 2)}</pre>
          {planAccepted &&
            [
              "accepted",
              "in_progress",
              "changes_requested",
              "at_risk",
            ].includes(t.state) && (
              <OwnerTaskExecution
                key={`${csrf}:${agent}:${plan}:${hash}:${t.id}`}
                agent={agent}
                plan={plan}
                hash={hash}
                taskID={t.id}
                csrf={csrf}
                onUnauthorized={onUnauthorized}
              />
            )}
          {planAccepted && (
            <OwnerContributions
              key={`contribution:${csrf}:${agent}:${plan}:${hash}:${t.id}`}
              agent={agent}
              plan={plan}
              hash={hash}
              taskID={t.id}
              csrf={csrf}
              onUnauthorized={onUnauthorized}
            />
          )}
          {saved[t.id] ? (
            <p role="status">
              {saved[t.id] === "accept"
                ? "Assignment acceptance recorded."
                : "Assignment decline recorded."}
            </p>
          ) : (
            t.state === "offered" &&
            Date.parse(t.response_deadline) > Date.now() && (
              <>
                <label>
                  <input
                    type="checkbox"
                    checked={!!ack[t.id]}
                    onChange={(e) =>
                      setAck((old) => ({ ...old, [t.id]: e.target.checked }))
                    }
                  />
                  I accept this task's responsibility under the reviewed
                  contribution terms and exact plan hash.
                </label>
                <button
                  disabled={
                    busy || !ack[t.id] || !termsURL || t.dependency_blocked
                  }
                  onClick={() => void decide(t, "accept")}
                >
                  Accept assignment responsibility
                </button>
                <button
                  disabled={busy}
                  onClick={() => void decide(t, "decline")}
                >
                  Decline assignment
                </button>
              </>
            )
          )}
        </article>
      ))}
    </section>
  );
}
