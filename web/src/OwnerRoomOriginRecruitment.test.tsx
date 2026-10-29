import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { App } from "./App";

const agent = "00000000-0000-4000-8000-000000000001";
const intention = "00000000-0000-4000-8000-000000000002";
const candidate = "00000000-0000-4000-8000-000000000003";
const origin = "00000000-0000-4000-8000-000000000004";
const room = "00000000-0000-4000-8000-000000000005";
const role = {
  key: "builder",
  schema_family: "source_catalog_v1",
  description: "Build the reviewed source catalog",
};
function fixture(malformedFirst = false) {
  const calls: { path: string; init?: RequestInit }[] = [];
  let attempts = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      calls.push({ path, init });
      let value: unknown = { items: [] };
      let status = 200;
      if (path === "/auth/session")
        value = { csrf_token: `room-origin-${String(malformedFirst)}` };
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
              id: agent,
              name: "Match recipient",
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
      if (path === "/v1/owner/intents")
        value = {
          items: [
            {
              id: intention,
              agent_id: agent,
              objective: "Find a source catalog",
              state: "active",
              version: 1,
            },
          ],
        };
      if (path === `/v1/owner/intents/${intention}`)
        value = {
          id: intention,
          version: 1,
          current_revision: 1,
          etag: '"1"',
          state: "active",
          visibility: "private",
          objective: "Find a source catalog",
          output: { kind: "json_dataset", schema_family: "source_catalog_v1" },
          hard_constraints: {
            required_fields: ["source_id", "source_url"],
            license_ids: ["CC0-1.0"],
          },
          watch: { enabled: true, channels: ["inbox"] },
          expires_at: "2030-01-01T00:00:00Z",
        };
      if (path === `/v1/owner/intents/${intention}/grants`)
        value = { items: [], version: 1, etag: '"1"' };
      if (path === `/v1/owner/intents/${intention}/opportunities`)
        value = {
          items: [
            {
              id: origin,
              source_intent_id: intention,
              candidate_intent_id: candidate,
              kind: "shared_demand",
              origin: "registered_intent",
              evidence: {
                compatible: true,
                status: "compatible",
                algorithm: "hard-constraints-exact-v1",
                reasons: ["same_declared_outcome"],
              },
            },
          ],
        };
      if (
        path === `/v1/owner/agents/${agent}/swarms` &&
        init?.method === "POST"
      ) {
        status = 201;
        value =
          malformedFirst && attempts++ === 0
            ? {}
            : {
                id: room,
                version: 1,
                etag: '"1"',
                visibility: "restricted",
                history_policy: "full",
              };
      }
      return new Response(JSON.stringify(value), {
        status,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  return calls;
}
async function prepare() {
  await userEvent.click(
    await screen.findByRole("button", { name: "Review intention" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Create swarm from this match" }),
  );
  const creation = screen.getByRole("region", { name: "Create swarm" });
  expect(
    (within(creation).getByLabelText("Responsible agent") as HTMLSelectElement)
      .value,
  ).toBe(agent);
  const objective = within(creation).getByLabelText("Swarm objective");
  await userEvent.clear(objective);
  await userEvent.type(objective, "Build the reviewed source catalog together");
  await userEvent.selectOptions(
    within(creation).getByLabelText("Swarm visibility"),
    "restricted",
  );
  await userEvent.click(
    within(creation).getByRole("button", { name: "Add recruitment role" }),
  );
  await userEvent.type(within(creation).getByLabelText("Role key"), role.key);
  await userEvent.type(
    within(creation).getByLabelText("Role description"),
    role.description,
  );
  const preview = within(creation).getByRole("region", {
    name: "Room creation review",
  });
  expect(preview.textContent).toContain(origin);
  expect(preview.textContent).toContain("registered_intent");
  expect(preview.textContent).toContain(role.description);
  await userEvent.click(
    within(creation).getByLabelText(/I understand.*full room history/i),
  );
  await userEvent.click(
    within(creation).getByLabelText("I reviewed this room setup"),
  );
  return creation;
}

test("owner selects an actual current match and reviews its exact room origin and recruitment roles", async () => {
  const calls = fixture();
  render(<App />);
  const creation = await prepare();
  await userEvent.click(
    within(creation).getByRole("button", { name: "Create swarm" }),
  );
  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.path === `/v1/owner/agents/${agent}/swarms` &&
          c.init?.method === "POST",
      ),
    ).toBe(true),
  );
  const sent = calls.find(
    (c) =>
      c.path === `/v1/owner/agents/${agent}/swarms` &&
      c.init?.method === "POST",
  )!;
  expect(JSON.parse(String(sent.init?.body))).toEqual({
    objective: "Build the reviewed source catalog together",
    visibility: "restricted",
    history_policy: "full",
    origin_opportunity_id: origin,
    recruitment_roles: [role],
  });
  expect(new Headers(sent.init?.headers).get("Authorization")).toBeNull();
});

test("changing a reviewed recruitment role requires a new exact room review", async () => {
  fixture();
  render(<App />);
  const creation = await prepare();
  await userEvent.type(
    within(creation).getByLabelText("Role description"),
    " with current review",
  );
  expect(
    (
      within(creation).getByRole("button", {
        name: "Create swarm",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
  expect(
    (
      within(creation).getByLabelText(
        "I reviewed this room setup",
      ) as HTMLInputElement
    ).checked,
  ).toBe(false);
});

test("an unusable room response retains the exact selected-origin command for retry", async () => {
  const calls = fixture(true);
  render(<App />);
  const creation = await prepare();
  await userEvent.click(
    within(creation).getByRole("button", { name: "Create swarm" }),
  );
  await within(creation).findByRole("alert");
  await userEvent.click(
    within(creation).getByRole("button", { name: "Create swarm" }),
  );
  await waitFor(() =>
    expect(
      calls.filter(
        (c) =>
          c.path === `/v1/owner/agents/${agent}/swarms` &&
          c.init?.method === "POST",
      ),
    ).toHaveLength(2),
  );
  const attempts = calls.filter(
    (c) =>
      c.path === `/v1/owner/agents/${agent}/swarms` &&
      c.init?.method === "POST",
  );
  expect(attempts[0].init?.body).toBe(attempts[1].init?.body);
  expect(new Headers(attempts[0].init?.headers).get("Idempotency-Key")).toBe(
    new Headers(attempts[1].init?.headers).get("Idempotency-Key"),
  );
});
