import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { test, expect, vi } from "vitest";
import { CompatibleGroups } from "./CompatibleGroups";

const source = "11111111-1111-4111-8111-111111111111";
const candidate = "22222222-2222-4222-8222-222222222222";
const other = "33333333-3333-4333-8333-333333333333";
const principal = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const secondPrincipal = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
function page() {
  return {
    items: [
      {
        members: [
          {
            intent_id: source,
            principal_id: principal,
            hard_constraints: {
              required_fields: ["source_url"],
              license_ids: ["CC0-1.0"],
            },
          },
          {
            intent_id: candidate,
            principal_id: secondPrincipal,
            hard_constraints: {
              required_fields: ["citation_id"],
              license_ids: ["CC0-1.0"],
              coverage: { required: ["EU"], excluded: [] as string[] },
              freshness: { max_age_seconds: 3600 },
              delivery: { deliver_by: "2030-01-01T00:00:00Z" },
              input_schema_families: ["source_catalog_v1"],
            },
          },
        ],
        common_license_ids: ["CC0-1.0"],
        comparison_mode: "pairwise_semantic_and_hard_constraints",
        identity_assurance: "common_control_unknown",
      },
    ],
    candidate_limit: 20,
    group_limit: 10,
    truncated: false,
    limitations: [
      "bounded_nonexhaustive_views",
      "principal_ids_do_not_prove_independent_owners",
    ],
  };
}
function mount() {
  const onUnauthorized = vi.fn();
  render(
    <CompatibleGroups
      intentionID={source}
      csrf="csrf"
      onUnauthorized={onUnauthorized}
    />,
  );
  return onUnauthorized;
}
function response(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), { status });
}

test("explicit view displays pairwise evidence, declared constraints and bounded unknown-owner disclosures", async () => {
  const value = page();
  value.truncated = true;
  const fetch = vi.fn(async () => response(value));
  vi.stubGlobal("fetch", fetch);
  mount();
  expect(fetch).not.toHaveBeenCalled();
  await userEvent.click(
    screen.getByRole("button", { name: "View compatible groups" }),
  );
  expect(
    await screen.findByText(
      /Pairwise semantic similarity and hard constraints/,
    ),
  ).toBeTruthy();
  expect(screen.getByText(candidate)).toBeTruthy();
  expect(screen.getByText(secondPrincipal)).toBeTruthy();
  expect(screen.getByText(/citation_id/)).toBeTruthy();
  expect(screen.getByText(/max_age_seconds/)).toBeTruthy();
  expect(screen.getByText(/common control is unknown/i)).toBeTruthy();
  expect(screen.getByText(/bounded.*nonexhaustive/i)).toBeTruthy();
  expect(screen.getByText(/20 candidates.*10 views/i)).toBeTruthy();
  expect(fetch.mock.calls).toHaveLength(1);
  expect((fetch.mock.calls as unknown as [string, RequestInit][])[0][0]).toBe(
    `/v1/owner/intents/${source}/compatible-groups`,
  );
  expect(document.querySelector("a")).toBeNull();
});

test("empty exact group response is a gap without demand claims", async () => {
  const value = page();
  value.items = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => response(value)),
  );
  mount();
  await userEvent.click(
    screen.getByRole("button", { name: "View compatible groups" }),
  );
  expect(await screen.findByText(/No current compatible groups/)).toBeTruthy();
});

test.each([401, 403, 404, 500])(
  "authority/runtime failure %s clears prior groups",
  async (status) => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(response(page()))
      .mockResolvedValueOnce(response({ error: { code: "denied" } }, status));
    vi.stubGlobal("fetch", fetch);
    const unauthorized = mount();
    await userEvent.click(
      screen.getByRole("button", { name: "View compatible groups" }),
    );
    await screen.findByText(candidate);
    await userEvent.click(
      screen.getByRole("button", { name: "Refresh compatible groups" }),
    );
    await screen.findByRole("alert");
    expect(screen.queryByText(candidate)).toBeNull();
    expect(unauthorized).toHaveBeenCalledTimes(status === 401 ? 1 : 0);
  },
);

