import { createHash } from "node:crypto";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { Participation } from "./Participation";

function fixture({
  malformedFirst = false,
  legacyPlan = false,
  invalidRequired = false,
}: {
  malformedFirst?: boolean;
  legacyPlan?: boolean;
  invalidRequired?: boolean;
} = {}) {
  const legacySpec = {
    parent_version: null,
    objective: "Existing accepted nullable plan",
    required_principals: invalidRequired ? null : ["owner-a", "owner-c"],
    acceptance_principals: ["owner-a"],
    publication_principals: null,
    rights: {
      template_id: "test-contribution-v1",
      license_id: "CC0-1.0",
      publication: "exact_manifest_approval",
    },
    tasks: [
      {
        key: "old-task",
        assignee_agent_id: "agent-c",
        depends_on: null,
        output_schema: "source_catalog_v1",
        checks: ["json_schema_v1"],
        progress_interval_seconds: 86400,
        deadline: "2030-02-01T00:00:35.123Z",
      },
    ],
    respond_by: new Date(Date.now() + 86400000).toISOString(),
  };
  const legacyCanonical = JSON.stringify({
    plan: legacySpec,
    template_hash: "a".repeat(64),
  });
  const legacy = {
    id: "legacy-plan",
    swarm_id: "room",
    version: 1,
    state: "accepted",
    content_hash: createHash("sha256").update(legacyCanonical).digest("hex"),
    canonical_json: legacyCanonical,
    specification: legacySpec,
    tasks: [],
  };

  let posts = 0;
  const receipts = new Map<string, unknown>();
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
          latest_plan: legacyPlan
            ? {
                id: legacy.id,
                version: legacy.version,
                state: legacy.state,
                content_hash: legacy.content_hash,
              }
            : null,
          items: [
            {
              id: "member-a",
              agent_id: "agent-a",
              principal_id: "owner-a",
              role: "coordinator",
            },
            {
              id: "member-c",
              agent_id: "agent-c",
              principal_id: "owner-c",
              role: "contributor",
            },
          ],
          next_after: "",
          limit: 100,
        };
      if (legacyPlan && path.endsWith("/plans/legacy-plan")) value = legacy;
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
      if (path.endsWith("/plans") && init?.method === "POST") {
        const key = new Headers(init.headers).get("Idempotency-Key")!;
        if (!receipts.has(key)) {
          const spec = JSON.parse(String(init.body));
          const canonical = JSON.stringify({
            plan: spec,
            template_hash: "a".repeat(64),
          });
          receipts.set(key, {
            id: `plan-${receipts.size + 1}`,
            swarm_id: "room",
            version: 1,
            state: "proposed",
            content_hash: createHash("sha256").update(canonical).digest("hex"),
            canonical_json: canonical,
            specification: spec,
            tasks: spec.tasks.map((_task: unknown, index: number) => ({
              id: `offered-${index}`,
              state: "offered",
              version: 1,
              etag: '\"1\"',
              claim_epoch: 1,
              response_deadline: spec.respond_by,
              progress_deadline: null,
              cure_deadline: null,
              dependency_blocked: false,
            })),
          });
        }
        value = malformedFirst && posts++ === 0 ? {} : receipts.get(key);
      }
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

async function openEditor() {
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
  await userEvent.click(
    await screen.findByRole("button", { name: "Load plan context" }),
  );
  await screen.findByRole("textbox", { name: "Plan objective" });
}
async function fillDependentDraft() {
  await userEvent.type(
    screen.getByRole("textbox", { name: "Plan objective" }),
    "A reviewed dependent catalogue",
  );
  for (const label of [
    "Required approval: owner-a",
    "Required approval: owner-c",
    "Acceptance principal: owner-a",
    "Publication principal: owner-a",
    "Publication principal: owner-c",
  ])
    await userEvent.click(screen.getByLabelText(label));
  await userEvent.selectOptions(
    screen.getByLabelText("Contribution template"),
    "test-contribution-v1",
  );
  await userEvent.selectOptions(
    screen.getByLabelText("Dataset license"),
    "CC0-1.0",
  );
  await userEvent.clear(screen.getByLabelText("Task 1 key"));
  await userEvent.type(screen.getByLabelText("Task 1 key"), "collect");
  await userEvent.selectOptions(
    screen.getByLabelText("Task 1 assignee"),
    "agent-c",
  );
  await userEvent.click(screen.getByRole("button", { name: "Add task" }));
  await userEvent.clear(screen.getByLabelText("Task 2 key"));
  await userEvent.type(screen.getByLabelText("Task 2 key"), "assemble");
  await userEvent.selectOptions(
    screen.getByLabelText("Task 2 assignee"),
    "agent-c",
  );
  await userEvent.click(screen.getByLabelText("Task 2 depends on collect"));
}

