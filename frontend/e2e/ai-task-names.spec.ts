// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, type Page, test } from "@playwright/test";
import type { components } from "../src/api/schema";
import { meFixture } from "../src/testing/mefixture";
import { aiAdminFixture } from "./ai-admin-fixture";
import { aiRouting, aiUsage } from "./mockapi/adminfixtures";
import { mockApi, servePrincipal } from "./seed";
import { signIn } from "./waits";

// An AI task is named by its name on Settings → AI models, in the tasks table
// and on its sheet. Its key and what it does are behind that name, so the key
// never stands in for it. With BASE_URL the same walk reads a seeded stack.

const TASK = "report_interpret";
const NAME = "Assessing a sales report";
const SUMMARY =
  "Writes an assessment beside a report's leading chart, citing the figures it rests on.";

const REPORT_INTERPRET: components["schemas"]["AiFeatureRoute"] = {
  task: TASK,
  display_name: NAME,
  summary: SUMMARY,
  execution_mode: "interactive",
  leading_tier: "premium",
  normal_candidates: [
    {
      tier: "premium",
      provider: "anthropic",
      model: "claude-sonnet",
      processing: "cloud_provider",
    },
    {
      tier: "cheap_cloud",
      provider: "gemini",
      model: "gemini-flash",
      processing: "cloud_provider",
    },
  ],
  effective_candidates: [
    {
      tier: "premium",
      provider: "anthropic",
      model: "claude-sonnet",
      processing: "cloud_provider",
    },
  ],
  impact: "unchanged",
  budget_exempt: false,
  decision_first: false,
  defaults: { decision_timeout_ms: 15000, attempt_timeout_ms: 300000 },
};

test.use({ locale: "en-US" });

test.beforeEach(async ({ page }) => {
  if (process.env.BASE_URL) {
    await signIn(page);
    return;
  }
  await mockApi(page);
  await servePrincipal(
    page,
    meFixture({
      allow: {
        ai_routing: ["read", "update"],
        ai_diagnostics: ["read"],
        ai_budget: ["read"],
      },
    }),
  );
  const status = aiAdminFixture(aiRouting.tiers, aiUsage.budget);
  await page.route("**/v1/ai/status", (route) =>
    route.fulfill({
      json: { ...status, features: [...status.features, REPORT_INTERPRET] },
    }),
  );
});

/** The aside a task name opens, named by the trigger that opened it. */
function popoverOf(page: Page, trigger: string) {
  return page.getByRole("region", { name: trigger, exact: true });
}

test("names an AI task by its name, with what it does and its key behind it", async ({
  page,
}) => {
  await page.goto("/#/settings/models/tasks");
  const row = page.getByRole("row").filter({ hasText: NAME });
  await expect(row).toBeVisible();
  await expect(row).not.toContainText(TASK);

  await row
    .getByRole("button", { name: `${NAME}: what it does`, exact: true })
    .click();
  const aside = popoverOf(page, `${NAME}: what it does`);
  await expect(aside).toContainText(SUMMARY);
  await expect(aside.locator("code", { hasText: TASK })).toBeVisible();
  await page.screenshot({ path: test.info().outputPath("tasks-popover.png") });
  await page.keyboard.press("Escape");

  await row.getByRole("button", { name: `Edit ${NAME}`, exact: true }).click();
  const sheet = page.getByRole("dialog", { name: NAME, exact: true });
  await expect(sheet).toBeVisible();
  await expect(sheet.getByRole("heading", { name: NAME })).toBeVisible();
  await expect(sheet).not.toContainText(TASK);
  await expect(sheet).not.toContainText("cheap_cloud");
  await page.screenshot({ path: test.info().outputPath("sheet.png") });

  // In the title the trigger says the name alone, so it names the dialog.
  await sheet.getByRole("button", { name: NAME, exact: true }).click();
  const sheetAside = popoverOf(page, NAME);
  await expect(sheetAside).toContainText(SUMMARY);
  await expect(sheetAside).toContainText(`${TASK} · While the user waits`);
  await page.screenshot({ path: test.info().outputPath("sheet-popover.png") });
});