test.each([
  "unknown mode",
  "private field",
  "duplicate owner",
  "missing source",
  "wrong bounds",
  "invalid constraints",
])("malformed group %s is rejected without displaying proof", async (fault) => {
  const value = page();
  if (fault === "unknown mode")
    value.items[0].comparison_mode = "confidence_probability";
  if (fault === "private field")
    Object.assign(value.items[0].members[1], {
      budget: { secret: "private ceiling" },
    });
  if (fault === "duplicate owner")
    value.items[0].members[1].principal_id = principal;
  if (fault === "missing source") value.items[0].members[0].intent_id = other;
  if (fault === "wrong bounds") value.candidate_limit = 999;
  if (fault === "invalid constraints")
    value.items[0].members[1].hard_constraints.freshness = {
      max_age_seconds: -1,
    };
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => response(value)),
  );
  mount();
  await userEvent.click(
    screen.getByRole("button", { name: "View compatible groups" }),
  );
  await screen.findByRole("alert");
  expect(screen.queryByText(candidate)).toBeNull();
  expect(screen.queryByText(/private ceiling/)).toBeNull();
});

test("intention or session change clears results and ignores a late response", async () => {
  let finish!: (result: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve;
        }),
    ),
  );
  const props = { intentionID: source, csrf: "csrf", onUnauthorized: vi.fn() };
  const view = render(<CompatibleGroups {...props} />);
  await userEvent.click(
    screen.getByRole("button", { name: "View compatible groups" }),
  );
  view.rerender(
    <CompatibleGroups {...props} intentionID={other} csrf="new-session" />,
  );
  finish(response(page()));
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "View compatible groups",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  expect(screen.queryByText(candidate)).toBeNull();
  expect(screen.queryByRole("alert")).toBeNull();
});

test("server views remain separate and untrusted labels remain inert text", async () => {
  const value = page();
  value.items[0].comparison_mode = "exact_declared_outcome";
  value.items[0].members[1].hard_constraints.required_fields = [
    "<script>https://example.invalid/execute</script>",
  ];
  const second = structuredClone(value.items[0]);
  second.members[1].intent_id = other;
  second.members[1].principal_id = "cccccccc-cccc-4ccc-8ccc-cccccccccccc";
  value.items.push(second);
  const fetch = vi.fn(async () => response(value));
  vi.stubGlobal("fetch", fetch);
  mount();
  await userEvent.click(
    screen.getByRole("button", { name: "View compatible groups" }),
  );
  expect(
    await screen.findByRole("heading", { name: "Compatible view 2" }),
  ).toBeTruthy();
  expect(screen.getAllByText(source)).toHaveLength(2);
  expect(
    screen.getAllByText("Exact declared outcome and hard constraints"),
  ).toHaveLength(2);
  expect(document.querySelector("script")).toBeNull();
  expect(document.querySelector("a")).toBeNull();
  expect(fetch).toHaveBeenCalledTimes(1);
});

test("unmount ignores an outstanding authority failure", async () => {
  let finish!: (result: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve;
        }),
    ),
  );
  const onUnauthorized = vi.fn();
  const view = render(
    <CompatibleGroups
      intentionID={source}
      csrf="csrf"
      onUnauthorized={onUnauthorized}
    />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "View compatible groups" }),
  );
  view.unmount();
  finish(response({ error: { code: "expired" } }, 401));
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(onUnauthorized).not.toHaveBeenCalled();
});

