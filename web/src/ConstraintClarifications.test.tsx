import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { ConstraintClarifications } from "./ConstraintClarifications";

const source = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const candidate = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
function page() {
  return {
    source_intent_id: source,
    source_version: 1,
    source_revision: 1,
    scope: "registered_demand_hard_evidence_v1",
    identity_assurance: "common_control_unknown",
    candidate_limit: 20,
    truncated: false,
    items: [
      {
        candidate_intent_id: candidate,
        candidate_version: 1,
        principal_id: "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
        public_summary: "Public catalogue request",
        output: { kind: "json_dataset", schema_family: "source_catalog_v1" },
        hard_constraints: {
          required_fields: ["source_id"],
          license_ids: ["CC0-1.0"],
        },
        hard_status: "unknown",
        outcome_comparison: "same_declared_outcome",
        clarification_required: true,
        constraint_evidence: [
          {
            dimension: "output",
            status: "aligned",
            reason: "same_output_schema",
          },
          {
            dimension: "license_ids",
            status: "aligned",
            reason: "common_permitted_license",
          },
          {
            dimension: "required_fields",
            status: "aligned",
            reason: "required_fields_can_be_combined",
          },
          {
            dimension: "coverage",
            status: "unknown",
            reason: "coverage_unspecified",
          },
        ],
      },
    ],
  };
}
function response(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), { status });
}
function mount() {
  return render(
    <ConstraintClarifications
      intentionID={source}
      sourceVersion={1}
      csrf="csrf"
      onUnauthorized={vi.fn()}
    />,
  );
}

test("explicitly loads missing declarations as clarification without claiming a match", async () => {
  const fetch = vi.fn(async (_path: string, _init?: RequestInit) =>
    response(page()),
  );
  vi.stubGlobal("fetch", fetch);
  mount();
  expect(fetch).not.toHaveBeenCalled();
  await userEvent.click(
    screen.getByRole("button", { name: "Check requirement gaps" }),
  );
  expect(await screen.findByText("Public catalogue request")).toBeTruthy();
  expect(screen.getByText("Needs clarification", { exact: true })).toBeTruthy();
  expect(screen.getByText("Coverage", { exact: true })).toBeTruthy();
  expect(
    screen.getByText(/Clarify this declaration before combining requirements/),
  ).toBeTruthy();
  expect(fetch).toHaveBeenCalledWith(
    `/v1/owner/intents/${source}/constraint-assessments`,
    expect.objectContaining({
      method: "GET",
      credentials: "same-origin",
      redirect: "error",
      cache: "no-store",
    }),
  );
  expect(
    new Headers(fetch.mock.calls[0]?.[1]?.headers).get("Authorization"),
  ).toBeNull();
});

test.each(["aligned", "incompatible"])(
  "hard %s remains distinct from outcome similarity",
  async (status) => {
    const value = page();
    const row = value.items[0];
    row.hard_status = status;
    row.outcome_comparison = "not_evaluated";
    row.clarification_required = false;
    row.constraint_evidence[3] = {
      dimension: "coverage",
      status: "aligned",
      reason: "coverage_requirements_can_be_combined",
    };
    if (status === "incompatible")
      row.constraint_evidence[1] = {
        dimension: "license_ids",
        status: "incompatible",
        reason: "no_common_license",
      };
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => response(value)),
    );
    mount();
    await userEvent.click(
      screen.getByRole("button", { name: "Check requirement gaps" }),
    );
    expect(
      await screen.findByText(
        status === "aligned"
          ? "Declared requirements can combine"
          : "Declared requirements conflict",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText("Outcome similarity is not evaluated here."),
    ).toBeTruthy();
    expect(
      screen.queryByText("Needs clarification", { exact: true }),
    ).toBeNull();
  },
);

