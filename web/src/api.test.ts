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

test("initial agent creation retries response loss and drops the old key after form change", async () => {
  const keys: string[] = [];
  let attempt = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_path: string, init?: RequestInit) => {
      keys.push(new Headers(init?.headers).get("Idempotency-Key")!);
      if (attempt++ < 2) throw new Error("response lost");
      return new Response('{"id":"agent","token":"ug_secret"}', {
        status: 201,
      });
    }),
  );
  const original = { name: "listener", scopes: ["intents:read"] };
  await expect(
    request("/v1/agents", "csrf-agent-create", "POST", original),
  ).rejects.toThrow("response lost");
  await expect(
    request("/v1/agents", "csrf-agent-create", "POST", {
      name: "edited listener",
      scopes: original.scopes,
    }),
  ).rejects.toThrow("response lost");
  await request("/v1/agents", "csrf-agent-create", "POST", original);
  expect(keys[0]).not.toBe(keys[1]);
  expect(keys[0]).not.toBe(keys[2]);
});
