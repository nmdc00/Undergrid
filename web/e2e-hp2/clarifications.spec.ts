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
type Group = {
  members: { intent_id: string; principal_id: string }[];
  common_license_ids: string[];
  comparison_mode: string;
  identity_assurance: string;
};
const outcome = "Clarification environmental source catalogue";
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
    await form
      .getByLabel("I reviewed this public projection", { exact: true })
      .check();
  }
  if (label !== "buyer-b") {
    await form.getByLabel("Declare coverage", { exact: true }).check();
    await form
      .getByLabel("Required coverage labels", { exact: true })
      .fill("region:eu");
  }
  if (label === "buyer-a")
    await form
      .getByLabel("Monitor matches in the agent inbox", { exact: true })
      .uncheck();
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

test("owner clarifies missing public requirements after publisher revision and reload", async ({
  browser,
}) => {
  const actors: Actor[] = [];
  try {
    actors.push(await register(browser, "buyer-a", "CC0-1.0"));
    actors.push(await register(browser, "buyer-b", "CC0-1.0"));
    actors.push(await register(browser, "builder-c", "ODC-BY-1.0"));
    const [a, b, c] = actors;
    expect(new Set(actors.map((x) => x.principal)).size).toBe(3); // Attribution only.
    await a.page
      .getByRole("button", { name: "Review intention", exact: true })
      .click();
    const inspect = async (refresh = false) => {
      const pending = a.page.waitForResponse(
        (r) =>
          new URL(r.url()).pathname ===
          `/v1/owner/intents/${a.intent}/constraint-assessments`,
      );
      await a.page
        .getByRole("button", {
          name: refresh ? "Refresh requirement gaps" : "Check requirement gaps",
          exact: true,
        })
        .click();
      const response = await pending;
      expect(response.status()).toBe(200);
      return await response.json();
    };
    const first = await inspect();
    expect(first.source_version).toBe(1);
    expect(first.items).toHaveLength(2);
    expect(
      first.items.find(
        (x: { candidate_intent_id: string }) =>
          x.candidate_intent_id === b.intent,
      ),
    ).toMatchObject({
      hard_status: "unknown",
      clarification_required: true,
      candidate_version: 1,
    });
    expect(
      first.items.find(
        (x: { candidate_intent_id: string }) =>
          x.candidate_intent_id === c.intent,
      ),
    ).toMatchObject({ hard_status: "incompatible" });
    const region = a.page.getByRole("region", {
      name: "Requirement clarification",
      exact: true,
    });
    await expect(region).toContainText("Needs clarification");
    await expect(region).toContainText("Declared requirements conflict");
    await b.page
      .getByRole("button", { name: "Edit intention", exact: true })
      .click();
    const revision = b.page.getByRole("group", {
      name: "Intention revision",
      exact: true,
    });
    await revision.getByLabel("Declare coverage", { exact: true }).check();
    await revision
      .getByLabel("Required coverage labels", { exact: true })
      .fill("region:eu");
    const saving = b.page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname === `/v1/owner/intents/${b.intent}` &&
        r.request().method() === "PATCH",
    );
    await revision
      .getByRole("button", { name: "Save new revision", exact: true })
      .click();
    expect((await saving).status()).toBe(200);
    const check = (report: typeof first) => {
      expect(report.source_version).toBe(1);
      expect(
        report.items.find(
          (x: { candidate_intent_id: string }) =>
            x.candidate_intent_id === b.intent,
        ),
      ).toMatchObject({
        hard_status: "aligned",
        clarification_required: false,
        candidate_version: 2,
      });
      expect(
        report.items.find(
          (x: { candidate_intent_id: string }) =>
            x.candidate_intent_id === c.intent,
        ),
      ).toMatchObject({ hard_status: "incompatible" });
    };
    check(await inspect(true));
    await expect(region).toContainText("Declared requirements can combine");
    await expect(region).not.toContainText("Needs clarification");
    await a.page.reload();
    await a.page
      .getByRole("button", { name: "Review intention", exact: true })
      .click();
    check(await inspect());
    expect(
      await a.page.evaluate(() => [localStorage.length, sessionStorage.length]),
    ).toEqual([0, 0]);
  } finally {
    for (const actor of actors) await actor.context.close();
  }
});
