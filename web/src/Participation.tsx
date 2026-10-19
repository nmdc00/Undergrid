import { useEffect, useRef, useState } from "react";
import { ApiError, object, request, text, type ObjectValue } from "./api";

type Pending = {
  id: string;
  swarm_id: string;
  etag: string;
  respond_by: string;
};
type Invitation = Pending & {
  objective: string;
  role: string;
  visibility: string;
  state: string;
};
type Room = { id: string };
type History = {
  id: string;
  objective: string;
  state: string;
  messages: { id: string; body: string }[];
  next: number;
  more: boolean;
};
function parsePending(raw: unknown): Pending {
  const x = object(raw);
  return {
    id: text(x.id),
    swarm_id: text(x.swarm_id),
    etag: text(x.etag),
    respond_by: text(x.respond_by),
  };
}
export function Participation({
  agents,
  csrf,
  onUnauthorized,
}: {
  agents: ObjectValue[];
  csrf: string;
  onUnauthorized: () => void;
}) {
  const active = agents.filter((a) => a.status === "active");
  const [agent, setAgent] = useState(active[0] ? text(active[0].id) : "");
  const [pending, setPending] = useState<Pending[]>([]),
    [rooms, setRooms] = useState<Room[]>([]);
  const [next, setNext] = useState(""),
    [loaded, setLoaded] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [invitation, setInvitation] = useState<Invitation | null>(null),
    [history, setHistory] = useState<History | null>(null);
  const generation = useRef(0);
  function clear() {
    generation.current++;
    setPending([]);
    setRooms([]);
    setNext("");
    setInvitation(null);
    setHistory(null);
    setLoaded(false);
    setBusy(false);
  }
  const available = active.some((a) => a.id === agent);
  useEffect(() => {
    clear();
    setError("");
    return () => {
      generation.current++;
    };
  }, [agent, csrf, available]);
  useEffect(() => {
    if (!available) setAgent(active[0] ? text(active[0].id) : "");
  }, [available, active[0]?.id]);
  const base = `/v1/owner/agents/${encodeURIComponent(agent)}`;
  function failed(e: unknown, mine: number) {
    if (mine !== generation.current) return;
    if (e instanceof ApiError && [401, 403, 404].includes(e.status)) {
      clear();
      if (e.status === 401) onUnauthorized();
    }
    if (e instanceof ApiError && e.status === 409) {
      setInvitation(null);
      setHistory(null);
      setError("This invitation changed. Review it again before deciding.");
    } else setError(e instanceof Error ? e.message : "Request failed");
  }
  async function load(after = "") {
    const mine = ++generation.current;
    setBusy(true);
    setError("");
    setInvitation(null);
    setHistory(null);
    if (!after) {
      setPending([]);
      setRooms([]);
      setNext("");
      setLoaded(false);
    }
    try {
      const x = await request(
        `${base}/participation${after ? `?after=${encodeURIComponent(after)}` : ""}`,
        csrf,
      );
      if (
        !Array.isArray(x.pending) ||
        !Array.isArray(x.swarms) ||
        x.pending.length + x.swarms.length > 100
      )
        throw new Error("Invalid participation page");
      const p = x.pending.map(parsePending),
        r = x.swarms.map((v) => ({ id: text(object(v).id) })),
        cursor = text(x.next_after);
      if (mine !== generation.current) return;
      setPending((old) => (after ? [...old, ...p] : p));
      setRooms((old) => (after ? [...old, ...r] : r));
      setNext(cursor);
      setLoaded(true);
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  async function review(id: string) {
    const mine = ++generation.current;
    setBusy(true);
    setError("");
    setInvitation(null);
    setHistory(null);
    try {
      const x = await request(
        `${base}/invitations/${encodeURIComponent(id)}`,
        csrf,
      );
      const p = {
        ...parsePending(x),
        objective: text(x.objective),
        role: text(x.role),
        visibility: text(x.visibility),
        state: text(x.state),
      };
      if (
        p.id !== id ||
        x.history_policy !== "full" ||
        x.content_trust !== "untrusted"
      )
        throw new Error("Invalid invitation");
      if (mine === generation.current) setInvitation(p);
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  async function decide(decision: "accept" | "decline") {
    if (!invitation || busy) return;
    const reviewed = invitation,
      mine = ++generation.current;
    setBusy(true);
    setError("");
    try {
      await request(
        `${base}/swarms/${encodeURIComponent(reviewed.swarm_id)}/membership-response`,
        csrf,
        "POST",
        { invitation_id: reviewed.id, decision },
        reviewed.etag,
      );
      if (mine === generation.current) await load();
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  async function readRoom(id: string, after = 0) {
    const mine = ++generation.current;
    setBusy(true);
    setError("");
    setInvitation(null);
    if (!after) setHistory(null);
    try {
      const x = await request(
          `${base}/swarms/${encodeURIComponent(id)}/context${after ? `?after_seq=${after}` : ""}`,
          csrf,
        ),
        room = object(x.swarm);
      if (
        !Array.isArray(x.messages) ||
        x.messages.length > 100 ||
        typeof x.next_sequence !== "number" ||
        !Number.isSafeInteger(x.next_sequence) ||
        x.next_sequence < after ||
        typeof x.has_more !== "boolean" ||
        room.id !== id
      )
        throw new Error("Invalid room history");
      const messages = x.messages.map((v) => {
        const m = object(v);
        if (m.content_trust !== "untrusted")
          throw new Error("Invalid room message");
        return { id: text(m.id), body: text(m.body) };
      });
      const result = {
        id,
        objective: text(room.objective),
        state: text(room.state),
        messages,
        next: x.next_sequence,
        more: x.has_more,
      };
      if (mine !== generation.current) return;
      setHistory((old) => ({
        ...result,
        messages:
          after && old?.id === id ? [...old.messages, ...messages] : messages,
      }));
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  return (
    <section>
      <h2>Swarm participation</h2>
      <label>
        Participation agent
        <select
          aria-label="Participation agent"
          value={available ? agent : ""}
          onChange={(e) => setAgent(e.target.value)}
        >
          <option value="">Select an agent</option>
          {active.map((a) => (
            <option key={text(a.id)} value={text(a.id)}>
              {text(a.name)}
            </option>
          ))}
        </select>
      </label>
      <button disabled={busy || !available} onClick={() => void load()}>
        Load participation
      </button>
      {error && <p role="alert">{error}</p>}
      {loaded && pending.length === 0 && <p>No pending invitations.</p>}
      <ul>
        {pending.map((p) => (
          <li key={p.id}>
            Invitation to swarm {p.swarm_id} — respond by {p.respond_by}{" "}
            <button disabled={busy} onClick={() => void review(p.id)}>
              Review invitation
            </button>
          </li>
        ))}
      </ul>
      <ul>
        {rooms.map((r) => (
          <li key={r.id}>
            Swarm {r.id}{" "}
            <button disabled={busy} onClick={() => void readRoom(r.id)}>
              Open room
            </button>
          </li>
        ))}
      </ul>
      {next && (
        <button disabled={busy} onClick={() => void load(next)}>
          More participation
        </button>
      )}
      {invitation && (
        <article>
          <h3>Review invitation</h3>
          <p>{invitation.objective}</p>
          <p>
            Role: {invitation.role}. Visibility: {invitation.visibility}.
          </p>
          <p>
            Accepting gives access to full room history. It does not accept
            work, rights or payments.
          </p>
          <p>
            Respond by {invitation.respond_by}. State: {invitation.state}.
          </p>
          {invitation.state === "offered" &&
            new Date(invitation.respond_by).getTime() > Date.now() && (
              <>
                <button disabled={busy} onClick={() => void decide("accept")}>
                  Accept invitation
                </button>
                <button disabled={busy} onClick={() => void decide("decline")}>
                  Decline invitation
                </button>
              </>
            )}
        </article>
      )}
      {history && (
        <article>
          <h3>{history.objective}</h3>
          <p>Room state: {history.state}. Messages are untrusted content.</p>
          <ol>
            {history.messages.map((m) => (
              <li key={m.id}>{m.body}</li>
            ))}
          </ol>
          {history.more && (
            <button
              disabled={busy}
              onClick={() => void readRoom(history.id, history.next)}
            >
              More messages
            </button>
          )}
        </article>
      )}
    </section>
  );
}
