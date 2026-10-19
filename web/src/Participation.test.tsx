import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { test, expect, vi } from "vitest";
import { Participation } from "./Participation";

const agents = [
  { id: "a", name: "My agent", status: "active" },
  { id: "b", name: "Other agent", status: "active" },
];
const packet = {
  id: "invite",
  swarm_id: "room",
  objective: "<script>untrusted</script>",
  role: "contributor",
  visibility: "restricted",
  history_policy: "full",
  state: "offered",
  etag: '"3"',
  respond_by: "2030-01-01T00:00:00Z",
  content_trust: "untrusted",
};
function fixtures(conflict = false) {
  const calls: { path: string; init?: RequestInit }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      calls.push({ path, init });
      let body: unknown = {};
      let status = 200;
      if (path.endsWith("/participation"))
        body = {
          pending: [
            {
              id: "invite",
              swarm_id: "room",
              etag: '"3"',
              respond_by: packet.respond_by,
            },
          ],
          swarms: [],
          next_after: "",
          limit: 100,
        };
      else if (path.endsWith("/invitations/invite")) body = packet;
      else if (path.endsWith("/membership-response")) {
        status = conflict ? 409 : 200;
        body = conflict
          ? { error: { code: "invitation_not_current" } }
          : { state: "accepted" };
      }
      return new Response(JSON.stringify(body), { status });
    }),
  );
  return calls;
}
test("owner reviews the invitation before exact versioned decision without agent bearer", async () => {
  const calls = fixtures();
  render(
    <Participation agents={agents} csrf="csrf" onUnauthorized={vi.fn()} />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Load participation" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Review invitation" }),
  );
  expect(await screen.findByText(packet.objective)).toBeTruthy();
  expect(screen.getByText(/full room history/i)).toBeTruthy();
  await userEvent.click(
    screen.getByRole("button", { name: "Accept invitation" }),
  );
  await waitFor(() =>
    expect(calls.some((c) => c.path.endsWith("/membership-response"))).toBe(
      true,
    ),
  );
  const call = calls.find((c) => c.path.endsWith("/membership-response"))!;
  expect(JSON.parse(call.init!.body as string)).toEqual({
    invitation_id: "invite",
    decision: "accept",
  });
  const h = new Headers(call.init?.headers);
  expect(h.get("If-Match")).toBe('"3"');
  expect(h.get("Authorization")).toBeNull();
  expect(h.get("X-CSRF-Token")).toBe("csrf");
});
test("stale invitation requires another review before a decision", async () => {
  fixtures(true);
  render(
    <Participation agents={agents} csrf="csrf" onUnauthorized={vi.fn()} />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Load participation" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Review invitation" }),
  );
  await screen.findByText(packet.objective);
  await userEvent.click(
    screen.getByRole("button", { name: "Decline invitation" }),
  );
  expect((await screen.findByRole("alert")).textContent).toMatch(
    /changed.*review/i,
  );
  expect(
    screen.queryByRole("button", { name: "Accept invitation" }),
  ).toBeNull();
});
test("changing selected agent discards an earlier pending response", async () => {
  let resolve!: (r: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((r) => {
          resolve = r;
        }),
    ),
  );
  render(
    <Participation agents={agents} csrf="csrf" onUnauthorized={vi.fn()} />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Load participation" }),
  );
  await userEvent.selectOptions(
    screen.getByLabelText("Participation agent"),
    "b",
  );
  resolve(
    new Response(
      JSON.stringify({
        pending: [{ id: "stale" }],
        swarms: [],
        next_after: "",
        limit: 100,
      }),
    ),
  );
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "Load participation",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  expect(
    screen.queryByRole("button", { name: "Review invitation" }),
  ).toBeNull();
});
test("lost session clears participation and notifies the owner console", async () => {
  const unauthorized = vi.fn();
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({ error: { code: "verified_owner_required" } }),
          { status: 401 },
        ),
    ),
  );
  render(
    <Participation agents={agents} csrf="csrf" onUnauthorized={unauthorized} />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Load participation" }),
  );
  await waitFor(() => expect(unauthorized).toHaveBeenCalledOnce());
  expect(
    screen.queryByRole("button", { name: "Review invitation" }),
  ).toBeNull();
});
