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
  await page.getByLabel("Objective", { exact: true }).fill("HP1 private environmental source catalogue");
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
});
