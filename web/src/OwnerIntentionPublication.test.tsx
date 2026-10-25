import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { App } from "./App";

let fixtureNumber = 0;
const privateObjective =
  "Private acquisition objective that must not be published";
const publicSummary = "Public environmental source catalogue";
const output = { kind: "json_dataset", schema_family: "source_catalog_v1" };
const constraints = {
  required_fields: ["source_id", "source_url"],
  license_ids: ["CC0-1.0"],
};

function fixture(visibility = "private", malformedFirstCreate = false) {
  const requests: { path: string; init?: RequestInit }[] = [];
  let createCount = 0;
  let record = {
    id: "intent",
    version: 1,
    current_revision: 1,
    etag: '"1"',
    state: "active",
    objective: privateObjective,
    output,
    hard_constraints: constraints,
    visibility,
    ...(visibility === "redacted_public"
      ? { public_summary: publicSummary }
      : {}),
    preferences: { region: "private-preference-sentinel" },
    budget: { kind: "stated_budget", amount_minor: 3500, currency: "EUR" },
    watch: { enabled: true, channels: ["inbox"] },
    expires_at: "2030-02-01T00:00:00Z",
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      requests.push({ path, init });
      let value: unknown = { items: [] };
      if (path === "/auth/session")
        value = { csrf_token: `publication-csrf-${++fixtureNumber}` };
      if (path === "/v1/owner/config")
        value = {
          terms_version: "fixture-terms",
          contribution_templates: { templates: [] },
        };
      if (path === "/v1/owner/me") value = { id: "owner" };
      if (path === "/v1/agents")
        value = {
          items: [
            {
              id: "agent",
              name: "Owner listener",
              status: "active",
              credentials: [],
            },
          ],
        };
      if (path === "/v1/owner/policy")
        value = {
          etag: '"1"',
          policy: {
            template_ids: [],
            license_ids: [],
            task_categories: [],
            max_active_tasks: 0,
            expires_at: "2030-01-01T00:00:00Z",
          },
        };
      if (path === "/v1/owner/intents") {
        value = {
          items: [
            {
              id: record.id,
              agent_id: "agent",
              objective: record.objective,
              version: record.version,
              state: record.state,
            },
          ],
        };
        if (init?.method === "POST") {
          const sent = JSON.parse(String(init.body)).intent;
          value = {
            ...sent,
            id: "created",
            version: 1,
            current_revision: 1,
            etag: '"1"',
            state: "active",
            budget: null,
          };
          if (malformedFirstCreate && createCount++ === 0) value = {};
        }
      }
      if (path === "/v1/owner/intents/intent") {
        if (init?.method === "PATCH") {
          const sent = JSON.parse(String(init.body));
          record = {
            ...record,
            ...sent.revision,
            version: 2,
            current_revision: 2,
            etag: '"2"',
          };
        if (sent.revision && sent.revision.public_summary === undefined)
          delete record.public_summary;
        }
        value = record;
      }
      if (path === "/v1/owner/intents/intent/grants")
        value = { items: [], version: record.version, etag: record.etag };
      return new Response(JSON.stringify(value), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  return requests;
}

async function preparePublicCreation() {
  await screen.findByRole("heading", { name: "Your workspace" });
  const visibility = screen.getByRole("combobox", {
    name: "Intention visibility",
    exact: true,
  });
  expect((visibility as HTMLSelectElement).value).toBe("private");
  await userEvent.selectOptions(visibility, "redacted_public");
  const creationForm = screen.getByRole("heading", { name: "Create a source-catalog intention", exact: true }).closest("form")!;
  await userEvent.selectOptions(
    within(creationForm).getByLabelText("Responsible agent", { exact: true }),
    "agent",
  );
  await userEvent.type(
    screen.getByLabelText("Objective", { exact: true }),
    privateObjective,
  );
  await userEvent.type(
    screen.getByLabelText("Public summary", { exact: true }),
    publicSummary,
  );
  const preview = screen.getByRole("region", {
    name: "Public projection preview",
    exact: true,
  });
  expect(preview.textContent).toContain(publicSummary);
  expect(preview.textContent).toContain("source_id");
  expect(preview.textContent).toContain("CC0-1.0");
  expect(preview.textContent).not.toContain(privateObjective);
  const publish = screen.getByRole("button", {
    name: "Create intention",
    exact: true,
  }) as HTMLButtonElement;
  expect(publish.disabled).toBe(true);
  await userEvent.click(
    screen.getByLabelText("I reviewed this public projection", { exact: true }),
  );
  expect(publish.disabled).toBe(false);
  return publish;
}

test("owner explicitly reviews the selected public projection before creating a public intention", async () => {
  const requests = fixture();
  render(<App />);
  const publish = await preparePublicCreation();
  await userEvent.click(publish);
  await waitFor(() =>
    expect(
      requests.some(
        (r) => r.path === "/v1/owner/intents" && r.init?.method === "POST",
      ),
    ).toBe(true),
  );
  const sent = requests.find(
    (r) => r.path === "/v1/owner/intents" && r.init?.method === "POST",
  )!;
  expect(JSON.parse(String(sent.init?.body)).intent).toMatchObject({
    objective: privateObjective,
    public_summary: publicSummary,
    visibility: "redacted_public",
    output,
    hard_constraints: constraints,
  });
});

test("owner revises a public intention back to private without losing private typed fields", async () => {
  const requests = fixture("redacted_public");
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", {
      name: "Review intention",
      exact: true,
    }),
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Edit intention", exact: true }),
  );
  const visibility = screen.getByRole("combobox", {
    name: "Revision visibility",
    exact: true,
  });
  expect((visibility as HTMLSelectElement).value).toBe("redacted_public");
  const preview = screen.getByRole("region", {
    name: "Public projection preview",
    exact: true,
  });
  expect(preview.textContent).not.toContain(privateObjective);
  expect(preview.textContent).not.toContain("private-preference-sentinel");
  expect(preview.textContent).not.toContain("3500");
  await userEvent.selectOptions(visibility, "private");
  await userEvent.click(
    screen.getByRole("button", { name: "Save new revision", exact: true }),
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
  const revision = JSON.parse(String(sent.init?.body)).revision;
  expect(revision).toMatchObject({
    visibility: "private",
    objective: privateObjective,
    output,
    hard_constraints: constraints,
    preferences: { region: "private-preference-sentinel" },
    budget: { amount_minor: 3500, currency: "EUR" },
  });
  expect(new Headers(sent.init?.headers).get("If-Match")).toBe('"1"');
  await screen.findByRole("button", { name: "Edit intention", exact: true });
});

