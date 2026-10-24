import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { test, expect, vi } from "vitest";
import { OwnerPlanApproval } from "./OwnerPlanApproval";
const plan = {
  id: "p",
  swarm_id: "room",
  version: 3,
  state: "proposed",
  content_hash: "a".repeat(64),
  canonical_json: "exact sealed bytes",
  contribution_template_hash: "b".repeat(64),
  terms_url: "https://example.org/terms",
  specification: {
    objective: "<script>Review untrusted plan</script>",
    respond_by: "2099-01-01T00:00:00Z",
    rights: {
      template_id: "test-terms",
      license_id: "CC0-1.0",
      publication: "exact_manifest_approval",
    },
    required_principals: ["principal"],
    acceptance_principals: ["principal"],
    publication_principals: ["principal"],
    tasks: [
      {
        key: "catalog",
        assignee_agent_id: "agent",
        depends_on: [],
        output_schema: "source_catalog_v1",
        checks: ["json_schema_v1"],
        progress_interval_seconds: 60,
        deadline: "2098-01-01T00:00:00Z",
      },
    ],
  },
  tasks: [
    { id: "task", state: "offered", response_deadline: "2099-01-01T00:00:00Z" },
  ],
};
function setup(status = 200) {
  const fetch = vi.fn(
    async (_url: unknown, init?: RequestInit) =>
      new Response(
        JSON.stringify(
          init?.method === "POST" && status !== 200
            ? { error: { code: "plan_not_current" } }
            : String(_url).endsWith("/plans")
              ? {
                  items: [
                    {
                      id: "p",
                      version: 3,
                      state: "proposed",
                      content_hash: plan.content_hash,
                    },
                  ],
                  next_before_version: "",
                }
              : init?.method === "POST" &&
                  JSON.parse(String(init.body)).decision === "reject"
                ? { ...plan, state: "rejected" }
                : plan,
        ),
        { status: init?.method === "POST" ? status : 200 },
      ),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}
test.each([
  ["approve", 200],
  ["reject", 200],
  ["approve", 500],
  ["reject", 500],
])(
  "unconfirmed %s response %s retries the exact command",
  async (decision, status) => {
    let posts = 0;
    const fetch = setup();
    fetch.mockImplementation(async (url, init) => {
      if (init?.method === "POST") {
        const first = posts++ === 0;
        return new Response(
          JSON.stringify(
            first
              ? {}
              : {
                  ...plan,
                  state: decision === "reject" ? "rejected" : "proposed",
                },
          ),
          { status: first ? Number(status) : 200 },
        );
      }
      return new Response(
        JSON.stringify(
          String(url).endsWith("/plans")
            ? { items: [plan], next_before_version: "" }
            : plan,
        ),
      );
    });
    render(
      <OwnerPlanApproval
        agent="agent"
        swarm="room"
        csrf={`malformed-${decision}-${status}`}
        onUnauthorized={vi.fn()}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Load plans" }));
    await userEvent.click(
      await screen.findByRole("button", { name: "Review plan 3" }),
    );
    const name =
      decision === "approve"
        ? "Approve exact plan and contribution terms"
        : "Reject exact reviewed plan";
    if (decision === "approve")
      await userEvent.click(
        screen.getByLabelText(/I approve the contribution terms/),
      );
    await userEvent.click(screen.getByRole("button", { name }));
    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.queryByRole("status")).toBeNull();
    if (decision === "approve")
      await userEvent.click(
        screen.getByLabelText(/I approve the contribution terms/),
      );
    await userEvent.click(screen.getByRole("button", { name }));
    await screen.findByRole("status");
    const calls = fetch.mock.calls.filter(
      ([, init]) => init?.method === "POST",
    );
    expect(calls).toHaveLength(2);
    expect(calls[0][1]?.body).toBe(calls[1][1]?.body);
    expect(new Headers(calls[0][1]?.headers).get("Idempotency-Key")).toBe(
      new Headers(calls[1][1]?.headers).get("Idempotency-Key"),
    );
  },
);
test("reviews exact plan and terms before explicit rights acknowledgement and approval", async () => {
  const fetch = setup();
  render(
    <OwnerPlanApproval
      agent="agent"
      swarm="room"
      csrf="csrf"
      onUnauthorized={vi.fn()}
    />,
  );
  await userEvent.click(screen.getByRole("button", { name: "Load plans" }));
  await userEvent.click(
    await screen.findByRole("button", { name: "Review plan 3" }),
  );
  expect(await screen.findByText(plan.specification.objective)).toBeTruthy();
  expect(screen.getByText(plan.content_hash)).toBeTruthy();
  expect(screen.getByText("exact sealed bytes")).toBeTruthy();
  expect(screen.getByText(/catalog/)).toBeTruthy();
  const approve = screen.getByRole("button", {
    name: "Approve exact plan and contribution terms",
  });
  expect((approve as HTMLButtonElement).disabled).toBe(true);
  await userEvent.click(
    screen.getByLabelText(/I approve the contribution terms/),
  );
  await userEvent.click(approve);
  await waitFor(() =>
    expect(fetch.mock.calls.some(([, init]) => init?.method === "POST")).toBe(
      true,
    ),
  );
  const call = fetch.mock.calls.find(([, init]) => init?.method === "POST")!;
  expect(call[0]).toBe("/v1/owner/agents/agent/plans/p/approvals");
  expect(JSON.parse(String(call[1]?.body))).toEqual({
    decision: "approve",
    content_hash: plan.content_hash,
    version: 3,
    acknowledge_contribution_terms: true,
  });
  expect(new Headers(call[1]?.headers).has("Authorization")).toBe(false);
});
test("stale approval clears reviewed authority and demands another review", async () => {
  setup(409);
  render(
    <OwnerPlanApproval
      agent="agent"
      swarm="room"
      csrf="csrf"
      onUnauthorized={vi.fn()}
    />,
  );
  await userEvent.click(screen.getByRole("button", { name: "Load plans" }));
  await userEvent.click(
    await screen.findByRole("button", { name: "Review plan 3" }),
  );
  await userEvent.click(
    await screen.findByLabelText(/I approve the contribution terms/),
  );
  await userEvent.click(
    screen.getByRole("button", {
      name: "Approve exact plan and contribution terms",
    }),
  );
  expect(await screen.findByRole("alert")).toHaveProperty(
    "textContent",
    expect.stringMatching(/Review.*again/),
  );
  expect(
    screen.queryByRole("button", {
      name: "Approve exact plan and contribution terms",
    }),
  ).toBeNull();
});
test("rejects the exact reviewed plan without contribution approval acknowledgement", async () => {
  const fetch = setup();
  render(
    <OwnerPlanApproval
      agent="agent"
      swarm="room"
      csrf="csrf"
      onUnauthorized={vi.fn()}
    />,
  );
  await userEvent.click(screen.getByRole("button", { name: "Load plans" }));
  await userEvent.click(
    await screen.findByRole("button", { name: "Review plan 3" }),
  );
  expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(
    false,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Reject exact reviewed plan" }),
  );
  expect(await screen.findByRole("status")).toHaveProperty(
    "textContent",
    "Exact reviewed plan rejection recorded.",
  );
  const call = fetch.mock.calls.find(([, init]) => init?.method === "POST")!;
  expect(JSON.parse(String(call[1]?.body))).toEqual({
    decision: "reject",
    content_hash: plan.content_hash,
    version: plan.version,
  });
  expect(
    screen.queryByRole("button", {
      name: "Approve exact plan and contribution terms",
    }),
  ).toBeNull();
});
test("changing attributed agent clears reviewed plan and rights acknowledgement", async () => {
  setup();
  const view = render(
    <OwnerPlanApproval
      agent="agent"
      swarm="room"
      csrf="csrf"
      onUnauthorized={vi.fn()}
    />,
  );
  await userEvent.click(screen.getByRole("button", { name: "Load plans" }));
  await userEvent.click(
    await screen.findByRole("button", { name: "Review plan 3" }),
  );
  await userEvent.click(
    await screen.findByLabelText(/I approve the contribution terms/),
  );
  view.rerender(
    <OwnerPlanApproval
      agent="other-agent"
      swarm="room"
      csrf="csrf"
      onUnauthorized={vi.fn()}
    />,
  );
  expect(screen.queryByText(plan.content_hash)).toBeNull();
  expect(
    screen.queryByLabelText(/I approve the contribution terms/),
  ).toBeNull();
});
test("revoked session response clears review and calls owner logout", async () => {
  setup(401);
  const logout = vi.fn();
  render(
    <OwnerPlanApproval
      agent="agent"
      swarm="room"
      csrf="csrf"
      onUnauthorized={logout}
    />,
  );
  await userEvent.click(screen.getByRole("button", { name: "Load plans" }));
  await userEvent.click(
    await screen.findByRole("button", { name: "Review plan 3" }),
  );
  await userEvent.click(
    await screen.findByLabelText(/I approve the contribution terms/),
  );
  await userEvent.click(
    screen.getByRole("button", {
      name: "Approve exact plan and contribution terms",
    }),
  );
  await waitFor(() => expect(logout).toHaveBeenCalledOnce());
  expect(screen.queryByText(plan.content_hash)).toBeNull();
});
test("unsafe contribution terms cannot authorize a vote", async () => {
  const fetch = setup();
  fetch.mockImplementation(
    async (url) =>
      new Response(
        JSON.stringify(
          String(url).endsWith("/plans")
            ? { items: [plan], next_before_version: "" }
            : { ...plan, terms_url: "javascript:alert('untrusted')" },
        ),
      ),
  );
  render(
    <OwnerPlanApproval
      agent="agent"
      swarm="room"
      csrf="csrf"
      onUnauthorized={vi.fn()}
    />,
  );
  await userEvent.click(screen.getByRole("button", { name: "Load plans" }));
  await userEvent.click(
    await screen.findByRole("button", { name: "Review plan 3" }),
  );
  expect(
    await screen.findByText(/Current contribution terms are unavailable/),
  ).toBeTruthy();
  await userEvent.click(
    screen.getByLabelText(/I approve the contribution terms/),
  );
  const approve = screen.getByRole("button", {
    name: "Approve exact plan and contribution terms",
  });
  expect((approve as HTMLButtonElement).disabled).toBe(true);
  await userEvent.click(approve);
  expect(fetch.mock.calls.some(([, init]) => init?.method === "POST")).toBe(
    false,
  );
  expect(
    screen.queryByRole("link", { name: "Read contribution terms" }),
  ).toBeNull();
});
test("late plan review cannot restore authority after owner agent selection changes", async () => {
  const fetch = setup();
  let complete!: (response: Response) => void;
  fetch.mockImplementation(async (url) =>
    String(url).endsWith("/plans")
      ? new Response(JSON.stringify({ items: [plan], next_before_version: "" }))
      : new Promise<Response>((resolve) => {
          complete = resolve;
        }),
  );
  const view = render(
    <OwnerPlanApproval
      agent="agent"
      swarm="room"
      csrf="csrf"
      onUnauthorized={vi.fn()}
    />,
  );
  await userEvent.click(screen.getByRole("button", { name: "Load plans" }));
  await userEvent.click(
    await screen.findByRole("button", { name: "Review plan 3" }),
  );
  view.rerender(
    <OwnerPlanApproval
      agent="other-agent"
      swarm="room"
      csrf="csrf"
      onUnauthorized={vi.fn()}
    />,
  );
  complete(new Response(JSON.stringify(plan)));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "Load plans" })).toHaveProperty(
      "disabled",
      false,
    ),
  );
  expect(screen.queryByText(plan.content_hash)).toBeNull();
  expect(
    screen.queryByLabelText(/I approve the contribution terms/),
  ).toBeNull();
});
