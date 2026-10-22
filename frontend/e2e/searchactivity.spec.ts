// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, test } from "@playwright/test";
import { mockApi } from "./seed";

// A meeting search found opens in a panel over the results, with the contact
// it was with one link away.

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

test("a meeting search found opens in the activity panel", async ({ page }) => {
  await page.goto("/#/search/brandt");
  await page
    .getByRole("button", { name: "Quarterly review with Brandt Automotive" })
    .click();

  const panel = page.getByRole("dialog", {
    name: "Quarterly review with Brandt Automotive",
  });
  await expect(panel).toBeVisible();
  const agreed = panel.getByText("agreed", { exact: true });
  await expect(agreed).toBeVisible();
  // The note's `**agreed**`: the global reset takes the weight off `strong`.
  await expect(agreed).toHaveCSS("font-weight", "700");
  await expect(panel.getByRole("link", { name: "Anna Weber" })).toBeVisible();
  await expect(page).toHaveURL(/#\/search\/brandt$/);
});
