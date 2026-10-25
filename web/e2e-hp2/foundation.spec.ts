import { test, expect, type BrowserContext } from "@playwright/test";

// Actual network app/session/API. This foundation does not prove discovery,
// builder profile, membership, plan or assignment flows still missing from HP2.
test("three controlled owners receive separate scoped agents and private watches", async ({ browser }) => {
  const issuer = process.env.UNDERGRID_HP2_ISSUER_URL;
  if (!issuer) throw new Error("HP2 requires the controlled issuer URL");
  const contexts: BrowserContext[] = [];
  const actors: { principal: string; agent: string; token: string; intent?: string }[] = [];
  try {
    for (const label of ["buyer-a", "buyer-b", "builder-c"]) {
      const context = await browser.newContext({ ignoreHTTPSErrors: true, baseURL: process.env.UNDERGRID_HP1_BASE_URL });
      contexts.push(context);
      // A test-provider identity selector, not an app cookie/session bypass.
      // The issuer binds this subject into its one-use PKCE authorization grant.
      await context.addCookies([{ name: "hp2-fixture-subject", value: label, url: issuer }]);
      const page = await context.newPage();
      await page.goto("/");
      await page.getByRole("link", { name: "Sign in", exact: true }).click();
      await expect(page.getByRole("heading", { name: "Register your owner account" })).toBeVisible();
      await page.getByLabel("Owner name", { exact: true }).fill(`HP2 controlled ${label}`);
      await page.getByLabel("I accept terms hp2-local-fixture-terms", { exact: true }).check();
      await page.getByRole("button", { name: "Register", exact: true }).click();
      await expect(page.getByRole("heading", { name: "Your workspace", exact: true })).toBeVisible();
      await page.getByLabel("Agent name", { exact: true }).fill(`HP2 ${label} agent`);
      const issuance = page.waitForResponse(r => new URL(r.url()).pathname === "/v1/agents" && r.request().method() === "POST");
      await page.getByRole("button", { name: "Register agent", exact: true }).click();
      const issued = await issuance;
      expect(issued.status()).toBe(201);
      const credential = await issued.json();
      expect(credential.token).toMatch(/^ug_/);
      await page.getByRole("button", { name: "Dismiss credential", exact: true }).click();
      const identityResponse = await page.request.get("/v1/me", { headers: { Authorization: `Bearer ${credential.token}` } });
      expect(identityResponse.status()).toBe(200);
      const identity = await identityResponse.json();
      expect(identity.agent_id).toBe(credential.id);
      const actor = { principal: identity.principal_id as string, agent: identity.agent_id as string, token: credential.token as string, intent: undefined as string | undefined };
      if (label !== "builder-c") {
        await page.getByLabel("Objective", { exact: true }).fill(`Private environmental catalogue requirements ${label}`);
        await expect(page.getByRole("combobox", { name: "Intention visibility", exact: true })).toHaveValue("private");
        await expect(page.getByLabel("Monitor matches in the agent inbox", { exact: true })).toBeChecked();
        const creation = page.waitForResponse(r => new URL(r.url()).pathname === "/v1/owner/intents" && r.request().method() === "POST");
        await page.getByRole("button", { name: "Create intention", exact: true }).click();
        const created = await creation;
        expect(created.status()).toBe(201);
        const intention = await created.json();
        actor.intent = intention.id;
        const own = await page.request.get(`/v1/intents/${encodeURIComponent(intention.id)}`, { headers: { Authorization: `Bearer ${credential.token}` } });
        expect(own.status()).toBe(200);
        const record = await own.json();
        expect(record.visibility).toBe("private");
        expect(record.state).toBe("active");
        expect(record.watch.enabled).toBe(true);
        expect(record.hard_constraints.required_fields).toEqual(["source_id", "source_url"]);
      }
      actors.push(actor);
      expect(await page.evaluate(() => [localStorage.length, sessionStorage.length])).toEqual([0, 0]);
      await page.reload();
      await expect(page.getByRole("heading", { name: "Your workspace", exact: true })).toBeVisible();
    }
    expect(new Set(actors.map(a => a.principal)).size).toBe(3);
    expect(new Set(actors.map(a => a.agent)).size).toBe(3);
    for (let owner = 0; owner < actors.length; owner++) {
      for (const target of actors.filter(a => a.intent && a.principal !== actors[owner].principal)) {
        const inaccessible = await contexts[owner].request.get(`/v1/intents/${encodeURIComponent(target.intent!)}`, {
          headers: { Authorization: `Bearer ${actors[owner].token}` },
        });
        expect(inaccessible.status()).toBe(404);
      }
    }
    // Names identify intended future roles only. No builder capability, opt-in,
    // independent ownership, collaboration or participant consent is inferred.
  } finally {
    for (const context of contexts) await context.close();
  }
});
