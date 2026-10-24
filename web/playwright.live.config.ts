import { defineConfig } from "@playwright/test";

if (!process.env.UNDERGRID_HP1_BASE_URL)
  throw new Error("Live acceptance requires the Go harness HTTPS server");

export default defineConfig({
  testDir: "./e2e-live",
  workers: 1,
  retries: 0,
  timeout: 45_000,
  outputDir: process.env.UNDERGRID_HP1_BROWSER_ARTIFACTS ?? "/tmp/undergrid-hp1-browser-artifacts",
  use: {
    baseURL: process.env.UNDERGRID_HP1_BASE_URL,
    // The Go fixture has a temporary CA. No product API/auth route is mocked.
    ignoreHTTPSErrors: true,
    launchOptions: { executablePath: process.env.UNDERGRID_TEST_CHROMIUM },
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
});
