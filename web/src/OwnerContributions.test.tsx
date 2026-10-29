import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { OwnerContributions } from "./OwnerContributions";
const agent = "a8391ea6-5003-401e-85e1-47c116fe3da0",
  artifact = "b8391ea6-5003-401e-85e1-47c116fe3da0",
  plan = "c8391ea6-5003-401e-85e1-47c116fe3da0",
  taskID = "d8391ea6-5003-401e-85e1-47c116fe3da0",
  room = "e8391ea6-5003-401e-85e1-47c116fe3da0",
  principal = "f8391ea6-5003-401e-85e1-47c116fe3da0";
const hash = "a".repeat(64),
  data =
    '{"sources":[{"source_id":"owner","source_url":"https://example.org/"}]}';
const task = {
  id: taskID,
  plan_id: plan,
  assigned_agent_id: agent,
  state: "accepted",
  version: 2,
  etag: '"2"',
  claim_epoch: 1,
  response_deadline: "2030-01-01T00:00:00Z",
  progress_deadline: "2030-01-02T00:00:00Z",
  cure_deadline: null,
  submitted_artifact_id: null,
  credited_progress_evidence_id: null,
  dependency_blocked: false,
  specification: {
    key: "catalog",
    assignee_agent_id: agent,
    depends_on: [],
    output_schema: "source_catalog_v1",
    checks: ["json_schema_v1"],
    progress_interval_seconds: 86400,
  },
};
const planReview = {
  id: plan,
  swarm_id: room,
  version: 1,
  state: "accepted",
  content_hash: hash,
  canonical_json: "{}",
  specification: {
    objective: "Owner catalog",
    respond_by: "2030-01-01T00:00:00Z",
    required_principals: [principal, agent],
    acceptance_principals: [principal, agent],
    publication_principals: [principal],
    rights: {
      template_id: "approved-v1",
      license_id: "CC0-1.0",
      publication: "exact_manifest_approval",
    },
    tasks: [task.specification],
  },
  tasks: [task],
  contribution_template_hash: hash,
  terms_url: "",
};
const upload = {
  id: artifact,
  generation: room,
  state: "quarantine",
  purpose: "output",
  upload_authentication: "owner_session",
  upload_url: `${window.location.origin}/v1/owner/agents/${agent}/artifacts/${artifact}/upload?token=${"a".repeat(64)}`,
  expires_at: "2030-01-01T00:00:00Z",
  size_bytes: data.length,
  media_type: "application/json",
};
const metadata = {
  id: artifact,
  state: "quarantine",
  sha256: "",
  manifest_hash: "",
  size_bytes: data.length,
  media_type: "application/json",
  license_id: "",
  limitations: "Fixed structural checks only",
  content_trust: "untrusted",
  swarm_id: room,
  plan_id: plan,
  task_id: taskID,
  purpose: "output",
  claim_epoch: 1,
  agreement_hash: "",
  submission_attestation:
    "authenticated_platform_submission_not_agent_signature_or_legal_ownership_proof",
};
function response(x: unknown, status = 200) {
  return new Response(status === 204 ? null : JSON.stringify(x), { status });
}
function setup(
  options: {
    lostFinalize?: boolean;
    badUpload?: boolean;
    draft?: boolean;
    sealed?: boolean;
    denied?: number;
  } = {},
) {
  let lost = !!options.lostFinalize;
  let purposeFields: Record<string, unknown> = {};
  const fetch = vi.fn(async (path: string, init?: RequestInit) => {
    if (path === "/v1/owner/me") return response({ id: principal });
    if (path.endsWith(`/plans/${plan}`)) return response(planReview);
    if (path.endsWith("/assignments")) return response({ items: [task] });
    if (path.includes("/tasks/") && path.endsWith("/artifacts"))
      return response({
        items: options.sealed
          ? [
              {
                ...metadata,
                state: "verified",
                sha256: hash,
                manifest_hash: hash,
                agreement_hash: hash,
                license_id: "CC0-1.0",
                manifest: {
                  title: "Saved catalog",
                  schema_family: "source_catalog_v1",
                  template_id: "approved-v1",
                  license_id: "CC0-1.0",
                  attribution: [
                    { principal_id: principal, text: "Declared data" },
                  ],
                  inputs: [],
                  source_declaration: "Controlled fixture",
                },
              },
            ]
          : options.draft
            ? [metadata]
            : [],
        next_after_id: "",
      });
    if (path.endsWith("/evaluations"))
      return response({
        items: [
          {
            id: room,
            result: "passed",
            code: "fixed_checks_passed",
            input_sha256: hash,
            manifest_sha256: hash,
            spec_sha256: hash,
            image_sha256: hash,
            verifier_sha256: hash,
            runtime_isolation: "development_process_not_qualified",
            bubblewrap_sha256: null,
            prlimit_sha256: null,
            resource_limits_evidence: "development_not_qualified",
            image_identity_evidence: "development_not_qualified",
            checked_at: "2030-01-01T00:00:00Z",
            source_count: 1,
            limitations: "Fixed structural checks only",
          },
        ],
      });
    if (path.endsWith("/artifact-uploads")) {
      const input = JSON.parse(init?.body as string);
      purposeFields =
        input.purpose === "progress"
          ? {
              purpose: "progress",
              audience_principal_ids: [...input.audience_principal_ids].sort(),
              reviewer_principal_id: input.reviewer_principal_id,
            }
          : {};
      return options.denied
        ? response({ error: { code: "denied" } }, options.denied)
        : response(
            {
              ...upload,
              ...purposeFields,
              ...(options.badUpload
                ? { upload_url: "https://evil.invalid/upload" }
                : {}),
            },
            201,
          );
    }
    if (path.includes("/upload?")) return response(null, 204);
    if (path.endsWith("/finalize")) {
      if (lost) {
        lost = false;
        throw Error("committed response lost");
      }
      const body = JSON.parse(init?.body as string);
      return response({
        ...metadata,
        ...purposeFields,
        state: "sealed",
        sha256: body.sha256,
        manifest_hash: hash,
        agreement_hash: hash,
        license_id: "CC0-1.0",
        manifest: body.manifest,
      });
    }
    throw Error("Unexpected request " + path);
  });
  vi.stubGlobal("fetch", fetch);
  // The unit oracle fixes digest bytes; actual byte hashing is proven in Chromium.
  vi.stubGlobal("crypto", {
    randomUUID: () => globalThis.Math.random().toString(),
    subtle: { digest: async () => new Uint8Array(32).fill(0xab).buffer },
  });
  const props = {
    agent,
    plan,
    hash,
    taskID,
    csrf: "contribution-csrf",
    onUnauthorized: vi.fn(),
  };
  return { fetch, props, ...render(<OwnerContributions {...props} />) };
}
async function review() {
  await userEvent.click(
    screen.getByRole("button", { name: "Review task contributions" }),
  );
  await screen.findByLabelText("JSON dataset file");
}
async function prepare() {
  await review();
  await userEvent.upload(
    screen.getByLabelText("JSON dataset file"),
    new File([data], "catalog.json", { type: "application/json" }),
  );
  await screen.findByText(/Dataset SHA256: ababab/);
  await userEvent.click(screen.getByLabelText(/I reviewed the exact dataset/));
}
test("reviewed owner file follows quarantine raw bytes and immutable finalization without acceptance consent", async () => {
  const { fetch } = setup();
  await prepare();
  await userEvent.click(
    screen.getByRole("button", { name: "Upload and seal contribution" }),
  );
  await screen.findByText(/Contribution sealed.*verification is pending/i);
  const calls = fetch.mock.calls as unknown as [string, RequestInit][];
  const create = calls.find(([p]) => p.endsWith("/artifact-uploads"))!;
  expect(JSON.parse(create[1].body as string)).toEqual({
    claim_epoch: 1,
    media_type: "application/json",
    size_bytes: data.length,
    purpose: "output",
  });
  expect(new Headers(create[1].headers).get("If-Match")).toBe('"2"');
  const bytes = calls.find(([p]) => p.includes("/upload?"))!;
  expect(new TextDecoder().decode(bytes[1].body as Uint8Array)).toBe(data);
  const seal = calls.find(([p]) => p.endsWith("/finalize"))!;
  const body = JSON.parse(seal[1].body as string);
  expect(body.plan_hash).toBe(hash);
  expect(body.claim_epoch).toBe(1);
  expect(body.manifest.template_id).toBe("approved-v1");
  expect(body.manifest.attribution[0].principal_id).toBe(principal);
  expect(
    calls.some(([p]) => /acceptances|publication|reuse-receipts/.test(p)),
  ).toBe(false);
});
test("lost finalization response retries exact body and command key without uploading sealed bytes again", async () => {
  const { fetch } = setup({ lostFinalize: true });
  await prepare();
  await userEvent.click(
    screen.getByRole("button", { name: "Upload and seal contribution" }),
  );
  await screen.findByRole("alert");
  await userEvent.click(
    screen.getByRole("button", { name: "Retry exact contribution" }),
  );
  await screen.findByText(/Contribution sealed/i);
  const calls = fetch.mock.calls as unknown as [string, RequestInit][];
  const seals = calls.filter(([p]) => p.endsWith("/finalize"));
  expect(seals).toHaveLength(2);
  expect(seals[0][1].body).toBe(seals[1][1].body);
  expect(new Headers(seals[0][1].headers).get("Idempotency-Key")).toBe(
    new Headers(seals[1][1].headers).get("Idempotency-Key"),
  );
  expect(calls.filter(([p]) => p.includes("/upload?")).length).toBe(1);
});
test("misdirected upload receipt never receives bytes or CSRF", async () => {
  const { fetch } = setup({ badUpload: true });
  await prepare();
  await userEvent.click(
    screen.getByRole("button", { name: "Upload and seal contribution" }),
  );
  await screen.findByRole("alert");
  expect(
    fetch.mock.calls.some(
      ([p]) => p.includes("evil.invalid") || p.includes("/upload?"),
    ),
  ).toBe(false);
});
test("returning owner finalizes existing observed bytes after explicit draft selection and file review", async () => {
  const { fetch } = setup({ draft: true });
  await review();
  await userEvent.click(
    screen.getByRole("button", {
      name: `Review pending contribution ${artifact}`,
    }),
  );
  await userEvent.upload(
    screen.getByLabelText("JSON dataset file"),
    new File([data], "catalog.json", { type: "application/json" }),
  );
  await screen.findByText(/Dataset SHA256: ababab/);
  await userEvent.click(screen.getByLabelText(/I reviewed the exact dataset/));
  await userEvent.click(
    screen.getByRole("button", { name: "Seal existing uploaded bytes" }),
  );
  await screen.findByText(/Contribution sealed/i);
  expect(
    fetch.mock.calls.some(
      ([p]) => p.includes("/upload?") || p.endsWith("/artifact-uploads"),
    ),
  ).toBe(false);
});
test.each([401, 403, 404, 409])(
  "denied contribution %s removes stale authority and local draft",
  async (status) => {
    setup({ denied: status });
    await prepare();
    await userEvent.click(
      screen.getByRole("button", { name: "Upload and seal contribution" }),
    );
    await screen.findByRole("alert");
    expect(screen.queryByLabelText("JSON dataset file")).toBeNull();
  },
);
test("agent or session switch discards late file/contribution state", async () => {
  const { rerender, props } = setup();
  await prepare();
  rerender(
    <OwnerContributions {...props} agent={room} csrf="another-session" />,
  );
  await waitFor(() => expect(screen.queryByText(/Dataset SHA256/)).toBeNull());
  expect(screen.queryByLabelText("JSON dataset file")).toBeNull();
});

