import { expect, type Page } from "@playwright/test";

// All mutations use owner controls. Agent reads independently check the history
// boundary. Controlled accounts do not establish independent beneficial owners.
export async function proveOwnerApplications(
  creator: Page,
  builder: Page,
  foreign: Page,
  room: string,
  coordinator: string,
  applicant: string,
  token: string,
) {
  const discovery = builder.getByRole("region", {
    name: "Recruitment and applications",
    exact: true,
  });
  await discovery
    .getByRole("button", { name: "Find recruitment", exact: true })
    .click();
  await discovery
    .getByRole("button", { name: "Review recruitment", exact: true })
    .click();
  await expect(discovery).toContainText("Build and validate catalogue data");
  await discovery
    .getByLabel("Declared capability references", { exact: true })
    .fill("owner-declared-catalogue");
  await discovery.getByLabel(/I reviewed this role/).check();
  const applyPath = `/v1/owner/agents/${applicant}/swarms/${room}/applications`;
  const applying = builder.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === applyPath &&
      r.request().method() === "POST",
  );
  await discovery
    .getByRole("button", { name: "Submit application", exact: true })
    .click();
  const applied = await applying;
  expect(applied.status()).toBe(201);
  const application = await applied.json();
  await expect(discovery.getByRole("status")).toContainText("not admitted");
  const historyPath = `/v1/swarms/${room}/context`,
    headers = { Authorization: `Bearer ${token}` };
  expect((await builder.request.get(historyPath, { headers })).status()).toBe(
    404,
  );
  await builder.reload();
  await discovery
    .getByRole("button", { name: "Load my applications", exact: true })
    .click();
  await expect(discovery).toContainText("Application state: applied");
  expect(
    (
      await foreign.request.get(`/v1/owner/agents/${applicant}/applications`)
    ).status(),
  ).toBe(404);
  await creator
    .getByRole("button", { name: "Load participation", exact: true })
    .click();
  await creator.getByRole("button", { name: "Open room", exact: true }).click();
  const queue = creator.getByRole("region", {
    name: "Room applications",
    exact: true,
  });
  await queue
    .getByRole("button", { name: "Load room applications", exact: true })
    .click();
  await queue
    .getByRole("button", { name: "Review application", exact: true })
    .click();
  await expect(queue).toContainText("owner-declared-catalogue");
  await queue.getByLabel(/I reviewed these claims/).check();
  const offerPath = `/v1/owner/agents/${coordinator}/swarms/${room}/applications/${application.id}/offer`;
  let body: string | null = null,
    key: string | undefined;
  await creator.route(`**${offerPath}`, async (route) => {
    body = route.request().postData();
    key = route.request().headers()["idempotency-key"];
    const response = await route.fetch();
    expect(response.status()).toBe(201);
    await route.abort("failed");
  });
  await queue
    .getByRole("button", { name: "Offer application", exact: true })
    .click();
  await expect(queue.getByRole("alert")).toBeVisible();
  await creator.unroute(`**${offerPath}`);
  const offering = creator.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === offerPath &&
      r.request().method() === "POST",
  );
  await queue
    .getByRole("button", { name: "Offer application", exact: true })
    .click();
  const offered = await offering;
  expect(offered.status()).toBe(201);
  expect(offered.request().postData()).toBe(body);
  expect(offered.request().headers()["idempotency-key"]).toBe(key);
  await expect(queue.getByRole("status")).toContainText("separately accept");
  expect((await builder.request.get(historyPath, { headers })).status()).toBe(
    404,
  );
  await builder.reload();
  await builder
    .getByRole("button", { name: "Load participation", exact: true })
    .click();
  await builder
    .getByRole("button", { name: "Review invitation", exact: true })
    .click();
  await expect(
    builder.getByText(/Accepting gives access to full room history/),
  ).toBeVisible();
  const admission = builder.waitForResponse(
    (r) =>
      new URL(r.url()).pathname.endsWith(
        `/swarms/${room}/membership-response`,
      ) && r.request().method() === "POST",
  );
  await builder
    .getByRole("button", { name: "Accept invitation", exact: true })
    .click();
  expect((await admission).status()).toBe(200);
  await builder.reload();
  await builder
    .getByRole("button", { name: "Load participation", exact: true })
    .click();
  await builder.getByRole("button", { name: "Open room", exact: true }).click();
  await expect(
    builder.getByRole("heading", {
      name: "Build the bounded declared catalogue",
      exact: true,
    }),
  ).toBeVisible();
  expect((await builder.request.get(historyPath, { headers })).status()).toBe(
    200,
  );
}
