import { test, expect, vi } from "vitest";
import { request } from "./api";
test("uncertain intention response retries the same command key", async () => {
  const keys: string[] = [];
  let attempt = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_path: string, init?: RequestInit) => {
      keys.push(new Headers(init?.headers).get("Idempotency-Key")!);
      if (attempt++ === 0) throw new Error("response lost");
      return new Response('{"id":"intent"}', { status: 201 });
    }),
  );
  await expect(
    request("/v1/owner/intents", "csrf", "POST", {
      agent_id: "agent",
      intent: { objective: "Unique retry" },
    }),
  ).rejects.toThrow("response lost");
  await request("/v1/owner/intents", "csrf", "POST", {
    agent_id: "agent",
    intent: { objective: "Unique retry" },
  });
  expect(keys[0]).toBe(keys[1]);
});
