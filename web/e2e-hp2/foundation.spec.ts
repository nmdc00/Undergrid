import { test, expect, type BrowserContext } from "@playwright/test";

// Actual network app/session/API. This foundation does not prove discovery,
// builder profile, membership, plan or assignment flows still missing from HP2.
test(
  process.env.UNDERGRID_HP2_OWNER_PUBLICATION === "1"
    ? "owner publishes a selected projection and revokes its foreign read through the console"
    : "three controlled owners receive separate scoped agents and private watches",
  async ({ browser }) => {
    const publication = process.env.UNDERGRID_HP2_OWNER_PUBLICATION === "1";
    const issuer = process.env.UNDERGRID_HP2_ISSUER_URL;
    if (!issuer) throw new Error("HP2 requires the controlled issuer URL");
    const contexts: BrowserContext[] = [];
    const actors: {
      principal: string;
      agent: string;
      token: string;
      intent?: string;
    }[] = [];
    try {
      for (const label of ["buyer-a", "buyer-b", "builder-c"]) {
        const context = await browser.newContext({
          ignoreHTTPSErrors: true,
          baseURL: process.env.UNDERGRID_HP1_BASE_URL,
        });
        contexts.push(context);
        // A test-provider identity selector, not an app cookie/session bypass.
        // The issuer binds this subject into its one-use PKCE authorization grant.
        await context.addCookies([
          { name: "hp2-fixture-subject", value: label, url: issuer },
        ]);
        const page = await context.newPage();
        await page.goto("/");
        await page.getByRole("link", { name: "Sign in", exact: true }).click();
        await expect(
          page.getByRole("heading", { name: "Register your owner account" }),
        ).toBeVisible();
        await page
          .getByLabel("Owner name", { exact: true })
          .fill(`HP2 controlled ${label}`);
        await page
          .getByLabel("I accept terms hp2-local-fixture-terms", { exact: true })
          .check();
        await page
          .getByRole("button", { name: "Register", exact: true })
          .click();
        await expect(
          page.getByRole("heading", { name: "Your workspace", exact: true }),
        ).toBeVisible();
        await page
          .getByLabel("Agent name", { exact: true })
          .fill(`HP2 ${label} agent`);
        const issuance = page.waitForResponse(
          (r) =>
            new URL(r.url()).pathname === "/v1/agents" &&
            r.request().method() === "POST",
        );
        await page
          .getByRole("button", { name: "Register agent", exact: true })
          .click();
        const issued = await issuance;
        expect(issued.status()).toBe(201);
        const credential = await issued.json();
        expect(credential.token).toMatch(/^ug_/);
        await page
          .getByRole("button", { name: "Dismiss credential", exact: true })
          .click();
        const identityResponse = await page.request.get("/v1/me", {
          headers: { Authorization: `Bearer ${credential.token}` },
        });
        expect(identityResponse.status()).toBe(200);
        const identity = await identityResponse.json();
        expect(identity.agent_id).toBe(credential.id);
        const actor = {
          principal: identity.principal_id as string,
          agent: identity.agent_id as string,
          token: credential.token as string,
          intent: undefined as string | undefined,
        };
        if (label !== "builder-c") {
          await page
            .getByLabel("Objective", { exact: true })
            .fill(`Private environmental catalogue requirements ${label}`);
          await expect(
            page.getByRole("combobox", {
              name: "Intention visibility",
              exact: true,
            }),
          ).toHaveValue("private");
          await expect(
            page.getByLabel("Monitor matches in the agent inbox", {
              exact: true,
            }),
          ).toBeChecked();
          if (publication && label === "buyer-a") {
            await page
              .getByRole("combobox", {
                name: "Intention visibility",
                exact: true,
              })
              .selectOption("redacted_public");
            await page
              .getByLabel("Public summary", { exact: true })
              .fill("Public environmental catalogue");
            const preview = page.getByRole("region", {
              name: "Public projection preview",
              exact: true,
            });
            await expect(preview).toContainText(
              "Public environmental catalogue",
            );
            await expect(preview).not.toContainText(
              "Private environmental catalogue requirements",
            );
            await page
              .getByLabel("I reviewed this public projection", { exact: true })
              .check();
          }
          const creation = page.waitForResponse(
            (r) =>
              new URL(r.url()).pathname === "/v1/owner/intents" &&
              r.request().method() === "POST",
          );
          await page
            .getByRole("button", { name: "Create intention", exact: true })
            .click();
          const created = await creation;
          expect(created.status()).toBe(201);
          const intention = await created.json();
          actor.intent = intention.id;
          const own = await page.request.get(
            `/v1/intents/${encodeURIComponent(intention.id)}`,
            { headers: { Authorization: `Bearer ${credential.token}` } },
          );
          expect(own.status()).toBe(200);
          const record = await own.json();
          expect(record.visibility).toBe(
            publication && label === "buyer-a" ? "redacted_public" : "private",
          );
          expect(record.state).toBe("active");
          expect(record.watch.enabled).toBe(true);
          expect(record.hard_constraints.required_fields).toEqual([
            "source_id",
            "source_url",
          ]);
        }
        actors.push(actor);
        expect(
          await page.evaluate(() => [
            localStorage.length,
            sessionStorage.length,
          ]),
        ).toEqual([0, 0]);
        await page.reload();
        await expect(
          page.getByRole("heading", { name: "Your workspace", exact: true }),
        ).toBeVisible();
      }
      expect(new Set(actors.map((a) => a.principal)).size).toBe(3);
      expect(new Set(actors.map((a) => a.agent)).size).toBe(3);
      if (publication) {
        const target = `/v1/intents/${encodeURIComponent(actors[0].intent!)}`;
        const headers = { Authorization: `Bearer ${actors[1].token}` };
        const visible = await contexts[1].request.get(target, { headers });
        expect(visible.status()).toBe(200);
        const projection = await visible.json();
        expect(Object.keys(projection).sort()).toEqual([
          "etag",
          "hard_constraints",
          "id",
          "output",
          "public_summary",
          "state",
          "version",
        ]);
        expect(projection.public_summary).toBe(
          "Public environmental catalogue",
        );
        expect(JSON.stringify(projection)).not.toContain(
          "Private environmental catalogue requirements",
        );
        const ownerPage = contexts[0].pages()[0];
        await ownerPage
          .getByRole("button", { name: "Review intention", exact: true })
          .click();
        await ownerPage
          .getByRole("button", { name: "Edit intention", exact: true })
          .click();
        await ownerPage
          .getByRole("combobox", { name: "Revision visibility", exact: true })
          .selectOption("private");
        const change = ownerPage.waitForResponse(
          (r) =>
            new URL(r.url()).pathname ===
              `/v1/owner/intents/${actors[0].intent}` &&
            r.request().method() === "PATCH",
        );
        await ownerPage
          .getByRole("button", { name: "Save new revision", exact: true })
          .click();
        const changed = await change;
        expect(changed.status()).toBe(200);
        expect((await changed.json()).visibility).toBe("private");
        expect(
          (await contexts[1].request.get(target, { headers })).status(),
        ).toBe(404);
        await ownerPage.reload();
        await ownerPage
          .getByRole("button", { name: "Review intention", exact: true })
          .click();
        await ownerPage
          .getByRole("button", { name: "Edit intention", exact: true })
          .click();
        await expect(
          ownerPage.getByRole("combobox", {
            name: "Revision visibility",
            exact: true,
          }),
        ).toHaveValue("private");
      }
      for (let owner = 0; owner < actors.length; owner++) {
        for (const target of actors.filter(
          (a) => a.intent && a.principal !== actors[owner].principal,
        )) {
          const inaccessible = await contexts[owner].request.get(
            `/v1/intents/${encodeURIComponent(target.intent!)}`,
            {
              headers: { Authorization: `Bearer ${actors[owner].token}` },
            },
          );
          expect(inaccessible.status()).toBe(404);
        }
      }
      // Names identify intended future roles only. No builder capability, opt-in,
      // independent ownership, collaboration or participant consent is inferred.
    } finally {
      for (const context of contexts) await context.close();
    }
  },
);
