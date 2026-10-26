import { test, expect } from "@playwright/test";

// Real HTTPS API, cookie/CSRF, PostgreSQL and signed OIDC fixture. This milestone
// preserves setup/gap and watch milestones; the explicit partial mode exercises
// the complete local HP1 journey without qualifying external sources/providers.
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
  await page.getByLabel("I reviewed this declared profile", { exact: true }).check();
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
    let first = (await opportunities())[0].id;
    const partialJourney = process.env.UNDERGRID_HP1_PARTIAL_JOURNEY === "1";
    await page.getByRole("button", { name: "Review intention", exact: true }).click();
    await expect(page.getByText("Controlled fixture metadata; not a qualified real source", { exact: false }).first()).toBeVisible();
    await page.getByRole("button", { name: "Find external leads", exact: true }).click();
    await page.getByRole("button", { name: "Review declared fit", exact: true }).click();
    await expect(page.getByText("Assessment uses unverified publisher declarations. Identity, rights and availability are unverified.", { exact: true })).toBeVisible();
    if (partialJourney) {
      // Wait for the loaded assessment, rather than its always-visible disclaimer.
      const assessment = page.getByRole("region", { name: "Selected lead assessment", exact: true });
      await expect(assessment.getByRole("table", { name: "Declared constraints", exact: true })).toBeVisible();
      await expect(assessment.getByText("Intention revision 1", { exact: true })).toBeVisible();
      await expect(assessment.getByRole("heading", { name: "Declared mismatch", exact: true })).toBeVisible();
      const fulfillment = page.getByRole("region", { name: "Record partial fulfillment", exact: true });
      await expect(fulfillment.getByLabel("I have fulfilled source_id", { exact: true })).not.toBeChecked();
      await expect(fulfillment.getByLabel("I have fulfilled source_url", { exact: true })).not.toBeChecked();
      await fulfillment.getByLabel("I have fulfilled source_id", { exact: true }).check();
      await expect(fulfillment.getByLabel("Unmet required fields", { exact: true })).toHaveText("source_url");
      const partialResponse = page.waitForResponse(r => new URL(r.url()).pathname === `/v1/owner/intents/${intention.id}/partial-satisfaction` && r.request().method() === "POST");
      await fulfillment.getByRole("button", { name: "Save unmet remainder", exact: true }).click();
      const recorded = await partialResponse;
      expect(recorded.status()).toBe(200);
      const remainder = await recorded.json();
      expect(remainder.current_revision).toBe(2);
      expect(remainder.state).toBe("active");
      expect(remainder.watch.enabled).toBe(true);
      expect(remainder.hard_constraints.required_fields).toEqual(["source_url"]);
      for (const field of ["objective", "visibility", "output", "preferences", "budget", "watch", "expires_at", "public_summary"]) {
        expect(remainder[field]).toEqual(intentions[field]);
      }
      expect(remainder.hard_constraints.license_ids).toEqual(intentions.hard_constraints.license_ids);
      expect(remainder.partial_satisfaction.evidence_class).toBe("owner_declared_partial_satisfaction");
      expect(remainder.partial_satisfaction.fulfilled_required_fields).toEqual(["source_id"]);
      expect(remainder.partial_satisfaction.remaining_required_fields).toEqual(["source_url"]);
      await expect(page.getByRole("region", { name: "Current unmet requirements", exact: true }).getByRole("listitem")).toHaveText(["source_url"]);
      await page.reload();
      await page.getByRole("button", { name: "Review intention", exact: true }).click();
      await expect(page.getByRole("region", { name: "Current unmet requirements", exact: true }).getByRole("listitem")).toHaveText(["source_url"]);
      const recovered = await page.request.get(`/v1/intents/${encodeURIComponent(intention.id)}`, { headers: { Authorization: `Bearer ${credential.token}` } });
      expect(recovered.status()).toBe(200);
      expect((await recovered.json()).hard_constraints.required_fields).toEqual(["source_url"]);
      // A changed intention revision is genuinely different matching input.
      // Settle that result before a later publisher change, accounting for both.
      const beforePartial = first;
      await expect.poll(async () => {
        const current = await opportunities();
        return current.length === 1 && current[0].id !== beforePartial;
      }, { timeout: 8_000 }).toBe(true);
      first = (await opportunities())[0].id;
      await control("/publish/2");
      const beforeUpdate = first;
      await expect.poll(async () => {
        const current = await opportunities();
        return current.length === 1 && current[0].id !== beforeUpdate;
      }, { timeout: 8_000 }).toBe(true);
      first = (await opportunities())[0].id;
      await page.getByRole("button", { name: "Review intention", exact: true }).click();
      await page.getByRole("button", { name: "Find external leads", exact: true }).click();
      await page.getByRole("button", { name: "Review declared fit", exact: true }).click();
      await expect(assessment.getByRole("table", { name: "Declared constraints", exact: true })).toBeVisible();
      await expect(assessment.getByText("Intention revision 2", { exact: true })).toBeVisible();
      await expect(assessment.getByRole("heading", { name: "Declared alignment", exact: true })).toBeVisible();
      await expect(assessment.getByRole("row").filter({ hasText: "source_url" })).toContainText("Declared alignment");
    }
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
    await control(partialJourney ? "/publish/3" : "/publish/2");
    await expect.poll(async () => (await opportunities()).length, { timeout: 8_000 }).toBe(1);
    expect((await opportunities())[0].id).not.toBe(first);
    const retired = await page.request.get(`/v1/opportunities/${first}`, { headers: { Authorization: `Bearer ${credential.token}` } });
    expect(retired.status()).toBe(404);
  }
});
