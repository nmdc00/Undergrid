import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { test, expect, vi } from "vitest";
import { CreateSwarm } from "./CreateSwarm";
const agents = [
  { id: "a", name: "My agent", status: "active" },
  { id: "b", name: "Other active agent", status: "active" },
  { id: "revoked", name: "Revoked agent", status: "revoked" },
];
test("successful but unusable creation recovers the original room command", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(new Response("{}", { status: 201 }))
    .mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: "recovered-room",
          visibility: "private",
          history_policy: "full",
          version: 1,
          etag: '"1"',
        }),
        { status: 201 },
      ),
    );
  vi.stubGlobal("fetch", fetch);
  render(
    <CreateSwarm
      agents={agents}
      csrf="malformed-create"
      onUnauthorized={vi.fn()}
    />,
  );
  await userEvent.selectOptions(
    screen.getByLabelText("Responsible agent"),
    "a",
  );
  await userEvent.type(
    screen.getByLabelText("Swarm objective"),
    "Exact room recovery",
  );
  await userEvent.click(
    screen.getByLabelText(/I understand.*full room history/i),
  );
  await userEvent.click(screen.getByLabelText("I reviewed this room setup"));
  await userEvent.click(screen.getByRole("button", { name: "Create swarm" }));
  await screen.findByRole("alert");
  await userEvent.click(screen.getByRole("button", { name: "Create swarm" }));
  await screen.findByText(/Swarm recovered-room created/);
  expect(
    new Headers(fetch.mock.calls[0][1].headers).get("Idempotency-Key"),
  ).toBe(new Headers(fetch.mock.calls[1][1].headers).get("Idempotency-Key"));
  expect(fetch.mock.calls[0][1].body).toBe(fetch.mock.calls[1][1].body);
});
function mock(status = 201) {
  const fetch = vi.fn(
    async () =>
      new Response(
        JSON.stringify(
          status === 201
            ? {
                id: "room",
                visibility: "private",
                history_policy: "full",
                version: 1,
                etag: '"1"',
              }
            : { error: { code: "denied" } },
        ),
        { status },
      ),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}
test("owner explicitly selects a responsible active agent and acknowledges full history without bearer or public option", async () => {
  const fetch = mock(),
    created = vi.fn();
  render(
    <CreateSwarm
      agents={agents}
      csrf="csrf"
      onUnauthorized={vi.fn()}
      onCreated={created}
    />,
  );
  expect(
    (screen.getByRole("button", { name: "Create swarm" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(screen.queryByRole("option", { name: /public|revoked/i })).toBeNull();
  await userEvent.selectOptions(
    screen.getByLabelText("Responsible agent"),
    "b",
  );
  await userEvent.type(
    screen.getByLabelText("Swarm objective"),
    "<script>private research</script>",
  );
  await userEvent.selectOptions(
    screen.getByLabelText("Swarm visibility"),
    "private",
  );
  expect(
    screen.getByText(
      /Admitted members can read the full room history, including/i,
    ),
  ).toBeTruthy();
  await userEvent.click(
    screen.getByLabelText(/I understand.*full room history/i),
  );
  await userEvent.click(screen.getByLabelText("I reviewed this room setup"));
  await userEvent.click(screen.getByRole("button", { name: "Create swarm" }));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  const [path, init] = fetch.mock.calls[0] as unknown as [string, RequestInit];
  expect(path).toBe("/v1/owner/agents/b/swarms");
  expect(JSON.parse(init.body as string)).toEqual({
    objective: "<script>private research</script>",
    visibility: "private",
    history_policy: "full",
    recruitment_roles: [],
  });
  expect(new Headers(init.headers).get("Authorization")).toBeNull();
  expect(new Headers(init.headers).get("X-CSRF-Token")).toBe("csrf");
  expect(new Headers(init.headers).get("Idempotency-Key")).toBeTruthy();
  await waitFor(() => expect(created).toHaveBeenCalledWith("b", "room"));
  expect(document.querySelector("script")).toBeNull();
});
test("changing the owner session suppresses a late create response and clears private form data", async () => {
  let finish!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve;
        }),
    ),
  );
  const created = vi.fn();
  const view = render(
    <CreateSwarm
      agents={agents}
      csrf="csrf"
      onUnauthorized={vi.fn()}
      onCreated={created}
    />,
  );
  await userEvent.selectOptions(
    screen.getByLabelText("Responsible agent"),
    "a",
  );
  await userEvent.type(
    screen.getByLabelText("Swarm objective"),
    "Private old objective",
  );
  await userEvent.click(
    screen.getByLabelText(/I understand.*full room history/i),
  );
  await userEvent.click(screen.getByLabelText("I reviewed this room setup"));
  await userEvent.click(screen.getByRole("button", { name: "Create swarm" }));
  expect(screen.getByLabelText("Responsible agent").matches(":disabled")).toBe(
    true,
  );
  view.rerender(
    <CreateSwarm
      agents={agents}
      csrf="new-owner-session"
      onUnauthorized={vi.fn()}
      onCreated={created}
    />,
  );
  finish(
    new Response(
      JSON.stringify({
        id: "old-room",
        visibility: "private",
        history_policy: "full",
        version: 1,
        etag: '"1"',
      }),
      { status: 201 },
    ),
  );
  await waitFor(() =>
    expect(
      (screen.getByLabelText("Swarm objective") as HTMLTextAreaElement).value,
    ).toBe(""),
  );
  expect(created).not.toHaveBeenCalled();
  expect(screen.queryByText(/old-room/)).toBeNull();
});
test("session loss clears the creation data and returns owner control", async () => {
  mock(401);
  const denied = vi.fn();
  render(<CreateSwarm agents={agents} csrf="csrf" onUnauthorized={denied} />);
  await userEvent.selectOptions(
    screen.getByLabelText("Responsible agent"),
    "a",
  );
  await userEvent.type(
    screen.getByLabelText("Swarm objective"),
    "Secret room objective",
  );
  await userEvent.click(
    screen.getByLabelText(/I understand.*full room history/i),
  );
  await userEvent.click(screen.getByLabelText("I reviewed this room setup"));
  await userEvent.click(screen.getByRole("button", { name: "Create swarm" }));
  await waitFor(() => expect(denied).toHaveBeenCalledTimes(1));
  expect(
    (screen.getByLabelText("Swarm objective") as HTMLTextAreaElement).value,
  ).toBe("");
});

test("uncertain creation retries preserve the command identity", async () => {
  const calls: RequestInit[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_path: string, init: RequestInit) => {
      calls.push(init);
      return new Response(
        JSON.stringify(
          calls.length === 1
            ? { error: { code: "unavailable" } }
            : {
                id: "room",
                visibility: "private",
                history_policy: "full",
                version: 1,
                etag: '"1"',
              },
        ),
        { status: calls.length === 1 ? 500 : 201 },
      );
    }),
  );
  render(
    <CreateSwarm
      agents={agents}
      csrf="retry-session"
      onUnauthorized={vi.fn()}
    />,
  );
  await userEvent.selectOptions(
    screen.getByLabelText("Responsible agent"),
    "a",
  );
  await userEvent.type(
    screen.getByLabelText("Swarm objective"),
    "Uncertain private creation",
  );
  await userEvent.selectOptions(
    screen.getByLabelText("Swarm visibility"),
    "private",
  );
  await userEvent.click(
    screen.getByLabelText(/I understand.*full room history/i),
  );
  await userEvent.click(screen.getByLabelText("I reviewed this room setup"));
  await userEvent.click(screen.getByRole("button", { name: "Create swarm" }));
  await screen.findByRole("alert");
  await userEvent.click(screen.getByRole("button", { name: "Create swarm" }));
  await waitFor(() => expect(calls.length).toBe(2));
  expect(new Headers(calls[0].headers).get("Idempotency-Key")).toBe(
    new Headers(calls[1].headers).get("Idempotency-Key"),
  );
  expect(await screen.findByText(/Swarm room created/)).toBeTruthy();
});
test("agent removal clears private draft and disables submission", async () => {
  const fetch = mock();
  const view = render(
    <CreateSwarm agents={agents} csrf="csrf" onUnauthorized={vi.fn()} />,
  );
  await userEvent.selectOptions(
    screen.getByLabelText("Responsible agent"),
    "a",
  );
  await userEvent.type(
    screen.getByLabelText("Swarm objective"),
    "Revoked draft",
  );
  await userEvent.click(
    screen.getByLabelText(/I understand.*full room history/i),
  );
  await userEvent.click(screen.getByLabelText("I reviewed this room setup"));
  view.rerender(
    <CreateSwarm
      agents={agents.filter((a) => a.id !== "a")}
      csrf="csrf"
      onUnauthorized={vi.fn()}
    />,
  );
  await waitFor(() =>
    expect(
      (screen.getByLabelText("Swarm objective") as HTMLTextAreaElement).value,
    ).toBe(""),
  );
  expect(
    (screen.getByRole("button", { name: "Create swarm" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(fetch).not.toHaveBeenCalled();
});

test("private remains the initial default and restricted needs an explicit choice", async () => {
  const fetch = vi.fn(
    async () =>
      new Response(
        JSON.stringify({
          id: "room",
          visibility: "restricted",
          history_policy: "full",
          version: 1,
          etag: '"1"',
        }),
        { status: 201 },
      ),
  );
  vi.stubGlobal("fetch", fetch);
  render(
    <CreateSwarm
      agents={agents}
      csrf="private-default"
      onUnauthorized={vi.fn()}
    />,
  );
  const visibility = screen.getByLabelText(
    "Swarm visibility",
  ) as HTMLSelectElement;
  expect(visibility.value).toBe("private");
  await userEvent.selectOptions(
    screen.getByLabelText("Responsible agent"),
    "a",
  );
  expect(visibility.value).toBe("private");
  await userEvent.selectOptions(visibility, "restricted");
  await userEvent.type(
    screen.getByLabelText("Swarm objective"),
    "Explicit restricted research room",
  );
  await userEvent.click(
    screen.getByLabelText(/I understand.*full room history/i),
  );
  await userEvent.click(screen.getByLabelText("I reviewed this room setup"));
  await userEvent.click(screen.getByRole("button", { name: "Create swarm" }));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  const [, init] = fetch.mock.calls[0] as unknown as [string, RequestInit];
  expect(JSON.parse(init.body as string).visibility).toBe("restricted");
  expect(await screen.findByText(/Swarm room created/)).toBeTruthy();
});
