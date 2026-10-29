import { render, screen, waitFor, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { PartialSatisfaction, type PartialReview } from "./PartialSatisfaction";
const review: PartialReview = {
  intentionID: "intent",
  leadID: "lead",
  revision: 1,
  version: 1,
  etag: '"1"',
  sourceVersion: "source-v1",
  projectionHash: "a".repeat(64),
  reportHash: "b".repeat(64),
  requiredFields: ["source_id", "source_url", "title"],
};
const result = () => ({
  id: "intent",
  version: 2,
  etag: '"2"',
  state: "active",
  current_revision: 2,
  objective: "European source catalog",
  output: { kind: "json_dataset", schema_family: "source_catalog_v1" },
  visibility: "private",
  expires_at: "2099-01-01T00:00:00Z",
  budget: null,
  watch: { enabled: true, channels: ["inbox"] },
  hard_constraints: {
    required_fields: ["source_url", "title"],
    license_ids: ["CC0-1.0"],
  },
  partial_satisfaction: {
    evidence_class: "owner_declared_partial_satisfaction",
    basis_revision: 1,
    result_revision: 2,
    fulfilled_required_fields: ["source_id"],
    remaining_required_fields: ["source_url", "title"],
    lead_id: "lead",
    source_version: review.sourceVersion,
    projection_hash: review.projectionHash,
    report_hash: review.reportHash,
  },
});
const props = () => ({
  review,
  csrf: "partial-csrf",
  onRecorded: vi.fn(),
  onUnauthorized: vi.fn(),
  onUnavailable: vi.fn(),
});
test("owner explicitly chooses fulfillment and reviews nonempty remaining fields before the pinned command", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(new Response(JSON.stringify(result()), { status: 200 }));
  vi.stubGlobal("fetch", fetch);
  const p = props();
  render(<PartialSatisfaction {...p} />);
  const chosen = screen.getByLabelText(
    "I have fulfilled source_id",
  ) as HTMLInputElement;
  expect(chosen.checked).toBe(false);
  expect(
    (
      screen.getByRole("button", {
        name: "Save unmet remainder",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
  await userEvent.click(chosen);
  expect(screen.getByLabelText("Unmet required fields").textContent).toContain(
    "source_url",
  );
  expect(
    screen.getByLabelText("Unmet required fields").textContent,
  ).not.toContain("source_id");
  await userEvent.click(
    screen.getByRole("button", { name: "Save unmet remainder" }),
  );
  await waitFor(() => expect(p.onRecorded).toHaveBeenCalledTimes(1));
  expect(fetch.mock.calls[0][0]).toBe(
    "/v1/owner/intents/intent/partial-satisfaction",
  );
  expect(new Headers(fetch.mock.calls[0][1].headers).get("If-Match")).toBe(
    '"1"',
  );
  expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({
    lead_id: "lead",
    source_version: "source-v1",
    projection_hash: review.projectionHash,
    report_hash: review.reportHash,
    intention_revision: 1,
    fulfilled_required_fields: ["source_id"],
  });
});
test("malformed success retains the exact retry key and does not claim recorded fulfillment", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(new Response("{}", { status: 200 }))
    .mockResolvedValueOnce(
      new Response(JSON.stringify(result()), { status: 200 }),
    );
  vi.stubGlobal("fetch", fetch);
  const p = props();
  render(<PartialSatisfaction {...p} />);
  await userEvent.click(screen.getByLabelText("I have fulfilled source_id"));
  await userEvent.click(
    screen.getByRole("button", { name: "Save unmet remainder" }),
  );
  await screen.findByRole("alert");
  expect(p.onRecorded).not.toHaveBeenCalled();
  await userEvent.click(
    screen.getByRole("button", { name: "Save unmet remainder" }),
  );
  await waitFor(() => expect(p.onRecorded).toHaveBeenCalledTimes(1));
  expect(
    new Headers(fetch.mock.calls[0][1].headers).get("Idempotency-Key"),
  ).toBe(new Headers(fetch.mock.calls[1][1].headers).get("Idempotency-Key"));
});
test("all fields cannot be submitted as partial fulfillment and selection changes suppress late results", async () => {
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
  const p = props();
  const view = render(<PartialSatisfaction {...p} />);
  for (const field of review.requiredFields)
    await userEvent.click(screen.getByLabelText(`I have fulfilled ${field}`));
  expect(
    (
      screen.getByRole("button", {
        name: "Save unmet remainder",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
  await userEvent.click(screen.getByLabelText("I have fulfilled source_url"));
  await userEvent.click(screen.getByLabelText("I have fulfilled title"));
  await userEvent.click(
    screen.getByRole("button", { name: "Save unmet remainder" }),
  );
  view.rerender(
    <PartialSatisfaction {...p} review={{ ...review, leadID: "other" }} />,
  );
  await act(async () =>
    finish(new Response(JSON.stringify(result()), { status: 200 })),
  );
  expect(p.onRecorded).not.toHaveBeenCalled();
  expect(
    (screen.getByLabelText("I have fulfilled source_id") as HTMLInputElement)
      .checked,
  ).toBe(false);
});

test.each(["objective", "expires_at", "current_revision", "output"])(
  "well-pinned success missing %s retains exact retry and never reaches the owner consumer",
  async (field) => {
    const valid = result();
    const malformed: Record<string, unknown> = { ...valid };
    delete malformed[field];
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify(malformed), { status: 200 }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(valid), { status: 200 }),
      );
    vi.stubGlobal("fetch", fetch);
    const p = props();
    render(<PartialSatisfaction {...p} csrf={`missing-${field}`} />);
    await userEvent.click(screen.getByLabelText("I have fulfilled source_id"));
    await userEvent.click(
      screen.getByRole("button", { name: "Save unmet remainder" }),
    );
    await screen.findByRole("alert");
    expect(p.onRecorded).not.toHaveBeenCalled();
    await userEvent.click(
      screen.getByRole("button", { name: "Save unmet remainder" }),
    );
    await waitFor(() => expect(p.onRecorded).toHaveBeenCalledTimes(1));
    expect(fetch.mock.calls[0][1].body).toBe(fetch.mock.calls[1][1].body);
    expect(
      new Headers(fetch.mock.calls[0][1].headers).get("Idempotency-Key"),
    ).toBe(new Headers(fetch.mock.calls[1][1].headers).get("Idempotency-Key"));
  },
);

test("a changed version pin invalidates an in-flight result even when the revision and lead are unchanged", async () => {
  let finish!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve;
        }),
    ),
  );
  const p = props();
  const view = render(
    <PartialSatisfaction {...p} csrf="changed-version-pin" />,
  );
  await userEvent.click(screen.getByLabelText("I have fulfilled source_id"));
  await userEvent.click(
    screen.getByRole("button", { name: "Save unmet remainder" }),
  );
  view.rerender(
    <PartialSatisfaction
      {...p}
      csrf="changed-version-pin"
      review={{ ...review, version: 2, etag: '"2"' }}
    />,
  );
  await act(async () =>
    finish(new Response(JSON.stringify(result()), { status: 200 })),
  );
  expect(p.onRecorded).not.toHaveBeenCalled();
  expect(
    (screen.getByLabelText("I have fulfilled source_id") as HTMLInputElement)
      .checked,
  ).toBe(false);
});

test("a well-pinned response with array visibility retains the exact retry command", async () => {
  const valid = result();
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      new Response(JSON.stringify({ ...valid, visibility: ["private"] }), {
        status: 200,
      }),
    )
    .mockResolvedValueOnce(
      new Response(JSON.stringify(valid), { status: 200 }),
    );
  vi.stubGlobal("fetch", fetch);
  const p = props();
  render(<PartialSatisfaction {...p} csrf="array-visibility" />);
  await userEvent.click(screen.getByLabelText("I have fulfilled source_id"));
  await userEvent.click(
    screen.getByRole("button", { name: "Save unmet remainder" }),
  );
  await screen.findByRole("alert");
  expect(p.onRecorded).not.toHaveBeenCalled();
  await userEvent.click(
    screen.getByRole("button", { name: "Save unmet remainder" }),
  );
  await waitFor(() => expect(p.onRecorded).toHaveBeenCalledTimes(1));
  expect(fetch.mock.calls[0][1].body).toBe(fetch.mock.calls[1][1].body);
  expect(
    new Headers(fetch.mock.calls[0][1].headers).get("Idempotency-Key"),
  ).toBe(new Headers(fetch.mock.calls[1][1].headers).get("Idempotency-Key"));
});
