import { test, expect } from "@playwright/test";
import { loginAsDefault, minimizeChat } from "./helpers";

test.describe("Settings", () => {
  test("updating workspace name reflects in sidebar immediately", async ({
    page,
  }) => {
    await loginAsDefault(page);

    const sidebarName = page.getByRole("button", { name: "Workspace switcher" });

    // Navigate to settings
    await page.getByRole("link", { name: "Settings", exact: true }).click();
    await page.waitForURL("**/settings");
    await page.getByRole("tab", { name: "General" }).click();
    await minimizeChat(page);

    // Change workspace name
    const nameInput = page
      .getByRole("tabpanel", { name: "General" })
      .locator('input[type="text"]')
      .first();
    const originalName = await nameInput.inputValue();
    await nameInput.clear();
    const newName = "Renamed WS " + Date.now();
    await nameInput.fill(newName);

    // Save
    await page.locator("button", { hasText: "Save" }).click();

    await expect(page.getByText("Workspace settings saved")).toBeVisible({ timeout: 5000 });

    // Sidebar should reflect the new name WITHOUT page refresh
    await expect(sidebarName).toContainText(newName);

    // Restore original name so other tests aren't affected
    await nameInput.clear();
    await nameInput.fill(originalName);
    await page.locator("button", { hasText: "Save" }).click();
    await expect(page.getByText("Workspace settings saved")).toBeVisible({ timeout: 5000 });
  });
});
