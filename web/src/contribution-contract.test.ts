import { expect, test, vi } from "vitest";
import {
  contributionManifest,
  ownerUpload,
  taskArtifactPage,
  putOwnerContribution,
  contributionEvaluations,
} from "./contribution-contract";
const agent = "a8391ea6-5003-401e-85e1-47c116fe3da0",
  artifact = "b8391ea6-5003-401e-85e1-47c116fe3da0",
  plan = "c8391ea6-5003-401e-85e1-47c116fe3da0",
  task = "d8391ea6-5003-401e-85e1-47c116fe3da0",
  room = "e8391ea6-5003-401e-85e1-47c116fe3da0";
const context = { agent, plan, task };
const upload = {
  id: artifact,
  generation: room,
  state: "quarantine",
  purpose: "output",
  upload_authentication: "owner_session",
  upload_url: `${window.location.origin}/v1/owner/agents/${agent}/artifacts/${artifact}/upload?token=${"a".repeat(64)}`,
  expires_at: "2030-01-01T00:00:00Z",
  size_bytes: 12,
  media_type: "application/json",
};
const manifest = {
  title: "Reviewed data",
  schema_family: "source_catalog_v1",
  template_id: "approved-v1",
  license_id: "CC0-1.0",
  attribution: [{ principal_id: agent, text: "Declared contribution" }],
  inputs: [],
  source_declaration: "Controlled data; references are not fetched",
};
const metadata = {
  id: artifact,
  state: "quarantine",
  sha256: "",
  manifest_hash: "",
  size_bytes: 12,
  media_type: "application/json",
  license_id: "",
  limitations: "Fixed structural checks only",
  content_trust: "untrusted",
  swarm_id: room,
  plan_id: plan,
  task_id: task,
  purpose: "output",
  claim_epoch: 1,
  agreement_hash: "",
  submission_attestation:
    "authenticated_platform_submission_not_agent_signature_or_legal_ownership_proof",
};
test("owner quarantine and manifest retain finite reviewed contracts", () => {
  expect(ownerUpload(upload, agent).id).toBe(artifact);
  expect(contributionManifest(manifest).inputs).toEqual([]);
  expect(
    taskArtifactPage({ items: [metadata], next_after_id: "" }, context).items[0]
      .task_id,
  ).toBe(task);
});
test.each([
  (x: Record<string, unknown>) => (x.upload_authentication = "credential"),
  (x: Record<string, unknown>) => (x.token = "secret"),
  (x: Record<string, unknown>) => (x.size_bytes = 0),
  (x: Record<string, unknown>) => (x.purpose = "accepted_output"),
  (x: Record<string, unknown>) => (x.expires_at = "tomorrow"),
  (x: Record<string, unknown>) =>
    (x.upload_url = `https://elsewhere.invalid/v1/owner/agents/${agent}/artifacts/${artifact}/upload?token=${"a".repeat(64)}`),
  (x: Record<string, unknown>) =>
    (x.upload_url = upload.upload_url + "&redirect=/v1/agents"),
  (x: Record<string, unknown>) =>
    (x.upload_url = upload.upload_url.replace(agent, room)),
  (x: Record<string, unknown>) =>
    (x.upload_url = upload.upload_url + "#fragment"),
  (x: Record<string, unknown>) =>
    (x.upload_url = upload.upload_url.replace(
      "?token=",
      "?token=short&token=",
    )),
])("rejects unusable or misdirected upload receipt %s", (mutate) => {
  const x = { ...upload };
  mutate(x);
  expect(() => ownerUpload(x, agent)).toThrow();
});
test("private progress requires exact finite audience and designated reviewer", () => {
  const progress = {
    ...upload,
    purpose: "progress",
    audience_principal_ids: [agent],
    reviewer_principal_id: agent,
  };
  expect(ownerUpload(progress, agent).purpose).toBe("progress");
  for (const change of [
    { audience_principal_ids: [] },
    { reviewer_principal_id: room },
    { audience_principal_ids: [agent, agent] },
  ])
    expect(() => ownerUpload({ ...progress, ...change }, agent)).toThrow();
  expect(() =>
    ownerUpload({ ...upload, audience_principal_ids: [agent] }, agent),
  ).toThrow();
});
test("lineage, attribution and response context cannot silently change", () => {
  for (const change of [
    {
      inputs: [
        { artifact_id: artifact, sha256: "unknown", license_id: "CC0-1.0" },
      ],
    },
    { execute: "shell" },
    { attribution: [] },
    { license_id: "unknown" },
  ])
    expect(() => contributionManifest({ ...manifest, ...change })).toThrow();
  for (const change of [
    { task_id: room },
    { plan_id: room },
    { claim_epoch: "1" },
    { upload_url: upload.upload_url },
    { content_trust: "trusted" },
  ])
    expect(() =>
      taskArtifactPage(
        { items: [{ ...metadata, ...change }], next_after_id: "" },
        context,
      ),
    ).toThrow();
  expect(() =>
    taskArtifactPage(
      { items: Array(101).fill(metadata), next_after_id: "" },
      context,
    ),
  ).toThrow();
  expect(() =>
    taskArtifactPage(
      { items: [metadata, metadata], next_after_id: "" },
      context,
    ),
  ).toThrow();
});
test("raw byte transfer validates destination before attaching cookie/CSRF and sends exact bytes", async () => {
  const fetch = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
  vi.stubGlobal("fetch", fetch);
  const bytes = new TextEncoder().encode("123456789012");
  await putOwnerContribution(
    ownerUpload(upload, agent),
    agent,
    "csrf-only",
    bytes,
  );
  const [path, init] = fetch.mock.calls[0] as [string, RequestInit];
  expect(path).toBe(
    new URL(upload.upload_url).pathname + new URL(upload.upload_url).search,
  );
  expect(init.body).toBe(bytes);
  expect(init.credentials).toBe("same-origin");
  expect(init.redirect).toBe("error");
  expect(new Headers(init.headers).get("X-CSRF-Token")).toBe("csrf-only");
  expect(new Headers(init.headers).get("Authorization")).toBeNull();
  await expect(
    putOwnerContribution(
      { ...ownerUpload(upload, agent), upload_url: "https://evil.invalid" },
      agent,
      "csrf-only",
      bytes,
    ),
  ).rejects.toThrow();
  expect(fetch).toHaveBeenCalledTimes(1);
});

