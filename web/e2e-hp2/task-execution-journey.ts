import { expect } from "@playwright/test";
import { type Actor } from "./owner-plan-journey";

export async function proveOwnerTaskExecution(builder: Actor) {
  const page = builder.page;
  const taskArticle = page
    .getByRole("heading", { name: "Task collect", exact: true })
    .locator("..");
  const reviewing = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname.endsWith("/assignments") &&
      r.request().method() === "GET",
  );
  await taskArticle
    .getByRole("button", { name: "Review task execution", exact: true })
    .click();
  const reviewed = await reviewing;
  expect(reviewed.status()).toBe(200);
  const before = (await reviewed.json()).items.find(
    (x: { specification: { key: string } }) =>
      x.specification.key === "collect",
  );
  expect(before.state).toBe("accepted");
  expect(before.claim_epoch).toBe(1);
  const activity = taskArticle.getByRole("region", {
    name: `Execution for ${before.id}`,
    exact: true,
  });
  await expect(
    activity.getByRole("button", {
      name: "Record unverified activity",
      exact: true,
    }),
  ).toBeDisabled();
  await activity.getByLabel(/I reviewed the current task/).check();
  const path = `/v1/owner/agents/${builder.agent}/tasks/${before.id}/progress`;
  let originalBody: string | null = null,
    key: string | undefined,
    match: string | undefined;
  await page.route(`**${path}`, async (route) => {
    originalBody = route.request().postData();
    key = route.request().headers()["idempotency-key"];
    match = route.request().headers()["if-match"];
    const response = await route.fetch();
    expect(response.status()).toBe(200);
    await route.abort("failed");
  });
  await activity
    .getByRole("button", { name: "Record unverified activity", exact: true })
    .click();
  await expect(activity.getByRole("alert")).toBeVisible();
  await page.unroute(`**${path}`);
  let recovered = false;
  for (let attempt = 0; attempt < 3; attempt++) {
    const pending = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname === path && r.request().method() === "POST",
    );
    await activity
      .getByRole("button", { name: "Record unverified activity", exact: true })
      .click();
    const response = await pending;
    expect(response.request().postData()).toBe(originalBody);
    expect(response.request().headers()["idempotency-key"]).toBe(key);
    expect(response.request().headers()["if-match"]).toBe(match);
    if (response.status() !== 200) {
      expect(response.status()).toBe(500);
      expect((await response.json()).error.retryable).toBe(true);
      await expect(activity.getByRole("alert")).toBeVisible();
      continue;
    }
    const result = await response.json();
    expect(result.version).toBe(before.version + 1);
    expect(result.claim_epoch).toBe(before.claim_epoch);
    expect(result.progress_deadline).toBe(before.progress_deadline);
    expect(result.cure_deadline).toBe(before.cure_deadline);
    recovered = true;
    break;
  }
  expect(recovered).toBe(true);
  expect(JSON.parse(originalBody!)).toEqual({
    state: "in_progress",
    claim_epoch: before.claim_epoch,
  });
  expect(match).toBe(before.etag);
  await expect(activity.getByRole("status")).toContainText(
    "No deadline was extended",
  );
  await page.reload();
  await page
    .getByRole("button", { name: "Load participation", exact: true })
    .click();
  await page.getByRole("button", { name: "Open room", exact: true }).click();
  await page.getByRole("button", { name: "Load plans", exact: true }).click();
  // The accepted revision is the latest plan, selected via its normal owner control.
  await page
    .getByRole("button", { name: "Review plan", exact: true })
    .first()
    .click();
  await page
    .getByRole("button", { name: "Review my assignments", exact: true })
    .click();
  const reloaded = page
    .getByRole("heading", { name: "Task collect", exact: true })
    .locator("..");
  await reloaded
    .getByRole("button", { name: "Review task execution", exact: true })
    .click();
  await expect(reloaded).toContainText("State: in_progress");
  await expect(reloaded).toContainText(before.progress_deadline);
}
