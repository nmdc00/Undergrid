import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { test, expect, vi } from "vitest";
import { CapabilityDiscovery } from "./CapabilityDiscovery";
const source = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const agent = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
function page() {
  return {
    source_intent_id: source,
    source_version: 1,
    source_revision: 1,
    identity_assurance: "common_control_unknown",
    agents: {
      limit: 20,
      truncated: false,
      items: [
        {
          agent_id: agent,
          principal_id: "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
          origin: "registered_capability",
          assurance: "self_declared",
          capabilities: ["source_catalog_v1"],
          schema_support: "declared",
          hard_status: "unknown",
          constraint_evidence: [
            {
              constraint: "output_kind",
              requested: ["json_dataset"],
              declared: [],
              status: "unknown",
              reason: "declaration_missing",
              evidence_field: "",
              evidence_class: "self_declared_agent_profile",
            },
            {
              constraint: "schema_family",
              requested: ["source_catalog_v1"],
              declared: ["source_catalog_v1"],
              status: "declared_aligned",
              reason: "declared_output_equal",
              evidence_field: "profile.capabilities",
              evidence_class: "self_declared_agent_profile",
            },
            {
              constraint: "license_ids",
              requested: ["CC0-1.0"],
              declared: [],
              status: "unknown",
              reason: "license_evidence_missing",
              evidence_field: "",
              evidence_class: "self_declared_agent_profile",
            },
          ],
        },
      ],
    },
    artifacts: { limit: 20, truncated: false, items: [] },
  };
}
function mount(csrf = "session", expired = vi.fn()) {
  return render(
    <CapabilityDiscovery
      intentionID={source}
      sourceVersion={1}
      csrf={csrf}
      onUnauthorized={expired}
    />,
  );
}
function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status });
}
test("explicit owner read shows declared schema and unknown hard fit without bearer or execution", async () => {
  const fetch = vi.fn(async (_path: string, _init?: RequestInit) =>
    response(page()),
  );
  vi.stubGlobal("fetch", fetch);
  mount();
  expect(fetch).not.toHaveBeenCalled();
  await userEvent.click(
    screen.getByRole("button", { name: "Discover registered capabilities" }),
  );
  await screen.findByText(agent);
  expect(screen.getByText("Declared schema support")).toBeTruthy();
  expect(screen.getAllByText(/Unknown/).length).toBeGreaterThan(0);
  expect(fetch.mock.calls[0][0]).toBe(
    `/v1/owner/intents/${source}/capability-fit`,
  );
  expect(fetch.mock.calls[0][1]?.credentials).toBe("same-origin");
  expect(
    new Headers(fetch.mock.calls[0][1]?.headers).get("Authorization"),
  ).toBeNull();
  expect(document.querySelector("a")).toBeNull();
});
test.each([
  [
    "foreign source",
    (v: ReturnType<typeof page>): unknown => (v.source_intent_id = agent),
  ],
  [
    "stale version",
    (v: ReturnType<typeof page>): unknown => (v.source_version = 2),
  ],
  [
    "false independence",
    (v: ReturnType<typeof page>): unknown =>
      (v.identity_assurance = "verified"),
  ],
  [
    "private routing",
    (v: ReturnType<typeof page>): unknown =>
      Object.assign(v.agents.items[0], { endpoint: "https://private.invalid" }),
  ],
  [
    "false competence",
    (v: ReturnType<typeof page>): unknown =>
      (v.agents.items[0].assurance = "verified"),
  ],
  [
    "false fit",
    (v: ReturnType<typeof page>): unknown =>
      (v.agents.items[0].hard_status = "aligned"),
  ],
  [
    "wrong evidence class",
    (v: ReturnType<typeof page>): unknown =>
      (v.agents.items[0].constraint_evidence[0].evidence_class =
        "published_manifest_declaration"),
  ],
  [
    "false bound",
    (v: ReturnType<typeof page>): unknown => (v.agents.limit = 21),
  ],
  [
    "duplicate principal",
    (v: ReturnType<typeof page>): unknown =>
      v.agents.items.push({
        ...v.agents.items[0],
        agent_id: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
      }),
  ],
  [
    "invalid truncation",
    (v: ReturnType<typeof page>): unknown => (v.artifacts.truncated = true),
  ],
] as const)("rejects %s", async (_label, mutate) => {
  const value = page();
  mutate(value);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => response(value)),
  );
  mount();
  await userEvent.click(
    screen.getByRole("button", { name: "Discover registered capabilities" }),
  );
  await screen.findByRole("alert");
  expect(screen.queryByText(agent)).toBeNull();
});
test.each([
  ["session", 200],
  ["session", 401],
  ["version", 200],
  ["version", 401],
  ["selection", 200],
  ["selection", 401],
] as const)("changed %s ignores late %s response", async (change, status) => {
  let finish!: (r: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((r) => {
          finish = r;
        }),
    ),
  );
  const expired = vi.fn();
  const view = mount("old", expired);
  await userEvent.click(
    screen.getByRole("button", { name: "Discover registered capabilities" }),
  );
  view.rerender(
    <CapabilityDiscovery
      intentionID={change === "selection" ? agent : source}
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
        .getByRole("button", { name: "Discover registered capabilities" })
        .matches(":disabled"),
    ).toBe(false),
  );
  expect(screen.queryByText(agent)).toBeNull();
  expect(expired).not.toHaveBeenCalled();
});
test("current expiry signs out", async () => {
  const expired = vi.fn();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => response({ error: { code: "expired" } }, 401)),
  );
  mount("session", expired);
  await userEvent.click(
    screen.getByRole("button", { name: "Discover registered capabilities" }),
  );
  await waitFor(() => expect(expired).toHaveBeenCalledTimes(1));
});
test("public capability strings remain inert", async () => {
  const value = page();
  value.agents.items[0].capabilities.push("<script>steal()</script>");
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => response(value)),
  );
  mount();
  await userEvent.click(
    screen.getByRole("button", { name: "Discover registered capabilities" }),
  );
  await screen.findByText(/<script>steal/);
  expect(document.querySelector("script")).toBeNull();
});

