import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { ExternalLeads } from "./ExternalLeads";

const lead = (id = "lead-one", title = "Reference catalog") => ({
  id,
  origin: "external_capability",
  canonical_url: "https://publisher.example/catalog",
  publisher: "Example publisher",
  permission_notice: "Metadata shared with permission",
  metadata: { title, summary: "A public catalog" },
  checked_at: "2026-10-18T09:00:00Z",
  fresh_until: "2030-01-01T00:00:00Z",
  identity_assurance: "unknown",
  fit_status: "not_assessed",
});
const page = (rows = [lead()], cursor = "") => ({
  items: rows,
  next_cursor: cursor,
});
function respond(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status });
}

test("explicit owner search renders provenance as text and pages with the applied filters", async () => {
  const fetcher = vi
    .fn()
    .mockResolvedValueOnce(
      respond(page([lead("one", "<img onerror=alert(1)>")], "opaque+/=")),
    )
    .mockResolvedValueOnce(respond(page([lead("two")])));
  vi.stubGlobal("fetch", fetcher);
  render(
    <ExternalLeads
      intentionID="intent"
      csrf="csrf"
      onUnauthorized={() => {}}
    />,
  );
  expect(fetcher).not.toHaveBeenCalled();
  await userEvent.type(
    screen.getByLabelText("Search external leads"),
    "catalog",
  );
  await userEvent.selectOptions(
    screen.getByLabelText("Lead type"),
    "external_capability",
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Find external leads" }),
  );
  expect(await screen.findByText("<img onerror=alert(1)>")).toBeTruthy();
  expect(document.querySelector("img")).toBeNull();
  expect(screen.getByText("Metadata shared with permission")).toBeTruthy();
  expect(screen.getByText(/Identity and fit are unverified/)).toBeTruthy();
  const [path, init] = fetcher.mock.calls[0];
  expect(path).toContain("/v1/owner/intents/intent/external-leads?");
  expect(new URL(path, "https://owner.example").searchParams.get("q")).toBe(
    "catalog",
  );
  expect(init.credentials).toBe("same-origin");
  expect(init.headers.has("Authorization")).toBe(false);
  fireEvent.change(screen.getByLabelText("Search external leads"), {
    target: { value: "draft change" },
  });
  await userEvent.click(
    screen.getByRole("button", { name: "Load more leads" }),
  );
  await screen.findByText("Reference catalog");
  const next = new URL(fetcher.mock.calls[1][0], "https://owner.example");
  expect(next.searchParams.get("cursor")).toBe("opaque+/=");
  expect(next.searchParams.get("q")).toBe("catalog");
  expect(screen.getAllByRole("link", { name: "Open source" })).toHaveLength(2);
});

test("session loss clears previously visible results and returns control to the owner session", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(respond(page()))
      .mockResolvedValueOnce(
        respond({ error: { code: "invalid_session" } }, 401),
      ),
  );
  const onUnauthorized = vi.fn();
  render(
    <ExternalLeads
      intentionID="intent"
      csrf="csrf"
      onUnauthorized={onUnauthorized}
    />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Find external leads" }),
  );
  await screen.findByText("Reference catalog");
  await userEvent.click(
    screen.getByRole("button", { name: "Find external leads" }),
  );
  await waitFor(() => expect(onUnauthorized).toHaveBeenCalledOnce());
  expect(screen.queryByText("Reference catalog")).toBeNull();
});

test("a response for a closed intention cannot populate a newly selected intention", async () => {
  let resolve!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    ),
  );
  const { rerender } = render(
    <ExternalLeads
      key="old"
      intentionID="old"
      csrf="csrf"
      onUnauthorized={() => {}}
    />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Find external leads" }),
  );
  rerender(
    <ExternalLeads
      key="new"
      intentionID="new"
      csrf="csrf"
      onUnauthorized={() => {}}
    />,
  );
  resolve(respond(page()));
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "Find external leads",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  expect(screen.queryByText("Reference catalog")).toBeNull();
});

test("an unsafe source URL is never rendered as a navigable link", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        respond(page([{ ...lead(), canonical_url: "javascript:alert(1)" }])),
      ),
  );
  render(
    <ExternalLeads
      intentionID="intent"
      csrf="csrf"
      onUnauthorized={() => {}}
    />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Find external leads" }),
  );
  await screen.findByText("Reference catalog");
  expect(screen.queryByRole("link", { name: "Open source" })).toBeNull();
});

test("an old unauthorized response cannot sign out a newer selection", async () => {
  let resolve!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    ),
  );
  const staleSession = vi.fn(),
    currentSession = vi.fn();
  const { rerender } = render(
    <ExternalLeads
      key="old"
      intentionID="old"
      csrf="old"
      onUnauthorized={staleSession}
    />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Find external leads" }),
  );
  rerender(
    <ExternalLeads
      key="new"
      intentionID="new"
      csrf="new"
      onUnauthorized={currentSession}
    />,
  );
  resolve(respond({ error: { code: "invalid_session" } }, 401));
  await new Promise((done) => setTimeout(done, 0));
  expect(staleSession).not.toHaveBeenCalled();
  expect(currentSession).not.toHaveBeenCalled();
  expect(screen.queryByRole("alert")).toBeNull();
});
