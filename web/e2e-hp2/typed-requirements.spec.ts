import {
  test,
  expect,
  type Browser,
  type BrowserContext,
  type Page,
} from "@playwright/test";

type Actor = {
  context: BrowserContext;
  page: Page;
  intent: string;
  principal: string;
};
const outcome = "Typed requirement source catalogue";
async function register(
  browser: Browser,
  label: string,
  licenses: string,
): Promise<Actor> {
  const issuer = process.env.UNDERGRID_HP2_ISSUER_URL;
  if (!issuer) throw Error("Controlled signed issuer required");
  const context = await browser.newContext({
    ignoreHTTPSErrors: true,
    baseURL: process.env.UNDERGRID_HP1_BASE_URL,
  });
  await context.addCookies([
    { name: "hp2-fixture-subject", value: label, url: issuer },
  ]);
  const page = await context.newPage();
  await page.goto("/");
  await page.getByRole("link", { name: "Sign in", exact: true }).click();
  await page
    .getByLabel("Owner name", { exact: true })
    .fill(`HP2 controlled ${label}`);
  await page
    .getByLabel("I accept terms hp2-local-fixture-terms", { exact: true })
    .check();
  await page.getByRole("button", { name: "Register", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Your workspace", exact: true }),
  ).toBeVisible();
  await page
    .getByLabel("Agent name", { exact: true })
    .fill(`HP2 ${label} agent`);
  await page
    .getByLabel("I reviewed this declared profile", { exact: true })
    .check();
  const issuing = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === "/v1/agents" &&
      r.request().method() === "POST",
  );
  await page
    .getByRole("button", { name: "Register agent", exact: true })
    .click();
  const issued = await issuing;
  expect(issued.status()).toBe(201);
  const credential = await issued.json();
  await page
    .getByRole("button", { name: "Dismiss credential", exact: true })
    .click();
  const identity = await page.request.get("/v1/me", {
    headers: { Authorization: `Bearer ${credential.token}` },
  });
  expect(identity.status()).toBe(200);
  const principal = (await identity.json()).principal_id as string;
  await expect(
    page.getByRole("button", { name: "Refresh", exact: true }),
  ).toBeEnabled();
  const form = page.getByRole("group", {
    name: "New intention details",
    exact: true,
  });
  await form
    .getByRole("textbox", { name: "Objective", exact: true })
    .fill(outcome);
  await form
    .getByLabel("Permitted license IDs", { exact: true })
    .fill(licenses);
  if (label !== "buyer-a") {
    await form
      .getByRole("combobox", { name: "Intention visibility", exact: true })
      .selectOption("redacted_public");
    await form
      .getByRole("textbox", { name: "Public summary", exact: true })
      .fill(outcome);
  }
  if (label !== "buyer-b") {
    await form.getByLabel("Declare coverage", { exact: true }).check();
    await form
      .getByLabel("Required coverage labels", { exact: true })
      .fill("region:eu");
  }
  if (label === "buyer-a") {
    await form
      .getByLabel("Declare freshness requirement", { exact: true })
      .check();
    await form
      .getByLabel("Maximum data age (seconds)", { exact: true })
      .fill("3600");
    await form.getByLabel("Set delivery window", { exact: true }).check();
    await form
      .getByLabel("Not before (RFC3339)", { exact: true })
      .fill("2027-01-02T10:00:00.123456789+01:00");
    await form
      .getByLabel("Deliver by (RFC3339)", { exact: true })
      .fill("2027-01-03T10:00:00Z");
    await form
      .getByLabel("Declare input schema alternatives", { exact: true })
      .check();
    await form
      .getByLabel("Permitted input schema families", { exact: true })
      .fill("source_catalog_v1, reference_v1");
  }
  if (label === "buyer-a")
    await form
      .getByLabel("Monitor matches in the agent inbox", { exact: true })
      .uncheck();
  if (label !== "buyer-a")
    await form
      .getByLabel("I reviewed this public projection", { exact: true })
      .check();
  const creating = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === "/v1/owner/intents" &&
      r.request().method() === "POST",
  );
  await form
    .getByRole("button", { name: "Create intention", exact: true })
    .click();
  const created = await creating;
  expect(created.status()).toBe(201);
  const record = await created.json();
  expect(record.visibility).toBe(
    label === "buyer-a" ? "private" : "redacted_public",
  );
  expect(record.watch.enabled).toBe(label !== "buyer-a");
  expect(record.hard_constraints.license_ids).toEqual(licenses.split(", "));
  await expect(
    page.getByRole("button", { name: "Refresh", exact: true }),
  ).toBeEnabled();
  return { context, page, principal, intent: record.id };
}

