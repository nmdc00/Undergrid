import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { OwnerApplications } from "./OwnerApplications";
const agent = "42e75827-4f79-4d96-8832-38835995ad67",
  room = "52e75827-4f79-4d96-8832-38835995ad67",
  id = "62e75827-4f79-4d96-8832-38835995ad67";
const recruitment = {
  id,
  kind: "builder_recruitment",
  origin: "registered_capability",
  swarm_id: room,
  objective: "<script>untrusted</script>",
  visibility: "restricted",
  swarm_version: 7,
  swarm_etag: '"7"',
  role: {
    key: "builder",
    schema_family: "source_catalog_v1",
    description: "Build catalogue",
  },
  input_hash: "a".repeat(64),
  evidence: {
    algorithm: "declared-capability-v1",
    reason: "declared_schema_capability",
    assurance: "self_declared",
  },
  history_policy: "full",
  identity_assurance: "common_control_unknown",
  content_trust: "untrusted",
};
const application = {
  id,
  swarm_id: room,
  agent_id: agent,
  principal_id: "72e75827-4f79-4d96-8832-38835995ad67",
  state: "applied",
  version: 1,
  etag: '"1"',
  respond_by: "2030-01-01T00:00:00Z",
  role_key: "builder",
  application_claims: {
    schema: "application-claims-v1",
    role_key: "builder",
    capability_refs: ["declared-catalogue"],
    role: recruitment.role,
    swarm_version: 7,
  },
  claims_state: "included",
  application_claims_sha256: "a".repeat(64),
  objective: recruitment.objective,
  visibility: "restricted",
  swarm_version: 7,
  history_policy: "full",
  content_trust: "untrusted",
};
function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status });
}
function setup(fetch: ReturnType<typeof vi.fn>, coordinator = false) {
  vi.stubGlobal("fetch", fetch);
  return render(
    <OwnerApplications
      agent={agent}
      csrf="csrf"
      room={coordinator ? room : undefined}
      roomEtag={coordinator ? '"7"' : undefined}
      onUnauthorized={vi.fn()}
      onStale={vi.fn()}
    />,
  );
}
test("applicant reviews role and submits exact room version then reloads pending without admission", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      response({ items: [recruitment], next_after: "", candidate_limit: 50 }),
    )
    .mockResolvedValueOnce(response(recruitment))
    .mockResolvedValueOnce(
      response({ id, state: "applied", version: 1, etag: '"1"' }, 201),
    )
    .mockResolvedValueOnce(response({ items: [application], next_after: "" }));
  setup(fetch);
  await userEvent.click(
    screen.getByRole("button", { name: "Find recruitment" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Review recruitment" }),
  );
  expect(await screen.findByText("Build catalogue")).toBeTruthy();
  expect(
    screen
      .getByRole("button", { name: "Submit application" })
      .matches(":disabled"),
  ).toBe(true);
  await userEvent.type(
    screen.getByLabelText("Declared capability references"),
    "declared-catalogue",
  );
  await userEvent.click(screen.getByLabelText(/I reviewed this role/));
  await userEvent.click(
    screen.getByRole("button", { name: "Submit application" }),
  );
  expect(
    await screen.findByText(/Application submitted.*not admitted/),
  ).toBeTruthy();
  const [path, init] = fetch.mock.calls[2] as [string, RequestInit];
  expect(path).toBe(`/v1/owner/agents/${agent}/swarms/${room}/applications`);
  expect(new Headers(init.headers).get("If-Match")).toBe('"7"');
  expect(JSON.parse(init.body as string)).toEqual({
    role_key: "builder",
    capability_refs: ["declared-catalogue"],
  });
  await userEvent.click(
    screen.getByRole("button", { name: "Load my applications" }),
  );
  expect(await screen.findByText(/Application state: applied/)).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: "Accept invitation" }),
  ).toBeNull();
  expect(document.querySelector("script")).toBeNull();
});
test.each([false, true])(
  "coordinator offer and uncertain retry (%s) keep reviewed versions and separate admission",
  async (retry) => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(
        response({ items: [application], next_after: "" }),
      );
    if (retry) fetch.mockRejectedValueOnce(new Error("connection lost"));
    fetch.mockResolvedValueOnce(
      response(
        {
          id,
          state: "offered",
          version: 2,
          etag: '"2"',
          history_policy: "full",
          respond_by: "2030-01-01T00:00:00Z",
        },
        201,
      ),
    );
    setup(fetch, true);
    await userEvent.click(
      screen.getByRole("button", { name: "Load room applications" }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Review application" }),
    );
    expect(await screen.findByText("declared-catalogue")).toBeTruthy();
    await userEvent.click(screen.getByLabelText(/I reviewed these claims/));
    await userEvent.click(
      screen.getByRole("button", { name: "Offer application" }),
    );
    if (retry) {
      await screen.findByRole("alert");
      await userEvent.click(
        screen.getByRole("button", { name: "Offer application" }),
      );
    }
    expect(
      await screen.findByText(/Application offered.*separately accept/),
    ).toBeTruthy();
    const [path, init] = fetch.mock.calls[1] as [string, RequestInit];
    expect(path).toBe(
      `/v1/owner/agents/${agent}/swarms/${room}/applications/${id}/offer`,
    );
    expect(new Headers(init.headers).get("If-Match")).toBe('"7"');
    expect(JSON.parse(init.body as string)).toMatchObject({
      application_version: 1,
      role: "contributor",
      history_policy: "full",
    });
    expect(JSON.parse(init.body as string).agent_id).toBeUndefined();
    if (retry) {
      const second = fetch.mock.calls[2][1] as RequestInit;
      expect(init.body).toBe(second.body);
      expect(new Headers(init.headers).get("Idempotency-Key")).toBe(
        new Headers(second.headers).get("Idempotency-Key"),
      );
    }
  },
);

