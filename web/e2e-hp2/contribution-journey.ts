import { createHash } from "node:crypto";
import { expect, type Locator } from "@playwright/test";
import { type Actor } from "./owner-plan-journey";

// Extends the same normal HP2 owners/plan, rather than seeding artifact/task rows.
export async function proveOwnerContributions(actors: Actor[]) {
  const [coordinator, excluded, builder] = actors,
    page = builder.page;
  const taskArticle = () =>
    page
      .getByRole("heading", { name: "Task collect", exact: true })
      .locator("..");
  let region: Locator = taskArticle().getByRole("region", {
    name: /Contributions for /,
  });
  // The review request itself supplies the owned task/plan context; no pasted IDs.
  const reviewing = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname.endsWith("/assignments") &&
      r.request().method() === "GET",
  );
  await region
    .getByRole("button", { name: "Review task contributions", exact: true })
    .click();
  const before = (await (await reviewing).json()).items.find(
    (x: { specification: { key: string } }) =>
      x.specification.key === "collect",
  );
  expect(before.state).toBe("in_progress");
  const plan = before.plan_id,
    taskID = before.id;
  const raw =
    '{"sources":[{"source_id":"owner-browser-catalog","source_url":"https://example.org/","instructions":"untrusted inert content; never execute"}]}';
  const bytes = Buffer.from(raw),
    sha = createHash("sha256").update(bytes).digest("hex");
  const file = {
    name: "owner-catalog.json",
    mimeType: "application/json",
    buffer: bytes,
  };
  const finalize = `**/v1/owner/agents/${builder.agent}/artifacts/*/finalize`;
  let progressID = "",
    progressKey: string | undefined,
    progressBody: string | null = null;
  await region
    .getByLabel("Contribution purpose", { exact: true })
    .selectOption("progress");
  await region.getByLabel(coordinator.principal, { exact: true }).check();
  await region
    .getByLabel("Designated progress reviewer", { exact: true })
    .selectOption(coordinator.principal);
  await region
    .getByLabel("JSON dataset file", { exact: true })
    .setInputFiles(file);
  await expect(region).toContainText(`Dataset SHA256: ${sha}`);
  await region.getByLabel(/I reviewed the exact dataset/).check();
  await page.route(finalize, async (route) => {
    const request = route.request();
    progressID = new URL(request.url()).pathname.split("/").at(-2)!;
    progressKey = request.headers()["idempotency-key"];
    progressBody = request.postData();
    const response = await route.fetch();
    expect(response.status()).toBe(200);
    await route.abort("failed");
  });
  await region
    .getByRole("button", { name: "Upload and seal contribution", exact: true })
    .click();
  await expect(region.getByRole("alert")).toBeVisible();
  await page.unroute(finalize);
  let recovered = false;
  for (let attempt = 0; attempt < 3; attempt++) {
    const pending = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname.endsWith(`/${progressID}/finalize`) &&
        r.request().method() === "POST",
    );
    await region
      .getByRole("button", { name: "Retry exact contribution", exact: true })
      .click();
    const response = await pending;
    expect(response.request().postData()).toBe(progressBody);
    expect(response.request().headers()["idempotency-key"]).toBe(progressKey);
    if (response.status() === 200) {
      recovered = true;
      break;
    }
    expect(response.status()).toBe(500);
    expect((await response.json()).error.retryable).toBe(true);
    await expect(region.getByRole("alert")).toBeVisible();
  }
  expect(recovered).toBe(true);
  await expect(region.getByRole("status")).toContainText("Contribution sealed");
  async function reviewVerified(id: string) {
    await expect(async () => {
      await region
        .getByRole("button", { name: "Review task contributions", exact: true })
        .click();
      await expect(region).toContainText(`Contribution ${id}: verified`);
    }).toPass({ timeout: 12000, intervals: [100, 250, 500] });
  }
  await reviewVerified(progressID);
  await region
    .getByRole("button", {
      name: `Inspect fixed evidence ${progressID}`,
      exact: true,
    })
    .click();
  await expect(
    region
      .locator("pre")
      .filter({ hasText: "development_process_not_qualified" }),
  ).toContainText(sha);
  for (const suffix of ["", "/evaluations", "/content"]) {
    const denied = await excluded.context.request.get(
      `/v1/owner/agents/${excluded.agent}/artifacts/${progressID}${suffix}`,
    );
    expect(denied.status()).toBe(404);
  }
  const afterProgress = await builder.context.request.get(
    `/v1/owner/agents/${builder.agent}/plans/${plan}/assignments`,
  );
  const current = (await afterProgress.json()).items.find(
    (x: { id: string }) => x.id === taskID,
  );
  expect(current.state).toBe(before.state);
  expect(current.progress_deadline).toBe(before.progress_deadline);
  expect(current.cure_deadline).toBe(before.cure_deadline);
  expect(current.credited_progress_evidence_id).toBeNull();
  // Lose the output request before finalization: uploaded bytes survive reload,
  // and the returning owner discovers the pending generation using normal UI.
  await region
    .getByLabel("JSON dataset file", { exact: true })
    .setInputFiles(file);
  await expect(region).toContainText(`Dataset SHA256: ${sha}`);
  await region.getByLabel(/I reviewed the exact dataset/).check();
  let outputID = "";
  await page.route(finalize, async (route) => {
    outputID = new URL(route.request().url()).pathname.split("/").at(-2)!;
    await route.abort("failed");
  });
  await region
    .getByRole("button", { name: "Upload and seal contribution", exact: true })
    .click();
  await expect(region.getByRole("alert")).toBeVisible();
  await page.unroute(finalize);
  expect(outputID).not.toBe("");
  await page.reload();
  await page
    .getByRole("button", { name: "Load participation", exact: true })
    .click();
  await page.getByRole("button", { name: "Open room", exact: true }).click();
  await page.getByRole("button", { name: "Load plans", exact: true }).click();
  await page
    .getByRole("button", { name: "Review plan 2", exact: true })
    .first()
    .click();
  await page
    .getByRole("button", { name: "Review my assignments", exact: true })
    .click();
  region = taskArticle().getByRole("region", { name: /Contributions for / });
  await region
    .getByRole("button", { name: "Review task contributions", exact: true })
    .click();
  await region
    .getByRole("button", {
      name: `Review pending contribution ${outputID}`,
      exact: true,
    })
    .click();
  await region
    .getByLabel("JSON dataset file", { exact: true })
    .setInputFiles(file);
  await expect(region).toContainText(`Dataset SHA256: ${sha}`);
  await region.getByLabel(/I reviewed the exact dataset/).check();
  await region
    .getByRole("button", { name: "Seal existing uploaded bytes", exact: true })
    .click();
  await expect(region.getByRole("status")).toContainText("Contribution sealed");
  await reviewVerified(outputID);
  await region
    .getByRole("button", {
      name: `Inspect fixed evidence ${outputID}`,
      exact: true,
    })
    .click();
  await expect(
    region
      .locator("pre")
      .filter({ hasText: "development_process_not_qualified" }),
  ).toContainText(sha);
  const restart = await builder.context.request.post(
    "/__fixture/restart-plan-worker",
  );
  expect(restart.status()).toBe(200);
  expect((await restart.json()).completed_startup_job).toBe(true);
  await reviewVerified(outputID);
  expect(
    await page.evaluate(() => ({
      local: localStorage.length,
      session: sessionStorage.length,
    })),
  ).toEqual({ local: 0, session: 0 });
}
