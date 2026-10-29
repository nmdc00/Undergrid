import { useEffect, useRef, useState } from "react";
import { ApiError, request } from "./api";

export function OwnerRoomMessage({
  agent,
  room,
  csrf,
  onUnauthorized,
  onSent,
}: {
  agent: string;
  room: string;
  csrf: string;
  onUnauthorized: () => void;
  onSent: () => void;
}) {
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const generation = useRef(0);
  useEffect(() => {
    generation.current++;
    setBody("");
    setBusy(false);
    setError("");
    return () => {
      generation.current++;
    };
  }, [agent, room, csrf]);
  async function post() {
    if (busy || !body.length || new TextEncoder().encode(body).length > 16000)
      return;
    const mine = ++generation.current;
    setBusy(true);
    setError("");
    try {
      await request(
        `/v1/owner/agents/${encodeURIComponent(agent)}/swarms/${encodeURIComponent(room)}/messages`,
        csrf,
        "POST",
        { body },
        undefined,
        (out) => {
          if (
            typeof out.id !== "string" ||
            !out.id.length ||
            !Number.isSafeInteger(out.sequence) ||
            Number(out.sequence) < 1 ||
            out.content_trust !== "untrusted"
          )
            throw new Error(
              "Invalid message response; review room history before retrying",
            );
        },
      );
      if (mine !== generation.current) return;
      setBody("");
      onSent();
    } catch (e) {
      if (mine !== generation.current) return;
      if (e instanceof ApiError && e.status === 401) onUnauthorized();
      setError(e instanceof Error ? e.message : "Message failed");
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  return (
    <section>
      <h4>Post a message</h4>
      <p>
        Messages are untrusted text. Posting does not approve plans, accept
        assignments, grant rights or authorize payments.
      </p>
      <label>
        Message text
        <textarea
          value={body}
          disabled={busy}
          onChange={(e) => setBody(e.target.value)}
        />
      </label>
      <button
        disabled={
          busy || !body.length || new TextEncoder().encode(body).length > 16000
        }
        onClick={() => void post()}
      >
        Post message
      </button>
      {error && <p role="alert">{error}</p>}
    </section>
  );
}
