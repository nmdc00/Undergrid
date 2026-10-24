import { test, expect } from "@playwright/test";

// Real HTTPS API, cookie/CSRF, PostgreSQL and signed OIDC fixture. This milestone
// proves setup and an honest empty-source gap, not the complete HP1 journey.
test("first owner signs in, creates a private watch and inspects an honest gap", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("link", { name: "Sign in", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Register your owner account" })).toBeVisible();
  await page.getByLabel("Owner name", { exact: true }).fill("HP1 local owner");
  await page.getByLabel("I accept terms hp1-local-fixture-terms").check();
  await page.getByRole("button", { name: "Register", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Your workspace" })).toBeVisible();
  await page.getByLabel("Agent name", { exact: true }).fill("HP1 private listener");
  const credentialResponse = page.waitForResponse(r => new URL(r.url()).pathname === "/v1/agents" && r.request().method() === "POST");
  await page.getByRole("button", { name: "Register agent", exact: true }).click();
  const response = await credentialResponse;
  expect(response.status()).toBe(201);
  const credential = await response.json();
  expect(credential.token).toMatch(/^ug_/);
  await page.getByRole("button", { name: "Dismiss credential", exact: true }).click();
  await page.getByLabel("Objective", { exact: true }).fill("Environmental source catalogue");
  await expect(page.getByRole("combobox", { name: "Intention visibility", exact: true })).toHaveValue("private");
  await expect(page.getByLabel("Monitor matches in the agent inbox")).toBeChecked();
  const intentionResponse = page.waitForResponse(r => new URL(r.url()).pathname === "/v1/owner/intents" && r.request().method() === "POST");
  await page.getByRole("button", { name: "Create intention", exact: true }).click();
  const created = await intentionResponse;
  expect(created.status()).toBe(201);
  const intention = await created.json();
  await page.getByRole("button", { name: "Review intention", exact: true }).click();
  await page.getByRole("button", { name: "Find external leads", exact: true }).click();
  await expect(page.getByText("No current permitted leads match these filters.", { exact: true })).toBeVisible();
  // Use only the normally issued scoped credential, never a test-owner header.
  const own = await page.request.get(`/v1/intents/${encodeURIComponent(intention.id)}`, { headers: { Authorization: `Bearer ${credential.token}` } });
  expect(own.status()).toBe(200);
  const intentions = await own.json();
  expect(intentions.id).toBe(intention.id);
  expect(intentions.visibility).toBe("private");
  expect(intentions.watch.enabled).toBe(true);
  expect(intentions.hard_constraints.required_fields).toEqual(["source_id", "source_url"]);
  expect(intentions.hard_constraints.license_ids).toEqual(["CC0-1.0"]);
  expect(Date.parse(intentions.expires_at)).toBeGreaterThan(Date.now());
  expect(await page.evaluate(() => [localStorage.length, sessionStorage.length])).toEqual([0, 0]);
  await page.reload();
  await expect(page.getByRole("heading", { name: "Your workspace" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Review intention", exact: true })).toBeVisible();

  const controller = process.env.UNDERGRID_HP1_WATCH_CONTROL_URL;
  if (controller) {
    const control = async (path: string) => {
      const result = await page.request.post(`${controller}${path}`);
      expect(result.status()).toBe(204);
    };
    await control("/start");
    await control("/publish/1");
    const opportunities = async () => {
      const result = await page.request.get(`/v1/intents/${encodeURIComponent(intention.id)}/opportunities`, {
        headers: { Authorization: `Bearer ${credential.token}` },
      });
      expect(result.status()).toBe(200);
      return (await result.json()).items as { id: string }[];
    };
    await expect.poll(async () => (await opportunities()).length, { timeout: 8_000 }).toBe(1);
    const first = (await opportunities())[0].id;
    await page.getByRole("button", { name: "Review intention", exact: true }).click();
    await expect(page.getByText("Controlled fixture metadata; not a qualified real source", { exact: false }).first()).toBeVisible();
    await page.getByRole("button", { name: "Find external leads", exact: true }).click();
    await page.getByRole("button", { name: "Review declared fit", exact: true }).click();
    await expect(page.getByText("Assessment uses unverified publisher declarations. Identity, rights and availability are unverified.", { exact: true })).toBeVisible();
    // Partial satisfaction is intentionally not substituted with terminal
    // satisfaction or a hidden revision. Its future owner control is a missing
    // full-HP1 step, separately recorded in the acceptance ledger.
    await page.getByRole("combobox", { name: "Inbox agent", exact: true }).selectOption(credential.id);
    await page.getByRole("button", { name: "Refresh inbox", exact: true }).click();
    const notification = page.getByRole("listitem").filter({ hasText: `/v1/opportunities/${first}` });
    await expect(notification).toHaveCount(1);
    await notification.getByRole("button", { name: "Acknowledge receipt", exact: true }).click();
    await expect(notification.getByText("Receipt acknowledged.", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Dismiss suggestion", exact: true }).click();
    await expect.poll(async () => (await opportunities()).length).toBe(0);
    // Cancellation must finish before the same database receives a new worker.
    await control("/restart-unchanged");
    await page.reload();
    await page.getByRole("button", { name: "Review intention", exact: true }).click();
    await expect(page.getByRole("button", { name: "Dismiss suggestion", exact: true })).toHaveCount(0);
    expect(await opportunities()).toHaveLength(0);
    await control("/publish/2");
    await expect.poll(async () => (await opportunities()).length, { timeout: 8_000 }).toBe(1);
    expect((await opportunities())[0].id).not.toBe(first);
    const retired = await page.request.get(`/v1/opportunities/${first}`, { headers: { Authorization: `Bearer ${credential.token}` } });
    expect(retired.status()).toBe(404);
  }
});
