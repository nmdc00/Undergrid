import { test, expect } from "@playwright/test";
// Actual built JavaScript in Chromium; API responses are explicit wire fixtures.
// PostgreSQL/cookie/RLS behavior is proved separately by Go runtime tests.
test("built console signs in, displays untrusted data safely and submits owner changes with CSRF", async ({
  page,
}) => {
  const sent: {
    path: string;
    method: string;
    csrf: string;
    key: string;
    body: unknown;
  }[] = [];
  let agentCreateAttempts = 0;
  await page.route("**/auth/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    sent.push({
      path,
      method: route.request().method(),
      csrf: route.request().headers()["x-csrf-token"] ?? "",
      key: route.request().headers()["idempotency-key"] ?? "",
      body: null,
    });
    await route.fulfill({
      status: path === "/auth/logout" ? 204 : 200,
      contentType: "application/json",
      body:
        path === "/auth/logout"
          ? ""
          : JSON.stringify({ csrf_token: "browser-fixture-csrf" }),
    });
  });
  await page.route("**/v1/**", async (route) => {
    const r = route.request(),
      path = new URL(r.url()).pathname;
    sent.push({
      path,
      method: r.method(),
      csrf: r.headers()["x-csrf-token"] ?? "",
      key: r.headers()["idempotency-key"] ?? "",
      body: r.postDataJSON(),
    });
    if (
      path === "/v1/agents" &&
      r.method() === "POST" &&
      agentCreateAttempts++ === 0
    ) {
      await route.abort();
      return;
    }
    const fixture: Record<string, unknown> = {
      "/v1/owner/config": {
        terms_version: "browser-fixture-approved-terms",
        contribution_templates: { templates: [] },
      },
      "/v1/owner/me": { id: "fixture-owner" },
      "/v1/agents":
        r.method() === "POST"
          ? {
              id: "agent",
              credential_id: "credential",
              token: "ug_browser_fixture_once",
            }
          : {
              items: [
                {
                  id: "agent",
                  name: "<img src=x onerror=alert(1)>",
                  status: "active",
                  credentials: [],
                },
              ],
            },
      "/v1/owner/intents": { items: [] },
      "/v1/owner/policy": {
        etag: '"v1"',
        policy: {
          template_ids: [],
          license_ids: [],
          task_categories: [],
          max_active_tasks: 0,
          expires_at: "2030-01-01T00:00:00Z",
        },
      },
    };
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(fixture[path] ?? {}),
    });
  });
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Your workspace" }),
  ).toBeVisible();
  await expect(
    page.getByText("<img src=x onerror=alert(1)>", { exact: true }).first(),
  ).toBeVisible();
  expect(await page.locator("img").count()).toBe(0);
  await page.getByLabel("Agent name").fill("Browser listener");
  await page
    .getByRole("button", { name: "Register agent", exact: true })
    .click();
  await expect(page.getByRole("alert")).toBeVisible();
  await page
    .getByRole("button", { name: "Register agent", exact: true })
    .click();
  await expect(
    page.getByText("ug_browser_fixture_once", { exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(() => [localStorage.length, sessionStorage.length]),
  ).toEqual([0, 0]);
  const creates = sent.filter(
    (r) => r.path === "/v1/agents" && r.method === "POST",
  );
  expect(creates.map((r) => r.csrf)).toEqual([
    "browser-fixture-csrf",
    "browser-fixture-csrf",
  ]);
  expect(creates[0].key).toBe(creates[1].key);
  await page.getByRole("button", { name: "Dismiss credential" }).click();
  await expect(
    page.getByText("ug_browser_fixture_once", { exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(page.getByRole("link", { name: "Sign in" })).toHaveAttribute(
    "href",
    "/auth/login",
  );
  expect(sent.find((r) => r.path === "/auth/logout")?.csrf).toBe(
    "browser-fixture-csrf",
  );
});
test("unauthenticated browser has an OIDC sign in link", async ({ page }) => {
  await page.route("**/auth/session", (route) =>
    route.fulfill({ status: 401, body: "" }),
  );
  await page.goto("/");
  await expect(page.getByRole("link", { name: "Sign in" })).toHaveAttribute(
    "href",
    "/auth/login",
  );
});

test("built console submits reviewed rotation and exact signing proof", async ({
  page,
}) => {
  const calls: { path: string; body: unknown; match: string; csrf: string }[] =
    [];
  await page.route("**/auth/session", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ csrf_token: "authority-fixture-csrf" }),
    }),
  );
  await page.route("**/v1/**", async (route) => {
    const r = route.request(),
      path = new URL(r.url()).pathname;
    calls.push({
      path,
      body: r.postDataJSON(),
      match: r.headers()["if-match"] ?? "",
      csrf: r.headers()["x-csrf-token"] ?? "",
    });
    const values: Record<string, unknown> = {
      "/v1/owner/config": {
        terms_version: "fixture-approved",
        contribution_templates: { templates: [] },
      },
      "/v1/owner/me": { id: "owner" },
      "/v1/agents": {
        items: [
          {
            id: "agent",
            etag: '"1"',
            name: "Authority listener",
            status: "active",
            key_control_verified: false,
            credentials: [
              {
                id: "credential",
                etag: '"1"',
                scopes: ["intents:read"],
                revoked_at: null,
              },
            ],
          },
        ],
      },
      "/v1/owner/intents": { items: [] },
      "/v1/owner/policy": { etag: '"1"', policy: {} },
      "/v1/owner/credentials/credential/rotate": {
        token: "ug_browser_rotation_fixture",
      },
      "/v1/owner/agents/agent/control-challenge": {
        challenge: "exact browser signing challenge",
        etag: '"2"',
      },
      "/v1/owner/agents/agent/control-proof": { key_control_verified: true },
    };
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(values[path] ?? {}),
    });
  });
  await page.goto("/");
  await page.getByText("Signing proof and credential recovery").click();
  await page.getByRole("button", { name: "Rotate credential" }).click();
  await expect(
    page.getByText("ug_browser_rotation_fixture", { exact: true }),
  ).toBeVisible();
  expect(calls.find((c) => c.path.endsWith("/rotate"))).toMatchObject({
    match: '"1"',
    csrf: "authority-fixture-csrf",
    body: { scopes: ["intents:read"] },
  });
  expect(
    await page.evaluate(() => [localStorage.length, sessionStorage.length]),
  ).toEqual([0, 0]);
  await page
    .getByLabel("Ed25519 public key (base64url)")
    .fill("fixture-public-key");
  await page.getByRole("button", { name: "Create signing challenge" }).click();
  await expect(
    page.getByText("exact browser signing challenge", { exact: true }),
  ).toBeVisible();
  await page
    .getByLabel("Ed25519 signature (base64url)")
    .fill("fixture-signature");
  await page.getByRole("button", { name: "Verify signing key" }).click();
  await expect(
    page.getByText("exact browser signing challenge", { exact: true }),
  ).toHaveCount(0);
  expect(calls.find((c) => c.path.endsWith("/control-proof"))).toMatchObject({
    match: '"2"',
    csrf: "authority-fixture-csrf",
    body: {
      challenge: "exact browser signing challenge",
      signature: "fixture-signature",
    },
  });
  await expect(
    page.getByText(
      "Endpoint control is unverified. Beneficial ownership is unknown.",
    ),
  ).toBeVisible();
});