test("owner creates revises and removes typed requirements with immutable reload recovery", async ({
  browser,
}) => {
  const actors: Actor[] = [];
  try {
    const a = await register(browser, "buyer-a", "CC0-1.0");
    actors.push(a);
    actors.push(await register(browser, "buyer-b", "CC0-1.0"));
    actors.push(await register(browser, "builder-c", "ODC-BY-1.0"));
    const current = async () => {
      const response = await a.page.request.get(
        `/v1/owner/intents/${a.intent}`,
      );
      expect(response.status()).toBe(200);
      return await response.json();
    };
    const initial = await current();
    expect(initial.current_revision).toBe(1);
    expect(initial.hard_constraints).toMatchObject({
      freshness: { max_age_seconds: 3600 },
      delivery: {
        not_before: "2027-01-02T10:00:00.123456789+01:00",
        deliver_by: "2027-01-03T10:00:00Z",
      },
      input_schema_families: ["source_catalog_v1", "reference_v1"],
      coverage: { required: ["region:eu"], excluded: [] },
    });
    await a.page
      .getByRole("button", { name: "Review intention", exact: true })
      .click();
    await a.page
      .getByRole("button", { name: "Edit intention", exact: true })
      .click();
    const form = a.page.getByRole("group", {
      name: "Intention revision",
      exact: true,
    });
    await expect(
      form.getByLabel("Maximum data age (seconds)", { exact: true }),
    ).toHaveValue("3600");
    await expect(
      form.getByLabel("Not before (RFC3339)", { exact: true }),
    ).toHaveValue("2027-01-02T10:00:00.123456789+01:00");
    await form
      .getByLabel("Maximum data age (seconds)", { exact: true })
      .fill("7200");
    await form
      .getByLabel("Deliver by (RFC3339)", { exact: true })
      .fill("2027-01-04T09:00:00.250000001-02:00");
    await form
      .getByLabel("Permitted input schema families", { exact: true })
      .fill("source_catalog_v1");
    const save = async () => {
      const pending = a.page.waitForResponse(
        (r) =>
          new URL(r.url()).pathname === `/v1/owner/intents/${a.intent}` &&
          r.request().method() === "PATCH",
      );
      await form
        .getByRole("button", { name: "Save new revision", exact: true })
        .click();
      const response = await pending;
      expect(response.status()).toBe(200);
      await expect(
        a.page.getByRole("button", { name: "Refresh", exact: true }),
      ).toBeEnabled();
      return await response.json();
    };
    const revised = await save();
    expect(revised.version).toBe(2);
    expect(revised.hard_constraints).toMatchObject({
      freshness: { max_age_seconds: 7200 },
      delivery: {
        not_before: "2027-01-02T10:00:00.123456789+01:00",
        deliver_by: "2027-01-04T09:00:00.250000001-02:00",
      },
      input_schema_families: ["source_catalog_v1"],
      coverage: { required: ["region:eu"], excluded: [] },
    });
    await a.page.reload();
    await a.page
      .getByRole("button", { name: "Review intention", exact: true })
      .click();
    await a.page
      .getByRole("button", { name: "Edit intention", exact: true })
      .click();
    await expect(
      form.getByLabel("Maximum data age (seconds)", { exact: true }),
    ).toHaveValue("7200");
    await form
      .getByLabel("Declare freshness requirement", { exact: true })
      .uncheck();
    const removed = await save();
    expect(removed.version).toBe(3);
    expect(removed.hard_constraints.freshness).toBeUndefined();
    expect(removed.hard_constraints.delivery).toEqual(
      revised.hard_constraints.delivery,
    );
    expect(removed.hard_constraints.input_schema_families).toEqual([
      "source_catalog_v1",
    ]);
    expect(removed.watch.enabled).toBe(false);
    await a.page.reload();
    await a.page
      .getByRole("button", { name: "Review intention", exact: true })
      .click();
    await a.page
      .getByRole("button", { name: "Edit intention", exact: true })
      .click();
    await expect(
      form.getByLabel("Declare freshness requirement", { exact: true }),
    ).not.toBeChecked();
    await expect(
      form.getByLabel("Deliver by (RFC3339)", { exact: true }),
    ).toHaveValue("2027-01-04T09:00:00.250000001-02:00");
    await expect(
      form.getByLabel("Permitted input schema families", { exact: true }),
    ).toHaveValue("source_catalog_v1");
    expect(
      await a.page.evaluate(() => [localStorage.length, sessionStorage.length]),
    ).toEqual([0, 0]);
  } finally {
    for (const actor of actors) await actor.context.close();
  }
});
