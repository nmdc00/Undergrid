import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { App } from "./App";

let fixtureNumber = 0;
const privateObjective =
  "Private acquisition objective that must not be published";
const publicSummary = "Public environmental source catalogue";
const exactExpiry = "2030-02-01T00:00:35.123Z";
const output = { kind: "json_dataset", schema_family: "source_catalog_v1" };
const constraints = {
  required_fields: ["source_id", "source_url"],
  license_ids: ["CC0-1.0"],
};

function fixture(
  visibility = "private",
  malformedFirstCreate = false,
  malformedFirstWithdrawal = false,
) {
  const requests: { path: string; init?: RequestInit }[] = [];
  let createCount = 0;
  let withdrawalCount = 0;
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
    expires_at: exactExpiry,
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
          if (sent.revision) {
            record = {
              ...record,
              ...sent.revision,
              version: 2,
              current_revision: 2,
              etag: '"2"',
            };
            if (sent.revision.public_summary === undefined)
              delete record.public_summary;
          } else {
            record = {
              ...record,
              version: 2,
              etag: '"2"',
              state: sent.state,
              watch: { ...record.watch, enabled: sent.state === "active" },
            };
          }
        }
        value = record;
        if (
          init?.method === "PATCH" &&
          JSON.parse(String(init.body)).state === "withdrawn" &&
          malformedFirstWithdrawal &&
          withdrawalCount++ === 0
        )
          value = {};
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
  });
  expect((visibility as HTMLSelectElement).value).toBe("private");
  await userEvent.selectOptions(visibility, "redacted_public");
  const creationForm = screen
    .getByRole("heading", {
      name: "Create a source-catalog intention",
    })
    .closest("form")!;
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
  });
  expect(preview.textContent).toContain(publicSummary);
  expect(preview.textContent).toContain("source_id");
  expect(preview.textContent).toContain("CC0-1.0");
  expect(preview.textContent).not.toContain(privateObjective);
  const publish = screen.getByRole("button", {
    name: "Create intention",
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

test("a public-summary revision keeps its fixed audience and private typed fields", async () => {
  const requests = fixture("redacted_public");
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", {
      name: "Review intention",
    }),
  );
  await userEvent.click(screen.getByRole("button", { name: "Edit intention" }));
  expect(
    screen.queryByRole("combobox", {
      name: "Revision visibility",
    }),
  ).toBeNull();
  expect(
    screen.getByText("Audience is fixed: Public summary and requirements"),
  ).toBeTruthy();
  const preview = screen.getByRole("region", {
    name: "Public projection preview",
  });
  expect(preview.textContent).not.toContain(privateObjective);
  expect(preview.textContent).not.toContain("private-preference-sentinel");
  expect(preview.textContent).not.toContain("3500");
  const summary = screen.getByLabelText("Public summary", { exact: true });
  await userEvent.clear(summary);
  await userEvent.type(summary, "Updated public environmental catalogue");
  expect(
    (
      screen.getByRole("button", {
        name: "Save new revision",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
  await userEvent.click(
    screen.getByLabelText("I reviewed this public projection", { exact: true }),
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Save new revision" }),
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
    visibility: "redacted_public",
    public_summary: "Updated public environmental catalogue",
    objective: privateObjective,
    output,
    hard_constraints: constraints,
    preferences: { region: "private-preference-sentinel" },
    budget: { amount_minor: 3500, currency: "EUR" },
    expires_at: exactExpiry,
  });
  expect(new Headers(sent.init?.headers).get("If-Match")).toBe('"1"');
  await screen.findByRole("button", { name: "Edit intention" });
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
    screen.getByRole("button", { name: "Create intention" }),
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
    screen.getByRole("combobox", { name: "Intention visibility" }),
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
    screen.getByRole("button", { name: "Review intention" }),
  );
  await userEvent.click(screen.getByRole("button", { name: "Edit intention" }));
  expect(
    screen.queryByRole("combobox", {
      name: "Revision visibility",
    }),
  ).toBeNull();
  expect(screen.getByText("Audience is fixed: Private")).toBeTruthy();
  const objective = screen.getAllByLabelText("Objective", { exact: true })[0];
  await userEvent.clear(objective);
  await userEvent.type(objective, "Updated private objective");
  await userEvent.click(
    screen.getByRole("button", { name: "Save new revision" }),
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

test("explicit withdrawal revokes public discovery without changing visibility or rewriting history", async () => {
  const requests = fixture("redacted_public");
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", {
      name: "Review intention",
    }),
  );
  await userEvent.click(screen.getByRole("button", { name: "Withdraw" }));
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
  expect(JSON.parse(String(sent.init?.body))).toEqual({ state: "withdrawn" });
  expect(new Headers(sent.init?.headers).get("If-Match")).toBe('"1"');
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "Edit intention" })).toBeNull(),
  );
  expect(screen.getByText(/"visibility": "redacted_public"/)).toBeTruthy();
});