test.each([
  "duplicate limitation",
  "duplicate coverage",
  "oversized coverage label",
  "empty input schemas",
  "unqualified delivery timestamp",
  "empty coverage label",
  "duplicate excluded coverage",
  "duplicate input schemas",
  "empty input schema label",
  "oversized input schema label",
])("canonical group constraint %s is rejected", async (fault) => {
  const value = page();
  const constraints = value.items[0].members[1].hard_constraints;
  if (!constraints.coverage || !constraints.delivery)
    throw new Error("Detailed fixture constraints required");
  if (fault === "duplicate limitation")
    value.limitations = [
      "bounded_nonexhaustive_views",
      "bounded_nonexhaustive_views",
    ];
  if (fault === "duplicate coverage")
    constraints.coverage.required = ["EU", "EU"];
  if (fault === "oversized coverage label")
    constraints.coverage.required = ["x".repeat(129)];
  if (fault === "empty input schemas") constraints.input_schema_families = [];
  if (fault === "empty coverage label") constraints.coverage.required = [""];
  if (fault === "duplicate excluded coverage")
    constraints.coverage.excluded = ["EU", "EU"];
  if (fault === "duplicate input schemas")
    constraints.input_schema_families = [
      "source_catalog_v1",
      "source_catalog_v1",
    ];
  if (fault === "empty input schema label")
    constraints.input_schema_families = [""];
  if (fault === "oversized input schema label")
    constraints.input_schema_families = ["x".repeat(129)];
  if (fault === "unqualified delivery timestamp")
    constraints.delivery.deliver_by = "2030-01-01T00:00:00";
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => response(value)),
  );
  mount();
  await userEvent.click(
    screen.getByRole("button", { name: "View compatible groups" }),
  );
  await screen.findByRole("alert");
  expect(screen.queryByText(candidate)).toBeNull();
});

test("canonical null constraints and Unicode codepoint boundaries remain displayable", async () => {
  const value = page();
  Object.assign(value.items[0].members[0].hard_constraints, {
    required_fields: null,
    license_ids: null,
    coverage: null,
    freshness: null,
    delivery: null,
    input_schema_families: null,
  });
  const coverage = value.items[0].members[1].hard_constraints.coverage;
  if (!coverage) throw new Error("Fixture coverage required");
  coverage.required = ["🌍".repeat(128)];
  value.items[0].members[1].hard_constraints.input_schema_families = [
    "🌍".repeat(128),
  ];
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => response(value)),
  );
  mount();
  await userEvent.click(
    screen.getByRole("button", { name: "View compatible groups" }),
  );
  expect(await screen.findByText(candidate)).toBeTruthy();
  expect(screen.queryByRole("alert")).toBeNull();
});

const deliveryCases: [string, string, boolean][] = [
  ["impossible February day", "2030-02-30T00:00:00Z", false],
  ["nonleap century", "2100-02-29T00:00:00Z", false],
  ["clock rollover", "2030-01-01T24:00:00Z", false],
  ["minute overflow", "2030-01-01T00:60:00Z", false],
  ["second overflow", "2030-01-01T23:59:61Z", false],
  ["leap second outside UTC day end", "2016-12-31T12:59:60Z", false],
  ["zone hour overflow", "2030-01-01T00:00:00+24:00", false],
  ["zone minute overflow", "2030-01-01T00:00:00+01:60", false],
  ["missing zone", "2030-01-01T00:00:00", false],
  ["leap century", "2000-02-29T00:00:00Z", true],
  ["nanosecond offset", "2032-02-29T00:00:00.123456789+05:45", true],
  ["lowercase timestamp", "2032-02-29t00:00:00.123456789z", true],
  ["unknown local offset", "2030-01-01T00:00:00-00:00", true],
  ["UTC leap second", "2016-12-31T23:59:60Z", true],
  ["offset leap second", "2017-01-01T00:59:60+01:00", true],
];
test.each(deliveryCases)(
  "delivery timestamp %s follows its published format",
  async (_name, timestamp, valid) => {
    const value = page();
    const delivery = value.items[0].members[1].hard_constraints.delivery;
    if (!delivery) throw new Error("Fixture delivery required");
    delivery.deliver_by = timestamp;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => response(value)),
    );
    mount();
    await userEvent.click(
      screen.getByRole("button", { name: "View compatible groups" }),
    );
    if (valid) {
      await screen.findByText(candidate);
      expect(screen.getByText((text) => text.includes(timestamp))).toBeTruthy();
      expect(screen.queryByRole("alert")).toBeNull();
    } else {
      await screen.findByRole("alert");
      expect(screen.queryByText(candidate)).toBeNull();
    }
  },
);
