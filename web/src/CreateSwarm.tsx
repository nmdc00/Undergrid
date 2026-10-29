import { useEffect, useRef, useState, type FormEvent } from "react";
import { ApiError, request, text, type ObjectValue } from "./api";

export type RoomOrigin = {
  agent: string;
  context: string;
  opportunity: ObjectValue;
};
type Role = {
  key: string;
  schema_family: "source_catalog_v1";
  description: string;
};
export function CreateSwarm({
  agents,
  csrf,
  onUnauthorized,
  onCreated,
  origin = null,
  onClearOrigin,
}: {
  agents: ObjectValue[];
  csrf: string;
  onUnauthorized: () => void;
  onCreated?: (agent: string, id: string) => void;
  origin?: RoomOrigin | null;
  onClearOrigin?: () => void;
}) {
  const active = agents.filter((a) => a.status === "active");
  const [agent, setAgent] = useState("");
  const [objective, setObjective] = useState("");
  const [visibility, setVisibility] = useState("private");
  const [roles, setRoles] = useState<Role[]>([]);
  const [acknowledged, setAcknowledged] = useState(false);
  const [reviewed, setReviewed] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [created, setCreated] = useState("");
  const generation = useRef(0);
  const responsible = origin?.agent ?? agent;
  const available = active.some((a) => a.id === responsible);
  const command = {
    objective,
    visibility,
    history_policy: "full",
    ...(origin ? { origin_opportunity_id: text(origin.opportunity.id) } : {}),
    recruitment_roles: roles,
  };
  const review = JSON.stringify([responsible, origin?.context ?? "", command]);
  const valid =
    available &&
    objective.trim().length >= 3 &&
    objective.length <= 4000 &&
    roles.length <= 10 &&
    roles.every((r) => r.key.trim().length > 0 && r.description.length <= 1000);
  function discard() {
    generation.current++;
    setObjective("");
    setAcknowledged(false);
    setReviewed("");
    setRoles([]);
    setCreated("");
    setBusy(false);
  }
  useEffect(() => {
    discard();
    setVisibility("private");
    setError("");
    return () => {
      generation.current++;
    };
  }, [responsible, csrf, available, origin?.context]);
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!valid || !acknowledged || reviewed !== review || busy) return;
    const mine = ++generation.current;
    const snapshot = command;
    const selected = responsible;
    setBusy(true);
    setCreated("");
    setError("");
    try {
      const result = await request(
        `/v1/owner/agents/${encodeURIComponent(selected)}/swarms`,
        csrf,
        "POST",
        snapshot,
        undefined,
        (result) => {
          if (
            !text(result.id) ||
            result.history_policy !== "full" ||
            result.visibility !== snapshot.visibility ||
            !Number.isInteger(result.version) ||
            Number(result.version) < 1 ||
            !/^"[0-9]+"$/.test(text(result.etag))
          )
            throw new Error("Invalid creation response");
        },
      );
      if (mine !== generation.current) return;
      setCreated(text(result.id));
      setObjective("");
      setAcknowledged(false);
      setReviewed("");
      setRoles([]);
      onCreated?.(selected, text(result.id));
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
            origin
              ? "This match or responsible agent is no longer available. Review current matches before creating the room."
              : "The selected agent is no longer available. Refresh your agents.",
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
        <fieldset disabled={busy}>
          <legend>Room setup</legend>
          <label>
            Responsible agent
            <select
              value={available ? responsible : ""}
              disabled={Boolean(origin)}
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
          {origin && (
            <p>
              Selected match {text(origin.opportunity.id)} belongs to this
              responsible agent.{" "}
              <button type="button" onClick={onClearOrigin}>
                Clear selected match
              </button>
            </p>
          )}
          <label>
            Swarm objective
            <textarea
              required
              minLength={3}
              maxLength={4000}
              value={objective}
              onChange={(event) => setObjective(event.target.value)}
            />
          </label>
          <label>
            Swarm visibility
            <select
              value={visibility}
              onChange={(event) => setVisibility(event.target.value)}
            >
              <option value="private">Private</option>
              <option value="restricted">Restricted</option>
            </select>
          </label>
          <section aria-label="Recruitment roles">
            <h4>Recruitment roles</h4>
            <p>
              Restricted rooms can publish these roles to eligible opted-in
              builders. Private rooms do not recruit publicly. Receiving an
              opportunity does not admit anyone.
            </p>
            {roles.map((role, index) => (
              <fieldset key={index}>
                <legend>Role {index + 1}</legend>
                <label>
                  Role key
                  <input
                    required
                    value={role.key}
                    onChange={(event) =>
                      setRoles((current) =>
                        current.map((r, i) =>
                          i === index ? { ...r, key: event.target.value } : r,
                        ),
                      )
                    }
                  />
                </label>
                <label>
                  Role schema family
                  <select value={role.schema_family} onChange={() => {}}>
                    <option value="source_catalog_v1">source_catalog_v1</option>
                  </select>
                </label>
                <label>
                  Role description
                  <textarea
                    maxLength={1000}
                    value={role.description}
                    onChange={(event) =>
                      setRoles((current) =>
                        current.map((r, i) =>
                          i === index
                            ? { ...r, description: event.target.value }
                            : r,
                        ),
                      )
                    }
                  />
                </label>
                <button
                  type="button"
                  onClick={() =>
                    setRoles((current) => current.filter((_, i) => i !== index))
                  }
                >
                  Remove recruitment role {index + 1}
                </button>
              </fieldset>
            ))}
            <button
              type="button"
              disabled={roles.length >= 10}
              onClick={() =>
                setRoles((current) => [
                  ...current,
                  {
                    key: "",
                    schema_family: "source_catalog_v1",
                    description: "",
                  },
                ])
              }
            >
              Add recruitment role
            </button>
          </section>
          <p>
            Admitted members can read the full room history, including earlier
            messages. Creation does not approve work, rights, publication or
            payments.
          </p>
          <label>
            <input
              type="checkbox"
              checked={acknowledged}
              onChange={(event) => setAcknowledged(event.target.checked)}
            />
            I understand admitted members can read the full room history.
          </label>
          <section aria-label="Room creation review">
            <h4>Room creation review</h4>
            {origin ? (
              <pre>{JSON.stringify(origin.opportunity, null, 2)}</pre>
            ) : (
              <p>
                No selected match. This room is based on the explicit objective.
              </p>
            )}
            <pre>{JSON.stringify(command, null, 2)}</pre>
            <label>
              <input
                type="checkbox"
                checked={reviewed === review}
                onChange={(event) =>
                  setReviewed(event.target.checked ? review : "")
                }
              />
              I reviewed this room setup
            </label>
          </section>
          <button disabled={!valid || !acknowledged || reviewed !== review}>
            Create swarm
          </button>
        </fieldset>
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
