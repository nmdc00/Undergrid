import { useEffect, useRef, useState } from "react";
import { ApiError, request } from "./api";
import { executionTask, type ExecutionTask } from "./task-execution-contract";
const activeStates = [
  "accepted",
  "in_progress",
  "changes_requested",
  "at_risk",
];
export function OwnerTaskExecution({
  agent,
  plan,
  hash,
  taskID,
  csrf,
  onUnauthorized,
}: {
  agent: string;
  plan: string;
  hash: string;
  taskID: string;
  csrf: string;
  onUnauthorized: () => void;
}) {
  const [task, setTask] = useState<ExecutionTask | null>(null),
    [ack, setAck] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [message, setMessage] = useState("");
  const generation = useRef(0);
  useEffect(() => {
    generation.current++;
    setTask(null);
    setAck(false);
    setBusy(false);
    setError("");
    setMessage("");
    return () => {
      generation.current++;
    };
  }, [agent, plan, hash, taskID, csrf]);
  const base = `/v1/owner/agents/${encodeURIComponent(agent)}`;
  function failed(e: unknown, mine: number) {
    if (mine !== generation.current) return;
    if (e instanceof ApiError && [401, 403, 404, 409, 410].includes(e.status)) {
      setTask(null);
      setAck(false);
      if (e.status === 401) onUnauthorized();
    }
    setError(e instanceof Error ? e.message : "Task execution request failed");
  }
  async function review() {
    if (busy) return;
    const mine = ++generation.current;
    setBusy(true);
    setTask(null);
    setAck(false);
    setError("");
    setMessage("");
    try {
      const page = await request(
        `${base}/plans/${encodeURIComponent(plan)}/assignments`,
        csrf,
      );
      if (
        Object.keys(page).length !== 1 ||
        !Array.isArray(page.items) ||
        page.items.length > 50
      )
        throw new Error("Invalid task review page");
      const tasks = page.items.map(executionTask);
      if (
        tasks.some(
          (x) => x.plan_id !== plan || x.assigned_agent_id !== agent,
        ) ||
        new Set(tasks.map((x) => x.id)).size !== tasks.length
      )
        throw new Error("Task review context changed");
      const current = tasks.find((x) => x.id === taskID);
      if (!current || !activeStates.includes(current.state))
        throw new Error(
          "This task is no longer executable. Review the current plan and assignments.",
        );
      if (mine === generation.current) setTask(current);
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  async function record() {
    if (!task || !ack || busy || task.dependency_blocked) return;
    const current = task,
      mine = ++generation.current;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const raw = await request(
        `${base}/tasks/${encodeURIComponent(taskID)}/progress`,
        csrf,
        "POST",
        { state: "in_progress", claim_epoch: current.claim_epoch },
        current.etag,
        (value) => {
          const result = executionTask(value);
          if (
            result.id !== taskID ||
            result.plan_id !== plan ||
            result.assigned_agent_id !== agent ||
            result.claim_epoch !== current.claim_epoch ||
            result.version !== current.version + 1 ||
            result.state !==
              (current.state === "at_risk" ? "at_risk" : "in_progress") ||
            result.progress_deadline !== current.progress_deadline ||
            result.cure_deadline !== current.cure_deadline
          )
            throw new Error(
              "Invalid activity receipt. Reload the task before assuming a result.",
            );
        },
      );
      if (mine === generation.current) {
        setTask(executionTask(raw));
        setAck(false);
        setMessage(
          "Unverified activity recorded. No deadline was extended and no output was accepted.",
        );
      }
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  return (
    <section aria-label={`Execution for ${taskID}`}>
      <h6>Task execution</h6>
      <p>
        Activity is unverified. It does not extend progress or cure deadlines,
        accept output, approve publication or authorize payments. Undergrid does
        not execute agent content.
      </p>
      <button disabled={busy} onClick={() => void review()}>
        Review task execution
      </button>
      {error && <p role="alert">{error}</p>}
      {message && <p role="status">{message}</p>}
      {task && (
        <>
          <p>
            State: {task.state}. Claim epoch: {task.claim_epoch}. Task version:{" "}
            {task.version}.
          </p>
          <p>Reviewed plan hash: {hash}</p>
          <pre>{JSON.stringify(task.specification, null, 2)}</pre>
          <p>
            Progress deadline: {task.progress_deadline ?? "none"}. Cure
            deadline: {task.cure_deadline ?? "none"}.
          </p>
          {task.dependency_blocked && (
            <p>
              Dependencies block this work. Review the plan before proceeding.
            </p>
          )}
          <label>
            <input
              type="checkbox"
              disabled={busy || task.dependency_blocked}
              checked={ack}
              onChange={(e) => setAck(e.target.checked)}
            />
            I reviewed the current task, dependencies and exact plan. I
            understand this records unverified activity only.
          </label>
          <button
            disabled={busy || !ack || task.dependency_blocked}
            onClick={() => void record()}
          >
            Record unverified activity
          </button>
        </>
      )}
    </section>
  );
}
