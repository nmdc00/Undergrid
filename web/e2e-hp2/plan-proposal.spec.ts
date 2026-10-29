import { test, expect } from "@playwright/test";
import {
  registerOwner,
  proveOwnerPlanJourney,
  type Actor,
} from "./owner-plan-journey";

test("owners activate an exact multitask plan then independently activate a revision while preserving the prior accepted head", async ({
  browser,
}) => {
  const actors: Actor[] = [];
  try {
    for (const label of ["buyer-a", "buyer-b", "builder-c"])
      actors.push(await registerOwner(browser, label));
    expect(new Set(actors.map((actor) => actor.principal)).size).toBe(3); // Authority separation, not beneficial-owner independence.
    const creator = actors[0],
      builder = actors[2];
    const created = await creator.page.request.post("/v1/swarms", {
      headers: {
        Authorization: `Bearer ${creator.token}`,
        "Idempotency-Key": "plan-fixture-room",
      },
      data: {
        objective: "Explicit API fixture room for owner plan controls",
        visibility: "restricted",
        recruitment_roles: [],
      },
    });
    expect(created.status()).toBe(201);
    const room = (await created.json()).id as string;
    for (const participant of actors.slice(1)) {
      const offered = await creator.page.request.post(
        `/v1/swarms/${room}/invitations`,
        {
          headers: {
            Authorization: `Bearer ${creator.token}`,
            "Idempotency-Key": `plan-fixture-offer-${participant.agent}`,
          },
          data: {
            agent_id: participant.agent,
            role: "contributor",
            respond_by: new Date(Date.now() + 3600000).toISOString(),
          },
        },
      );
      expect(offered.status()).toBe(201);
      await participant.page
        .getByRole("button", { name: "Load participation", exact: true })
        .click();
      await participant.page
        .getByRole("button", { name: "Review invitation", exact: true })
        .click();
      const admission = participant.page.waitForResponse(
        (r) =>
          new URL(r.url()).pathname.endsWith("/membership-response") &&
          r.request().method() === "POST",
      );
      await participant.page
        .getByRole("button", { name: "Accept invitation", exact: true })
        .click();
      const accepted = await admission;
      expect(accepted.status()).toBe(200);
      expect((await accepted.json()).state).toBe("accepted");
    }
    await proveOwnerPlanJourney(actors, room);
  } finally {
    for (const actor of actors) await actor.context.close();
  }
});