test("an uncertain public-create response retains the original reviewed command key and payload", async () => {
  const requests = fixture("private", true);
  render(<App />);
  await userEvent.click(await preparePublicCreation());
  await waitFor(() =>
    expect(
      requests.filter(
        (r) => r.path === "/v1/owner/intents" && r.init?.method === "POST",
      ),
    ).toHaveLength(1),
  );
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "Create intention",
          exact: true,
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  expect(
    (
      screen.getByLabelText("Public summary", {
        exact: true,
      }) as HTMLTextAreaElement
    ).value,
  ).toBe(publicSummary);
  await userEvent.click(
    screen.getByRole("button", { name: "Create intention", exact: true }),
  );
  await waitFor(() =>
    expect(
      requests.filter(
        (r) => r.path === "/v1/owner/intents" && r.init?.method === "POST",
      ),
    ).toHaveLength(2),
  );
  const sent = requests.filter(
    (r) => r.path === "/v1/owner/intents" && r.init?.method === "POST",
  );
  expect(sent[0].init?.body).toBe(sent[1].init?.body);
  expect(new Headers(sent[0].init?.headers).get("Idempotency-Key")).toBe(
    new Headers(sent[1].init?.headers).get("Idempotency-Key"),
  );
});

test("a separate public creation draft cannot publish an unrelated private objective revision", async () => {
  const requests = fixture();
  render(<App />);
  await screen.findByRole("heading", { name: "Your workspace" });
  await userEvent.selectOptions(
    screen.getByRole("combobox", { name: "Intention visibility", exact: true }),
    "redacted_public",
  );
  await userEvent.type(
    screen.getByLabelText("Public summary", { exact: true }),
    publicSummary,
  );
  await userEvent.click(
    screen.getByLabelText("I reviewed this public projection", { exact: true }),
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Review intention", exact: true }),
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Edit intention", exact: true }),
  );
  expect(
    (
      screen.getByRole("combobox", {
        name: "Revision visibility",
        exact: true,
      }) as HTMLSelectElement
    ).value,
  ).toBe("private");
  const objective = screen.getAllByLabelText("Objective", { exact: true })[0];
  await userEvent.clear(objective);
  await userEvent.type(objective, "Updated private objective");
  await userEvent.click(
    screen.getByRole("button", { name: "Save new revision", exact: true }),
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
  const revision = JSON.parse(String(sent.init?.body)).revision;
  expect(revision.visibility).toBe("private");
  expect(revision.objective).toBe("Updated private objective");
  expect(revision.public_summary).toBeUndefined();
});

test("changing the selected public summary invalidates the prior projection review", async () => {
  fixture();
  render(<App />);
  const publish = await preparePublicCreation();
  await userEvent.type(
    screen.getByLabelText("Public summary", { exact: true }),
    " changed",
  );
  expect(
    (
      screen.getByLabelText("I reviewed this public projection", {
        exact: true,
      }) as HTMLInputElement
    ).checked,
  ).toBe(false);
  expect(publish.disabled).toBe(true);
  await userEvent.click(
    screen.getByLabelText("I reviewed this public projection", { exact: true }),
  );
  expect(publish.disabled).toBe(false);
});

test("a late publication response cannot refresh an expired owner session", async () => {
  fixture();
  const normalFetch = globalThis.fetch;
  let finishCreate!: (response: Response) => void;
  let expired = false;
  const fetch = vi.fn(async (path: string, init?: RequestInit) => {
    if (path === "/v1/owner/intents" && init?.method === "POST")
      return new Promise<Response>((resolve) => {
        finishCreate = resolve;
      });
    if (expired && path.includes("/inbox"))
      return new Response(
        JSON.stringify({ error: { code: "session_expired" } }),
        { status: 401 },
      );
    return normalFetch(path, init);
  });
  vi.stubGlobal("fetch", fetch);
  render(<App />);
  await userEvent.click(await preparePublicCreation());
  await waitFor(() => expect(finishCreate).toBeTypeOf("function"));
  await userEvent.selectOptions(
    screen.getByRole("combobox", { name: "Inbox agent", exact: true }),
    "agent",
  );
  expired = true;
  await userEvent.click(
    screen.getByRole("button", { name: "Refresh inbox", exact: true }),
  );
  await screen.findByRole("link", { name: "Sign in", exact: true });
  const readsBefore = fetch.mock.calls.filter(
    ([path, init]) => path === "/v1/owner/intents" && init?.method !== "POST",
  ).length;
  const command = fetch.mock.calls.find(
    ([path, init]) => path === "/v1/owner/intents" && init?.method === "POST",
  )!;
  const intent = JSON.parse(String(command[1]?.body)).intent;
  await act(async () =>
    finishCreate(
      new Response(
        JSON.stringify({
          ...intent,
          id: "created",
          version: 1,
          current_revision: 1,
          etag: '"1"',
          state: "active",
          budget: null,
        }),
        { status: 201 },
      ),
    ),
  );
  expect(
    fetch.mock.calls.filter(
      ([path, init]) => path === "/v1/owner/intents" && init?.method !== "POST",
    ),
  ).toHaveLength(readsBefore);
  expect(
    screen.getByRole("link", { name: "Sign in", exact: true }),
  ).toBeTruthy();
});