const evidence = {
  id: room,
  result: "passed",
  code: "fixed_checks_passed",
  input_sha256: "a".repeat(64),
  manifest_sha256: "b".repeat(64),
  spec_sha256: "c".repeat(64),
  image_sha256: "d".repeat(64),
  verifier_sha256: "e".repeat(64),
  runtime_isolation: "development_process_not_qualified",
  bubblewrap_sha256: null,
  prlimit_sha256: null,
  resource_limits_evidence: "development_not_qualified",
  image_identity_evidence: "development_not_qualified",
  checked_at: "2030-01-01T00:00:00Z",
  source_count: 1,
  limitations: "Fixed structural checks only",
};
test("fixed evidence binds exact immutable bytes and manifest without implying host qualification", () => {
  const artifact = {
    ...metadata,
    state: "verified",
    sha256: evidence.input_sha256,
    manifest_hash: evidence.manifest_sha256,
    agreement_hash: "f".repeat(64),
    license_id: "CC0-1.0",
    manifest,
  };
  const parsed = taskArtifactPage(
    { items: [artifact], next_after_id: "" },
    context,
  ).items[0];
  expect(
    contributionEvaluations({ items: [evidence] }, parsed)[0].runtime_isolation,
  ).toBe("development_process_not_qualified");
  for (const change of [
    { input_sha256: "f".repeat(64) },
    { manifest_sha256: "f".repeat(64) },
    { legal_ownership: true },
    { source_count: "1" },
  ])
    expect(() =>
      contributionEvaluations({ items: [{ ...evidence, ...change }] }, parsed),
    ).toThrow();
});
