import { useEffect, useRef, useState, type FormEvent } from "react";
import { ApiError, request, text, type ObjectValue } from "./api";

export function CreateSwarm({
  agents,
  csrf,
  onUnauthorized,
  onCreated,
}: {
  agents: ObjectValue[];
  csrf: string;
  onUnauthorized: () => void;
  onCreated?: (agent: string, id: string) => void;
}) {
  const active = agents.filter((a) => a.status === "active");
  const [agent, setAgent] = useState("");
  const [objective, setObjective] = useState("");
  const [visibility, setVisibility] = useState("private");
  const [acknowledged, setAcknowledged] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [created, setCreated] = useState("");
  const generation = useRef(0);
  const available = active.some((a) => a.id === agent);
  function discard() {
    generation.current++;
    setObjective("");
    setAcknowledged(false);
    setCreated("");
    setBusy(false);
  }
  useEffect(() => {
    discard();
    setError("");
    setVisibility("private");
    return () => {
      generation.current++;
    };
  }, [agent, csrf, available]);
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!available || !acknowledged || objective.trim().length < 3 || busy)
      return;
    const mine = ++generation.current;
    setBusy(true);
    setCreated("");
    setError("");
    try {
      const result = await request(
        `/v1/owner/agents/${encodeURIComponent(agent)}/swarms`,
        csrf,
        "POST",
        {
          objective,
          visibility,
          history_policy: "full",
          recruitment_roles: [],
        },
        undefined,
        (result) => {
          if (
            !text(result.id) ||
            result.history_policy !== "full" ||
            result.visibility !== visibility ||
            !Number.isInteger(result.version) ||
            !/^"[0-9]+"$/.test(text(result.etag))
          )
            throw new Error("Invalid creation response");
        },
      );
      if (mine !== generation.current) return;
      const id = text(result.id);
      setCreated(id);
      setObjective("");
      setAcknowledged(false);
      onCreated?.(agent, id);
    } catch (failure) {
      if (mine !== generation.current) return;
      if (
        failure instanceof ApiError &&
        [401, 403, 404].includes(failure.status)
      ) {
        discard();
        if (failure.status === 401) {
          onUnauthorized();
          setError("Your session expired. Sign in again.");
        } else
          setError(
            "The selected agent is no longer available. Refresh your agents.",
          );
      } else
        setError(
          "Swarm creation could not be confirmed. Retry with the same details or check participation.",
        );
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  return (
    <section aria-label="Create swarm">
      <h3>Create swarm</h3>
      <form onSubmit={(event) => void submit(event)}>
        <label>
          Responsible agent
          <select
            value={available ? agent : ""}
            onChange={(event) => setAgent(event.target.value)}
          >
            <option value="">Select an agent</option>
            {active.map((a) => (
              <option key={text(a.id)} value={text(a.id)}>
                {text(a.name)}
              </option>
            ))}
          </select>
        </label>
        <label>
          Swarm objective
          <textarea
            required
            minLength={3}
            maxLength={4000}
            disabled={busy}
            value={objective}
            onChange={(event) => setObjective(event.target.value)}
          />
        </label>
        <label>
          Swarm visibility
          <select
            disabled={busy}
            value={visibility}
            onChange={(event) => setVisibility(event.target.value)}
          >
            <option value="private">Private</option>
            <option value="restricted">Restricted</option>
          </select>
        </label>
        <p>
          Admitted members can read the full room history, including earlier
          messages. Creation does not approve work, rights, publication or
          payments.
        </p>
        <label>
          <input
            type="checkbox"
            disabled={busy}
            checked={acknowledged}
            onChange={(event) => setAcknowledged(event.target.checked)}
          />
          I understand admitted members can read the full room history.
        </label>
        <button
          disabled={
            busy || !available || !acknowledged || objective.trim().length < 3
          }
        >
          Create swarm
        </button>
      </form>
      {busy && <p role="status">Creating swarm…</p>}
      {error && <p role="alert">{error}</p>}
      {created && (
        <p role="status">
          Swarm {created} created. Load participation to open the room.
        </p>
      )}
    </section>
  );
}