test("a malformed withdrawal response preserves the same reviewed command for recovery", async () => {
  const requests = fixture("redacted_public", false, true);
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", {
      name: "Review intention",
    }),
  );
  await userEvent.click(screen.getByRole("button", { name: "Withdraw" }));
  await waitFor(() =>
    expect(
      requests.filter(
        (r) =>
          r.path === "/v1/owner/intents/intent" && r.init?.method === "PATCH",
      ),
    ).toHaveLength(1),
  );
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "Withdraw",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  await userEvent.click(screen.getByRole("button", { name: "Withdraw" }));
  await waitFor(() =>
    expect(
      requests.filter(
        (r) =>
          r.path === "/v1/owner/intents/intent" && r.init?.method === "PATCH",
      ),
    ).toHaveLength(2),
  );
  const sent = requests.filter(
    (r) => r.path === "/v1/owner/intents/intent" && r.init?.method === "PATCH",
  );
  expect(sent[0].init?.body).toBe(sent[1].init?.body);
  expect(new Headers(sent[0].init?.headers).get("If-Match")).toBe(
    new Headers(sent[1].init?.headers).get("If-Match"),
  );
  expect(new Headers(sent[0].init?.headers).get("Idempotency-Key")).toBe(
    new Headers(sent[1].init?.headers).get("Idempotency-Key"),
  );
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "Edit intention" })).toBeNull(),
  );
});

test("a late withdrawal response cannot refresh an expired owner session", async () => {
  fixture("redacted_public");
  const normalFetch = globalThis.fetch;
  let finishWithdrawal!: (response: Response) => void;
  let expired = false;
  const fetch = vi.fn(async (path: string, init?: RequestInit) => {
    if (
      path === "/v1/owner/intents/intent" &&
      init?.method === "PATCH" &&
      JSON.parse(String(init.body)).state === "withdrawn"
    )
      return new Promise<Response>((resolve) => {
        finishWithdrawal = resolve;
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
  await userEvent.click(
    await screen.findByRole("button", {
      name: "Review intention",
    }),
  );
  await userEvent.click(screen.getByRole("button", { name: "Withdraw" }));
  await waitFor(() => expect(finishWithdrawal).toBeTypeOf("function"));
  await userEvent.selectOptions(
    screen.getByRole("combobox", { name: "Inbox agent" }),
    "agent",
  );
  expired = true;
  await userEvent.click(screen.getByRole("button", { name: "Refresh inbox" }));
  await screen.findByRole("link", { name: "Sign in" });
  const readsBefore = fetch.mock.calls.filter(
    ([path, init]) => path === "/v1/owner/intents" && init?.method !== "POST",
  ).length;
  await act(async () =>
    finishWithdrawal(
      new Response(
        JSON.stringify({
          id: "intent",
          version: 2,
          current_revision: 1,
          etag: '"2"',
          state: "withdrawn",
          objective: privateObjective,
          visibility: "redacted_public",
          public_summary: publicSummary,
          output,
          hard_constraints: constraints,
          preferences: { region: "private-preference-sentinel" },
          budget: {
            kind: "stated_budget",
            amount_minor: 3500,
            currency: "EUR",
          },
          watch: { enabled: false, channels: ["inbox"] },
          expires_at: exactExpiry,
        }),
        { status: 200 },
      ),
    ),
  );
  expect(
    fetch.mock.calls.filter(
      ([path, init]) => path === "/v1/owner/intents" && init?.method !== "POST",
    ),
  ).toHaveLength(readsBefore);
  expect(screen.getByRole("link", { name: "Sign in" })).toBeTruthy();
});

test("a late withdrawal response cannot reopen a closed intention", async () => {
  fixture("redacted_public");
  const normalFetch = globalThis.fetch;
  let finishWithdrawal!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      if (
        path === "/v1/owner/intents/intent" &&
        init?.method === "PATCH" &&
        JSON.parse(String(init.body)).state === "withdrawn"
      )
        return new Promise<Response>((resolve) => {
          finishWithdrawal = resolve;
        });
      return normalFetch(path, init);
    }),
  );
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", {
      name: "Review intention",
    }),
  );
  await userEvent.click(screen.getByRole("button", { name: "Withdraw" }));
  await waitFor(() => expect(finishWithdrawal).toBeTypeOf("function"));
  await userEvent.click(
    screen.getByRole("button", { name: "Close intention" }),
  );
  expect(screen.queryByRole("heading", { name: privateObjective })).toBeNull();
  await act(async () =>
    finishWithdrawal(
      new Response(
        JSON.stringify({
          id: "intent",
          version: 2,
          current_revision: 1,
          etag: '"2"',
          state: "withdrawn",
          objective: privateObjective,
          visibility: "redacted_public",
          public_summary: publicSummary,
          output,
          hard_constraints: constraints,
          preferences: { region: "private-preference-sentinel" },
          budget: {
            kind: "stated_budget",
            amount_minor: 3500,
            currency: "EUR",
          },
          watch: { enabled: false, channels: ["inbox"] },
          expires_at: exactExpiry,
        }),
        { status: 200 },
      ),
    ),
  );
  expect(screen.queryByRole("heading", { name: privateObjective })).toBeNull();
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
    screen.getByRole("combobox", { name: "Inbox agent" }),
    "agent",
  );
  expired = true;
  await userEvent.click(screen.getByRole("button", { name: "Refresh inbox" }));
  await screen.findByRole("link", { name: "Sign in" });
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
  expect(screen.getByRole("link", { name: "Sign in" })).toBeTruthy();
});

