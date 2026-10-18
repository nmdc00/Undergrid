import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  ApiError,
  items,
  object,
  request,
  text,
  type ObjectValue,
} from "./api";

type Lead = {
  id: string;
  origin: string;
  url: string;
  publisher: string;
  notice: string;
  title: string;
  summary: string;
  checked: string;
  freshUntil: string;
};
function parseLead(raw: ObjectValue): Lead {
  const metadata = object(raw.metadata);
  const checked = text(raw.checked_at),
    freshUntil = text(raw.fresh_until);
  if (
    !Number.isFinite(Date.parse(checked)) ||
    !Number.isFinite(Date.parse(freshUntil))
  )
    throw new Error("Invalid lead timestamps");
  const origin = text(raw.origin);
  if (origin !== "external_capability" && origin !== "external_request")
    throw new Error("Invalid lead origin");
  return {
    id: text(raw.id),
    origin,
    url: text(raw.canonical_url),
    publisher: text(raw.publisher),
    notice: text(raw.permission_notice),
    title:
      metadata.title === undefined ? "External listing" : text(metadata.title),
    summary: metadata.summary === undefined ? "" : text(metadata.summary),
    checked,
    freshUntil,
  };
}
function safeSourceURL(raw: string): string | undefined {
  try {
    const url = new URL(raw);
    if (url.protocol === "https:" && !url.username && !url.password)
      return url.href;
  } catch {
    /* Leave malformed links inert. */
  }
}
export function ExternalLeads({
  intentionID,
  csrf,
  onUnauthorized,
}: {
  intentionID: string;
  csrf: string;
  onUnauthorized: () => void;
}) {
  const [query, setQuery] = useState(""),
    [kind, setKind] = useState("");
  const [leads, setLeads] = useState<Lead[]>([]),
    [cursor, setCursor] = useState("");
  const [applied, setApplied] = useState(""),
    [busy, setBusy] = useState(false);
  const [error, setError] = useState(""),
    [searched, setSearched] = useState(false);
  const generation = useRef(0);
  useEffect(() => {
    generation.current++;
    setLeads([]);
    setCursor("");
    setError("");
    setBusy(false);
    setSearched(false);
    return () => {
      generation.current++;
    };
  }, [intentionID, csrf]);
  async function load(base: string, after = "") {
    const mine = ++generation.current;
    setBusy(true);
    setError("");
    if (!after) {
      setLeads([]);
      setCursor("");
      setSearched(false);
    }
    try {
      const params = new URLSearchParams(base);
      if (after) params.set("cursor", after);
      const result = await request(
        `/v1/owner/intents/${encodeURIComponent(intentionID)}/external-leads?${params}`,
        csrf,
      );
      const rows = items(result);
      if (rows.length > 20) throw new Error("Invalid lead page");
      const next = text(result.next_cursor);
      const parsed = rows.map(parseLead);
      if (mine !== generation.current) return;
      setLeads((previous) =>
        Array.from(
          new Map(
            (after ? [...previous, ...parsed] : parsed).map((lead) => [
              lead.id,
              lead,
            ]),
          ).values(),
        ).slice(0, 200),
      );
      setCursor(next);
      setApplied(base);
      setSearched(true);
    } catch (failure) {
      if (mine !== generation.current) return;
      setLeads([]);
      setCursor("");
      setSearched(false);
      if (failure instanceof ApiError && failure.status === 401) {
        setError("Your session expired. Sign in again.");
        onUnauthorized();
      } else if (
        failure instanceof ApiError &&
        (failure.status === 403 || failure.status === 404)
      ) {
        setError(
          "This intention is no longer available for discovery. Refresh your intentions.",
        );
      } else
        setError("External leads could not be loaded. Try searching again.");
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  function search(event: FormEvent) {
    event.preventDefault();
    const params = new URLSearchParams({ limit: "20" });
    if (query) params.set("q", query);
    if (kind) params.set("kind", kind);
    void load(params.toString());
  }
  return (
    <section aria-label="External leads">
      <h4>External leads</h4>
      <p>
        These listings come from external sources. Identity and fit are
        unverified.
      </p>
      <form onSubmit={search}>
        <label>
          Search external leads
          <input
            value={query}
            maxLength={200}
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>
        <label>
          Lead type
          <select
            value={kind}
            onChange={(event) => setKind(event.target.value)}
          >
            <option value="">All external leads</option>
            <option value="external_capability">External capabilities</option>
            <option value="external_request">External requests</option>
          </select>
        </label>
        <button disabled={busy}>Find external leads</button>
      </form>
      {busy && <p role="status">Loading external leads…</p>}
      {error && <p role="alert">{error}</p>}
      {searched && leads.length === 0 && (
        <p>No current permitted leads match these filters.</p>
      )}
      <ul>
        {leads.map((lead) => {
          const source = safeSourceURL(lead.url);
          return (
            <li key={lead.id}>
              <h5>{lead.title}</h5>
              <p>
                {lead.origin === "external_capability"
                  ? "External capability"
                  : "External request"}{" "}
                · {lead.publisher}
              </p>
              {lead.summary && <p>{lead.summary}</p>}
              <p>{lead.notice}</p>
              <p>
                Checked{" "}
                <time dateTime={lead.checked}>
                  {new Date(lead.checked).toLocaleString()}
                </time>
                . Fresh until{" "}
                <time dateTime={lead.freshUntil}>
                  {new Date(lead.freshUntil).toLocaleString()}
                </time>
                .
              </p>
              {source && (
                <a href={source} target="_blank" rel="noopener noreferrer">
                  Open source
                </a>
              )}
            </li>
          );
        })}
      </ul>
      {cursor && leads.length < 200 && (
        <button disabled={busy} onClick={() => void load(applied, cursor)}>
          Load more leads
        </button>
      )}
      {leads.length >= 200 && cursor && (
        <p>Refine your search to see more leads.</p>
      )}
    </section>
  );
}
