import {
  render,
  screen,
  waitFor,
  fireEvent,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi, test, expect } from "vitest";
import { App } from "./App";

function fixture(
  registered = true,
  intentState = "active",
  intentVisibility = "private",
  semanticThreshold?: number,
  privateQuery?: Record<string, unknown>,
) {
  const requests: { path: string; init: RequestInit | undefined }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      requests.push({ path, init });
      const values: Record<string, unknown> = {
        "/auth/session": { csrf_token: "session-csrf" },
        "/v1/owner/matching-policy": {
          enabled: true,
          allow_private_projections: true,
          profile_hash: "a".repeat(64),
          provider_id: "selected-provider",
          model: "selected-model",
          model_revision: "r1",
          privacy_policy_sha256: "b".repeat(64),
          permission_evidence: "operator approval",
        },
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
          ...(privateQuery ? { private_semantic_query: privateQuery } : {}),
          ...(semanticThreshold === undefined
            ? {}
            : { semantic_minimum_similarity: semanticThreshold }),
          output: { kind: "json_dataset", schema_family: "source_catalog_v1" },
          hard_constraints: {
            required_fields: ["source_id", "source_url"],
            license_ids: ["CC0-1.0"],
          },
          visibility: intentVisibility,
          watch: { enabled: true, channels: ["inbox"] },
          expires_at: "2030-02-01T00:00:00Z",
          budget: null,
        },
        "/v1/owner/intents/intent/opportunities": { items: [] },
        "/v1/owner/intents/intent/grants": {
          items:
            intentVisibility === "restricted"
              ? [
                  {
                    principal_id: "recipient",
                    fields: ["objective"],
                    status: "current",
                  },
                ]
              : [],
          etag: '"v1"',
          version: 1,
        },
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
      if (
        path === "/v1/owner/intents/intent/grants" &&
        init?.method === "POST"
      ) {
        result = {
          intent_id: "intent",
          principal_id: "recipient",
          version: 2,
          etag: '"v2"',
        };
      }
      if (
        path.endsWith("/grants/recipient/revoke") &&
        init?.method === "POST"
      ) {
        result = {
          intent_id: "intent",
          principal_id: "recipient",
          version: 3,
          etag: '"v3"',
        };
      }
      if (path === "/v1/owner/intents/intent" && init?.method === "PATCH") {
        const sent = JSON.parse(String(init.body)) as {
          revision?: Record<string, unknown>;
          state?: string;
        };
        result = {
          ...(values[path] as Record<string, unknown>),
          ...sent.revision,
          etag: '"v2"',
          version: 2,
          state:
            sent.state ?? (intentState === "expired" ? "active" : intentState),
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
  const requests = fixture(true, "expired", "private", 0.99);
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
      semantic_minimum_similarity: 0.99,
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
  await userEvent.click(screen.getByLabelText("I reviewed this declared profile"));
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
    allow_coordinator_succession: false,
    template_ids: [],
    license_ids: [],
    task_categories: [],
    max_active_tasks: 0,
    expires_at: "2030-01-02T00:00:00.000Z",
  });
});
test("new intention defaults to private and can explicitly choose restricted visibility", async () => {
  const requests = fixture();
  render(<App />);
  const visibility = await screen.findByRole("combobox", {
    name: "Intention visibility",
  });
  expect((visibility as HTMLSelectElement).value).toBe("private");
  await userEvent.selectOptions(visibility, "restricted");
  const threshold = screen.getByLabelText("Minimum semantic similarity (0–1)");
  expect((threshold as HTMLInputElement).value).toBe("");
  fireEvent.change(threshold, { target: { value: "0.91" } });
  await userEvent.type(
    screen.getByRole("textbox", { name: "Objective" }),
    "Shareable catalog request",
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Create intention" }),
  );
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
  expect(JSON.parse(sent.init!.body as string).intent.visibility).toBe(
    "restricted",
  );
  expect(
    JSON.parse(sent.init!.body as string).intent.semantic_minimum_similarity,
  ).toBe(0.91);
  expect(new Headers(sent.init?.headers).get("X-CSRF-Token")).toBe(
    "session-csrf",
  );
});
test("restricted grant controls require a named principal and explicit field selection", async () => {
  const requests = fixture(true, "active", "restricted");
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", { name: "Review intention" }),
  );
  const recipient = await screen.findByLabelText("Recipient principal ID");
  expect(
    (screen.getByLabelText("budget (explicit opt-in)") as HTMLInputElement)
      .checked,
  ).toBe(false);
  await userEvent.type(recipient, "recipient-principal-uuid");
  await userEvent.click(
    screen.getByLabelText("objective", { selector: "input" }),
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Grant selected fields" }),
  );
  await waitFor(() =>
    expect(
      requests.some(
        (r) =>
          r.path === "/v1/owner/intents/intent/grants" &&
          r.init?.method === "POST",
      ),
    ).toBe(true),
  );
  const sent = requests.find(
    (r) =>
      r.path === "/v1/owner/intents/intent/grants" && r.init?.method === "POST",
  )!;
  expect(JSON.parse(sent.init!.body as string)).toMatchObject({
    principal_id: "recipient-principal-uuid",
    fields: ["objective"],
  });
  expect(new Headers(sent.init?.headers).get("If-Match")).toBe('"v1"');
  expect(new Headers(sent.init?.headers).get("X-CSRF-Token")).toBe(
    "session-csrf",
  );
});
test("owner revokes a current restricted grant through the versioned CSRF command", async () => {
  const requests = fixture(true, "active", "restricted");
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", { name: "Review intention" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Revoke grant" }),
  );
  await waitFor(() =>
    expect(
      requests.some(
        (r) =>
          r.path === "/v1/owner/intents/intent/grants/recipient/revoke" &&
          r.init?.method === "POST",
      ),
    ).toBe(true),
  );
  const sent = requests.find(
    (r) =>
      r.path === "/v1/owner/intents/intent/grants/recipient/revoke" &&
      r.init?.method === "POST",
  )!;
  expect(new Headers(sent.init?.headers).get("If-Match")).toBe('"v1"');
  expect(new Headers(sent.init?.headers).get("X-CSRF-Token")).toBe(
    "session-csrf",
  );
});

