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
const outcome = "Capability environmental source catalogue";
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
  if (label === "builder-c") {
    await page
      .getByLabel("Make declared profile public for recruitment", {
        exact: true,
      })
      .check();
    await page
      .getByLabel("Declare source_catalog_v1 capability", { exact: true })
      .check();
  }
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

test("owner discovers a normal public builder declaration and recovers current source access", async ({
  browser,
}) => {
  const actors: Actor[] = [];
  try {
    const a = await register(browser, "buyer-a", "CC0-1.0");
    actors.push(a);
    const b = await register(browser, "buyer-b", "CC0-1.0");
    actors.push(b);
    await a.page
      .getByRole("button", { name: "Review intention", exact: true })
      .click();
    const inspect = async (refresh = false) => {
      const pending = a.page.waitForResponse(
        (r) =>
          new URL(r.url()).pathname ===
          `/v1/owner/intents/${a.intent}/capability-fit`,
      );
      await a.page
        .getByRole("button", {
          name: refresh
            ? "Refresh registered capabilities"
            : "Discover registered capabilities",
          exact: true,
        })
        .click();
      const response = await pending;
      expect(response.status()).toBe(200);
      return await response.json();
    };
    const empty = await inspect();
    expect(empty.agents.items).toHaveLength(0);
    expect(empty.artifacts.items).toHaveLength(0);
    await expect(
      a.page.getByText(
        "No current registered capabilities in these bounded samples.",
      ),
    ).toBeVisible();
    const c = await register(browser, "builder-c", "ODC-BY-1.0");
    actors.push(c);
    expect(new Set(actors.map((x) => x.principal)).size).toBe(3); // Attribution only.
    const check = (report: typeof empty, version = 1) => {
      expect(report.source_intent_id).toBe(a.intent);
      expect(report.source_version).toBe(version);
      expect(report.identity_assurance).toBe("common_control_unknown");
      expect(report.agents.items).toHaveLength(1);
      expect(report.artifacts.items).toHaveLength(0);
      expect(report.agents.items[0]).toMatchObject({
        principal_id: c.principal,
        origin: "registered_capability",
        assurance: "self_declared",
        schema_support: "declared",
        hard_status: "unknown",
        capabilities: ["source_catalog_v1"],
      });
      expect(report.agents.items[0].constraint_evidence).toContainEqual(
        expect.objectContaining({ constraint: "coverage", status: "unknown" }),
      );
    };
    check(await inspect(true));
    const region = a.page.getByRole("region", {
      name: "Registered capability discovery",
      exact: true,
    });
    await expect(region).toContainText("Declared schema support");
    await expect(region).toContainText("Unknown requirements remain");
    await expect(region).toContainText(
      "no membership, assignment, rights, reuse receipt or spending authority",
    );
    expect(
      (
        await b.page.request.get(`/v1/owner/intents/${a.intent}/capability-fit`)
      ).status(),
    ).toBe(404);
    await a.page.reload();
    await a.page
      .getByRole("button", { name: "Review intention", exact: true })
      .click();
    check(await inspect());
    await a.page.getByRole("button", { name: "Pause", exact: true }).click();
    await expect(
      a.page.getByRole("button", { name: "Resume", exact: true }),
    ).toBeEnabled();
    await expect(
      a.page.getByRole("button", {
        name: "Discover registered capabilities",
        exact: true,
      }),
    ).toHaveCount(0);
    expect(
      (
        await a.page.request.get(`/v1/owner/intents/${a.intent}/capability-fit`)
      ).status(),
    ).toBe(404);
    await a.page.getByRole("button", { name: "Resume", exact: true }).click();
    await expect(
      a.page.getByRole("button", { name: "Pause", exact: true }),
    ).toBeEnabled();
    check(await inspect(false), 3);
    expect(
      await a.page.evaluate(() => [localStorage.length, sessionStorage.length]),
    ).toEqual([0, 0]);
  } finally {
    for (const actor of actors) await actor.context.close();
  }
});
