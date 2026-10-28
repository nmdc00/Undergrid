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
const outcome = "Group environmental source catalogue";
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
  expect(record.watch.enabled).toBe(true);
  expect(record.hard_constraints.license_ids).toEqual(licenses.split(", "));
  await expect(
    page.getByRole("button", { name: "Refresh", exact: true }),
  ).toBeEnabled();
  return { context, page, principal, intent: record.id };
}
async function currentGroups(actor: Actor): Promise<Group[]> {
  const response = await actor.page.request.get(
    `/v1/owner/intents/${actor.intent}/compatible-groups`,
  );
  expect(response.status()).toBe(200);
  return (await response.json()).items;
}
function members(groups: Group[]) {
  return groups
    .map((group) =>
      group.members
        .map((member) => member.intent_id)
        .sort()
        .join(","),
    )
    .sort();
}
async function inspect(actor: Actor) {
  const loaded = actor.page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname ===
      `/v1/owner/intents/${actor.intent}/compatible-groups`,
  );
  await actor.page
    .getByRole("button", { name: "View compatible groups", exact: true })
    .click();
  const response = await loaded;
  expect(response.status()).toBe(200);
  return (await response.json()).items as Group[];
}

test("owner inspects separate nontransitive views then refreshes a new source revision", async ({
  browser,
}) => {
  const actors: Actor[] = [];
  try {
    actors.push(await register(browser, "buyer-a", "CC0-1.0, ODC-BY-1.0"));
    actors.push(await register(browser, "buyer-b", "CC0-1.0"));
    actors.push(await register(browser, "builder-c", "ODC-BY-1.0"));
    expect(new Set(actors.map((actor) => actor.principal)).size).toBe(3); // Attribution, not beneficial-owner independence.
    const [a, b, c] = actors;
    const expected = [
      [a.intent, b.intent],
      [a.intent, c.intent],
    ]
      .map((ids) => ids.sort().join(","))
      .sort();
    await expect
      .poll(async () => members(await currentGroups(a)))
      .toEqual(expected);
    await a.page
      .getByRole("button", { name: "Review intention", exact: true })
      .click();
    const groups = await inspect(a);
    expect(members(groups)).toEqual(expected);
    expect(
      groups.map((group) => group.common_license_ids.join(",")).sort(),
    ).toEqual(["CC0-1.0", "ODC-BY-1.0"]);
    for (const group of groups) {
      expect(group.members).toHaveLength(2);
      expect(group.comparison_mode).toBe("exact_declared_outcome");
      expect(group.identity_assurance).toBe("common_control_unknown");
    }
    const region = a.page.getByRole("region", {
      name: "Compatible groups",
      exact: true,
    });
    await expect(
      region.getByRole("heading", { name: "Compatible view 2", exact: true }),
    ).toBeVisible();
    await expect(region).toContainText(
      "principal IDs do not establish independent owners",
    );
    await expect(region).toContainText(
      "no membership, assignment, rights or spending authority",
    );
    await expect(region.getByText(b.intent, { exact: true })).toBeVisible();
    await expect(region.getByText(c.intent, { exact: true })).toBeVisible();
    await a.page
      .getByRole("button", { name: "Edit intention", exact: true })
      .click();
    const revision = a.page.getByRole("group", {
      name: "Intention revision",
      exact: true,
    });
    await revision
      .getByLabel("Permitted license IDs", { exact: true })
      .fill("CC0-1.0");
    const saving = a.page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname === `/v1/owner/intents/${a.intent}` &&
        r.request().method() === "PATCH",
    );
    await revision
      .getByRole("button", { name: "Save new revision", exact: true })
      .click();
    const saved = await saving;
    expect(saved.status()).toBe(200);
    expect((await saved.json()).version).toBe(2);
    await expect(region.getByText(c.intent, { exact: true })).toHaveCount(0);
    await expect(
      a.page.getByRole("button", { name: "Refresh", exact: true }),
    ).toBeEnabled();
    const revised = [[a.intent, b.intent].sort().join(",")];
    await expect
      .poll(async () => members(await currentGroups(a)))
      .toEqual(revised);
    expect(members(await inspect(a))).toEqual(revised);
    await expect(
      region.getByRole("heading", { name: "Compatible view 2", exact: true }),
    ).toHaveCount(0);
    await expect(region.getByText(b.intent, { exact: true })).toBeVisible();
    await expect(region.getByText(c.intent, { exact: true })).toHaveCount(0);
    await a.page.reload();
    await a.page
      .getByRole("button", { name: "Review intention", exact: true })
      .click();
    expect(members(await inspect(a))).toEqual(revised);
    expect(
      await a.page.evaluate(() => [localStorage.length, sessionStorage.length]),
    ).toEqual([0, 0]);
  } finally {
    for (const actor of actors) await actor.context.close();
  }
});