test("empty stale recruitment window advances to current evidence", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      response({ items: [], next_after: id, candidate_limit: 50 }),
    )
    .mockResolvedValueOnce(
      response({ items: [recruitment], next_after: "", candidate_limit: 50 }),
    );
  setup(fetch);
  await userEvent.click(
    screen.getByRole("button", { name: "Find recruitment" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "More recruitment" }),
  );
  await screen.findByRole("button", { name: "Review recruitment" });
  expect(fetch.mock.calls[1][0]).toBe(
    `/v1/owner/agents/${agent}/recruitment?after=${id}`,
  );
});
test("late response is discarded after agent change", async () => {
  let resolve!: (x: Response) => void;
  const fetch = vi.fn(
    () =>
      new Promise<Response>((r) => {
        resolve = r;
      }),
  );
  const view = setup(fetch);
  await userEvent.click(
    screen.getByRole("button", { name: "Find recruitment" }),
  );
  view.rerender(
    <OwnerApplications agent={room} csrf="csrf" onUnauthorized={vi.fn()} />,
  );
  resolve(
    response({ items: [recruitment], next_after: "", candidate_limit: 50 }),
  );
  await new Promise((r) => setTimeout(r, 0));
  expect(
    screen.queryByRole("button", { name: "Review recruitment" }),
  ).toBeNull();
});
test("stale offer clears reviewed authority and requests fresh room", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(response({ items: [application], next_after: "" }))
    .mockResolvedValueOnce(
      response({ error: { code: "application_version_conflict" } }, 409),
    );
  vi.stubGlobal("fetch", fetch);
  const stale = vi.fn();
  render(
    <OwnerApplications
      agent={agent}
      csrf="csrf"
      room={room}
      roomEtag={'"7"'}
      onUnauthorized={vi.fn()}
      onStale={stale}
    />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Load room applications" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Review application" }),
  );
  await userEvent.click(screen.getByLabelText(/I reviewed these claims/));
  await userEvent.click(
    screen.getByRole("button", { name: "Offer application" }),
  );
  await screen.findByRole("alert");
  expect(stale).toHaveBeenCalledTimes(1);
  expect(
    screen.queryByRole("button", { name: "Offer application" }),
  ).toBeNull();
});
test.each([
  { name: "wrong room", packet: { ...application, swarm_id: agent } },
  { name: "string version", packet: { ...application, version: "1" } },
  { name: "transcript leak", packet: { ...application, transcript: [] } },
  {
    name: "fabricated independence",
    packet: { ...application, identity_assurance: "independent" },
  },
  {
    name: "contradictory claims",
    packet: { ...application, claims_state: "historical_unknown" },
  },
])("rejects malformed packet: $name", async ({ packet }) => {
  setup(
    vi
      .fn()
      .mockResolvedValueOnce(response({ items: [packet], next_after: "" })),
    true,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Load room applications" }),
  );
  await screen.findByRole("alert");
  expect(
    screen.queryByRole("button", { name: "Review application" }),
  ).toBeNull();
});
test.each([
  {
    name: "historical_unknown",
    packet: {
      ...application,
      claims_state: "historical_unknown",
      application_claims: null,
      application_claims_sha256: null,
    },
  },
  {
    name: "retained_over_budget",
    packet: {
      ...application,
      claims_state: "retained_over_budget",
      application_claims: null,
    },
  },
])(
  "honest $name projection invents no capabilities",
  async ({ name, packet }) => {
    setup(
      vi
        .fn()
        .mockResolvedValueOnce(response({ items: [packet], next_after: "" })),
      true,
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Load room applications" }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Review application" }),
    );
    expect(screen.getByText(new RegExp(`Claims: ${name}`))).toBeTruthy();
    expect(screen.queryByText("declared-catalogue")).toBeNull();
  },
);
test("expired application cannot be offered", async () => {
  setup(
    vi
      .fn()
      .mockResolvedValueOnce(
        response({
          items: [{ ...application, respond_by: "2020-01-01T00:00:00Z" }],
          next_after: "",
        }),
      ),
    true,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Load room applications" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Review application" }),
  );
  await userEvent.click(screen.getByLabelText(/I reviewed these claims/));
  expect(
    screen
      .getByRole("button", { name: "Offer application" })
      .matches(":disabled"),
  ).toBe(true);
});
