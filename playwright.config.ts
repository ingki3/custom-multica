import "./e2e/env";
import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 30000,
  retries: 0,
  // The suite intentionally shares one E2E user/workspace and resets that
  // user's verification-code rows during login. Parallel workers race those
  // resets and trigger the production 60-second send-code rate limit.
  workers: 1,
  use: {
    // FRONTEND_ORIGIN is the backend CORS allow-list and may contain multiple
    // comma-separated origins, so it is not a valid Playwright navigation URL.
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3000",
    headless: true,
  },
  projects: [
    {
      name: "chromium",
      use: { browserName: "chromium" },
    },
  ],
  // Don't auto-start servers — they must be running already
  // This avoids complexity and port conflicts during testing
});
