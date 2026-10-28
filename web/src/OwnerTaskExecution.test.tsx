import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { OwnerTaskExecution } from "./OwnerTaskExecution";
const agent = "42e75827-4f79-4d96-8832-38835995ad67",
  plan = "52e75827-4f79-4d96-8832-38835995ad67",
  id = "62e75827-4f79-4d96-8832-38835995ad67";
const task = {
  id,
  plan_id: plan,
  assigned_agent_id: agent,
  state: "accepted",
  version: 2,
  etag: '"2"',
  claim_epoch: 1,
  response_deadline: "2030-01-01T00:00:00Z",
  progress_deadline: "2030-01-02T00:00:00Z",
  cure_deadline: null,
  submitted_artifact_id: null,
  credited_progress_evidence_id: null,
  dependency_blocked: false,
  specification: {
    key: "catalog",
    assignee_agent_id: agent,
    depends_on: [],
    output_schema: "source_catalog_v1",
    checks: ["json_schema_v1"],
    progress_interval_seconds: 86400,
  },
};
const result = { ...task, state: "in_progress", version: 3, etag: '"3"' };
function response(x: unknown, status = 200) {
  return new Response(JSON.stringify(x), { status });
}
function mount(fetch: ReturnType<typeof vi.fn>) {
  vi.stubGlobal("fetch", fetch);
  return render(
    <OwnerTaskExecution
      agent={agent}
      plan={plan}
      hash={"a".repeat(64)}
      taskID={id}
      csrf="execution-csrf"
      onUnauthorized={vi.fn()}
    />,
  );
}
async function review() {
  await userEvent.click(
    screen.getByRole("button", { name: "Review task execution" }),
  );
  await screen.findByText(/Claim epoch: 1/);
  await userEvent.click(screen.getByLabelText(/I reviewed the current task/));
}
test("reviewed execution sends exact task version and epoch without deadline renewal or output consent", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(response({ items: [task] }))
    .mockResolvedValueOnce(response(result));
  mount(fetch);
  await review();
  await userEvent.click(
    screen.getByRole("button", { name: "Record unverified activity" }),
  );
  expect(
    await screen.findByText(
      /Unverified activity recorded.*No deadline was extended/,
    ),
  ).toBeTruthy();
  const [path, init] = fetch.mock.calls[1] as [string, RequestInit];
  expect(path).toBe(`/v1/owner/agents/${agent}/tasks/${id}/progress`);
  expect(new Headers(init.headers).get("If-Match")).toBe('"2"');
  expect(JSON.parse(init.body as string)).toEqual({
    state: "in_progress",
    claim_epoch: 1,
  });
});
test("uncertain execution retry preserves body epoch version and idempotency key", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(response({ items: [task] }))
    .mockRejectedValueOnce(new Error("connection lost"))
    .mockResolvedValueOnce(response(result));
  mount(fetch);
  await review();
  await userEvent.click(
    screen.getByRole("button", { name: "Record unverified activity" }),
  );
  await screen.findByRole("alert");
  await userEvent.click(
    screen.getByRole("button", { name: "Record unverified activity" }),
  );
  await screen.findByText(/Unverified activity recorded/);
  const first = fetch.mock.calls[1][1] as RequestInit,
    last = fetch.mock.calls[2][1] as RequestInit;
  expect(first.body).toBe(last.body);
  for (const name of ["If-Match", "Idempotency-Key"])
    expect(new Headers(first.headers).get(name)).toBe(
      new Headers(last.headers).get(name),
    );
});
test.each([401, 403, 404, 409])(
  "denied execution %s clears old task review",
  async (status) => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(response({ items: [task] }))
      .mockResolvedValueOnce(response({ error: { code: "denied" } }, status));
    mount(fetch);
    await review();
    await userEvent.click(
      screen.getByRole("button", { name: "Record unverified activity" }),
    );
    await screen.findByRole("alert");
    expect(
      screen.queryByRole("button", { name: "Record unverified activity" }),
    ).toBeNull();
    expect(screen.queryByText(/Unverified activity recorded/)).toBeNull();
  },
);
test("server response that renews the clock cannot be presented as heartbeat success", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(response({ items: [task] }))
    .mockResolvedValueOnce(
      response({ ...result, progress_deadline: "2030-01-03T00:00:00Z" }),
    );
  mount(fetch);
  await review();
  await userEvent.click(
    screen.getByRole("button", { name: "Record unverified activity" }),
  );
  await screen.findByRole("alert");
  expect(screen.queryByText(/Unverified activity recorded/)).toBeNull();
});

test.each([
 {name:"output link",packet:{...result,submitted_artifact_id:"72e75827-4f79-4d96-8832-38835995ad67"}},
 {name:"credited evidence",packet:{...result,credited_progress_evidence_id:"72e75827-4f79-4d96-8832-38835995ad67"}},
 {name:"task specification",packet:{...result,specification:{...task.specification,output_schema:"other_schema"}}},
])("heartbeat receipt cannot silently change $name",async({packet})=>{
 const fetch=vi.fn().mockResolvedValueOnce(response({items:[task]})).mockResolvedValueOnce(response(packet));mount(fetch);await review();await userEvent.click(screen.getByRole("button",{name:"Record unverified activity"}));await screen.findByRole("alert");expect(screen.queryByText(/Unverified activity recorded/)).toBeNull();
});
test.each([
 {name:"foreign assignment",items:[{...task,assigned_agent_id:plan,specification:{...task.specification,assignee_agent_id:plan}}]},
 {name:"duplicate task",items:[task,task]},
 {name:"rolled-over deadline",items:[{...task,progress_deadline:"2030-02-30T00:00:00Z"}]},
 {name:"string epoch",items:[{...task,claim_epoch:"1"}]},
])("invalid task review never enables execution: $name",async({items})=>{
 mount(vi.fn().mockResolvedValueOnce(response({items})));await userEvent.click(screen.getByRole("button",{name:"Review task execution"}));await screen.findByRole("alert");expect(screen.queryByRole("button",{name:"Record unverified activity"})).toBeNull();
});
