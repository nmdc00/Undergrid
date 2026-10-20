import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { InviteMember } from "./InviteMember";

test("reviewed invitation requires target and history acknowledgment and sends reviewed ETag", async () => {
  const fetch = vi.fn(
    async () =>
      new Response(
        JSON.stringify({
          id: "invite",
          state: "offered",
          history_policy: "full",
          version: 1,
          etag: '"1"',
          respond_by: "2030-01-01T00:00:00Z",
        }),
        { status: 201 },
      ),
  );
  vi.stubGlobal("fetch", fetch);
  render(
    <InviteMember
      agent="coordinator"
      room="room"
      etag={'"7"'}
      csrf="csrf"
      onUnauthorized={vi.fn()}
      onStale={vi.fn()}
    />,
  );
  expect(
    (
      screen.getByRole("button", {
        name: "Send invitation",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
  await userEvent.type(
    screen.getByLabelText("Target agent ID"),
    "42e75827-4f79-4d96-8832-38835995ad67",
  );
  await userEvent.click(
    screen.getByLabelText(/I understand.*full room history/),
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Send invitation" }),
  );
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  const [path, init] = fetch.mock.calls[0] as unknown as [string, RequestInit];
  expect(path).toBe("/v1/owner/agents/coordinator/swarms/room/invitations");
  expect(new Headers(init.headers).get("If-Match")).toBe('"7"');
  expect(JSON.parse(init.body as string)).toMatchObject({
    agent_id: "42e75827-4f79-4d96-8832-38835995ad67",
    role: "contributor",
    history_policy: "full",
  });
  expect(screen.getByText(/Invitation offered.*not admitted/i)).toBeTruthy();
});
test.each([401, 403, 409])(
  "failed invitation %s never reports admission",
  async (status) => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ error: { code: "denied" } }), {
            status,
          }),
      ),
    );
    const unauthorized = vi.fn(),
      stale = vi.fn();
    render(
      <InviteMember
        agent="coordinator"
        room="room"
        etag={'"7"'}
        csrf="csrf"
        onUnauthorized={unauthorized}
        onStale={stale}
      />,
    );
    await userEvent.type(
      screen.getByLabelText("Target agent ID"),
      "42e75827-4f79-4d96-8832-38835995ad67",
    );
    await userEvent.click(
      screen.getByLabelText(/I understand.*full room history/),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Send invitation" }),
    );
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
    expect(screen.queryByText(/Invitation offered/)).toBeNull();
    expect(unauthorized).toHaveBeenCalledTimes(status === 401 ? 1 : 0);
    expect(stale).toHaveBeenCalledTimes(status === 409 ? 1 : 0);
  },
);
test("uncertain transport retry retains exact deadline and idempotency key", async () => {
  const fetch = vi
    .fn()
    .mockRejectedValueOnce(new Error("connection lost"))
    .mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: "invite",
          state: "offered",
          history_policy: "full",
        }),
        { status: 201 },
      ),
    );
  vi.stubGlobal("fetch", fetch);
  render(
    <InviteMember
      agent="coordinator"
      room="retry-room"
      etag={'"7"'}
      csrf="retry-csrf"
      onUnauthorized={vi.fn()}
      onStale={vi.fn()}
    />,
  );
  await userEvent.type(
    screen.getByLabelText("Target agent ID"),
    "42e75827-4f79-4d96-8832-38835995ad67",
  );
  await userEvent.click(
    screen.getByLabelText(/I understand.*full room history/),
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Send invitation" }),
  );
  await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  await userEvent.click(
    screen.getByRole("button", { name: "Send invitation" }),
  );
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
  expect(fetch.mock.calls[0][1].body).toBe(fetch.mock.calls[1][1].body);
  expect(
    new Headers(fetch.mock.calls[0][1].headers).get("Idempotency-Key"),
  ).toBe(new Headers(fetch.mock.calls[1][1].headers).get("Idempotency-Key"));
});
test("room change ignores a late response and clears target and consent", async () => {
  let finish!: (r: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve;
        }),
    ),
  );
  const props = {
    agent: "coordinator",
    room: "first",
    etag: '"1"',
    csrf: "csrf",
    onUnauthorized: vi.fn(),
    onStale: vi.fn(),
  };
  const view = render(<InviteMember {...props} />);
  await userEvent.type(
    screen.getByLabelText("Target agent ID"),
    "42e75827-4f79-4d96-8832-38835995ad67",
  );
  await userEvent.click(
    screen.getByLabelText(/I understand.*full room history/),
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Send invitation" }),
  );
  view.rerender(<InviteMember {...props} room="second" />);
  finish(
    new Response(
      JSON.stringify({
        id: "invite",
        state: "offered",
        history_policy: "full",
      }),
      { status: 201 },
    ),
  );
  await waitFor(() =>
    expect(
      (screen.getByLabelText("Target agent ID") as HTMLInputElement).value,
    ).toBe(""),
  );
  expect(screen.queryByText(/Invitation offered/)).toBeNull();
});
