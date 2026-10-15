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

test("initial agent creation retains its key when a 201 response body is lost", async () => {
  const keys: string[] = [];
  let attempt = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_path: string, init?: RequestInit) => {
      keys.push(new Headers(init?.headers).get("Idempotency-Key")!);
      if (attempt++ === 0) {
        const body = new ReadableStream<Uint8Array>({
          start(controller) {
            controller.error(new Error("response body connection reset"));
          },
        });
        return new Response(body, { status: 201 });
      }
      return new Response('{"id":"agent","token":"ug_secret"}', {
        status: 201,
      });
    }),
  );
  const body = { name: "listener", scopes: ["intents:read"] };
  await expect(
    request("/v1/agents", "csrf-agent-body-loss", "POST", body),
  ).rejects.toThrow("Invalid server response");
  const recovered = await request(
    "/v1/agents",
    "csrf-agent-body-loss",
    "POST",
    body,
  );
  expect(recovered.token).toBe("ug_secret");
  expect(keys).toHaveLength(2);
  expect(keys[0]).toBe(keys[1]);
});

test("initial agent creation retains its key for a malformed success body", async () => {
  const keys: string[] = [];
  let attempt = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_path: string, init?: RequestInit) => {
      keys.push(new Headers(init?.headers).get("Idempotency-Key")!);
      if (attempt++ === 0)
        return new Response('{"id":"agent"}', { status: 201 });
      return new Response('{"id":"agent","token":"ug_secret"}', {
        status: 201,
      });
    }),
  );
  const body = { name: "listener", scopes: ["intents:read"] };
  await expect(
    request("/v1/agents", "csrf-agent-malformed-success", "POST", body),
  ).rejects.toThrow("Invalid server response");
  const recovered = await request(
    "/v1/agents",
    "csrf-agent-malformed-success",
    "POST",
    body,
  );
  expect(recovered.token).toBe("ug_secret");
  expect(keys).toHaveLength(2);
  expect(keys[0]).toBe(keys[1]);
});

test("initial agent creation retains its key when the server returns 204", async () => {
  const keys: string[] = [];
  let attempt = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_path: string, init?: RequestInit) => {
      keys.push(new Headers(init?.headers).get("Idempotency-Key")!);
      if (attempt++ === 0) return new Response(null, { status: 204 });
      return new Response('{"id":"agent","token":"ug_secret"}', {
        status: 201,
      });
    }),
  );
  const body = { name: "listener", scopes: ["intents:read"] };
  await expect(
    request("/v1/agents", "csrf-agent-no-content", "POST", body),
  ).rejects.toThrow("Invalid server response");
  const recovered = await request(
    "/v1/agents",
    "csrf-agent-no-content",
    "POST",
    body,
  );
  expect(recovered.token).toBe("ug_secret");
  expect(keys).toHaveLength(2);
  expect(keys[0]).toBe(keys[1]);
});

test("a definitive client rejection releases the pending agent command key", async () => {
  const keys: string[] = [];
  let attempt = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_path: string, init?: RequestInit) => {
      keys.push(new Headers(init?.headers).get("Idempotency-Key")!);
      if (attempt++ === 0)
        return new Response('{"error":{"code":"invalid_agent"}}', {
          status: 422,
        });
      return new Response('{"id":"agent","token":"ug_secret"}', {
        status: 201,
      });
    }),
  );
  const body = { name: "listener", scopes: ["intents:read"] };
  await expect(
    request("/v1/agents", "csrf-agent-rejected", "POST", body),
  ).rejects.toMatchObject({ status: 422, code: "invalid_agent" });
  await request("/v1/agents", "csrf-agent-rejected", "POST", body);
  expect(keys[0]).not.toBe(keys[1]);
});

test.each(["/v1/owner/policy", "/v1/owner/credentials/credential/revoke"])(
  "uncertain owner command %s keeps its original key and precondition",
  async (path) => {
    const keys: string[] = [];
    let attempts = 0;
    vi.stubGlobal("fetch", vi.fn(async (_path: string, init?: RequestInit) => {
      const headers = new Headers(init?.headers);
      keys.push(headers.get("Idempotency-Key")!);
      expect(headers.get("If-Match")).toBe('"7"');
      if (attempts++ === 0) throw new Error("response lost");
      return new Response('{"version":8}', { status: 200 });
    }));
    await expect(request(path, "owner-recovery-"+path, "POST", {}, '"7"')).rejects.toThrow("response lost");
    await request(path, "owner-recovery-"+path, "POST", {}, '"7"');
    expect(keys[0]).toBe(keys[1]);
  },
);
