import { createHash } from "node:crypto";
import {
  test,
  expect,
  type BrowserContext,
  type Page,
  type Browser,
} from "@playwright/test";

// Controlled signed issuer and normal browser/API commands. Fixture API room
// setup is explicit; this case does not complete HP2's missing room-origin UI.
type Actor = {
  context: BrowserContext;
  page: Page;
  agent: string;
  principal: string;
  token: string;
};
type Plan = {
  id: string;
  version: number;
  state: string;
  swarm_id: string;
  content_hash: string;
  canonical_json: string;
  specification: {
    parent_version: number | null;
    objective: string;
    tasks: { key: string; depends_on: string[] }[];
  };
};
type Task = {
  id: string;
  state: string;
  version: number;
  claim_epoch: number;
  dependency_blocked: boolean;
  specification: { key: string; depends_on: string[] };
  [key: string]: unknown;
};
type StoredPlan = {
  id: string;
  version: number;
  state: string;
  parent_id: string | null;
  content_hash: string;
  sealed: unknown;
  plan_votes: number;
  rights_votes: number;
  tasks: Task[];
};
type Checkpoint = {
  id: string;
  active_plan_id: string | null;
  version: number;
  plans: StoredPlan[];
};

async function registerOwner(browser: Browser, label: string): Promise<Actor> {
  const issuer = process.env.UNDERGRID_HP2_ISSUER_URL;
  if (!issuer) throw Error("Controlled issuer required");
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
  for (const scope of label === "buyer-a"
    ? ["swarms:read", "swarms:create", "members:invite"]
    : ["swarms:read"])
    await page.getByLabel(scope, { exact: true }).check();
  if (label !== "buyer-a")
    await page
      .getByLabel("Accept foreign invitations", { exact: true })
      .check();
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
  const me = await page.request.get("/v1/me", {
    headers: { Authorization: `Bearer ${credential.token}` },
  });
  expect(me.status()).toBe(200);
  const identity = await me.json();
  expect(identity.agent_id).toBe(credential.id);
  if (label !== "builder-c") {
    await page
      .getByRole("textbox", { name: "Objective", exact: true })
      .fill(`Private environmental catalogue requirements ${label}`);
    const creation = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname === "/v1/owner/intents" &&
        r.request().method() === "POST",
    );
    await page
      .getByRole("button", { name: "Create intention", exact: true })
      .click();
    const response = await creation;
    expect(response.status()).toBe(201);
    const intent = await response.json();
    expect(intent.visibility).toBe("private");
    expect(intent.watch.enabled).toBe(true);
  }
  // A successful creation response precedes the console's workspace refresh.
  // Wait for that action to finish before editing the refreshed policy record.
  await expect(
    page.getByRole("button", { name: "Refresh", exact: true }),
  ).toBeEnabled();
  await page.getByRole("checkbox", { name: /test-contribution-v1/ }).check();
  await page.getByRole("checkbox", { name: "CC0-1.0", exact: true }).check();
  await page.getByLabel("Permit JSON dataset tasks", { exact: true }).check();
  await page.getByLabel("Maximum active tasks", { exact: true }).fill("8");
  await page
    .getByLabel("Policy expires at (UTC)", { exact: true })
    .fill(new Date(Date.now() + 7 * 86400000).toISOString().slice(0, 16));
  await expect(
    page.getByRole("checkbox", { name: /test-contribution-v1/ }),
  ).toBeChecked();
  await expect(
    page.getByLabel("Maximum active tasks", { exact: true }),
  ).toHaveValue("8");
  const savedPolicy = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === "/v1/owner/policy" &&
      r.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Save policy", exact: true }).click();
  expect((await savedPolicy).status()).toBe(200);
  await expect(
    page.getByRole("button", { name: "Save policy", exact: true }),
  ).toBeEnabled();
  const persistedPolicy = await page.request.get("/v1/owner/policy");
  expect(persistedPolicy.status()).toBe(200);
  expect((await persistedPolicy.json()).policy).toMatchObject({
    template_ids: ["test-contribution-v1"],
    license_ids: ["CC0-1.0"],
    task_categories: ["json_dataset"],
    max_active_tasks: 8,
  });
  expect(
    await page.evaluate(() => [localStorage.length, sessionStorage.length]),
  ).toEqual([0, 0]);
  return {
    context,
    page,
    agent: identity.agent_id,
    principal: identity.principal_id,
    token: credential.token,
  };
}
async function openRoom(actor: Actor) {
  await actor.page
    .getByRole("button", { name: "Load participation", exact: true })
    .click();
  await actor.page
    .getByRole("button", { name: "Open room", exact: true })
    .click();
  await expect(
    actor.page.getByRole("heading", { name: "Plan proposal", exact: true }),
  ).toBeVisible();
}
async function checkpoint(actor: Actor): Promise<Checkpoint> {
  const response = await actor.page.request.get("/__fixture/plan-checkpoint");
  expect(response.status()).toBe(200);
  return await response.json();
}
function stored(snapshot: Checkpoint, plan: Plan): StoredPlan {
  const found = snapshot.plans.find((row) => row.id === plan.id);
  if (!found) throw Error(`Missing normalized plan ${plan.id}`);
  return found;
}
function verifySeal(plan: Plan) {
  expect(createHash("sha256").update(plan.canonical_json).digest("hex")).toBe(
    plan.content_hash,
  );
}
function unchangedAccepted(snapshot: Checkpoint, initial: StoredPlan) {
  expect(snapshot.active_plan_id).toBe(initial.id);
  const current = snapshot.plans.find((plan) => plan.id === initial.id)!;
  expect(current.state).toBe("accepted");
  expect(current.content_hash).toBe(initial.content_hash);
  expect(current.sealed).toEqual(initial.sealed);
  expect(current.tasks).toEqual(initial.tasks);
}
async function propose(
  actor: Actor,
  actors: Actor[],
  room: string,
  revision: boolean,
): Promise<Plan> {
  const page = actor.page;
  await page
    .getByRole("button", { name: "Load plan context", exact: true })
    .click();
  if (revision)
    await page
      .getByRole("button", {
        name: "Load latest plan for revision",
        exact: true,
      })
      .click();
  await page
    .getByRole("textbox", { name: "Plan objective", exact: true })
    .fill(
      revision
        ? "An independently reviewed immutable catalogue revision"
        : "A bounded two-task source catalogue plan",
    );
  if (!revision) {
    for (const participant of actors) {
      await page
        .getByLabel(`Required approval: ${participant.principal}`, {
          exact: true,
        })
        .check();
      await page
        .getByLabel(`Publication principal: ${participant.principal}`, {
          exact: true,
        })
        .check();
    }
    for (const participant of actors.slice(0, 2))
      await page
        .getByLabel(`Acceptance principal: ${participant.principal}`, {
          exact: true,
        })
        .check();
    await page
      .getByRole("combobox", { name: "Contribution template", exact: true })
      .selectOption("test-contribution-v1");
    await page
      .getByRole("combobox", { name: "Dataset license", exact: true })
      .selectOption("CC0-1.0");
    await page.getByLabel("Task 1 key", { exact: true }).fill("collect");
    await page
      .getByRole("combobox", { name: "Task 1 assignee", exact: true })
      .selectOption(actors[2].agent);
    const deadline = new Date(Date.now() + 2 * 86400000)
      .toISOString()
      .replace(/\.\d{3}Z$/, ".123Z")
      .slice(0, -1);
    await page
      .getByLabel("Task 1 final deadline (UTC, optional)", { exact: true })
      .fill(deadline);
    await page.getByRole("button", { name: "Add task", exact: true }).click();
    await page.getByLabel("Task 2 key", { exact: true }).fill("assemble");
    await page
      .getByRole("combobox", { name: "Task 2 assignee", exact: true })
      .selectOption(actors[2].agent);
    await page
      .getByLabel("Task 2 final deadline (UTC, optional)", { exact: true })
      .fill(deadline);
    await page.getByLabel("Task 2 depends on collect", { exact: true }).check();
  }
  await page
    .getByLabel("I reviewed this plan proposal", { exact: true })
    .check();
  const write = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname ===
        `/v1/owner/agents/${actor.agent}/swarms/${room}/plans` &&
      r.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Propose plan", exact: true }).click();
  const response = await write;
  expect(response.status()).toBe(201);
  const plan = (await response.json()) as Plan;
  expect(plan.version).toBe(revision ? 2 : 1);
  expect(plan.specification.parent_version).toBe(revision ? 1 : null);
  expect(plan.state).toBe("proposed");
  expect(plan.swarm_id).toBe(room);
  verifySeal(plan);
  await expect(
    page.getByRole("status").filter({ hasText: `Plan proposed: ${plan.id}` }),
  ).toBeVisible();
  return plan;
}
async function readPlan(actor: Actor, plan: Plan) {
  const page = actor.page;
  await page.getByRole("button", { name: "Load plans", exact: true }).click();
  const read = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname ===
        `/v1/owner/agents/${actor.agent}/plans/${plan.id}` &&
      r.request().method() === "GET",
  );
  await page
    .getByRole("button", { name: `Review plan ${plan.version}`, exact: true })
    .click();
  const response = await read;
  expect(response.status()).toBe(200);
  const reviewed = (await response.json()) as Plan;
  expect(reviewed.id).toBe(plan.id);
  expect(reviewed.content_hash).toBe(plan.content_hash);
  verifySeal(reviewed);
  await expect(
    page.getByRole("button", { name: "Review my assignments", exact: true }),
  ).toBeVisible();
  return reviewed;
}
async function vote(actor: Actor, plan: Plan) {
  await readPlan(actor, plan);
  const page = actor.page;
  await page
    .getByRole("checkbox", {
      name: /I approve the contribution terms and rights for this exact plan hash/,
    })
    .check();
  const write = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname ===
        `/v1/owner/agents/${actor.agent}/plans/${plan.id}/approvals` &&
      r.request().method() === "POST",
  );
  await page
    .getByRole("button", {
      name: "Approve exact plan and contribution terms",
      exact: true,
    })
    .click();
  expect((await write).status()).toBe(200);
}
async function acceptTask(actor: Actor, key: string) {
  const page = actor.page,
    task = page
      .getByRole("heading", { name: `Task ${key}`, exact: true })
      .locator("..");
  await task
    .getByRole("checkbox", { name: /I accept this task's responsibility/ })
    .check();
  const write = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname.includes(
        `/v1/owner/agents/${actor.agent}/tasks/`,
      ) &&
      new URL(r.url()).pathname.endsWith("/assignment-response") &&
      r.request().method() === "POST",
  );
  await task
    .getByRole("button", {
      name: "Accept assignment responsibility",
      exact: true,
    })
    .click();
  expect((await write).status()).toBe(200);
  await expect(task.getByRole("status", { name: "" })).toContainText(
    "Assignment acceptance recorded.",
  );
}

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
    for (const actor of actors) await openRoom(actor);
    const first = await propose(creator, actors, room, false);
    let snapshot = await checkpoint(creator);
    expect(snapshot.active_plan_id).toBeNull();
    expect(stored(snapshot, first).plan_votes).toBe(0);
    expect(stored(snapshot, first).rights_votes).toBe(0);
    expect(stored(snapshot, first).tasks.map((task) => task.state)).toEqual([
      "offered",
      "offered",
    ]);
    for (let index = 0; index < actors.length; index++) {
      await vote(actors[index], first);
      snapshot = await checkpoint(creator);
      expect(stored(snapshot, first).plan_votes).toBe(index + 1);
      expect(stored(snapshot, first).rights_votes).toBe(index + 1);
      expect(snapshot.active_plan_id).toBeNull();
      expect(stored(snapshot, first).state).toBe("proposed");
    }
    await builder.page
      .getByRole("button", { name: "Review my assignments", exact: true })
      .click();
    await acceptTask(builder, "collect");
    snapshot = await checkpoint(creator);
    expect(snapshot.active_plan_id).toBeNull();
    await acceptTask(builder, "assemble");
    snapshot = await checkpoint(creator);
    const initial = stored(snapshot, first);
    expect(initial.state).toBe("accepted");
    expect(snapshot.active_plan_id).toBe(first.id);
    expect(initial.tasks.map((task) => task.state)).toEqual([
      "accepted",
      "accepted",
    ]);
    expect(initial.tasks[0].specification.depends_on).toEqual(["collect"]); // SQL order is assemble, collect.
    const second = await propose(creator, actors, room, true);
    snapshot = await checkpoint(creator);
    unchangedAccepted(snapshot, initial);
    expect(stored(snapshot, second).parent_id).toBe(first.id);
    expect(stored(snapshot, second).plan_votes).toBe(0);
    expect(stored(snapshot, second).rights_votes).toBe(0);
    for (let index = 0; index < actors.length; index++) {
      await vote(actors[index], second);
      snapshot = await checkpoint(creator);
      unchangedAccepted(snapshot, initial);
      expect(stored(snapshot, second).plan_votes).toBe(index + 1);
      expect(stored(snapshot, second).rights_votes).toBe(index + 1);
    }
    await builder.page
      .getByRole("button", { name: "Review my assignments", exact: true })
      .click();
    await acceptTask(builder, "collect");
    unchangedAccepted(await checkpoint(creator), initial);
    await acceptTask(builder, "assemble");
    snapshot = await checkpoint(creator);
    expect(snapshot.active_plan_id).toBe(second.id);
    expect(stored(snapshot, second).state).toBe("accepted");
    expect(stored(snapshot, second).tasks.map((task) => task.state)).toEqual([
      "accepted",
      "accepted",
    ]);
    const prior = stored(snapshot, first);
    expect(prior.state).toBe("superseded");
    expect(prior.sealed).toEqual(initial.sealed);
    expect(prior.content_hash).toBe(initial.content_hash);
    for (const task of prior.tasks) {
      const old = initial.tasks.find((old) => old.id === task.id)!;
      expect(task.state).toBe("cancelled");
      expect(task.claim_epoch).toBe(old.claim_epoch + 1);
      expect(task.version).toBe(old.version + 1);
    }
    const beforeRestart = snapshot;
    const restarted = await creator.page.request.post(
      "/__fixture/restart-plan-worker",
    );
    expect(restarted.status()).toBe(200);
    expect(await restarted.json()).toEqual({
      restarted: true,
      completed_startup_job: true,
    });
    expect(await checkpoint(creator)).toEqual(beforeRestart);
    for (const actor of actors) {
      await actor.page.reload();
      await openRoom(actor);
      expect((await readPlan(actor, second)).state).toBe("accepted");
    }
    await builder.page
      .getByRole("button", { name: "Review my assignments", exact: true })
      .click();
    await expect(
      builder.page.getByRole("button", {
        name: "Accept assignment responsibility",
        exact: true,
      }),
    ).toHaveCount(0);
    const recovery = await builder.page.request.get("/v1/agent-context", {
      headers: { Authorization: `Bearer ${builder.token}` },
    });
    expect(recovery.status()).toBe(200);
    const assignments = (await recovery.json()).assignments as {
      id: string;
      plan_id: string;
      plan_hash: string;
      state: string;
      claim_epoch: number;
    }[];
    expect(assignments).toHaveLength(2);
    for (const task of assignments) {
      expect(task.plan_id).toBe(second.id);
      expect(task.plan_hash).toBe(second.content_hash);
      expect(task.state).toBe("accepted");
      expect(task.claim_epoch).toBe(
        stored(beforeRestart, second).tasks.find((row) => row.id === task.id)!
          .claim_epoch,
      );
    }
    expect(await checkpoint(creator)).toEqual(beforeRestart);
  } finally {
    for (const actor of actors) await actor.context.close();
  }
});
