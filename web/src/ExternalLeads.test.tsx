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
  observed_at: "2026-10-17T09:00:00Z",
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
  expect(
    document.querySelector('time[datetime="2026-10-17T09:00:00Z"]'),
  ).toBeTruthy();
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

test("owner can inspect declared constraint gaps for a selected capability lead", async () => {
  const id = "00000000-0000-4000-8000-000000000001";
  const fetcher = vi
    .fn()
    .mockResolvedValueOnce(respond(page([lead(id)])))
    .mockResolvedValueOnce(
      respond({
        intention_id: "intent",
        intention_revision: 1,
        lead: { id },
        assessment: "needs_clarification",
        verification: "unverified_publisher_claim",
        identity_assurance: "unknown",
        rights_assurance: "unknown",
        claim_status: "declared",
        constraints: [
          {
            constraint: "required_field",
            requested: ["email"],
            declared: [],
            status: "unknown",
            reason: "__proto__",
          },
        ],
      }),
    );
  vi.stubGlobal("fetch", fetcher);
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
  await userEvent.click(
    screen.getByRole("button", { name: "Review declared fit" }),
  );
  expect(
    await screen.findByRole("heading", { name: "Needs clarification" }),
  ).toBeTruthy();
  expect(screen.getByRole("cell", { name: "email" })).toBeTruthy();
  expect(screen.getByRole("cell", { name: "Unknown" })).toBeTruthy();
  const requested = new URL(fetcher.mock.calls[1][0], "https://owner.example");
  expect(requested.pathname).toBe(
    "/v1/owner/intents/intent/external-fit-report",
  );
  expect(requested.searchParams.get("lead_id")).toBe(id);
});

test("external requests are never presented as capability fit candidates", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        respond(page([{ ...lead(), origin: "external_request" }])),
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
  expect(
    screen.queryByRole("button", { name: "Review declared fit" }),
  ).toBeNull();
});

test("revoked report availability removes the previously visible source metadata", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(respond(page()))
      .mockResolvedValueOnce(respond({ error: { code: "not_found" } }, 404)),
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
  await userEvent.click(
    screen.getByRole("button", { name: "Review declared fit" }),
  );
  await screen.findByText("This lead is no longer available. Search again.");
  expect(screen.queryByText("Reference catalog")).toBeNull();
  expect(screen.queryByRole("table")).toBeNull();
});

test("late fit reports cannot replace a newer selected lead", async () => {
  let resolve!: (response: Response) => void;
  const report = (id: string, assessment: string) => ({
    intention_id: "intent",
    intention_revision: 1,
    lead: { id },
    assessment,
    verification: "unverified_publisher_claim",
    identity_assurance: "unknown",
    rights_assurance: "unknown",
    claim_status: "missing",
    constraints: [],
  });
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(
        respond(
          page([
            lead("first", "First catalog"),
            lead("second", "Second catalog"),
          ]),
        ),
      )
      .mockImplementationOnce(
        () =>
          new Promise<Response>((done) => {
            resolve = done;
          }),
      )
      .mockResolvedValueOnce(respond(report("second", "incompatible"))),
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
  await screen.findByText("First catalog");
  await userEvent.click(
    screen.getAllByRole("button", { name: "Review declared fit" })[0],
  );
  await userEvent.click(
    screen.getAllByRole("button", { name: "Review declared fit" })[1],
  );
  await screen.findByRole("heading", { name: "Declared mismatch" });
  resolve(respond(report("first", "declared_aligned")));
  await new Promise((done) => setTimeout(done, 0));
  expect(
    screen.queryByRole("heading", { name: "Declared alignment" }),
  ).toBeNull();
  expect(
    screen.getByRole("heading", { name: "Declared mismatch" }),
  ).toBeTruthy();
});

test("an expired assessment session clears results without claiming an empty search", async () => {
  const unauthorized = vi.fn();
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(respond(page()))
      .mockResolvedValueOnce(
        respond({ error: { code: "invalid_session" } }, 401),
      ),
  );
  render(
    <ExternalLeads
      intentionID="intent"
      csrf="csrf"
      onUnauthorized={unauthorized}
    />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Find external leads" }),
  );
  await screen.findByText("Reference catalog");
  await userEvent.click(
    screen.getByRole("button", { name: "Review declared fit" }),
  );
  await waitFor(() => expect(unauthorized).toHaveBeenCalledOnce());
  expect(screen.queryByText("Reference catalog")).toBeNull();
  expect(
    screen.queryByText("No current permitted leads match these filters."),
  ).toBeNull();
});

test("a report bound to another intention is rejected without displaying its requirements", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(respond(page()))
      .mockResolvedValueOnce(
        respond({
          intention_id: "another-owner-intent",
          intention_revision: 1,
          lead: { id: "lead-one" },
          assessment: "declared_aligned",
          verification: "unverified_publisher_claim",
          identity_assurance: "unknown",
          rights_assurance: "unknown",
          claim_status: "declared",
          constraints: [
            {
              constraint: "required_field",
              requested: ["PRIVATE REQUIREMENT"],
              declared: [],
              status: "unknown",
              reason: "declaration_missing",
            },
          ],
        }),
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
  await userEvent.click(
    screen.getByRole("button", { name: "Review declared fit" }),
  );
  await screen.findByText("The assessment could not be loaded. Try again.");
  expect(screen.queryByText("PRIVATE REQUIREMENT")).toBeNull();
  expect(screen.queryByRole("table")).toBeNull();
});

test.each([401, 404])(
  "assessment%d invalidates pending pagination before it can restore listings",
  async (status) => {
    let assessment!: (response: Response) => void,
      pagination!: (response: Response) => void;
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(respond(page([lead()], "next")))
        .mockImplementationOnce(
          () =>
            new Promise<Response>((done) => {
              assessment = done;
            }),
        )
        .mockImplementationOnce(
          () =>
            new Promise<Response>((done) => {
              pagination = done;
            }),
        ),
    );
    const unauthorized = vi.fn();
    render(
      <ExternalLeads
        intentionID="intent"
        csrf="csrf"
        onUnauthorized={unauthorized}
      />,
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Find external leads" }),
    );
    await screen.findByText("Reference catalog");
    await userEvent.click(
      screen.getByRole("button", { name: "Review declared fit" }),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Load more leads" }),
    );
    assessment(respond({ error: { code: "not_found" } }, status));
    await waitFor(() =>
      expect(screen.queryByText("Reference catalog")).toBeNull(),
    );
    pagination(respond(page([lead("stale", "STALE PAGINATION")])));
    await new Promise((done) => setTimeout(done, 0));
    expect(screen.queryByText("STALE PAGINATION")).toBeNull();
    expect(
      (
        screen.getByRole("button", {
          name: "Find external leads",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false);
    if (status === 401) expect(unauthorized).toHaveBeenCalledOnce();
  },
);
