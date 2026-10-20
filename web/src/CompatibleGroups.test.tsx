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
              coverage: { required: ["EU"], excluded: [] },
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