test("satisfied intention cannot be edited or resumed", async () => {
  fixture(true, "satisfied");
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", { name: "Review intention" }),
  );
  expect(screen.queryByRole("button", { name: "Edit intention" })).toBeNull();
  for (const name of ["Resume", "Pause", "Withdraw"]) {
    expect(
      (screen.getByRole("button", { name }) as HTMLButtonElement).disabled,
    ).toBe(true);
  }
});

test("owner explicitly marks an intention satisfied using its reviewed version", async () => {
  const requests = fixture();
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", { name: "Review intention" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Mark satisfied" }),
  );
  const sent = requests.find(
    (r) => r.path === "/v1/owner/intents/intent" && r.init?.method === "PATCH",
  )!;
  expect(JSON.parse(sent.init!.body as string)).toEqual({ state: "satisfied" });
  expect(new Headers(sent.init?.headers).get("If-Match")).toBe('"v1"');
});

test("active intention exposes permitted external lead discovery", async () => {
  fixture();
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", { name: "Review intention" }),
  );
  expect(
    await screen.findByRole("button", { name: "Find external leads" }),
  ).toBeTruthy();
  expect(screen.getByRole("heading", { name: "External leads" })).toBeTruthy();
});

test.each(["paused", "satisfied", "withdrawn", "expired"])(
  "%s intention does not offer external discovery",
  async (state) => {
    fixture(true, state);
    render(<App />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Review intention" }),
    );
    await screen.findByRole("heading", { name: "Private catalog" });
    expect(
      screen.queryByRole("button", { name: "Find external leads" }),
    ).toBeNull();
  },
);

test("owner workspace exposes participation without asking for an agent bearer", async () => {
  fixture();
  render(<App />);
  expect(
    await screen.findByRole("heading", { name: "Swarm participation" }),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Load participation" }),
  ).toBeTruthy();
});

test("the signed-in console exposes private swarm creation alongside participation", async () => {
  fixture();
  render(<App />);
  expect(
    await screen.findByRole("heading", { name: "Swarm participation" }),
  ).toBeTruthy();
  expect(screen.getByRole("heading", { name: "Create swarm" })).toBeTruthy();
  const creation = within(screen.getByRole("region", { name: "Create swarm" }));
  expect(creation.getByLabelText("Responsible agent")).toBeTruthy();
  expect(creation.queryByRole("option", { name: "Public" })).toBeNull();
});

test("create intention sends only explicitly selected private query with reviewed profile", async () => {
  const requests = fixture();
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", {
      name: "Review private matching policy",
    }),
  );
  await screen.findByText("selected-provider / selected-model / r1");
  fireEvent.change(screen.getByLabelText("Selected private matching query"), {
    target: { value: "Selected public citations" },
  });
  await userEvent.click(
    screen.getByLabelText(
      "Allow this selected query to be processed by this provider profile",
    ),
  );
  await userEvent.type(
    screen.getByRole("textbox", { name: "Objective" }),
    "Secret private objective",
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Create intention" }),
  );
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
  expect(
    JSON.parse(sent.init!.body as string).intent.private_semantic_query,
  ).toEqual({
    projection: "Selected public citations",
    profile_hash: "a".repeat(64),
    acknowledge_external_processing: true,
  });
});
test("selected query cannot be submitted before disclosure consent", async () => {
  const requests = fixture();
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", {
      name: "Review private matching policy",
    }),
  );
  await screen.findByLabelText("Selected private matching query");
  fireEvent.change(screen.getByLabelText("Selected private matching query"), {
    target: { value: "Selected citations" },
  });
  await userEvent.type(
    screen.getByRole("textbox", { name: "Objective" }),
    "Private objective",
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Create intention" }),
  );
  expect(
    requests.some(
      (r) => r.path === "/v1/owner/intents" && r.init?.method === "POST",
    ),
  ).toBe(false);
  await userEvent.click(
    screen.getByRole("button", { name: "Remove private matching query" }),
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Create intention" }),
  );
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
  expect(
    JSON.parse(sent.init!.body as string).intent.private_semantic_query,
  ).toBeUndefined();
});