test("returning owner can inspect bound fixed evidence with qualification limits visible", async () => {
  setup({ sealed: true });
  await review();
  await userEvent.click(
    screen.getByRole("button", { name: `Inspect fixed evidence ${artifact}` }),
  );
  expect(
    await screen.findByText(/development_process_not_qualified/),
  ).toBeTruthy();
  expect(
    screen.getByText(/Fixed evaluation is structural evidence only/),
  ).toBeTruthy();
});

test("progress review sends normalized private audience and preserves the designated reviewer", async () => {
  const { fetch } = setup();
  await review();
  await userEvent.selectOptions(
    screen.getByLabelText("Contribution purpose"),
    "progress",
  );
  await userEvent.click(screen.getByLabelText(agent));
  await userEvent.selectOptions(
    screen.getByLabelText("Designated progress reviewer"),
    agent,
  );
  await userEvent.upload(
    screen.getByLabelText("JSON dataset file"),
    new File([data], "progress.json", { type: "application/json" }),
  );
  await screen.findByText(/Dataset SHA256: ababab/);
  await userEvent.click(screen.getByLabelText(/I reviewed the exact dataset/));
  await userEvent.click(
    screen.getByRole("button", { name: "Upload and seal contribution" }),
  );
  await screen.findByText(/Contribution sealed/);
  const call = fetch.mock.calls.find(([p]) => p.endsWith("/artifact-uploads"))!;
  expect(JSON.parse((call[1] as RequestInit).body as string)).toMatchObject({
    purpose: "progress",
    audience_principal_ids: [agent, principal],
    reviewer_principal_id: agent,
  });
});

test("contribution selectors have stable accessible names independent of option text", async () => {
  setup();
  await review();
  await userEvent.selectOptions(
    screen.getByRole("combobox", { name: /^Contribution purpose$/ }),
    "progress",
  );
  expect(
    screen.getByRole("combobox", { name: /^Designated progress reviewer$/ }),
  ).toBeTruthy();
});