test("creation reviews the actual typed requirements draft and invalidates review after a change", async () => {
  const requests = fixture();
  render(<App />);
  const publish = await preparePublicCreation();
  await userEvent.click(screen.getByLabelText("Declare coverage"));
  await userEvent.type(screen.getByLabelText("Required coverage labels"), "EU");
  const licenses = screen.getByLabelText("Permitted license IDs");
  await userEvent.clear(licenses);
  await userEvent.type(licenses, "MIT");
  expect((publish as HTMLButtonElement).disabled).toBe(true);
  const preview = screen.getByRole("region", {
    name: "Public projection preview",
  });
  expect(preview.textContent).toContain('"required": [');
  expect(preview.textContent).toContain("EU");
  expect(preview.textContent).toContain("MIT");
  expect(preview.textContent).not.toContain(privateObjective);
  await userEvent.click(
    screen.getByLabelText("I reviewed this public projection", { exact: true }),
  );
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
  expect(JSON.parse(String(sent.init?.body)).intent.hard_constraints).toEqual({
    required_fields: ["source_id", "source_url"],
    license_ids: ["MIT"],
    coverage: { required: ["EU"], excluded: [] },
  });
});

test("public revision reviews edited typed requirements rather than the stored detail", async () => {
  const requests = fixture("redacted_public");
  render(<App />);
  await screen.findByRole("heading", { name: "Your workspace" });
  await userEvent.click(
    screen.getByRole("button", { name: "Review intention" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Edit intention" }),
  );
  const form = screen
    .getByRole("button", { name: "Save new revision" })
    .closest("form")!;
  await userEvent.click(
    within(form).getByLabelText("I reviewed this public projection", {
      exact: true,
    }),
  );
  await userEvent.click(within(form).getByLabelText("Declare coverage"));
  await userEvent.type(
    within(form).getByLabelText("Excluded coverage labels"),
    "US",
  );
  expect(
    (
      within(form).getByRole("button", {
        name: "Save new revision",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
  expect(
    within(form).getByRole("region", { name: "Public projection preview" })
      .textContent,
  ).toContain("US");
  await userEvent.click(
    within(form).getByLabelText("I reviewed this public projection", {
      exact: true,
    }),
  );
  await userEvent.click(
    within(form).getByRole("button", { name: "Save new revision" }),
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
  expect(
    JSON.parse(String(sent.init?.body)).revision.hard_constraints.coverage,
  ).toEqual({ required: [], excluded: ["US"] });
});

test.each(["create", "revise"] as const)(
  "typed requirement controls freeze while an exact %s request is pending",
  async (mode) => {
    fixture(mode === "revise" ? "redacted_public" : "private");
    const originalFetch = globalThis.fetch;
    let release = () => {};
    const pending = new Promise<void>((resolve) => {
      release = resolve;
    });
    let entered = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (...args: Parameters<typeof fetch>) => {
        const [path, init] = args;
        if (
          (mode === "create" &&
            path === "/v1/owner/intents" &&
            init?.method === "POST") ||
          (mode === "revise" &&
            path === "/v1/owner/intents/intent" &&
            init?.method === "PATCH")
        ) {
          entered = true;
          await pending;
        }
        return originalFetch(...args);
      }),
    );
    render(<App />);
    let form: HTMLFormElement;
    if (mode === "create") {
      const submit = await preparePublicCreation();
      form = submit.closest("form")!;
      await userEvent.click(submit);
    } else {
      await userEvent.click(
        await screen.findByRole("button", { name: "Review intention" }),
      );
      await userEvent.click(
        screen.getByRole("button", { name: "Edit intention" }),
      );
      const submit = screen.getByRole("button", { name: "Save new revision" });
      form = submit.closest("form")!;
      await userEvent.click(
        within(form).getByLabelText("I reviewed this public projection", {
          exact: true,
        }),
      );
      await userEvent.click(submit);
    }
    await waitFor(() => expect(entered).toBe(true));
    try {
      for (const label of [
        "Required field IDs",
        "Permitted license IDs",
        "Declare coverage",
      ]) {
        expect(
          within(form).getByLabelText(label).matches(":disabled"),
          label,
        ).toBe(true);
      }
    } finally {
      await act(async () => {
        release();
      });
    }
  },
);
