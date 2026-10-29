import { useEffect, useRef, useState } from "react";
import { ApiError, object, request, text } from "./api";
import { reviewedPlan, type Review } from "./plan-contract";
import { executionTask, type ExecutionTask } from "./task-execution-contract";
import {
  contributionEvaluations,
  contributionID,
  contributionManifest,
  maxContributionBytes,
  ownerUpload,
  putOwnerContribution,
  taskArtifact,
  taskArtifactPage,
  type ContributionManifest,
  type OwnerUpload,
  type TaskArtifact,
} from "./contribution-contract";
const writable = ["accepted", "in_progress", "changes_requested", "at_risk"];
type ReviewState = { task: ExecutionTask; plan: Review; principal: string };
type Prepared = { bytes: Uint8Array<ArrayBuffer>; sha: string };
type Command = {
  prepared: Prepared;
  manifest: ContributionManifest;
  purpose: "output" | "progress";
  audience: string[];
  reviewer: string;
  existing: TaskArtifact | null;
  upload: OwnerUpload | null;
  phase: "create" | "bytes" | "finalize";
};
function readFile(file: File): Promise<ArrayBuffer> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () =>
      reader.result instanceof ArrayBuffer
        ? resolve(reader.result)
        : reject(Error("Cannot read dataset bytes"));
    reader.onerror = () => reject(Error("Cannot read dataset file"));
    reader.onabort = () => reject(Error("Dataset reading cancelled"));
    reader.readAsArrayBuffer(file);
  });
}
export function OwnerContributions({
  agent,
  plan,
  hash,
  taskID,
  csrf,
  onUnauthorized,
}: {
  agent: string;
  plan: string;
  hash: string;
  taskID: string;
  csrf: string;
  onUnauthorized: () => void;
}) {
  const [review, setReview] = useState<ReviewState | null>(null),
    [page, setPage] = useState<{
      items: TaskArtifact[];
      next_after_id: string;
    } | null>(null),
    [prepared, setPrepared] = useState<Prepared | null>(null),
    [manifestText, setManifestText] = useState(""),
    [purpose, setPurpose] = useState<"output" | "progress">("output"),
    [audience, setAudience] = useState<string[]>([]),
    [reviewer, setReviewer] = useState(""),
    [existing, setExisting] = useState<TaskArtifact | null>(null),
    [ack, setAck] = useState(false),
    [busy, setBusy] = useState(false),
    [pending, setPending] = useState(false),
    [error, setError] = useState(""),
    [message, setMessage] = useState("");
  const [evaluations, setEvaluations] = useState<
    Record<string, ReturnType<typeof contributionEvaluations>>
  >({});
  const generation = useRef(0),
    fileGeneration = useRef(0),
    command = useRef<Command | null>(null);
  const context = { agent, plan, task: taskID },
    base = `/v1/owner/agents/${encodeURIComponent(agent)}`,
    taskBase = `${base}/tasks/${encodeURIComponent(taskID)}`;
  function clearDraft() {
    fileGeneration.current++;
    setPrepared(null);
    setExisting(null);
    setAck(false);
    setPending(false);
    command.current = null;
  }
  useEffect(() => {
    generation.current++;
    clearDraft();
    setReview(null);
    setPage(null);
    setEvaluations({});
    setManifestText("");
    setPurpose("output");
    setAudience([]);
    setReviewer("");
    setBusy(false);
    setError("");
    setMessage("");
    return () => {
      generation.current++;
      fileGeneration.current++;
    };
  }, [agent, plan, hash, taskID, csrf]);
  function failed(e: unknown, mine: number) {
    if (mine !== generation.current) return;
    if (e instanceof ApiError && [401, 403, 404, 409, 410].includes(e.status)) {
      clearDraft();
      setReview(null);
      setPage(null);
      setEvaluations({});
      if (e.status === 401) onUnauthorized();
    }
    setError(e instanceof Error ? e.message : "Contribution request failed");
  }
  async function load() {
    if (busy) return;
    const mine = ++generation.current;
    setBusy(true);
    clearDraft();
    setReview(null);
    setPage(null);
    setEvaluations({});
    setError("");
    setMessage("");
    try {
      const [rawPlan, rawTasks, rawOwner, rawPage] = await Promise.all([
        request(`${base}/plans/${encodeURIComponent(plan)}`, csrf),
        request(`${base}/plans/${encodeURIComponent(plan)}/assignments`, csrf),
        request("/v1/owner/me", csrf),
        request(`${taskBase}/artifacts`, csrf),
      ]);
      const current = reviewedPlan(rawPlan),
        principal = contributionID(rawOwner.id);
      if (
        Object.keys(rawOwner).length !== 1 ||
        current.id !== plan ||
        current.content_hash !== hash ||
        current.state !== "accepted" ||
        Object.keys(rawTasks).length !== 1 ||
        !Array.isArray(rawTasks.items) ||
        rawTasks.items.length > 50
      )
        throw Error(
          "Contribution plan context changed; review the current accepted plan.",
        );
      const tasks = rawTasks.items.map(executionTask);
      if (
        tasks.some(
          (t) => t.plan_id !== plan || t.assigned_agent_id !== agent,
        ) ||
        new Set(tasks.map((t) => t.id)).size !== tasks.length
      )
        throw Error("Invalid contribution assignment context");
      const task = tasks.find((t) => t.id === taskID);
      if (!task)
        throw Error("The selected task is not assigned to this agent.");
      const rights = object(current.specification.rights);
      if (
        rights.license_id !== "CC0-1.0" ||
        task.specification.output_schema !== "source_catalog_v1"
      )
        throw Error(
          "This task does not use the supported dataset and rights contract.",
        );
      const found = taskArtifactPage(rawPage, context);
      if (mine !== generation.current) return;
      setReview({ task, plan: current, principal });
      setPage(found);
      setPurpose("output");
      setAudience([principal]);
      setReviewer("");
      setManifestText(
        JSON.stringify(
          {
            title: "Dataset contribution",
            schema_family: "source_catalog_v1",
            template_id: text(rights.template_id),
            license_id: text(rights.license_id),
            attribution: [
              {
                principal_id: principal,
                text: "Owner-declared contribution; review before sealing.",
              },
            ],
            inputs: [],
            source_declaration:
              "Owner-supplied data; sources and rights are declarations, not independently verified.",
          },
          null,
          2,
        ),
      );
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  async function more() {
    if (busy || !page?.next_after_id) return;
    const mine = generation.current,
      after = page.next_after_id;
    setBusy(true);
    setError("");
    try {
      const found = taskArtifactPage(
        await request(
          `${taskBase}/artifacts?after_id=${encodeURIComponent(after)}`,
          csrf,
        ),
        context,
      );
      if (found.items.some((x) => x.id <= after))
        throw Error("Contribution continuation repeated entries");
      if (mine === generation.current) setPage(found);
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  async function inspect(artifact: TaskArtifact) {
    if (busy) return;
    const mine = generation.current;
    setBusy(true);
    setError("");
    try {
      const evidence = contributionEvaluations(
        await request(
          `${base}/artifacts/${encodeURIComponent(artifact.id)}/evaluations`,
          csrf,
        ),
        artifact,
      );
      if (mine === generation.current)
        setEvaluations((old) => ({ ...old, [artifact.id]: evidence }));
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  async function selectFile(file: File | undefined) {
    const mine = generation.current,
      fileMine = ++fileGeneration.current;
    setPrepared(null);
    setAck(false);
    setError("");
    setMessage("");
    if (!file) return;
    try {
      if (file.size < 1 || file.size > maxContributionBytes)
        throw Error("Select a JSON dataset between 1 byte and 10 MiB.");
      const buffer = await readFile(file),
        bytes = new Uint8Array(buffer);
      const decoded = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
      object(JSON.parse(decoded));
      const digest = await crypto.subtle.digest("SHA-256", buffer),
        sha = Array.from(new Uint8Array(digest), (x) =>
          x.toString(16).padStart(2, "0"),
        ).join("");
      if (mine === generation.current && fileMine === fileGeneration.current)
        setPrepared({ bytes, sha });
    } catch (e) {
      if (fileMine === fileGeneration.current) failed(e, mine);
    }
  }
  function selectExisting(artifact: TaskArtifact) {
    if (!review || busy || pending) return;
    clearDraft();
    setExisting(artifact);
    setPurpose(artifact.purpose);
    setAudience(artifact.audience_principal_ids ?? [review.principal]);
    setReviewer(artifact.reviewer_principal_id ?? "");
    setMessage("");
    setError("");
  }
  async function submit() {
    if (
      !review ||
      busy ||
      !ack ||
      !prepared ||
      review.task.dependency_blocked ||
      !writable.includes(review.task.state)
    )
      return;
    const mine = generation.current;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      if (!command.current) {
        if (
          purpose === "progress" &&
          (!Array.isArray(review.plan.specification.acceptance_principals) ||
            !review.plan.specification.acceptance_principals.includes(reviewer))
        )
          throw Error(
            "Choose a designated acceptance principal to review progress.",
          );
        const manifest = contributionManifest(JSON.parse(manifestText)),
          rights = object(review.plan.specification.rights);
        if (
          new TextEncoder().encode(JSON.stringify(manifest)).length > 65536 ||
          manifest.template_id !== rights.template_id ||
          manifest.license_id !== rights.license_id
        )
          throw Error(
            "Manifest differs from the reviewed plan or exceeds 64 KiB.",
          );
        if (
          purpose === "progress" &&
          (!audience.includes(review.principal) || !audience.includes(reviewer))
        )
          throw Error(
            "Select a private audience including yourself and the designated reviewer.",
          );
        if (
          existing &&
          (existing.claim_epoch !== review.task.claim_epoch ||
            existing.size_bytes !== prepared.bytes.byteLength ||
            existing.purpose !== purpose)
        )
          throw Error(
            "Pending contribution epoch, size or purpose differs; review another generation or explicitly start a new upload.",
          );
        command.current = {
          prepared,
          manifest,
          purpose,
          audience: [...audience].sort(),
          reviewer,
          existing,
          upload: null,
          phase: existing ? "finalize" : "create",
        };
        setPending(true);
      }
      const c = command.current;
      if (c.phase === "create") {
        const body = {
          claim_epoch: review.task.claim_epoch,
          media_type: "application/json",
          size_bytes: c.prepared.bytes.byteLength,
          purpose: c.purpose,
          ...(c.purpose === "progress"
            ? {
                audience_principal_ids: c.audience,
                reviewer_principal_id: c.reviewer,
              }
            : {}),
        };
        const validate = (raw: unknown) => {
          const value = ownerUpload(raw, agent);
          if (
            value.size_bytes !== c.prepared.bytes.byteLength ||
            value.purpose !== c.purpose ||
            (c.purpose === "progress" &&
              (JSON.stringify(value.audience_principal_ids) !==
                JSON.stringify(c.audience) ||
                value.reviewer_principal_id !== c.reviewer))
          )
            throw Error(
              "Contribution receipt changed reviewed size, purpose or audience",
            );
          return value;
        };
        const raw = await request(
          `${taskBase}/artifact-uploads`,
          csrf,
          "POST",
          body,
          review.task.etag,
          validate,
        );
        if (mine !== generation.current) return;
        c.upload = validate(raw);
        c.phase = "bytes";
      }
      if (c.phase === "bytes") {
        await putOwnerContribution(c.upload!, agent, csrf, c.prepared.bytes);
        if (mine !== generation.current) return;
        c.phase = "finalize";
      }
      const id = c.existing?.id ?? c.upload!.id;
      const body = {
        sha256: c.prepared.sha,
        plan_hash: hash,
        claim_epoch: review.task.claim_epoch,
        manifest: c.manifest,
      };
      let sealed: TaskArtifact | null = null;
      await request(
        `${base}/artifacts/${encodeURIComponent(id)}/finalize`,
        csrf,
        "POST",
        body,
        undefined,
        (raw) => {
          const value = taskArtifact(raw, context);
          if (
            value.id !== id ||
            value.state !== "sealed" ||
            value.sha256 !== c.prepared.sha ||
            value.size_bytes !== c.prepared.bytes.byteLength ||
            value.claim_epoch !== review.task.claim_epoch ||
            value.purpose !== c.purpose ||
            JSON.stringify(value.manifest) !== JSON.stringify(c.manifest) ||
            (c.purpose === "progress" &&
              (JSON.stringify(value.audience_principal_ids) !==
                JSON.stringify(c.audience) ||
                value.reviewer_principal_id !== c.reviewer))
          )
            throw Error(
              "Immutable finalization receipt differs from reviewed contribution",
            );
          sealed = value;
        },
      );
      if (mine !== generation.current) return;
      clearDraft();
      setPage({ items: sealed ? [sealed] : [], next_after_id: "" });
      setReview(null);
      setMessage(
        "Contribution sealed; verification is pending. No output acceptance or publication consent was recorded. Review task contributions again to inspect durable state.",
      );
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  const allowed =
    !!review &&
    writable.includes(review.task.state) &&
    !review.task.dependency_blocked;
  const candidates = review
    ? Array.from(
        new Set([
          review.principal,
          ...(Array.isArray(review.plan.specification.required_principals)
            ? review.plan.specification.required_principals.map(contributionID)
            : []),
        ]),
      )
    : [];
  return (
    <section aria-label={`Contributions for ${taskID}`}>
      <h6>Dataset contributions</h6>
      <p>
        Upload JSON data and declared lineage only. Content is untrusted and is
        never executed. Sealing and fixed verification do not accept output,
        approve publication or establish legal ownership.
      </p>
      <button disabled={busy} onClick={() => void load()}>
        Review task contributions
      </button>
      {error && <p role="alert">{error}</p>}
      {message && <p role="status">{message}</p>}
      {page && (
        <>
          <p>Saved task contributions: {page.items.length}</p>
          {page.items.map((a) => (
            <article key={a.id}>
              <p>
                Contribution {a.id}: {a.state}; purpose {a.purpose}; claim epoch{" "}
                {a.claim_epoch}.
              </p>
              {a.sha256 && <p>Observed SHA256: {a.sha256}</p>}
              {a.manifest && <pre>{JSON.stringify(a.manifest, null, 2)}</pre>}
              {a.sha256 && (
                <button disabled={busy} onClick={() => void inspect(a)}>
                  Inspect fixed evidence {a.id}
                </button>
              )}
              {evaluations[a.id] && (
                <>
                  <p>
                    Fixed evaluation is structural evidence only. Runtime
                    qualification, legal rights, factual truth and
                    output/publication consent remain separate.
                  </p>
                  {evaluations[a.id].length === 0 ? (
                    <p>
                      Verification is pending; no fixed evidence is available
                      yet.
                    </p>
                  ) : (
                    <pre>{JSON.stringify(evaluations[a.id], null, 2)}</pre>
                  )}
                </>
              )}
              {review &&
                a.state === "quarantine" &&
                a.claim_epoch === review.task.claim_epoch && (
                  <button
                    disabled={busy || pending || !allowed}
                    onClick={() => selectExisting(a)}
                  >
                    Review pending contribution {a.id}
                  </button>
                )}
            </article>
          ))}
          {page.next_after_id && (
            <button disabled={busy} onClick={() => void more()}>
              Next contribution page
            </button>
          )}
        </>
      )}
      {review && (
        <>
          <p>
            Task state: {review.task.state}. Version: {review.task.version}.
            Claim epoch: {review.task.claim_epoch}. Reviewed plan hash: {hash}.
          </p>
          {!allowed && (
            <p>
              This task cannot accept a new contribution. Review its
              dependencies, assignment and current plan.
            </p>
          )}
          {existing && (
            <p>
              Reviewing pending contribution {existing.id}. Reselect the
              original file and exact manifest to seal already uploaded bytes.
              Missing or mismatched server bytes will be rejected. Upload
              capabilities are not renewed.
            </p>
          )}
          <label>
            Contribution purpose
            <select
              value={purpose}
              disabled={busy || pending || !!existing || !allowed}
              onChange={(e) => {
                setPurpose(e.target.value as "output" | "progress");
                setAck(false);
              }}
            >
              <option value="output">Task output</option>
              <option value="progress">Private progress evidence</option>
            </select>
          </label>
          {purpose === "progress" && (
            <fieldset disabled={busy || pending || !!existing || !allowed}>
              <legend>Private evidence audience</legend>
              {candidates.map((id) => (
                <label key={id}>
                  <input
                    type="checkbox"
                    checked={audience.includes(id)}
                    onChange={(e) => {
                      setAudience((old) =>
                        e.target.checked
                          ? [...old, id]
                          : old.filter((x) => x !== id),
                      );
                      setAck(false);
                    }}
                  />
                  {id}
                </label>
              ))}
              <label>
                Designated progress reviewer
                <select
                  value={reviewer}
                  onChange={(e) => {
                    setReviewer(e.target.value);
                    setAck(false);
                  }}
                >
                  <option value="">Choose a reviewer</option>
                  {audience
                    .filter(
                      (id) =>
                        Array.isArray(
                          review.plan.specification.acceptance_principals,
                        ) &&
                        review.plan.specification.acceptance_principals.includes(
                          id,
                        ),
                    )
                    .map((id) => (
                      <option key={id} value={id}>
                        {id}
                      </option>
                    ))}
                </select>
              </label>
              <p>
                Verification alone does not credit progress or extend deadlines.
                A separate progress report and named review are required.
              </p>
            </fieldset>
          )}
          <label>
            JSON dataset file
            <input
              type="file"
              accept="application/json,.json"
              disabled={busy || pending || !allowed}
              onChange={(e) => void selectFile(e.target.files?.[0])}
            />
          </label>
          {prepared && (
            <p>
              Dataset SHA256: {prepared.sha}. Bytes: {prepared.bytes.byteLength}
              .
            </p>
          )}
          <label>
            Lineage and attribution manifest (JSON)
            <textarea
              value={manifestText}
              disabled={busy || pending || !allowed}
              onChange={(e) => {
                setManifestText(e.target.value);
                setAck(false);
              }}
            />
          </label>
          <p>
            Review every source declaration, attribution and immutable input
            reference. Referenced sources are not fetched or executed.
          </p>
          <label>
            <input
              type="checkbox"
              checked={ack}
              disabled={busy || !prepared || !allowed}
              onChange={(e) => setAck(e.target.checked)}
            />
            I reviewed the exact dataset, manifest, current task and plan, and
            any private audience. This submits evidence without output
            acceptance, publication or payment consent.
          </label>
          <button
            disabled={busy || !ack || !prepared || !allowed}
            onClick={() => void submit()}
          >
            {pending
              ? "Retry exact contribution"
              : existing
                ? "Seal existing uploaded bytes"
                : "Upload and seal contribution"}
          </button>
          {existing && !pending && (
            <button
              disabled={busy}
              onClick={() => {
                clearDraft();
                setPurpose("output");
                setAudience([review.principal]);
                setReviewer("");
              }}
            >
              Start a new upload instead
            </button>
          )}
        </>
      )}
    </section>
  );
}