test.each([
  "source",
  "version",
  "scope",
  "identity",
  "private",
  "coherence",
  "clarification",
  "duplicate",
  "self",
  "dimension",
  "truncation",
])("rejects malformed %s reports before disclosure", async (fault) => {
  const value = page();
  if (fault === "source") value.source_intent_id = candidate;
  if (fault === "version") value.source_version = 2;
  if (fault === "scope") value.scope = "verified_match";
  if (fault === "identity")
    value.identity_assurance = "verified_independent_owners";
  if (fault === "private")
    Object.assign(value.items[0], { budget: { amount_minor: 100 } });
  if (fault === "coherence") value.items[0].hard_status = "aligned";
  if (fault === "clarification") value.items[0].clarification_required = false;
  if (fault === "duplicate") value.items.push(structuredClone(value.items[0]));
  if (fault === "self") value.items[0].candidate_intent_id = source;
  if (fault === "dimension")
    value.items[0].constraint_evidence[0].dimension = "private_budget";
  if (fault === "truncation") value.truncated = true;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => response(value)),
  );
  mount();
  await userEvent.click(
    screen.getByRole("button", { name: "Check requirement gaps" }),
  );
  await screen.findByRole("alert");
  expect(screen.queryByText("Public catalogue request")).toBeNull();
});

test("refresh clears an earlier view before a pending read and current denial", async () => {
  let finish!: (r: Response) => void;
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(response(page()))
    .mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve;
        }),
    );
  vi.stubGlobal("fetch", fetch);
  mount();
  await userEvent.click(
    screen.getByRole("button", { name: "Check requirement gaps" }),
  );
  await screen.findByText("Public catalogue request");
  await userEvent.click(
    screen.getByRole("button", { name: "Refresh requirement gaps" }),
  );
  expect(screen.queryByText("Public catalogue request")).toBeNull();
  expect(
    screen
      .getByRole("button", { name: "Refresh requirement gaps" })
      .matches(":disabled"),
  ).toBe(true);
  finish(response({ error: { code: "not_found" } }, 404));
  await screen.findByRole("alert");
  expect(screen.queryByText("Public catalogue request")).toBeNull();
});

test.each([
  ["version", 200],
  ["version", 401],
  ["session", 200],
  ["session", 401],
  ["selection", 200],
  ["selection", 401],
] as const)("changed %s ignores a late %s report", async (change, status) => {
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
  const expired = vi.fn();
  const view = render(
    <ConstraintClarifications
      intentionID={source}
      sourceVersion={1}
      csrf="old"
      onUnauthorized={expired}
    />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Check requirement gaps" }),
  );
  view.rerender(
    <ConstraintClarifications
      intentionID={
        change === "selection" ? "00000000-0000-4000-8000-000000000099" : source
      }
      sourceVersion={change === "version" ? 2 : 1}
      csrf={change === "session" ? "new" : "old"}
      onUnauthorized={expired}
    />,
  );
  finish(
    response(status === 200 ? page() : { error: { code: "expired" } }, status),
  );
  await waitFor(() =>
    expect(
      screen
        .getByRole("button", { name: "Check requirement gaps" })
        .matches(":disabled"),
    ).toBe(false),
  );
  expect(screen.queryByText("Public catalogue request")).toBeNull();
  expect(screen.queryByRole("alert")).toBeNull();
  expect(expired).not.toHaveBeenCalled();
});

test("current session expiry clears the view and calls the owner sign-out path", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => response({ error: { code: "expired" } }, 401)),
  );
  const expired = vi.fn();
  render(
    <ConstraintClarifications
      intentionID={source}
      sourceVersion={1}
      csrf="csrf"
      onUnauthorized={expired}
    />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Check requirement gaps" }),
  );
  await screen.findByRole("alert");
  expect(expired).toHaveBeenCalledTimes(1);
});

test("publisher text stays inert", async () => {
  const value = page();
  value.items[0].public_summary =
    "<script>fetch('https://example.invalid/steal')</script>";
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => response(value)),
  );
  mount();
  await userEvent.click(
    screen.getByRole("button", { name: "Check requirement gaps" }),
  );
  await screen.findByText(value.items[0].public_summary);
  expect(document.querySelector("script")).toBeNull();
  expect(document.querySelector("a")).toBeNull();
});
