import { expect, test } from "@playwright/test";
import { loadLocale } from "../src/i18n/catalogs";
import { de } from "../src/i18n/de";
import { expectNoAaViolations, settleAnimations } from "./a11y";
import { mockApi } from "./seed";
import { pageOverflow } from "./waits";

test.beforeAll(async () => {
  await loadLocale("de");
});

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

test("AC-settings-agents: Agent rules opens at its own address, under Your agent", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/settings/agents/rules");
  await expect(
    page.getByRole("heading", {
      level: 2,
      name: de["aiAccess.yourAgent.title"],
    }),
  ).toBeVisible();
  const rules = page.getByRole("button", {
    name: de["aiAccess.dev.tab.rules"],
  });
  await expect(rules).toHaveAttribute("aria-pressed", "true");
  await expect(
    page.getByRole("heading", { name: de["settings.autonomy"] }),
  ).toBeVisible();
  expect(await pageOverflow(page)).toEqual([]);
  await settleAnimations(page);
  await expectNoAaViolations(page, "settings/agents/rules (390px)");
  // The keyboard reaches the other tab, and choosing it keeps focus there: the
  // switch stays in the page rather than moving to another address.
  await rules.focus();
  await page.keyboard.press("Shift+Tab");
  await page.keyboard.press("Enter");
  const passports = page.getByRole("button", {
    name: de["aiAccess.dev.tab.passports"],
  });
  await expect(passports).toHaveAttribute("aria-pressed", "true");
  await expect(passports).toBeFocused();
});
