import { proveOwnerContributions } from "./contribution-journey";
import { proveOwnerTaskExecution } from "./task-execution-journey";
import {
  configurePlanPolicy,
  proveOwnerPlanJourney,
  type Actor,
} from "./owner-plan-journey";
import { admitPeerAndExchangeMessage } from "./hp2-continuous-journey";
import { proveOwnerApplications } from "./owner-applications-journey";
import { test, expect, type BrowserContext } from "@playwright/test";

// Actual network app/session/API. Separate modes prove setup, publication or
// reviewed builder registration/recruitment; none proves the full HP2 journey.
test(
  process.env.UNDERGRID_HP2_CONTINUOUS === "1"
    ? "same owners discover demand, recruit and admit, then approve exact plans and separately accept work across restart"
    : process.env.UNDERGRID_HP2_ROOM_ORIGIN === "1"
      ? "owner selects a current match, recovers a reviewed room, and receives recruitment without admission"
      : process.env.UNDERGRID_HP2_BUILDER_REGISTRATION === "1"
        ? "owner reviews builder declarations and receives worker recruitment without admission"
        : process.env.UNDERGRID_HP2_OWNER_PUBLICATION === "1"
          ? "owner publishes a selected projection and revokes its foreign read through the console"
          : "three controlled owners receive separate scoped agents and private watches",
  async ({ browser }) => {
    const continuous = process.env.UNDERGRID_HP2_CONTINUOUS === "1";
    const applications = process.env.UNDERGRID_HP2_APPLICATIONS === "1";
    const roomOrigin = process.env.UNDERGRID_HP2_ROOM_ORIGIN === "1";
    const builderRegistration =
      roomOrigin || process.env.UNDERGRID_HP2_BUILDER_REGISTRATION === "1";
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
        if (builderRegistration && label === "buyer-a") {
          for (const scope of [
            "swarms:create",
            "members:invite",
            "swarms:read",
          ])
            await page.getByLabel(scope, { exact: true }).check();
        }
        if (continuous && label === "buyer-b")
          await page
            .getByLabel("Accept foreign invitations", { exact: true })
            .check();
        if (builderRegistration && label === "builder-c") {
          await page.getByLabel("swarms:read", { exact: true }).check();
          await page
            .getByLabel("Make declared profile public for recruitment", {
              exact: true,
            })
            .check();
          await page
            .getByLabel("Receive recruitment opportunities", { exact: true })
            .check();
          await page
            .getByLabel("Declare source_catalog_v1 capability", { exact: true })
            .check();
          await expect(
            page.getByLabel("Accept foreign invitations", { exact: true }),
          ).not.toBeChecked();
        }
        await page
          .getByLabel("I reviewed this declared profile", { exact: true })
          .check();
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
            .fill(
              roomOrigin && label === "buyer-a"
                ? "Shared environmental catalogue"
                : `Private environmental catalogue requirements ${label}`,
            );
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
          const publicIntention =
            (publication && label === "buyer-a") ||
            (roomOrigin && label === "buyer-b");
          if (publicIntention) {
            await page
              .getByRole("combobox", {
                name: "Intention visibility",
                exact: true,
              })
              .selectOption("redacted_public");
            await page
              .getByRole("textbox", { name: "Public summary", exact: true })
              .fill(
                roomOrigin
                  ? "Shared environmental catalogue"
                  : "Public environmental catalogue",
              );
            const preview = page.getByRole("region", {
              name: "Public projection preview",
              exact: true,
            });
            await expect(preview).toContainText(
              roomOrigin
                ? "Shared environmental catalogue"
                : "Public environmental catalogue",
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
            publicIntention ? "redacted_public" : "private",
          );
          expect(record.state).toBe("active");
          expect(record.watch.enabled).toBe(true);
          expect(record.hard_constraints.required_fields).toEqual([
            "source_id",
            "source_url",
          ]);
        }
        if (builderRegistration && label === "builder-c") {
          const saved = page.getByRole("region", {
            name: "Saved profile for HP2 builder-c agent",
            exact: true,
          });
          await expect(saved).toContainText('"recruitment": true');
          await page.reload();
          await expect(saved).toContainText('"accept_invitations": false');
          await expect(saved).toContainText('"source_catalog_v1"');
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
      const planActors: Actor[] = actors.map((a, i) => ({
        ...a,
        context: contexts[i],
        page: contexts[i].pages()[0],
      }));
      if (continuous)
        for (const actor of planActors) await configurePlanPolicy(actor.page);
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
        await expect(
          ownerPage.getByRole("combobox", {
            name: "Revision visibility",
            exact: true,
          }),
        ).toHaveCount(0);
        const revisionForm = ownerPage.locator("form").filter({
          has: ownerPage.getByRole("heading", {
            name: "New intention revision",
            exact: true,
          }),
        });
        await revisionForm
          .getByRole("textbox", { name: "Public summary", exact: true })
          .fill("Updated public environmental catalogue");
        await revisionForm
          .getByLabel("I reviewed this public projection", { exact: true })
          .check();
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
        const updated = await changed.json();
        expect(updated.visibility).toBe("redacted_public");
        expect(updated.current_revision).toBe(2);
        const revisedProjection = await contexts[1].request.get(target, {
          headers,
        });
        expect(revisedProjection.status()).toBe(200);
        expect((await revisedProjection.json()).public_summary).toBe(
          "Updated public environmental catalogue",
        );
        // Audience conversion remains a rejected generic revision. Explicit
        // withdrawal is the current supported way to stop foreign discovery.
        const forbidden = await ownerPage.request.patch(target, {
          headers: {
            Authorization: `Bearer ${actors[0].token}`,
            "If-Match": updated.etag,
            "Idempotency-Key": "publication-visibility-remains-fixed",
          },
          data: {
            revision: {
              objective: updated.objective,
              output: updated.output,
              hard_constraints: updated.hard_constraints,
              visibility: "private",
              budget: updated.budget,
              watch: updated.watch,
              expires_at: updated.expires_at,
              ...(updated.preferences === undefined
                ? {}
                : { preferences: updated.preferences }),
            },
          },
        });
        expect(forbidden.status()).toBe(409);
        expect((await forbidden.json()).error.code).toBe(
          "visibility_change_requires_grant",
        );
        const withdrawalResponse = ownerPage.waitForResponse(
          (r) =>
            new URL(r.url()).pathname ===
              `/v1/owner/intents/${actors[0].intent}` &&
            r.request().method() === "PATCH",
        );
        await ownerPage
          .getByRole("button", { name: "Withdraw", exact: true })
          .click();
        const withdrawal = await withdrawalResponse;
        expect(withdrawal.status()).toBe(200);
        const withdrawn = await withdrawal.json();
        expect(withdrawn.visibility).toBe("redacted_public");
        expect(withdrawn.state).toBe("withdrawn");
        expect(withdrawn.watch.enabled).toBe(false);
        expect(withdrawn.current_revision).toBe(2);
        expect(
          (await contexts[1].request.get(target, { headers })).status(),
        ).toBe(404);
        await ownerPage.reload();
        await ownerPage
          .getByRole("button", { name: "Review intention", exact: true })
          .click();
        await expect(
          ownerPage.getByRole("button", {
            name: "Edit intention",
            exact: true,
          }),
        ).toHaveCount(0);
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
          expect(inaccessible.status()).toBe(
            roomOrigin && target.agent === actors[1].agent ? 200 : 404,
          );
        }
      }
      if (builderRegistration) {
        const creator = contexts[0].pages()[0];
        let room: { id: string };
        if (roomOrigin) {
          let selected = "";
          await expect
            .poll(
              async () => {
                const response = await creator.request.get(
                  `/v1/intents/${actors[0].intent}/opportunities`,
                  { headers: { Authorization: `Bearer ${actors[0].token}` } },
                );
                expect(response.status()).toBe(200);
                const result = await response.json();
                const matching = result.items.filter(
                  (item: { id: string; candidate_intent_id: string }) =>
                    item.candidate_intent_id === actors[1].intent,
                );
                if (matching.length === 1) selected = matching[0].id;
                return matching.length;
              },
              { timeout: 5000 },
            )
            .toBe(1);
          await creator
            .getByRole("button", { name: "Review intention", exact: true })
            .click();
          if (continuous) {
            const groups = creator.getByRole("region", {
              name: "Compatible groups",
              exact: true,
            });
            await groups
              .getByRole("button", {
                name: "View compatible groups",
                exact: true,
              })
              .click();
            await expect(
              groups.getByRole("heading", {
                name: "Compatible view 1",
                exact: true,
              }),
            ).toBeVisible();
            await expect(groups).toContainText(actors[0].intent!);
            await expect(groups).toContainText(actors[1].intent!);
            await expect(groups).toContainText(
              "Exact declared outcome and hard constraints",
            );
            const capabilities = creator.getByRole("region", {
              name: "Registered capability discovery",
              exact: true,
            });
            await capabilities
              .getByRole("button", {
                name: "Discover registered capabilities",
                exact: true,
              })
              .click();
            const declared = capabilities.getByRole("article", {
              name: `Capability ${actors[2].agent}`,
              exact: true,
            });
            await expect(declared).toContainText("Declared schema support");
            await expect(declared).toContainText(actors[2].principal);
          }
          await creator
            .getByRole("button", {
              name: "Create swarm from this match",
              exact: true,
            })
            .click();
          const form = creator.getByRole("region", {
            name: "Create swarm",
            exact: true,
          });
          await expect(
            form.getByRole("combobox", {
              name: "Responsible agent",
              exact: true,
            }),
          ).toHaveValue(actors[0].agent);
          await expect(
            form.getByRole("combobox", {
              name: "Responsible agent",
              exact: true,
            }),
          ).toBeDisabled();
          await form
            .getByLabel("Swarm objective", { exact: true })
            .fill("Build the bounded declared catalogue");
          await form
            .getByRole("combobox", { name: "Swarm visibility", exact: true })
            .selectOption("restricted");
          await form
            .getByRole("button", { name: "Add recruitment role", exact: true })
            .click();
          await form
            .getByLabel("Role key", { exact: true })
            .fill("catalog-builder");
          await form
            .getByLabel("Role description", { exact: true })
            .fill("Build and validate catalogue data");
          await expect(
            form.getByRole("region", {
              name: "Room creation review",
              exact: true,
            }),
          ).toContainText(selected);
          await form
            .getByLabel(
              "I understand admitted members can read the full room history.",
              { exact: true },
            )
            .check();
          await form
            .getByLabel("I reviewed this room setup", { exact: true })
            .check();
          const path = `/v1/owner/agents/${actors[0].agent}/swarms`;
          let originalBody: string | null = null,
            originalKey: string | undefined,
            committed: { id: string } | undefined;
          await creator.route(`**${path}`, async (route) => {
            originalBody = route.request().postData();
            originalKey = route.request().headers()["idempotency-key"];
            const response = await route.fetch();
            expect(response.status()).toBe(201);
            committed = await response.json();
            await route.abort("failed");
          });
          await form
            .getByRole("button", { name: "Create swarm", exact: true })
            .click();
          await expect(form.getByRole("alert")).toContainText(
            "could not be confirmed",
          );
          await creator.unroute(`**${path}`);
          const retry = creator.waitForResponse(
            (response) =>
              new URL(response.url()).pathname === path &&
              response.request().method() === "POST",
          );
          await form
            .getByRole("button", { name: "Create swarm", exact: true })
            .click();
          const recovered = await retry;
          expect(recovered.status()).toBe(201);
          room = await recovered.json();
          expect(room.id).toBe(committed!.id);
          expect(recovered.request().postData()).toBe(originalBody);
          expect(recovered.request().headers()["idempotency-key"]).toBe(
            originalKey,
          );
          expect(JSON.parse(originalBody!)).toEqual({
            objective: "Build the bounded declared catalogue",
            visibility: "restricted",
            history_policy: "full",
            origin_opportunity_id: selected,
            recruitment_roles: [
              {
                key: "catalog-builder",
                schema_family: "source_catalog_v1",
                description: "Build and validate catalogue data",
              },
            ],
          });
          await expect(form.getByRole("status")).toContainText(room.id);
          await creator.reload();
          await expect(
            creator.getByRole("heading", {
              name: "Your workspace",
              exact: true,
            }),
          ).toBeVisible();
        } else {
          const response = await creator.request.post("/v1/swarms", {
            headers: {
              Authorization: `Bearer ${actors[0].token}`,
              "Idempotency-Key": "hp2-builder-role-room",
            },
            data: {
              objective: "Build the bounded declared catalogue",
              visibility: "restricted",
              recruitment_roles: [
                {
                  key: "catalog-builder",
                  schema_family: "source_catalog_v1",
                  description: "Build and validate catalogue data",
                },
              ],
            },
          });
          expect(response.status()).toBe(201);
          room = await response.json();
        }
        const builder = contexts[2].pages()[0];
        let offers: { items: { swarm_id: string; role_key: string }[] } = {
          items: [],
        };
        await expect
          .poll(
            async () => {
              const response = await builder.request.get(
                "/v1/opportunities?kind=builder_recruitment",
                { headers: { Authorization: `Bearer ${actors[2].token}` } },
              );
              expect(response.status()).toBe(200);
              offers = await response.json();
              return offers.items.filter(
                (item) =>
                  item.swarm_id === room.id &&
                  item.role_key === "catalog-builder",
              ).length;
            },
            { timeout: 5_000 },
          )
          .toBe(1);
        const blocked = await creator.request.post(
          `/v1/swarms/${room.id}/invitations`,
          {
            headers: {
              Authorization: `Bearer ${actors[0].token}`,
              "Idempotency-Key": "hp2-builder-invitation-off",
            },
            data: {
              agent_id: actors[2].agent,
              role: "contributor",
              respond_by: new Date(Date.now() + 3600000).toISOString(),
            },
          },
        );
        expect(blocked.status()).toBe(404);
        const context = await builder.request.get(
          `/v1/swarms/${room.id}/context`,
          { headers: { Authorization: `Bearer ${actors[2].token}` } },
        );
        expect(context.status()).toBe(404);
        if (applications)
          await proveOwnerApplications(
            creator,
            builder,
            contexts[1].pages()[0],
            room.id,
            actors[0].agent,
            actors[2].agent,
            actors[2].token,
          );
        if (continuous) {
          await admitPeerAndExchangeMessage(planActors, room.id);
          await proveOwnerPlanJourney(planActors, room.id);
          if (process.env.UNDERGRID_HP3_TASK_EXECUTION === "1")
            await proveOwnerTaskExecution(planActors[2]);
          if (process.env.UNDERGRID_HP3_OWNER_CONTRIBUTIONS === "1")
            await proveOwnerContributions(planActors);
        }
        const own = await builder.request.get("/v1/agents");
        expect(own.status()).toBe(200);
        expect(
          (await own.json()).items.map((item: { id: string }) => item.id),
        ).toEqual([actors[2].agent]);
      }
      // Role names alone prove no capability or consent. The builder mode above
      // proves explicit declarations; independent ownership and full HP2 remain open.
    } finally {
      for (const context of contexts) await context.close();
    }
  },
);
