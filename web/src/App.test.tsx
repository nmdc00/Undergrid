import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi, test, expect } from "vitest";
import { App } from "./App";

function fixture(registered = true, intentState = "active") {
  const requests: { path: string; init: RequestInit | undefined }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      requests.push({ path, init });
      const values: Record<string, unknown> = {
        "/auth/session": { csrf_token: "session-csrf" },
        "/v1/owner/config": {
          terms_version: "explicit-test-terms",
          contribution_templates: { templates: [] },
        },
        "/v1/owner/me": registered
          ? { id: "owner" }
          : { error: { code: "registration_required" } },
        "/v1/agents":
          init?.method === "POST"
            ? {
                id: "agent",
                token: "ug_one_time_secret",
                credential_id: "credential",
              }
            : {
                items: [
                  {
                    id: "agent",
                    name: "My listener",
                    status: "active",
                    credentials: [
                      {
                        id: "credential",
                        scopes: ["intents:read"],
                        revoked_at: null,
                      },
                    ],
                  },
                ],
              },
        "/v1/owner/intents": {
          items: [
            {
              id: "intent",
              objective: "Private catalog",
              state: intentState,
              version: 1,
            },
          ],
        },
        "/v1/owner/intents/intent": {
          id: "intent",
          etag: '"v1"',
          version: 1,
          state: intentState,
          objective: "Private catalog",
          output: { kind: "json_dataset", schema_family: "source_catalog_v1" },
          hard_constraints: {
            required_fields: ["source_id", "source_url"],
            license_ids: ["CC0-1.0"],
          },
          visibility: "private",
          watch: { enabled: true, channels: ["inbox"] },
          expires_at: "2030-02-01T00:00:00Z",
          budget: null,
        },
        "/v1/owner/intents/intent/opportunities": { items: [] },
        "/v1/owner/policy": {
          etag: '"v1"',
          policy: {
            template_ids: [],
            license_ids: [],
            task_categories: [],
            max_active_tasks: 0,
            expires_at: "2030-01-01T00:00:00Z",
          },
        },
      };
      let result = values[path] ?? {};
      if (path === "/v1/owner/intents/intent" && init?.method === "PATCH") {
        const sent = JSON.parse(String(init.body)) as {
          revision: Record<string, unknown>;
        };
        result = {
          ...(values[path] as Record<string, unknown>),
          ...sent.revision,
          etag: '"v2"',
          version: 2,
          state: intentState === "expired" ? "active" : intentState,
        };
      }
      return new Response(JSON.stringify(result), {
        status: path === "/v1/owner/me" && !registered ? 403 : 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  return requests;
}
test("signed-out owner gets the existing OIDC login route", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("", { status: 401 })),
  );
  render(<App />);
  const login = await screen.findByRole("link", { name: "Sign in" });
  expect(login.getAttribute("href")).toBe("/auth/login");
});
test("registration accepts configured terms explicitly and carries session CSRF", async () => {
  const requests = fixture(false);
  render(<App />);
  await userEvent.type(
    await screen.findByLabelText("Owner name"),
    "Test owner",
  );
  expect(
    (screen.getByRole("button", { name: "Register" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  await userEvent.click(
    screen.getByLabelText("I accept terms explicit-test-terms"),
  );
  await userEvent.click(screen.getByRole("button", { name: "Register" }));
  await waitFor(() =>
    expect(requests.some((r) => r.path === "/v1/principals")).toBe(true),
  );
  const sent = requests.find((r) => r.path === "/v1/principals")!;
  expect(JSON.parse(sent.init!.body as string)).toEqual({
    name: "Test owner",
    terms_version: "explicit-test-terms",
  });
  expect(new Headers(sent.init?.headers).get("X-CSRF-Token")).toBe(
    "session-csrf",
  );
});
test("shows owner records, renders untrusted names as text, logs out with CSRF", async () => {
  const requests = fixture();
  render(<App />);
  await screen.findAllByText("My listener");
  await screen.findByText("Private catalog");
  await userEvent.click(screen.getByRole("button", { name: "Sign out" }));
  await waitFor(() =>
    expect(requests.some((r) => r.path === "/auth/logout")).toBe(true),
  );
  const sent = requests.find((r) => r.path === "/auth/logout")!;
  expect(sent.init?.method).toBe("POST");
  expect(new Headers(sent.init?.headers).get("X-CSRF-Token")).toBe(
    "session-csrf",
  );
});
test("owner can append a revision or renew while preserving typed fields and ETag", async () => {
  const requests = fixture(true, "expired");
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", { name: "Review intention" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Renew intention" }),
  );
  const objectives = screen.getAllByLabelText("Objective");
  await userEvent.clear(objectives[0]);
  await userEvent.type(objectives[0], "Renewed private catalog");
  fireEvent.change(screen.getByLabelText("Expires at (local time)"), {
    target: { value: "2030-03-01T00:00" },
  });
  await userEvent.click(
    screen.getByRole("button", { name: "Renew intention" }),
  );
  await waitFor(() =>
    expect(
      requests.some(
        (r) =>
          r.path === "/v1/owner/intents/intent" && r.init?.method === "PATCH",
      ),
    ).toBe(true),
  );
  const sent = requests.find(
    (r) => r.path === "/v1/owner/intents/intent" && r.init?.method === "PATCH",
  )!;
  expect(new Headers(sent.init?.headers).get("If-Match")).toBe('"v1"');
  expect(new Headers(sent.init?.headers).get("X-CSRF-Token")).toBe(
    "session-csrf",
  );
  expect(JSON.parse(sent.init!.body as string)).toEqual({
    revision: {
      objective: "Renewed private catalog",
      output: { kind: "json_dataset", schema_family: "source_catalog_v1" },
      hard_constraints: {
        required_fields: ["source_id", "source_url"],
        license_ids: ["CC0-1.0"],
      },
      visibility: "private",
      budget: null,
      watch: { enabled: true, channels: ["inbox"] },
      expires_at: new Date("2030-03-01T00:00").toISOString(),
    },
  });
});
test("credential issuance is one-time display and never persisted in browser storage", async () => {
  fixture();
  const storage = vi.spyOn(Storage.prototype, "setItem");
  render(<App />);
  await userEvent.type(
    await screen.findByLabelText("Agent name"),
    "New listener",
  );
  await userEvent.click(screen.getByRole("button", { name: "Register agent" }));
  await screen.findByText("ug_one_time_secret");
  expect(storage).not.toHaveBeenCalled();
  await userEvent.click(
    screen.getByRole("button", { name: "Dismiss credential" }),
  );
  expect(screen.queryByText("ug_one_time_secret")).toBeNull();
  storage.mockRestore();
});
test("policy controls send explicit bounded fields and the reviewed ETag", async () => {
  const requests = fixture();
  render(<App />);
  const expires = await screen.findByLabelText("Policy expires at (UTC)");
  await waitFor(() =>
    expect((expires as HTMLInputElement).value).toBe("2030-01-01T00:00"),
  );
  fireEvent.change(expires, { target: { value: "2030-01-02T00:00" } });
  await userEvent.click(screen.getByRole("button", { name: "Save policy" }));
  await waitFor(() =>
    expect(
      requests.some(
        (r) => r.path === "/v1/owner/policy" && r.init?.method === "POST",
      ),
    ).toBe(true),
  );
  const sent = requests.find(
    (r) => r.path === "/v1/owner/policy" && r.init?.method === "POST",
  )!;
  expect(new Headers(sent.init?.headers).get("If-Match")).toBe('"v1"');
  expect(JSON.parse(sent.init!.body as string)).toEqual({
    template_ids: [],
    license_ids: [],
    task_categories: [],
    max_active_tasks: 0,
    expires_at: "2030-01-02T00:00:00.000Z",
  });
});
