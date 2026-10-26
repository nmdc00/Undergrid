import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { Participation } from "./Participation";

function fixture() {
  const calls: { path: string; init?: RequestInit }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      calls.push({ path, init });
      let value: unknown = { items: [], next_before_version: "" };
      if (path.endsWith("/participation"))
        value = {
          pending: [],
          swarms: [{ id: "room" }],
          next_after: "",
          limit: 100,
        };
      if (path.endsWith("/context"))
        value = {
          swarm: {
            id: "room",
            objective: "Current private room",
            state: "active",
            version: 1,
            etag: '"1"',
          },
          messages: [],
          next_sequence: 0,
          head_sequence: 0,
          has_more: false,
        };
      if (path.endsWith("/plan-context"))
        value = {
          swarm: { id: "room", state: "active", version: 1, etag: '"1"' },
          latest_plan: null,
          items: [
            {
              agent_id: "agent-a",
              principal_id: "owner-a",
              role: "coordinator",
            },
            {
              agent_id: "agent-c",
              principal_id: "owner-c",
              role: "contributor",
            },
          ],
          next_after: "",
          limit: 100,
        };
      if (path === "/v1/owner/config")
        value = {
          terms_version: "test-v1",
          contribution_templates: {
            templates: [
              {
                id: "test-contribution-v1",
                sha256: "a".repeat(64),
                license_ids: ["CC0-1.0"],
                terms_url: "https://example.org/test-terms",
                test_only: true,
              },
            ],
          },
        };
      return new Response(JSON.stringify(value), {
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  return calls;
}

test("a current owner room offers a structured plan proposal using loaded participants", async () => {
  fixture();
  render(
    <Participation
      agents={[{ id: "agent-a", name: "Owned agent", status: "active" }]}
      csrf="plan-owner-csrf"
      onUnauthorized={vi.fn()}
    />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Load participation" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Open room" }),
  );
  expect(
    await screen.findByRole("heading", { name: "Plan proposal" }),
  ).toBeTruthy();
  await userEvent.click(
    screen.getByRole("button", { name: "Load plan context" }),
  );
  await screen.findByRole("textbox", { name: "Plan objective" });
  expect(screen.getByRole("button", { name: "Add task" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "Propose plan" })).toBeTruthy();
  expect(screen.getByLabelText("I reviewed this plan proposal")).toBeTruthy();
});
