import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { OwnerRoomMessage } from "./OwnerRoomMessage";

test("owner message composer retries exact untrusted text without inferring consent", async () => {
  const fetch = vi
    .fn()
    .mockRejectedValueOnce(new Error("lost response"))
    .mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: "message",
          sequence: 1,
          content_trust: "untrusted",
        }),
        { status: 201 },
      ),
    );
  vi.stubGlobal("fetch", fetch);
  const refresh = vi.fn();
  render(
    <OwnerRoomMessage
      agent="agent"
      room="room"
      csrf="csrf"
      onUnauthorized={vi.fn()}
      onSent={refresh}
    />,
  );
  await userEvent.type(
    screen.getByLabelText("Message text"),
    "<script>example</script>",
  );
  await userEvent.click(screen.getByRole("button", { name: "Post message" }));
  await screen.findByRole("alert");
  await userEvent.click(screen.getByRole("button", { name: "Post message" }));
  await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
  expect(fetch.mock.calls[0][1].body).toBe(fetch.mock.calls[1][1].body);
  expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({
    body: "<script>example</script>",
  });
  expect(
    new Headers(fetch.mock.calls[0][1].headers).get("Idempotency-Key"),
  ).toBe(new Headers(fetch.mock.calls[1][1].headers).get("Idempotency-Key"));
  expect(screen.getByText(/does not approve/i)).toBeTruthy();
});

test("room change clears draft and suppresses late completion", async () => {
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
  const onSent = vi.fn();
  const props = {
    agent: "a",
    room: "one",
    csrf: "csrf",
    onUnauthorized: vi.fn(),
    onSent,
  };
  const view = render(<OwnerRoomMessage {...props} />);
  await userEvent.type(screen.getByLabelText("Message text"), "hello");
  await userEvent.click(screen.getByRole("button", { name: "Post message" }));
  view.rerender(<OwnerRoomMessage {...props} room="two" />);
  finish(
    new Response(
      JSON.stringify({
        id: "message",
        sequence: 1,
        content_trust: "untrusted",
      }),
      { status: 201 },
    ),
  );
  await waitFor(() =>
    expect(
      (screen.getByLabelText("Message text") as HTMLTextAreaElement).value,
    ).toBe(""),
  );
  expect(onSent).not.toHaveBeenCalled();
});

test.each([401, 403, 409])(
  "denied message %s preserves draft and never refreshes as sent",
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
    const onSent = vi.fn(),
      onUnauthorized = vi.fn();
    render(
      <OwnerRoomMessage
        agent="a"
        room="room"
        csrf="csrf"
        onUnauthorized={onUnauthorized}
        onSent={onSent}
      />,
    );
    await userEvent.type(screen.getByLabelText("Message text"), "unsent");
    await userEvent.click(screen.getByRole("button", { name: "Post message" }));
    await screen.findByRole("alert");
    expect(
      (screen.getByLabelText("Message text") as HTMLTextAreaElement).value,
    ).toBe("unsent");
    expect(onSent).not.toHaveBeenCalled();
    expect(onUnauthorized).toHaveBeenCalledTimes(status === 401 ? 1 : 0);
  },
);

test.each(["{}", "{broken json"])(
  "unusable successful response %s retains committed command identity",
  async (responseBody) => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(new Response(responseBody, { status: 201 }))
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            id: "message",
            sequence: 1,
            content_trust: "untrusted",
          }),
          { status: 201 },
        ),
      );
    vi.stubGlobal("fetch", fetch);
    const onSent = vi.fn();
    render(
      <OwnerRoomMessage
        agent="a"
        room={`malformed-${responseBody}`}
        csrf="malformed-csrf"
        onUnauthorized={vi.fn()}
        onSent={onSent}
      />,
    );
    await userEvent.type(
      screen.getByLabelText("Message text"),
      "one committed message",
    );
    await userEvent.click(screen.getByRole("button", { name: "Post message" }));
    await screen.findByRole("alert");
    expect(onSent).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Post message" }));
    await waitFor(() => expect(onSent).toHaveBeenCalledTimes(1));
    expect(
      new Headers(fetch.mock.calls[0][1].headers).get("Idempotency-Key"),
    ).toBe(new Headers(fetch.mock.calls[1][1].headers).get("Idempotency-Key"));
    expect(fetch.mock.calls[0][1].body).toBe(fetch.mock.calls[1][1].body);
  },
);