test.each(["keep", "change", "remove"])(
  "revision must explicitly %s existing private query",
  async (decision) => {
    const query = {
      projection: "Existing selected query",
      profile_hash: "a".repeat(64),
      acknowledge_external_processing: true,
    };
    const requests = fixture(true, "active", "private", undefined, query);
    render(<App />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Review intention" }),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Edit intention" }),
    );
    const form = screen
      .getByRole("button", { name: "Save new revision" })
      .closest("form")!;
    await userEvent.click(
      within(form).getByRole("button", { name: "Save new revision" }),
    );
    expect(
      requests.some(
        (r) =>
          r.path === "/v1/owner/intents/intent" && r.init?.method === "PATCH",
      ),
    ).toBe(false);
    if (decision !== "remove") {
      await userEvent.click(
        within(form).getByRole("button", {
          name: "Review private matching policy",
        }),
      );
      await within(form).findByText("selected-provider / selected-model / r1");
      expect(
        (
          within(form).getByLabelText(
            "Selected private matching query",
          ) as HTMLTextAreaElement
        ).value,
      ).toBe(query.projection);
      if (decision === "change")
        fireEvent.change(
          within(form).getByLabelText("Selected private matching query"),
          { target: { value: "Changed selected query" } },
        );
      await userEvent.click(
        within(form).getByLabelText(
          "Allow this selected query to be processed by this provider profile",
        ),
      );
    } else {
      await userEvent.click(
        within(form).getByRole("button", {
          name: "Remove private matching query",
        }),
      );
    }
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
      (r) =>
        r.path === "/v1/owner/intents/intent" && r.init?.method === "PATCH",
    )!;
    expect(
      JSON.parse(sent.init!.body as string).revision.private_semantic_query,
    ).toEqual(
      decision === "remove"
        ? undefined
        : {
            ...query,
            projection:
              decision === "change"
                ? "Changed selected query"
                : query.projection,
          },
    );
  },
);

test("active monitored intention opens a read-only compatible group view", async () => {
  const requests = fixture();
  const baseFetch = globalThis.fetch;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      if (path === "/v1/owner/intents/intent/compatible-groups") {
        requests.push({ path, init });
        return new Response(
          JSON.stringify({
            items: [],
            candidate_limit: 20,
            group_limit: 10,
            truncated: false,
            limitations: [
              "bounded_nonexhaustive_views",
              "principal_ids_do_not_prove_independent_owners",
            ],
          }),
          { status: 200 },
        );
      }
      return baseFetch(path, init);
    }),
  );
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", { name: "Review intention" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "View compatible groups" }),
  );
  expect(await screen.findByText(/No current compatible groups/)).toBeTruthy();
  const call = requests.find((r) => r.path.endsWith("/compatible-groups"));
  expect(call?.init?.method).toBe("GET");
  expect(call?.init?.credentials).toBe("same-origin");
  expect(
    requests.filter((r) => r.path.endsWith("/compatible-groups")),
  ).toHaveLength(1);
});

test.each(["paused", "watch-off"])(
  "group viewer is unavailable for %s intentions",
  async (condition) => {
    const requests = fixture(
      true,
      condition === "paused" ? "paused" : "active",
    );
    const baseFetch = globalThis.fetch;
    if (condition === "watch-off")
      vi.stubGlobal(
        "fetch",
        vi.fn(async (path: string, init?: RequestInit) => {
          const response = await baseFetch(path, init);
          if (path !== "/v1/owner/intents/intent") return response;
          const body = await response.json();
          return new Response(
            JSON.stringify({
              ...body,
              watch: { enabled: false, channels: ["inbox"] },
            }),
            { status: 200 },
          );
        }),
      );
    render(<App />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Review intention" }),
    );
    await screen.findByRole("button", { name: "Close intention" });
    expect(
      screen.queryByRole("button", { name: "View compatible groups" }),
    ).toBeNull();
    expect(requests.some((r) => r.path.endsWith("/compatible-groups"))).toBe(
      false,
    );
  },
);
