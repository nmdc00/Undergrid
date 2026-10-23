import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { OwnerInbox } from "./OwnerInbox";
const event = {
  event_id: "event",
  sequence: 1,
  type: "intent.created",
  resource_path: "/v1/intents/intent",
  content_trust: "untrusted",
};
const agents = [
  { id: "agent", name: "First", status: "active" },
  { id: "other", name: "Second", status: "active" },
];
function mount() {
  return render(
    <OwnerInbox agents={agents} csrf="inbox-csrf" onUnauthorized={() => {}} />,
  );
}
test.each([200, 500])(
  "review bounded inbox and retain receipt command after unknown response %s",
  async (status) => {
    const posts: RequestInit[] = [];
    const fetch = vi.fn(async (_url: unknown, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts.push(init);
        return new Response(
          JSON.stringify(posts.length === 1 ? {} : { acknowledged: true }),
          { status: posts.length === 1 ? status : 200 },
        );
      }
      return new Response(
        JSON.stringify({ events: [event], next_cursor: "cursor-one" }),
      );
    });
    vi.stubGlobal("fetch", fetch);
    mount();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Refresh inbox" }));
    await screen.findByText("intent.created");
    await user.click(
      screen.getByRole("button", { name: "Acknowledge receipt" }),
    );
    await screen.findByText(/could not be confirmed/);
    expect(screen.queryByText("Receipt acknowledged.")).toBeNull();
    await user.click(
      screen.getByRole("button", { name: "Acknowledge receipt" }),
    );
    await screen.findByText("Receipt acknowledged.");
    expect(posts).toHaveLength(2);
    expect(posts[0].body).toBe("{}");
    expect(new Headers(posts[0].headers).get("Idempotency-Key")).toBe(
      new Headers(posts[1].headers).get("Idempotency-Key"),
    );
    expect(
      fetch.mock.calls.some(([url]) => String(url).includes("wait_seconds")),
    ).toBe(false);
  },
);
test("expired cursor requires explicit retained-inbox reset", async () => {
  let pages = 0;
  const fetch = vi.fn(
    async () =>
      new Response(
        JSON.stringify(
          ++pages === 1
            ? { events: [event], next_cursor: "old-cursor" }
            : { error: { code: "cursor_expired" } },
        ),
        { status: pages === 1 ? 200 : 410 },
      ),
  );
  vi.stubGlobal("fetch", fetch);
  mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Refresh inbox" }));
  await screen.findByText("intent.created");
  await user.click(screen.getByRole("button", { name: "Load more inbox" }));
  await screen.findByRole("button", { name: "Reset to retained inbox" });
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(screen.queryByText("intent.created")).toBeNull();
  expect(
    screen.getByText(/Review pending invitations and assignments separately/),
  ).toBeTruthy();
});
test("old agent response cannot populate newly selected inbox", async () => {
  let resolve: (value: Response) => void = () => {};
  const fetch = vi.fn(
    () =>
      new Promise<Response>((done) => {
        resolve = done;
      }),
  );
  vi.stubGlobal("fetch", fetch);
  mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Refresh inbox" }));
  await user.selectOptions(
    screen.getByRole("combobox", { name: "Inbox agent" }),
    "other",
  );
  await act(async () => {
    resolve(
      new Response(JSON.stringify({ events: [event], next_cursor: "old" })),
    );
  });
  await waitFor(() => expect(screen.queryByText("intent.created")).toBeNull());
});

test("new discovery notification guides owner to current intention review", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            events: [
              {
                ...event,
                type: "opportunity.available",
                resource_path: "/v1/opportunities/current",
              },
            ],
            next_cursor: "cursor",
          }),
        ),
    ),
  );
  mount();
  await userEvent
    .setup()
    .click(screen.getByRole("button", { name: "Refresh inbox" }));
  await screen.findByText("New discovery result");
  expect(
    screen
      .getByRole("link", { name: "Review current intention results" })
      .getAttribute("href"),
  ).toBe("#intentions-monitoring");
  expect(
    screen.getByText(/Choose your intention and select Review intention/),
  ).toBeTruthy();
});
