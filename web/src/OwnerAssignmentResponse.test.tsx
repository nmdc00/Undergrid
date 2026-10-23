import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { test, expect, vi } from "vitest";
import { OwnerAssignmentResponse } from "./OwnerAssignmentResponse";

const task = {
  id: "task",
  plan_id: "plan",
  assigned_agent_id: "agent",
  state: "offered",
  version: 1,
  etag: '"1"',
  claim_epoch: 1,
  response_deadline: "2099-01-01T00:00:00Z",
  dependency_blocked: false,
  specification: {
    key: "catalog",
    assignee_agent_id: "agent",
    depends_on: [],
    output_schema: "source_catalog_v1",
    checks: ["json_schema_v1"],
    progress_interval_seconds: 60,
  },
};
function mount() {
  render(
    <OwnerAssignmentResponse
      agent="agent"
      plan="plan"
      hash={"a".repeat(64)}
      termsURL="https://example.org/terms"
      csrf="assignment-csrf"
      onUnauthorized={() => {}}
    />,
  );
}
test.each([
  ["accept", 200],
  ["decline", 200],
  ["accept", 500],
  ["decline", 500],
] as const)(
  "%s only exact reviewed assignment, keeping unknown %s command retry",
  async (decision, status) => {
    const posts: RequestInit[] = [];
    const fetch = vi.fn(async (_url: unknown, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts.push(init);
        return new Response(
          JSON.stringify(
            posts.length === 1
              ? {}
              : {
                  ...task,
                  state: decision === "accept" ? "accepted" : "open",
                  version: 2,
                  etag: '"2"',
                },
          ),
          { status: posts.length === 1 ? status : 200 },
        );
      }
      return new Response(JSON.stringify({ items: [task] }));
    });
    vi.stubGlobal("fetch", fetch);
    mount();
    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: "Review my assignments" }),
    );
    await screen.findByText(/source_catalog_v1/);
    const button = screen.getByRole("button", {
      name:
        decision === "accept"
          ? "Accept assignment responsibility"
          : "Decline assignment",
    });
    const ack = screen.getByRole("checkbox");
    if (decision === "accept") {
      expect((button as HTMLButtonElement).disabled).toBe(true);
      await user.click(ack);
    }
    await user.click(button);
    await screen.findByText(/could not be confirmed/);
    expect(screen.queryByText(/Assignment .* recorded/)).toBeNull();
    if (decision === "accept") {
      expect((ack as HTMLInputElement).checked).toBe(false);
      await user.click(ack);
    }
    await user.click(button);
    await waitFor(() => expect(posts).toHaveLength(2));
    expect(posts[0].body).toBe(posts[1].body);
    expect(new Headers(posts[0].headers).get("Idempotency-Key")).toBe(
      new Headers(posts[1].headers).get("Idempotency-Key"),
    );
    expect(new Headers(posts[0].headers).get("If-Match")).toBe(task.etag);
    expect(JSON.parse(String(posts[0].body))).toEqual({
      decision,
      plan_hash: "a".repeat(64),
      ...(decision === "accept"
        ? { acknowledge_responsibility_and_terms: true }
        : {}),
    });
    await screen.findByText(
      decision === "accept"
        ? "Assignment acceptance recorded."
        : "Assignment decline recorded.",
    );
  },
);

test("malformed task ETag cannot become reviewed responsibility", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ items: [{ ...task, etag: '"999"' }] })),
    ),
  );
  mount();
  await userEvent
    .setup()
    .click(screen.getByRole("button", { name: "Review my assignments" }));
  await screen.findByRole("alert");
  expect(
    screen.queryByRole("button", { name: "Accept assignment responsibility" }),
  ).toBeNull();
});

test.each([401, 403, 409])(
  "current authority denial %s discards reviewed assignments",
  async (status) => {
    const unauthorized = vi.fn();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async (_url: unknown, init?: RequestInit) =>
          new Response(
            JSON.stringify(
              init?.method === "POST"
                ? { error: { code: "assignment_not_current" } }
                : { items: [task] },
            ),
            { status: init?.method === "POST" ? status : 200 },
          ),
      ),
    );
    render(
      <OwnerAssignmentResponse
        agent="agent"
        plan="plan"
        hash={"a".repeat(64)}
        termsURL="https://example.org/terms"
        csrf={`denied-${status}`}
        onUnauthorized={unauthorized}
      />,
    );
    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: "Review my assignments" }),
    );
    await screen.findByRole("button", { name: "Decline assignment" });
    await user.click(
      screen.getByRole("button", { name: "Decline assignment" }),
    );
    await screen.findByRole("alert");
    expect(
      screen.queryByRole("button", {
        name: "Accept assignment responsibility",
      }),
    ).toBeNull();
    expect(unauthorized).toHaveBeenCalledTimes(status === 401 ? 1 : 0);
  },
);
