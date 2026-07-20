import { type Page } from "@playwright/test";
import { TestApiClient } from "./fixtures";

const DEFAULT_E2E_NAME = "E2E User";
const DEFAULT_E2E_EMAIL = "e2e@multica.ai";
const DEFAULT_E2E_WORKSPACE = "e2e-workspace";

/**
 * Log in as the default E2E user and ensure the workspace exists first.
 * Authenticates via API (send-code → DB read → verify-code), then injects
 * the token into localStorage so the browser session is authenticated.
 *
 * Returns the E2E workspace slug so callers can build workspace-scoped URLs.
 */
export async function loginAsDefault(page: Page): Promise<string> {
  const api = new TestApiClient();
  await api.login(DEFAULT_E2E_EMAIL, DEFAULT_E2E_NAME);
  const workspace = await api.ensureWorkspace(
    "E2E Workspace",
    DEFAULT_E2E_WORKSPACE,
  );

  const token = api.getToken();
  if (!token) throw new Error("E2E login returned no token");
  const authToken: string = token;
  // Install the legacy token before the first Multica document boots. The web
  // provider chooses cookie-vs-token auth once at startup; setting localStorage
  // after visiting /login leaves the already-initialized provider in cookie mode.
  await page.addInitScript((t) => {
    localStorage.setItem("multica_token", t);
  }, authToken);
  await page.goto(`/${workspace.slug}/issues`);
  await page.waitForURL("**/issues", { timeout: 10000 });

  // Keep the floating chat window from covering actions in unrelated specs.
  // Chat behavior itself is covered by its component tests.
  await minimizeChat(page);
  return workspace.slug;
}

export async function minimizeChat(page: Page) {
  const button = page.getByRole("button", { name: "Minimize chat" });
  if (await button.isVisible().catch(() => false)) {
    await button.click();
  }
}

/**
 * Create a TestApiClient logged in as the default E2E user.
 * Call api.cleanup() in afterEach to remove test data created during the test.
 */
export async function createTestApi(): Promise<TestApiClient> {
  const api = new TestApiClient();
  await api.login(DEFAULT_E2E_EMAIL, DEFAULT_E2E_NAME);
  await api.ensureWorkspace("E2E Workspace", DEFAULT_E2E_WORKSPACE);
  return api;
}

export async function openWorkspaceMenu(page: Page) {
  // The responsive sidebar no longer renders an <aside>; target the switcher
  // by its accessible workspace name instead of DOM structure/CSS classes.
  await page.getByRole("button", { name: "Workspace switcher" }).click();
  await page.getByText("Log out", { exact: true }).waitFor({ state: "visible" });
}
