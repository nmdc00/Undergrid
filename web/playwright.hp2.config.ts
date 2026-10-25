import { defineConfig } from "@playwright/test";
import live from "./playwright.live.config";

// Share browser/session fixture policy but keep scenarios separate: HP1's
// existing runner must not silently start an unfinished HP2 journey.
export default defineConfig({
  ...live,
  testDir: "./e2e-hp2",
  timeout: 90_000,
});
