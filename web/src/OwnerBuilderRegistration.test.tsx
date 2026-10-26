import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { App } from "./App";

let sequence = 0;
const privateProfile = { public: false, accept_invitations: false, recruitment: false, capabilities: [] };
function fixture(malformedFirst = false) {
  const calls: { path: string; init?: RequestInit }[] = [];
  let profile = privateProfile;
  let created = false, attempts = 0;
  vi.stubGlobal("fetch", vi.fn(async (path: string, init?: RequestInit) => {
    calls.push({ path, init });
    let value: unknown = { items: [] };
    if (path === "/auth/session") value = { csrf_token: `builder-csrf-${++sequence}` };
    if (path === "/v1/owner/config") value = { terms_version: "fixture-terms", contribution_templates: { templates: [] } };
    if (path === "/v1/owner/me") value = { id: "owner" };
    if (path === "/v1/owner/policy") value = { etag: '"1"', policy: { template_ids: [], license_ids: [], task_categories: [], max_active_tasks: 0, expires_at: "2030-01-01T00:00:00Z" } };
    if (path === "/v1/agents") {
      if (init?.method === "POST") {
        profile = JSON.parse(String(init.body)).profile;
        created = true;
        value = malformedFirst && attempts++ === 0 ? {} : { id: "builder", credential_id: "credential", token: "ug_builder_secret" };
      } else value = { items: created ? [{ id: "builder", name: "Declared builder", status: "active", version: 1, etag: '"1"', profile, credentials: [] }] : [] };
    }
    return new Response(JSON.stringify(value), { headers: { "Content-Type": "application/json" } });
  }));
  return calls;
}
function creation(calls: ReturnType<typeof fixture>) { return calls.filter(c => c.path === "/v1/agents" && c.init?.method === "POST"); }
async function name() { await userEvent.type(await screen.findByLabelText("Agent name"), "Declared builder"); }

test("registration reviews exact private defaults before sending its explicit profile", async () => {
  const calls = fixture(); render(<App />); await name();
  expect((screen.getByRole("button", { name: "Register agent" }) as HTMLButtonElement).disabled).toBe(true);
  for (const label of ["Make declared profile public for recruitment", "Accept foreign invitations", "Receive recruitment opportunities", "Declare source_catalog_v1 capability"])
    expect((screen.getByLabelText(label) as HTMLInputElement).checked).toBe(false);
  const preview = screen.getByRole("region", { name: "Declared profile review" });
  expect(JSON.parse(within(preview).getByTestId("profile-json").textContent!)).toEqual(privateProfile);
  await userEvent.click(screen.getByLabelText("I reviewed this declared profile"));
  await userEvent.click(screen.getByRole("button", { name: "Register agent" }));
  await screen.findByText("ug_builder_secret");
  expect(JSON.parse(String(creation(calls)[0].init?.body))).toEqual({ name: "Declared builder", scopes: ["intents:read", "intents:write", "inbox:read"], profile: privateProfile });
  expect(screen.getByRole("region", { name: "Saved profile for Declared builder" }).textContent).toContain('"public": false');
});

test("each profile change invalidates review and recruitment does not select invitations", async () => {
  const calls = fixture(); render(<App />); await name();
  await userEvent.click(screen.getByLabelText("I reviewed this declared profile"));
  await userEvent.click(screen.getByLabelText("Make declared profile public for recruitment"));
  expect((screen.getByLabelText("I reviewed this declared profile") as HTMLInputElement).checked).toBe(false);
  await userEvent.click(screen.getByLabelText("Receive recruitment opportunities"));
  await userEvent.click(screen.getByLabelText("Declare source_catalog_v1 capability"));
  expect((screen.getByLabelText("Accept foreign invitations") as HTMLInputElement).checked).toBe(false);
  await userEvent.click(screen.getByLabelText("I reviewed this declared profile"));
  await userEvent.click(screen.getByRole("button", { name: "Register agent" }));
  await screen.findByText("ug_builder_secret");
  expect(JSON.parse(String(creation(calls)[0].init?.body)).profile).toEqual({ public: true, recruitment: true, accept_invitations: false, capabilities: ["source_catalog_v1"] });
});

test("uncertain creation keeps reviewed profile and identical command key for retry", async () => {
  const calls = fixture(true); render(<App />); await name();
  await userEvent.click(screen.getByLabelText("Receive recruitment opportunities"));
  await userEvent.click(screen.getByLabelText("I reviewed this declared profile"));
  await userEvent.click(screen.getByRole("button", { name: "Register agent" }));
  await screen.findByRole("alert");
  expect((screen.getByLabelText("Agent name") as HTMLInputElement).value).toBe("Declared builder");
  await userEvent.click(screen.getByRole("button", { name: "Register agent" }));
  await screen.findByText("ug_builder_secret");
  await waitFor(() => expect(creation(calls)).toHaveLength(2));
  const [first, retry] = creation(calls);
  expect(retry.init?.body).toBe(first.init?.body);
  expect(new Headers(retry.init?.headers).get("Idempotency-Key")).toBe(new Headers(first.init?.headers).get("Idempotency-Key"));
});

test("a completed registration does not clear a different draft edited while pending", async () => {
  fixture();
  const original = globalThis.fetch;
  let complete!: () => void;
  const waiting = new Promise<void>(resolve => { complete = resolve; });
  vi.stubGlobal("fetch", vi.fn(async (path: string, init?: RequestInit) => {
    if (path === "/v1/agents" && init?.method === "POST") await waiting;
    return original(path, init);
  }));
  render(<App />); await name();
  await userEvent.click(screen.getByLabelText("I reviewed this declared profile"));
  await userEvent.click(screen.getByRole("button", { name: "Register agent" }));
  const input = screen.getByLabelText("Agent name") as HTMLInputElement;
  // If editing is disabled during submission there is no second mutable draft.
  const editingDisabled = input.disabled;
  if (!editingDisabled) {
    await userEvent.clear(input);
    await userEvent.type(input, "Next separate builder");
  }
  complete();
  await screen.findByText("ug_builder_secret");
  expect(input.value).toBe(editingDisabled ? "" : "Next separate builder");
});
