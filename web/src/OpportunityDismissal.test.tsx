import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { OpportunityDismissal } from "./OpportunityDismissal";

test("dismissal validates exact receipt before accepting success and preserves unknown-outcome retry", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(new Response("{}", { status: 200 }))
    .mockResolvedValueOnce(
      new Response(JSON.stringify({ id: "suggestion", state: "dismissed" }), {
        status: 200,
      }),
    );
  vi.stubGlobal("fetch", fetch);
  const onDismissed = vi.fn();
  render(
    <OpportunityDismissal
      agent="agent"
      opportunity="suggestion"
      csrf="csrf-dismiss"
      onDismissed={onDismissed}
      onUnauthorized={vi.fn()}
    />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Dismiss suggestion" }),
  );
  await screen.findByRole("alert");
  expect(onDismissed).not.toHaveBeenCalled();
  await userEvent.click(
    screen.getByRole("button", { name: "Dismiss suggestion" }),
  );
  await waitFor(() => expect(onDismissed).toHaveBeenCalledTimes(1));
  expect(fetch.mock.calls[0][0]).toBe(
    "/v1/owner/agents/agent/opportunities/suggestion/dismiss",
  );
  expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({});
  expect(
    new Headers(fetch.mock.calls[0][1].headers).get("Idempotency-Key"),
  ).toBe(new Headers(fetch.mock.calls[1][1].headers).get("Idempotency-Key"));
});

test("selection changes suppress a late dismissal result", async () => {
  let complete!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          complete = resolve;
        }),
    ),
  );
  const props = {
    agent: "a",
    opportunity: "one",
    csrf: "csrf-late",
    onDismissed: vi.fn(),
    onUnauthorized: vi.fn(),
  };
  const view = render(<OpportunityDismissal {...props} />);
  await userEvent.click(
    screen.getByRole("button", { name: "Dismiss suggestion" }),
  );
  view.rerender(<OpportunityDismissal {...props} opportunity="two" />);
  await act(async () => {
    complete(
      new Response(JSON.stringify({ id: "one", state: "dismissed" }), {
        status: 200,
      }),
    );
  });
  expect(props.onDismissed).not.toHaveBeenCalled();
});