test("owner reviews an actual two-task dependency draft and sends current selected IDs and room ETag", async () => {
  const calls = fixture();
  await openEditor();
  await fillDependentDraft();
  expect(
    (screen.getByRole("button", { name: "Propose plan" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  await userEvent.click(screen.getByLabelText("I reviewed this plan proposal"));
  await userEvent.type(
    screen.getByRole("textbox", { name: "Plan objective" }),
    " revised",
  );
  expect(
    (screen.getByLabelText("I reviewed this plan proposal") as HTMLInputElement)
      .checked,
  ).toBe(false);
  await userEvent.click(screen.getByLabelText("I reviewed this plan proposal"));
  await userEvent.click(screen.getByRole("button", { name: "Propose plan" }));
  await screen.findByText(/Plan proposed: plan-1/);
  const post = calls.find(
    (c) => c.path.endsWith("/plans") && c.init?.method === "POST",
  )!;
  const sent = JSON.parse(String(post.init?.body));
  expect(sent.parent_version).toBeNull();
  expect(sent.required_principals).toEqual(["owner-a", "owner-c"]);
  expect(sent.acceptance_principals).toEqual(["owner-a"]);
  expect(sent.publication_principals).toEqual(["owner-a", "owner-c"]);
  expect(
    sent.tasks.map(
      (task: {
        key: string;
        assignee_agent_id: string;
        depends_on: string[];
      }) => [task.key, task.assignee_agent_id, task.depends_on],
    ),
  ).toEqual([
    ["collect", "agent-c", []],
    ["assemble", "agent-c", ["collect"]],
  ]);
  expect(sent.rights).toEqual({
    template_id: "test-contribution-v1",
    license_id: "CC0-1.0",
    publication: "exact_manifest_approval",
  });
  expect(new Headers(post.init?.headers).get("If-Match")).toBe('"1"');
  expect(new Headers(post.init?.headers).get("Authorization")).toBeNull();
  expect(new Headers(post.init?.headers).get("X-CSRF-Token")).toBe(
    "plan-owner-csrf",
  );
});

test("malformed proposal receipt preserves exact draft, parent, ETag and key for recovery", async () => {
  const calls = fixture({ malformedFirst: true });
  await openEditor();
  await fillDependentDraft();
  await userEvent.click(screen.getByLabelText("I reviewed this plan proposal"));
  await userEvent.click(screen.getByRole("button", { name: "Propose plan" }));
  await screen.findByRole("alert");
  const review = screen.getByLabelText(
    "I reviewed this plan proposal",
  ) as HTMLInputElement;
  if (!review.checked) await userEvent.click(review);
  await userEvent.click(screen.getByRole("button", { name: "Propose plan" }));
  await screen.findByText(/Plan proposed: plan-1/);
  const posts = calls.filter(
    (c) => c.path.endsWith("/plans") && c.init?.method === "POST",
  );
  expect(posts).toHaveLength(2);
  expect(posts[1].init?.body).toBe(posts[0].init?.body);
  const first = new Headers(posts[0].init?.headers),
    retry = new Headers(posts[1].init?.headers);
  expect(retry.get("Idempotency-Key")).toBe(first.get("Idempotency-Key"));
  expect(retry.get("If-Match")).toBe(first.get("If-Match"));
});

test("a valid existing plan with nullable publication and dependencies loads as a new immutable revision", async () => {
  fixture({ legacyPlan: true });
  render(
    <Participation
      agents={[{ id: "agent-a", name: "Owned agent", status: "active" }]}
      csrf="nullable-plan-owner"
      onUnauthorized={vi.fn()}
    />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Load participation" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Open room" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Load plan context" }),
  );
  await userEvent.click(
    await screen.findByRole("button", {
      name: "Load latest plan for revision",
    }),
  );
  expect(
    (
      (await screen.findByRole("textbox", {
        name: "Plan objective",
      })) as HTMLTextAreaElement
    ).value,
  ).toBe("Existing accepted nullable plan");
  expect(
    (
      screen.getByLabelText(
        "Task 1 final deadline (UTC, optional)",
      ) as HTMLInputElement
    ).value,
  ).toBe("2030-02-01T00:00:35.123");
  expect(
    (
      screen.getByLabelText(
        "Publication principal: owner-a",
      ) as HTMLInputElement
    ).checked,
  ).toBe(false);
  expect(screen.getByText("New revision of plan 1")).toBeTruthy();
});

test("nullable compatibility does not accept a missing required-principal array", async () => {
  fixture({ legacyPlan: true, invalidRequired: true });
  render(
    <Participation
      agents={[{ id: "agent-a", name: "Owned agent", status: "active" }]}
      csrf="invalid-required-owner"
      onUnauthorized={vi.fn()}
    />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Load participation" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Open room" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Load plan context" }),
  );
  await userEvent.click(
    await screen.findByRole("button", {
      name: "Load latest plan for revision",
    }),
  );
  await screen.findByRole("alert");
  expect(screen.queryByRole("textbox", { name: "Plan objective" })).toBeNull();
});
