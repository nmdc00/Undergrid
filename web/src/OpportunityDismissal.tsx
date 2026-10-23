import { useEffect, useRef, useState } from "react";
import { ApiError, request } from "./api";

export function OpportunityDismissal({
  agent,
  opportunity,
  csrf,
  onDismissed,
  onUnauthorized,
}: {
  agent: string;
  opportunity: string;
  csrf: string;
  onDismissed: () => void;
  onUnauthorized: () => void;
}) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const generation = useRef(0);
  useEffect(() => {
    generation.current++;
    setBusy(false);
    setError("");
    return () => {
      generation.current++;
    };
  }, [agent, opportunity, csrf]);
  async function dismiss() {
    if (busy) return;
    const mine = ++generation.current;
    setBusy(true);
    setError("");
    try {
      await request(
        `/v1/owner/agents/${encodeURIComponent(agent)}/opportunities/${encodeURIComponent(opportunity)}/dismiss`,
        csrf,
        "POST",
        {},
        undefined,
        (out) => {
          if (out.id !== opportunity || out.state !== "dismissed")
            throw new Error(
              "Invalid dismissal response; retry to recover the result",
            );
        },
      );
      if (mine === generation.current) onDismissed();
    } catch (e) {
      if (mine !== generation.current) return;
      if (e instanceof ApiError && e.status === 401) onUnauthorized();
      setError(e instanceof Error ? e.message : "Dismissal failed");
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  return (
    <>
      <button disabled={busy} onClick={() => void dismiss()}>
        Dismiss suggestion
      </button>
      {error && <p role="alert">{error}</p>}
    </>
  );
}
