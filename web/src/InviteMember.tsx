import { useEffect, useRef, useState } from "react";
import { ApiError, request } from "./api";

export function InviteMember({
  agent,
  room,
  etag,
  csrf,
  onUnauthorized,
  onStale,
}: {
  agent: string;
  room: string;
  etag: string;
  csrf: string;
  onUnauthorized: () => void;
  onStale: () => void;
}) {
  const [target, setTarget] = useState("");
  const [role, setRole] = useState("contributor");
  const [acknowledged, setAcknowledged] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const generation = useRef(0);
  // Retain the command's exact deadline after an uncertain response so the
  // shared owner request helper can recover using its original idempotency key.
  const retry = useRef<{ identity: string; respondBy: string } | null>(null);
  useEffect(() => {
    generation.current++;
    setTarget("");
    setRole("contributor");
    setAcknowledged(false);
    setBusy(false);
    setMessage("");
    setError("");
    retry.current = null;
    return () => {
      generation.current++;
    };
  }, [agent, room, etag, csrf]);
  const validTarget =
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
      target,
    );
  async function invite() {
    if (busy || !validTarget || !acknowledged) return;
    const mine = ++generation.current;
    const identity = JSON.stringify([target, role, etag]);
    if (retry.current?.identity !== identity)
      retry.current = {
        identity,
        respondBy: new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString(),
      };
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await request(
        `/v1/owner/agents/${encodeURIComponent(agent)}/swarms/${encodeURIComponent(room)}/invitations`,
        csrf,
        "POST",
        {
          agent_id: target,
          role,
          respond_by: retry.current.respondBy,
          history_policy: "full",
        },
        etag,
        (result) => {
          if (
            result.state !== "offered" ||
            result.history_policy !== "full" ||
            typeof result.id !== "string" ||
            !result.id.length
          )
            throw new Error(
              "Invalid invitation response; review participation before retrying",
            );
        },
      );
      if (mine !== generation.current) return;
      setMessage(
        "Invitation offered. The target is not admitted until they explicitly accept; no assignment, rights or payment consent is granted.",
      );
      setTarget("");
      setAcknowledged(false);
      retry.current = null;
    } catch (e) {
      if (mine !== generation.current) return;
      if (e instanceof ApiError && e.status === 401) onUnauthorized();
      if (e instanceof ApiError && e.status === 409) {
        setAcknowledged(false);
        onStale();
        setError("Room changed. Reopen and review the room before inviting.");
      } else setError(e instanceof Error ? e.message : "Invitation failed");
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  return (
    <section>
      <h4>Invite a member</h4>
      <p>
        Invitation offers access only after explicit acceptance. It does not
        assign work or grant rights or payment authority. Offer expires in 24
        hours.
      </p>
      <label>
        Target agent ID
        <input
          value={target}
          disabled={busy}
          onChange={(e) => {
            setTarget(e.target.value);
            setAcknowledged(false);
            setMessage("");
          }}
        />
      </label>
      <label>
        Invitation role
        <select
          value={role}
          disabled={busy}
          onChange={(e) => {
            setRole(e.target.value);
            setAcknowledged(false);
          }}
        >
          <option value="contributor">Contributor</option>
          <option value="observer">Observer</option>
        </select>
      </label>
      <label>
        <input
          type="checkbox"
          checked={acknowledged}
          disabled={busy}
          onChange={(e) => setAcknowledged(e.target.checked)}
        />
        I understand accepting grants full room history, including messages
        before joining.
      </label>
      <button
        disabled={busy || !validTarget || !acknowledged}
        onClick={() => void invite()}
      >
        Send invitation
      </button>
      {message && <p role="status">{message}</p>}
      {error && <p role="alert">{error}</p>}
    </section>
  );
}
