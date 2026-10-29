import { useEffect, useRef, useState } from "react";
import { ApiError, object, request, text, type ObjectValue } from "./api";
type InboxEvent = {
  event_id: string;
  sequence: number;
  type: string;
  resource_path: string;
};
function eventItem(raw: unknown): InboxEvent {
  const x = object(raw);
  if (
    typeof x.sequence !== "number" ||
    !Number.isSafeInteger(x.sequence) ||
    x.sequence < 1 ||
    x.content_trust !== "untrusted"
  )
    throw Error("Invalid inbox event");
  const path = text(x.resource_path);
  // References are displayed as text, never executed or followed as callbacks.
  if (!/^\/v1\/[A-Za-z0-9/_-]+$/.test(path))
    throw Error("Invalid inbox resource reference");
  return {
    event_id: text(x.event_id),
    sequence: x.sequence,
    type: text(x.type),
    resource_path: path,
  };
}
export function OwnerInbox({
  agents,
  csrf,
  onUnauthorized,
}: {
  agents: ObjectValue[];
  csrf: string;
  onUnauthorized: () => void;
}) {
  const active = agents.filter((a) => a.status === "active"),
    [agent, setAgent] = useState(active[0] ? text(active[0].id) : "");
  const [events, setEvents] = useState<InboxEvent[]>([]),
    [cursor, setCursor] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [loaded, setLoaded] = useState(false),
    [expired, setExpired] = useState(false),
    [acknowledged, setAcknowledged] = useState<Record<string, boolean>>({});
  const generation = useRef(0),
    available = active.some((a) => a.id === agent);
  function clear() {
    generation.current++;
    setEvents([]);
    setCursor("");
    setAcknowledged({});
    setLoaded(false);
    setBusy(false);
    setExpired(false);
    setError("");
  }
  useEffect(() => {
    clear();
    return () => {
      generation.current++;
    };
  }, [agent, csrf, available]);
  useEffect(() => {
    if (!available) setAgent(active[0] ? text(active[0].id) : "");
  }, [available, active[0]?.id]);
  const base = `/v1/owner/agents/${encodeURIComponent(agent)}/inbox`;
  function failed(e: unknown, mine: number) {
    if (mine !== generation.current) return;
    if (e instanceof ApiError && [401, 403, 404, 410].includes(e.status)) {
      setEvents([]);
      setCursor("");
      setAcknowledged({});
      setLoaded(false);
      if (e.status === 410) {
        setExpired(true);
        setError(
          "Inbox cursor expired. Reset explicitly to inspect retained notifications.",
        );
        return;
      }
      if (e.status === 401) onUnauthorized();
    }
    setError(e instanceof Error ? e.message : "Inbox request failed");
  }
  async function load(after = "") {
    const mine = ++generation.current;
    setBusy(true);
    setError("");
    if (!after) {
      setEvents([]);
      setCursor("");
      setAcknowledged({});
      setLoaded(false);
    }
    try {
      const x = await request(
        `${base}?limit=50${after ? `&cursor=${encodeURIComponent(after)}` : ""}`,
        csrf,
      );
      if (!Array.isArray(x.events) || x.events.length > 50)
        throw Error("Invalid inbox page");
      const items = x.events.map(eventItem),
        next = text(x.next_cursor);
      if (
        !next ||
        next.length > 1024 ||
        items.some((e, i) => i > 0 && e.sequence <= items[i - 1].sequence)
      )
        throw Error("Invalid inbox cursor or sequence");
      if (mine !== generation.current) return;
      setEvents((old) =>
        after
          ? [
              ...old,
              ...items.filter(
                (e) => !old.some((prior) => prior.event_id === e.event_id),
              ),
            ]
          : items,
      );
      setCursor(next);
      setExpired(false);
      setLoaded(true);
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  async function acknowledge(event: InboxEvent) {
    if (busy) return;
    const mine = ++generation.current;
    setBusy(true);
    setError("");
    try {
      await request(
        `${base}/${encodeURIComponent(event.event_id)}/ack`,
        csrf,
        "POST",
        {},
        undefined,
        (result) => {
          if (result.acknowledged !== true)
            throw Error("Invalid receipt confirmation");
        },
      );
      if (mine === generation.current)
        setAcknowledged((old) => ({ ...old, [event.event_id]: true }));
    } catch (e) {
      if (e instanceof ApiError && e.status < 500) failed(e, mine);
      else if (mine === generation.current)
        setError(
          "Receipt could not be confirmed. Retry acknowledgement of the same event.",
        );
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  return (
    <section>
      <h2>Agent inbox</h2>
      <p>
        Acknowledgement records receipt only. It does not accept invitations,
        assignments, plans, rights or spending.
      </p>
      <label>
        Inbox agent
        <select
          aria-label="Inbox agent"
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
      <button
        disabled={busy || !available || expired}
        onClick={() => void load()}
      >
        Refresh inbox
      </button>
      {error && <p role="alert">{error}</p>}
      {expired && (
        <>
          <p>
            Review pending invitations and assignments separately. Reset shows
            retained notifications and does not recover expired history.
          </p>
          <button disabled={busy || !available} onClick={() => void load()}>
            Reset to retained inbox
          </button>
        </>
      )}
      {loaded && events.length === 0 && (
        <p>No retained notifications on this page.</p>
      )}
      <ol>
        {events.map((e) => (
          <li key={e.event_id}>
            <strong>
              {e.type === "opportunity.available"
                ? "New discovery result"
                : e.type === "intent.created"
                  ? "Intention saved"
                  : "Agent update"}
            </strong>
            <small>{e.type}</small>
            {e.type === "opportunity.available" && (
              <p>
                <a href="#intentions-monitoring">
                  Review current intention results
                </a>
                . Choose your intention and select Review intention to inspect
                current evidence and source details.
              </p>
            )}
            <p>
              Reference: <code>{e.resource_path}</code>. Untrusted event
              reference.
            </p>
            {acknowledged[e.event_id] ? (
              <p role="status">Receipt acknowledged.</p>
            ) : (
              <button disabled={busy} onClick={() => void acknowledge(e)}>
                Acknowledge receipt
              </button>
            )}
          </li>
        ))}
      </ol>
      {cursor && !expired && (
        <button disabled={busy || !available} onClick={() => void load(cursor)}>
          Load more inbox
        </button>
      )}
    </section>
  );
}