function artifactPage() {
  const value = page();
  const facts = value.agents.items[0].constraint_evidence.map((fact) => ({
    ...fact,
    evidence_class: "published_manifest_declaration",
    evidence_field:
      fact.constraint === "output_kind"
        ? "artifact.media_type"
        : fact.constraint === "schema_family"
          ? "manifest.schema_family"
          : "manifest.license_id",
    declared: fact.requested,
    status: "declared_aligned",
    reason:
      fact.constraint === "license_ids"
        ? "declared_license_option_intersection"
        : "declared_output_equal",
  }));
  facts.push({
    constraint: "required_field",
    requested: ["source_id"],
    declared: [],
    status: "unknown",
    reason: "field_set_missing",
    evidence_field: "",
    evidence_class: "published_manifest_declaration",
  });
  return {
    ...value,
    artifacts: {
      limit: 20,
      truncated: false,
      items: [
        {
          artifact_id: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
          principal_id: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
          version: 1,
          sha256: "a".repeat(64),
          manifest_hash: "b".repeat(64),
          title: "Published catalogue",
          schema_family: "source_catalog_v1",
          license_id: "CC0-1.0",
          origin: "registered_capability",
          assurance: "published_manifest_declaration",
          content_trust: "untrusted",
          limitations: "Fixed structural checks only",
          hard_status: "unknown",
          constraint_evidence: facts,
        },
      ],
    },
  };
}
test("renders immutable artifact hashes and unknown required fields", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => response(artifactPage())),
  );
  mount();
  await userEvent.click(
    screen.getByRole("button", { name: "Discover registered capabilities" }),
  );
  await screen.findByText("Published catalogue");
  expect(screen.getByText(`Content hash: ${"a".repeat(64)}`)).toBeTruthy();
  expect(screen.getByText(`Manifest hash: ${"b".repeat(64)}`)).toBeTruthy();
  expect(screen.getAllByText(/Unknown requirements remain/).length).toBe(2);
});
test.each([
  [
    "private artifact context",
    (v: ReturnType<typeof artifactPage>): unknown =>
      Object.assign(v.artifacts.items[0], { swarm_id: source }),
  ],
  [
    "invalid immutable hash",
    (v: ReturnType<typeof artifactPage>): unknown =>
      (v.artifacts.items[0].sha256 = "unbound"),
  ],
  [
    "false artifact verification",
    (v: ReturnType<typeof artifactPage>): unknown =>
      (v.artifacts.items[0].assurance = "verified"),
  ],
  [
    "incoherent hard fit",
    (v: ReturnType<typeof artifactPage>): unknown =>
      (v.artifacts.items[0].hard_status = "aligned"),
  ],
] as const)("rejects %s", async (_label, mutate) => {
  const value = artifactPage();
  mutate(value);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => response(value)),
  );
  mount();
  await userEvent.click(
    screen.getByRole("button", { name: "Discover registered capabilities" }),
  );
  await screen.findByRole("alert");
  expect(screen.queryByText("Published catalogue")).toBeNull();
});
test("refresh clears prior artifacts and authority failure leaves no old view", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(response(artifactPage()))
    .mockResolvedValueOnce(response({ error: { code: "not_found" } }, 404));
  vi.stubGlobal("fetch", fetch);
  mount();
  await userEvent.click(
    screen.getByRole("button", { name: "Discover registered capabilities" }),
  );
  await screen.findByText("Published catalogue");
  await userEvent.click(
    screen.getByRole("button", { name: "Refresh registered capabilities" }),
  );
  await screen.findByRole("alert");
  expect(screen.queryByText("Published catalogue")).toBeNull();
  expect(screen.queryByText(agent)).toBeNull();
});
